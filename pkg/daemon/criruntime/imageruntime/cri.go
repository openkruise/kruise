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
	"fmt"
	"io"
	"reflect"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/util/secret"

	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	runtimeapiv1alpha2 "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/util"
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

	imageClientV1, err := determineImageClientAPIVersion(conn)
	if err != nil {
		klog.ErrorS(err, "Failed to determine CRI image API version")
		return nil, err
	}

	return &commonCRIImageService{
		accountManager: accountManager,
		criImageClient: imageClientV1,
	}, nil
}

type commonCRIImageService struct {
	accountManager         daemonutil.ImagePullAccountManager
	criImageClient         runtimeapi.ImageServiceClient
	criImageClientV1alpha2 runtimeapiv1alpha2.ImageServiceClient
}

func (c *commonCRIImageService) useV1API() bool {
	return c.criImageClientV1alpha2 == nil || reflect.ValueOf(c.criImageClientV1alpha2).IsNil()
}

// PullImage implements ImageService.PullImage.
func (c *commonCRIImageService) PullImage(ctx context.Context, imageName, tag string, pullSecrets []v1.Secret, sandboxConfig *appsv1alpha1.SandboxConfig) (ImagePullStatusReader, error) {
	if c.useV1API() {
		return c.pullImageV1(ctx, imageName, tag, pullSecrets, sandboxConfig)
	}
	return c.pullImageV1alpha2(ctx, imageName, tag, pullSecrets, sandboxConfig)
}

// ListImages implements ImageService.ListImages.
func (c *commonCRIImageService) ListImages(ctx context.Context) ([]ImageInfo, error) {
	if c.useV1API() {
		return c.listImagesV1(ctx)
	}
	return c.listImagesV1alpha2(ctx)
}

// PullImage implements ImageService.PullImage using v1 CRI client.
func (c *commonCRIImageService) pullImageV1(ctx context.Context, imageName, tag string, pullSecrets []v1.Secret, sandboxConfig *appsv1alpha1.SandboxConfig) (ImagePullStatusReader, error) {
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

	var authInfos []daemonutil.AuthInfo
	authInfos, err = secret.ConvertToRegistryAuths(pullSecrets, repoToPull)
	if err == nil {
		var pullErrs []error
		for _, authInfo := range authInfos {
			var pullErr error
			klog.V(5).InfoS("Pull image with user", "imageName", imageName, "tag", tag, "user", authInfo.Username)
			pullImageReq.Auth = &runtimeapi.AuthConfig{
				Username: authInfo.Username,
				Password: authInfo.Password,
			}
			_, pullErr = c.criImageClient.PullImage(ctx, pullImageReq)
			if pullErr == nil {
				pipeW.CloseWithError(io.EOF)
				return newImagePullStatusReader(pipeR), nil
			}
			klog.ErrorS(pullErr, "Failed to pull image with user", "imageName", imageName, "tag", tag, "user", authInfo.Username)
			pullErrs = append(pullErrs, pullErr)

		}
		if len(pullErrs) > 0 {
			err = utilerrors.NewAggregate(pullErrs)
		}
	} else {
		klog.ErrorS(err, "Failed to convert to auth info for registry")
	}

	// Try the default secret
	if c.accountManager != nil {
		var authInfo *daemonutil.AuthInfo
		var defaultErr error
		authInfo, defaultErr = c.accountManager.GetAccountInfo(registry)
		if defaultErr != nil {
			klog.ErrorS(defaultErr, "Failed to get account for registry", "registry", registry)
			// When the default account acquisition fails, try to pull anonymously
		} else if authInfo != nil {
			klog.V(5).InfoS("Pull image with user", "imageName", imageName, "tag", tag, "user", authInfo.Username)
			pullImageReq.Auth = &runtimeapi.AuthConfig{
				Username: authInfo.Username,
				Password: authInfo.Password,
			}
			_, err = c.criImageClient.PullImage(ctx, pullImageReq)
			if err == nil {
				pipeW.CloseWithError(io.EOF)
				return newImagePullStatusReader(pipeR), nil
			}
			klog.ErrorS(err, "Failed to pull image", "imageName", imageName, "tag", tag)
			return nil, err
		}
	}

	if err != nil {
		return nil, err
	}

	// Anonymous pull
	_, err = c.criImageClient.PullImage(ctx, pullImageReq)
	if err != nil {
		return nil, fmt.Errorf("Failed to pull image reference %q: %w", fullImageName, err)
	}
	pipeW.CloseWithError(io.EOF)
	return newImagePullStatusReader(pipeR), nil
}

