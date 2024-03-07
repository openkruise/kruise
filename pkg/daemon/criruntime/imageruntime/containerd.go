/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package imageruntime

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/alibaba/pouch/pkg/jsonstream"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/docker/distribution/reference"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/util/secret"
	"golang.org/x/net/http/httpproxy"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
)

const (
	defaultTag             = "latest"
	k8sContainerdNamespace = "k8s.io"
)

// NewContainerdImageService returns containerd-type ImageService
func NewContainerdImageService(
	runtimeURI string,
	accountManager daemonutil.ImagePullAccountManager,
) (ImageService, error) {
	client, err := containerd.New(runtimeURI)
	if err != nil {
		return nil, err
	}
	conn := client.Conn()
	snapshotter, httpProxy, err := getDefaultValuesFromCRIStatus(conn)
	if err != nil {
		return nil, err
	}

	return &containerdImageClient{
		accountManager: accountManager,
		snapshotter:    snapshotter,
		client:         client,
		// TODO: compatible with v1alpha2 cri api
		criImageClient: runtimeapi.NewImageServiceClient(conn),
		httpProxy:      httpProxy,
	}, nil
}

type containerdImageClient struct {
	accountManager daemonutil.ImagePullAccountManager
	snapshotter    string
	client         *containerd.Client
	criImageClient runtimeapi.ImageServiceClient
	httpProxy      string
}

// PullImage implements ImageService.PullImage.
func (d *containerdImageClient) PullImage(ctx context.Context, imageName, tag string, pullSecrets []v1.Secret, _ *appsv1alpha1.SandboxConfig) (ImagePullStatusReader, error) {
	ctx = namespaces.WithNamespace(ctx, k8sContainerdNamespace)

	if tag == "" {
		tag = defaultTag
	}

	imageRef := fmt.Sprintf("%s:%s", imageName, tag)
	namedRef, err := daemonutil.NormalizeImageRef(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %q: %w", imageRef, err)
	}

	resolver, isSchema1, err := d.getResolver(ctx, namedRef, pullSecrets)
	if err != nil {
		return nil, err
	}
	return d.doPullImage(ctx, namedRef, isSchema1, resolver), nil
}

// ListImages implements ImageService.ListImages.
func (d *containerdImageClient) ListImages(ctx context.Context) ([]ImageInfo, error) {
	resp, err := d.criImageClient.ListImages(ctx, &runtimeapi.ListImagesRequest{})
	if err != nil {
		return nil, err
	}

	collection := make([]ImageInfo, 0, len(resp.Images))
	for _, info := range resp.Images {
		collection = append(collection, ImageInfo{
			ID:          info.Id,
			RepoTags:    info.RepoTags,
			RepoDigests: info.RepoDigests,
			Size:        int64(info.Size_),
		})
	}
	return collection, nil
}

// doPullImage returns pipe reader as ImagePullStatusReader to notify the progressing.
func (d *containerdImageClient) doPullImage(ctx context.Context, ref reference.Named, isSchema1 bool, resolver remotes.Resolver) ImagePullStatusReader {
	ongoing := newFetchJobs(ref.String())

	opts := []containerd.RemoteOpt{
		containerd.WithSchema1Conversion,
		containerd.WithResolver(resolver),
		containerd.WithPullUnpack,
		containerd.WithPullSnapshotter(d.snapshotter),
		containerd.WithImageHandler(
			images.HandlerFunc(
				func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
					if desc.MediaType != images.MediaTypeDockerSchema1Manifest {
						ongoing.add(desc)
					}
					return nil, nil
				}),
		),
	}

	pipeR, pipeW := io.Pipe()
	stream := jsonstream.New(pipeW, nil)

	pctx, pcancel := context.WithCancel(ctx)
	waitCh := make(chan struct{})

	// for upload progress
	go func() {
		defer close(waitCh)
		fetchProgress(pctx, d.client.ContentStore(), ongoing, stream)
	}()

	// for fetch image progressing
	go func() {
		defer func() {
			stream.Close()
			stream.Wait()
			pipeW.Close()
		}()

		img, err := d.client.Pull(pctx, ref.String(), opts...)

		// stop fetchProgress goroutine
		pcancel()
		<-waitCh

		// need to update digest if fetch successfully
		if err == nil {
			err = d.createRepoDigestRecord(ctx, ref, img.Target(), isSchema1)
		}

		if err != nil {
			stream.WriteObject(jsonstream.JSONMessage{
				Error: &jsonstream.JSONError{
					Code:    http.StatusInternalServerError,
					Message: err.Error(),
				},
				ErrorMessage: err.Error(),
			})
		}

	}()
	return newImagePullStatusReader(pipeR)
}

