/*
Copyright 2022 The Kruise Authors.
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
	"io"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/cri/remote/util"
	"k8s.io/kubernetes/pkg/util/parsers"
)

const (
	maxMsgSize                    = 1024 * 1024 * 16
	pullingImageSandboxConfigAnno = "apps.kruise.io/pulling-image-by"
)

// NewCRIImageService create a common CRI runtime
func NewCRIImageService(runtimeURI string, accountManager daemonutil.ImagePullAccountManager) (ImageService, error) {
	klog.V(3).InfoS("Connecting to image service", "endpoint", runtimeURI)
	addr, dialer, err := util.GetAddressAndDialer(runtimeURI)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithContextDialer(dialer), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)))
	if err != nil {
		klog.ErrorS(err, "Connect remote image service failed", "address", addr)
		return nil, err
	}

	klog.V(4).InfoS("Finding the CRI API image version")
	imageClient := runtimeapi.NewImageServiceClient(conn)

	if _, err := imageClient.ImageFsInfo(ctx, &runtimeapi.ImageFsInfoRequest{}); err == nil {
		klog.V(2).InfoS("Using CRI v1 image API")
	}

	return &commonCRIImageService{
		accountManager: accountManager,
		criImageClient: imageClient,
	}, nil
}

type commonCRIImageService struct {
	accountManager daemonutil.ImagePullAccountManager
	criImageClient runtimeapi.ImageServiceClient
}

// PullImage implements ImageService.PullImage.
func (c *commonCRIImageService) PullImage(ctx context.Context, imageName, tag string, pullSecrets []v1.Secret, sandboxConfig *appsv1alpha1.SandboxConfig) (ImagePullStatusReader, error) {
	registry := daemonutil.ParseRegistry(imageName)
	fullImageName := imageName + ":" + tag
	repoToPull, _, _, err := parsers.ParseImageName(fullImageName)
	if err != nil {
		return nil, err
	}
	// Reader
	pipeR, pipeW := io.Pipe()
	defer pipeW.Close()

	var auth *runtimeapi.AuthConfig
	pullImageReq := &runtimeapi.PullImageRequest{
		Image: &runtimeapi.ImageSpec{
			Image:       fullImageName,
			Annotations: make(map[string]string),
		},
		Auth: auth, //default is nil
	}
	if sandboxConfig != nil {
		pullImageReq.SandboxConfig = &runtimeapi.PodSandboxConfig{
			Annotations: sandboxConfig.Annotations,
			Labels:      sandboxConfig.Labels,
		}
		if pullImageReq.SandboxConfig.Annotations == nil {
			pullImageReq.SandboxConfig.Annotations = map[string]string{}
		}
	} else {
		pullImageReq.SandboxConfig = &runtimeapi.PodSandboxConfig{
			Annotations: map[string]string{},
		}
	}
	// Add this default annotation to avoid unexpected panic caused by sandboxConfig is nil
	// for some runtime implementations.
	pullImageReq.SandboxConfig.Annotations[pullingImageSandboxConfigAnno] = "kruise-daemon"

	if len(pullSecrets) > 0 {
		var authInfos []daemonutil.AuthInfo
		authInfos, err = convertToRegistryAuths(pullSecrets, repoToPull)
		if err == nil {
			var pullErrs []error
			for _, authInfo := range authInfos {
				var pullErr error
				klog.V(5).Infof("Pull image %v:%v with user %v", imageName, tag, authInfo.Username)
				pullImageReq.Auth = &runtimeapi.AuthConfig{
					Username: authInfo.Username,
					Password: authInfo.Password,
				}
				_, pullErr = c.criImageClient.PullImage(ctx, pullImageReq)
				if pullErr == nil {
					pipeW.CloseWithError(io.EOF)
					return newImagePullStatusReader(pipeR), nil
				}
				klog.Warningf("Failed to pull image %v:%v with user %v, err %v", imageName, tag, authInfo.Username, pullErr)
				pullErrs = append(pullErrs, pullErr)

			}
			if len(pullErrs) > 0 {
				err = utilerrors.NewAggregate(pullErrs)
			}
		}
	}

	// Try the default secret
	if c.accountManager != nil {
		var authInfo *daemonutil.AuthInfo
		var defaultErr error
		authInfo, defaultErr = c.accountManager.GetAccountInfo(registry)
		if defaultErr != nil {
			klog.Warningf("Failed to get account for registry %v, err %v", registry, defaultErr)
			// When the default account acquisition fails, try to pull anonymously
		} else if authInfo != nil {
			klog.V(5).Infof("Pull image %v:%v with user %v", imageName, tag, authInfo.Username)
			pullImageReq.Auth = &runtimeapi.AuthConfig{
				Username: authInfo.Username,
				Password: authInfo.Password,
			}
			_, err = c.criImageClient.PullImage(ctx, pullImageReq)
			if err == nil {
				pipeW.CloseWithError(io.EOF)
				return newImagePullStatusReader(pipeR), nil
			}
			klog.Warningf("Failed to pull image %v:%v, err %v", imageName, tag, err)
			return nil, err
		}
	}

	if err != nil {
		return nil, err
	}

	// Anonymous pull
	_, err = c.criImageClient.PullImage(ctx, pullImageReq)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to pull image reference %q", fullImageName)
	}
	pipeW.CloseWithError(io.EOF)
	return newImagePullStatusReader(pipeR), nil
}

// ListImages implements ImageService.ListImages.
func (c *commonCRIImageService) ListImages(ctx context.Context) ([]ImageInfo, error) {
	listImagesReq := &runtimeapi.ListImagesRequest{}
	listImagesResp, err := c.criImageClient.ListImages(ctx, listImagesReq)
	if err != nil {
		return nil, err
	}
	collection := make([]ImageInfo, 0, len(listImagesResp.GetImages()))
	for _, img := range listImagesResp.GetImages() {
		collection = append(collection, ImageInfo{
			ID:          img.GetId(),
			RepoTags:    img.GetRepoTags(),
			RepoDigests: img.GetRepoDigests(),
			Size:        int64(img.GetSize_()),
		})
	}
	return collection, nil
}