// ListImages implements ImageService.ListImages using V1 CRI client.
func (c *commonCRIImageService) listImagesV1(ctx context.Context) ([]ImageInfo, error) {
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

// PullImage implements ImageService.PullImage using v1alpha2 CRI client.
func (c *commonCRIImageService) pullImageV1alpha2(ctx context.Context, imageName, tag string, pullSecrets []v1.Secret, sandboxConfig *appsv1alpha1.SandboxConfig) (ImagePullStatusReader, error) {
	registry := daemonutil.ParseRegistry(imageName)
	fullImageName := imageName + ":" + tag
	repoToPull, _, _, err := parsers.ParseImageName(fullImageName)
	if err != nil {
		return nil, err
	}
	// Reader
	pipeR, pipeW := io.Pipe()
	defer pipeW.Close()

	var auth *runtimeapiv1alpha2.AuthConfig
	pullImageReq := &runtimeapiv1alpha2.PullImageRequest{
		Image: &runtimeapiv1alpha2.ImageSpec{
			Image:       fullImageName,
			Annotations: make(map[string]string),
		},
		Auth: auth, //default is nil
	}
	if sandboxConfig != nil {
		pullImageReq.SandboxConfig = &runtimeapiv1alpha2.PodSandboxConfig{
			Annotations: sandboxConfig.Annotations,
			Labels:      sandboxConfig.Labels,
		}
		if pullImageReq.SandboxConfig.Annotations == nil {
			pullImageReq.SandboxConfig.Annotations = map[string]string{}
		}
	} else {
		pullImageReq.SandboxConfig = &runtimeapiv1alpha2.PodSandboxConfig{
			Annotations: map[string]string{},
		}
	}
	// Add this default annotation to avoid unexpected panic caused by sandboxConfig is nil
	// for some runtime implementations.
	pullImageReq.SandboxConfig.Annotations[pullingImageSandboxConfigAnno] = "kruise-daemon"

	if len(pullSecrets) > 0 {
		var authInfos []daemonutil.AuthInfo
		authInfos, err = secret.ConvertToRegistryAuths(pullSecrets, repoToPull)
		if err == nil {
			var pullErrs []error
			for _, authInfo := range authInfos {
				var pullErr error
				klog.V(5).InfoS("Pull image with user", "imageName", imageName, "tag", tag, "user", authInfo.Username)
				pullImageReq.Auth = &runtimeapiv1alpha2.AuthConfig{
					Username: authInfo.Username,
					Password: authInfo.Password,
				}
				_, pullErr = c.criImageClientV1alpha2.PullImage(ctx, pullImageReq)
				if pullErr == nil {
					pipeW.CloseWithError(io.EOF)
					return newImagePullStatusReader(pipeR), nil
				}
				klog.ErrorS(pullErr, "Failed to pull image with user", "imageName", imageName, "tag", tag, "user", authInfo.Username)
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
			klog.ErrorS(defaultErr, "Failed to get account for registry %v, err %v", "registry", registry)
			// When the default account acquisition fails, try to pull anonymously
		} else if authInfo != nil {
			klog.V(5).InfoS("Pull image with user", "imageName", imageName, "tag", tag, "user", authInfo.Username)
			pullImageReq.Auth = &runtimeapiv1alpha2.AuthConfig{
				Username: authInfo.Username,
				Password: authInfo.Password,
			}
			_, err = c.criImageClientV1alpha2.PullImage(ctx, pullImageReq)
			if err == nil {
				pipeW.CloseWithError(io.EOF)
				return newImagePullStatusReader(pipeR), nil
			}
			klog.ErrorS(err, "Failed to pull image", "imageName", imageName, "tag", tag)
			return nil, err
		}
	}

	if err != nil {
		return nil, err
	}

	// Anonymous pull
	_, err = c.criImageClientV1alpha2.PullImage(ctx, pullImageReq)
	if err != nil {
		return nil, fmt.Errorf("Failed to pull image reference %q: %w", fullImageName, err)
	}
	pipeW.CloseWithError(io.EOF)
	return newImagePullStatusReader(pipeR), nil
}

// ListImages implements ImageService.ListImages using V1alpha2 CRI client.
func (c *commonCRIImageService) listImagesV1alpha2(ctx context.Context) ([]ImageInfo, error) {
	listImagesReq := &runtimeapiv1alpha2.ListImagesRequest{}
	listImagesResp, err := c.criImageClientV1alpha2.ListImages(ctx, listImagesReq)
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