// getResolver returns fetchable resolver for pulling image.
//
// FIXME(yuge.fw): need extra config for insecure registry settings.
func (d *containerdImageClient) getResolver(ctx context.Context, ref reference.Named, secrets []v1.Secret) (_ remotes.Resolver, isSchema1 bool, _ error) {
	var registry = daemonutil.ParseRegistry(ref.String())
	var lastErr error // last resolver error

	// Stage 1: try to resolving reference by given secrets
	if len(secrets) > 0 {
		var authInfos []daemonutil.AuthInfo
		authInfos, lastErr = secret.ConvertToRegistryAuths(secrets, registry)
		if lastErr == nil {
			var pullErrs []error
			for _, authInfo := range authInfos {
				resolver := d.resolverGenerator(&authInfo)
				_, desc, err := resolver.Resolve(ctx, ref.String())
				if err == nil {
					return resolver, desc.MediaType == images.MediaTypeDockerSchema1Manifest, nil
				}
				klog.Warningf("failed to resolve reference %s: %v", ref, err)
				pullErrs = append(pullErrs, err)
			}
			if len(pullErrs) > 0 {
				lastErr = utilerrors.NewAggregate(pullErrs)
			}
		}
	}

	// Stage 2: try to resolving reference with default account info or
	// anonymous user.
	authInfos := make([]*daemonutil.AuthInfo, 0, 2)

	// NOTE: It maybe be slow if the GetAccountInfo fetches remote.
	if d.accountManager != nil {
		defaultAuthInfo, err := d.accountManager.GetAccountInfo(registry)
		if err != nil {
			klog.Warningf("failed to get default account for registry %v: %v", registry, err)
		} else if defaultAuthInfo != nil {
			authInfos = append(authInfos, defaultAuthInfo)
		}
	}

	// last one is anonymous
	authInfos = append(authInfos, nil)

	for _, authInfo := range authInfos {
		resolver := d.resolverGenerator(authInfo)
		_, desc, err := resolver.Resolve(ctx, ref.String())
		if err != nil {
			if authInfo != nil {
				lastErr = fmt.Errorf("pulling with user %v failed, err %v", authInfo.Username, err)
			} else {
				lastErr = fmt.Errorf("anonymous pulling failed, err %v", err)
			}
			klog.Warningf("failed to resolve reference %s: %v", ref, err)
			continue
		}
		return resolver, desc.MediaType == images.MediaTypeDockerSchema1Manifest, nil
	}
	return nil, false, lastErr
}

// resolverGenerator returns resolver based on authInfo.
func (d *containerdImageClient) resolverGenerator(authInfo *daemonutil.AuthInfo) remotes.Resolver {
	var username string
	var secret string
	var cfg = httpproxy.FromEnvironment()

	if authInfo != nil {
		username, secret = authInfo.Username, authInfo.Password
	}

	// for dragonfly p2p proxy
	if d.httpProxy != "" {
		cfg.HTTPProxy = d.httpProxy
	}

	tr := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return cfg.ProxyFunc()(req.URL)
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		TLSClientConfig:       &tls.Config{},
		ExpectContinueTimeout: 5 * time.Second,
	}

	return docker.NewResolver(docker.ResolverOptions{
		Credentials: func(host string) (string, string, error) {
			return username, secret, nil
		},
		Client: &http.Client{
			Transport: tr,
		},
	})
}

// createRepoDigestRecord creates digest type record in containerd.
//
// NOTE: We don't use CRI-API to pull image but we use CRI-API to retrieve
// image list. For the repo:tag image, the containerd will receive image create
// event and then update local cache with the mapping between image ID and
// image name. But there is no mapping between image ID and image digest. We
// need to create digest record in containerd.
func (d *containerdImageClient) createRepoDigestRecord(ctx context.Context, ref reference.Named, desc ocispec.Descriptor, isSchema1 bool) error {
	repoDigest := getRepoDigest(ref, desc.Digest, isSchema1)
	// do nothing if ref is digest reference or schema1
	if repoDigest == ref.String() || repoDigest == "" {
		return nil
	}

	img := images.Image{
		Name:   repoDigest,
		Target: desc,
	}

	_, err := d.client.ImageService().Create(ctx, img)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// getDefaultValuesFromCRIStatus returns default snapshotter/httpproxy from cri-containerd.
func getDefaultValuesFromCRIStatus(conn *grpc.ClientConn) (snapshotter string, httpproxy string, _ error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	// TODO: compatible with v1alpha2 cri api
	rclient := runtimeapi.NewRuntimeServiceClient(conn)
	resp, err := rclient.Status(ctx, &runtimeapi.StatusRequest{Verbose: true})
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch cri-containerd status: %w", err)
	}

	var partInfo struct {
		ContainerdConfig struct {
			Snapshotter string `json:"snapshotter"`
		} `json:"containerd"`
		Registry struct {
			Proxy string `json:"proxy"`
		} `json:"registry"`
	}

	config, ok := resp.Info["config"]
	if !ok {
		return "", "", fmt.Errorf("failed to get config info from containerd: %w", err)
	}

	if err := json.Unmarshal([]byte(config), &partInfo); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal config(%v): %w", config, err)
	}

	snapshotter = partInfo.ContainerdConfig.Snapshotter
	httpproxy = partInfo.Registry.Proxy
	return
}

// getRepoDigest returns image digest string
//
// NOTE: Schema1 digest will be changed after pull because it will be converted
// into schema2 in containerd.
func getRepoDigest(ref reference.Named, d digest.Digest, isSchema1 bool) string {
	var repoDigest string
	if _, ok := ref.(reference.Canonical); ok {
		repoDigest = ref.String()
	} else if !isSchema1 {
		repoDigest = ref.Name() + "@" + d.String()
	}
	return repoDigest
}
