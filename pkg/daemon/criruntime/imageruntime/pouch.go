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
	"fmt"
	"io"
	"sync"

	pouchfilters "github.com/alibaba/pouch/apis/filters"
	pouchtypes "github.com/alibaba/pouch/apis/types"
	pouchapi "github.com/alibaba/pouch/client"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/util/secret"
	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
)

// NewPouchImageService create a pouch runtime client
func NewPouchImageService(runtimeURI string, accountManager daemonutil.ImagePullAccountManager) (ImageService, error) {
	r := &pouchImageService{runtimeURI: runtimeURI, accountManager: accountManager}
	if err := r.createRuntimeClientIfNecessary(); err != nil {
		return nil, err
	}
	return r, nil
}

type pouchImageService struct {
	sync.Mutex
	runtimeURI     string
	accountManager daemonutil.ImagePullAccountManager

	client pouchapi.ImageAPIClient
}

func (d *pouchImageService) createRuntimeClientIfNecessary() error {
	d.Lock()
	defer d.Unlock()
	if d.client != nil {
		return nil
	}
	c, err := pouchapi.NewAPIClient(d.runtimeURI, pouchapi.TLSConfig{})
	if err != nil {
		return err
	}
	d.client = c
	return nil
}

func (d *pouchImageService) handleRuntimeError(err error) {
	if daemonutil.FilterCloseErr(err) {
		d.Lock()
		defer d.Unlock()
		d.client = nil
	}
}

func (d *pouchImageService) PullImage(ctx context.Context, imageName, tag string, pullSecrets []v1.Secret, _ *appsv1alpha1.SandboxConfig) (reader ImagePullStatusReader, err error) {
	if err = d.createRuntimeClientIfNecessary(); err != nil {
		return nil, err
	}

	registry := daemonutil.ParseRegistry(imageName)
	var ioReader io.ReadCloser

	if len(pullSecrets) > 0 {
		var authInfos []daemonutil.AuthInfo
		authInfos, err = secret.ConvertToRegistryAuths(pullSecrets, registry)
		if err == nil {
			var pullErrs []error
			for _, authInfo := range authInfos {
				var pullErr error
				klog.V(5).Infof("Pull image %v:%v with user %v", imageName, tag, authInfo.Username)
				ioReader, pullErr = d.client.ImagePull(ctx, imageName, tag, authInfo.EncodeToString())
				if pullErr == nil {
					return newImagePullStatusReader(ioReader), nil
				}
				d.handleRuntimeError(pullErr)
				klog.Warningf("Failed to pull image %v:%v with user %v, err %v", imageName, tag, authInfo.Username, pullErr)
				pullErrs = append(pullErrs, pullErr)
			}
			if len(pullErrs) > 0 {
				err = utilerrors.NewAggregate(pullErrs)
			}
		}
	}

	// Try the default secret
	if d.accountManager != nil {
		var authInfo *daemonutil.AuthInfo
		var defaultErr error
		authInfo, defaultErr = d.accountManager.GetAccountInfo(registry)
		if defaultErr != nil {
			klog.Warningf("Failed to get account for registry %v, err %v", registry, defaultErr)
			// When the default account acquisition fails, try to pull anonymously
		} else if authInfo != nil {
			klog.V(5).Infof("Pull image %v:%v with user %v", imageName, tag, authInfo.Username)
			ioReader, err = d.client.ImagePull(ctx, imageName, tag, authInfo.EncodeToString())
			if err == nil {
				return newImagePullStatusReader(ioReader), nil
			}
			d.handleRuntimeError(err)
			klog.Warningf("Failed to pull image %v:%v with user %v, err %v", imageName, tag, authInfo.Username, err)
		}
	}

	if err != nil {
		return nil, err
	}

	// Anonymous pull
	klog.V(5).Infof("Pull image %v:%v anonymous", imageName, tag)
	ioReader, err = d.client.ImagePull(ctx, imageName, tag, "")
	if err != nil {
		d.handleRuntimeError(err)
		return nil, fmt.Errorf("anonymous pulling failed, err %v", err)
	}
	return newImagePullStatusReader(ioReader), nil
}

func (d *pouchImageService) ListImages(ctx context.Context) ([]ImageInfo, error) {
	if err := d.createRuntimeClientIfNecessary(); err != nil {
		return nil, err
	}

	infos, err := d.client.ImageList(ctx, pouchfilters.NewArgs())
	if err != nil {
		d.handleRuntimeError(err)
		return nil, err
	}
	return newImageCollectionPouch(infos), nil
}

func newImageCollectionPouch(infos []pouchtypes.ImageInfo) []ImageInfo {
	collection := make([]ImageInfo, 0, len(infos))
	for _, info := range infos {
		collection = append(collection, ImageInfo{
			ID:          info.ID,
			RepoTags:    info.RepoTags,
			RepoDigests: info.RepoDigests,
			Size:        info.Size,
		})
	}
	return collection
}
