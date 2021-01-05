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

package runtime

import (
	"context"
	"io"
	"sync"

	dockertypes "github.com/docker/docker/api/types"
	dockerapi "github.com/docker/docker/client"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

// NewDockerImageRuntime create a docker runtime
func NewDockerImageRuntime(runtimeURI string, accountManager daemonutil.ImagePullAccountManager) (ImageRuntime, error) {
	r := &dockerImageRuntime{runtimeURI: runtimeURI, accountManager: accountManager}
	if err := r.createRuntimeClientIfNecessary(); err != nil {
		return nil, err
	}
	return r, nil
}

type dockerImageRuntime struct {
	sync.Mutex
	runtimeURI     string
	accountManager daemonutil.ImagePullAccountManager

	client *dockerapi.Client
}

func (d *dockerImageRuntime) createRuntimeClientIfNecessary() error {
	d.Lock()
	defer d.Unlock()
	if d.client != nil {
		return nil
	}
	c, err := dockerapi.NewClient(d.runtimeURI, "1.23", nil, nil)
	if err != nil {
		return err
	}
	d.client = c
	return nil
}

func (d *dockerImageRuntime) handleRuntimeError(err error) {
	if filterCloseErr(err) {
		d.Lock()
		defer d.Unlock()
		d.client = nil
	}
}

func (d *dockerImageRuntime) PullImage(ctx context.Context, imageName, tag string, pullSecrets []v1.Secret) (reader ImagePullStatusReader, err error) {
	if err = d.createRuntimeClientIfNecessary(); err != nil {
		return nil, err
	}

	registry := daemonutil.ParseRegistry(imageName)
	fullName := imageName + ":" + tag
	var ioReader io.ReadCloser
	var authInfo *daemonutil.AuthInfo

	if len(pullSecrets) > 0 {
		for _, secret := range pullSecrets {
			authInfo, err = convertToRegistryAuthInfo(secret, registry)
			if err == nil {
				klog.V(5).Infof("Pull image %v:%v with user %v", imageName, tag, authInfo.Username)
				ioReader, err = d.client.ImagePull(ctx, fullName, dockertypes.ImagePullOptions{RegistryAuth: authInfo.EncodeToString()})
				if err == nil {
					return newImagePullStatusReader(ioReader), nil
				}
				d.handleRuntimeError(err)
				klog.Warningf("Failed to pull image %v:%v with user %v, err %v", imageName, tag, authInfo.Username, err)
			}
		}
	}

	// Try the default secret
	if d.accountManager != nil {
		authInfo, err = d.accountManager.GetAccountInfo(registry)
		if err != nil {
			klog.Warningf("Failed to get account for registry %v, err %v", registry, err)
			// When the default account acquisition fails, try to pull anonymously
		} else if authInfo != nil {
			klog.V(5).Infof("Pull image %v:%v with user %v", imageName, tag, authInfo.Username)
			ioReader, err = d.client.ImagePull(ctx, fullName, dockertypes.ImagePullOptions{RegistryAuth: authInfo.EncodeToString()})
			if err == nil {
				return newImagePullStatusReader(ioReader), nil
			}
			d.handleRuntimeError(err)
			klog.Warningf("Failed to pull image %v:%v, err %v", imageName, tag, err)
		}
	}

	// Anonymous pull
	if len(pullSecrets) == 0 {
		klog.V(5).Infof("Pull image %v:%v anonymous", imageName, tag)
		ioReader, err = d.client.ImagePull(ctx, fullName, dockertypes.ImagePullOptions{})
		if err != nil {
			d.handleRuntimeError(err)
			return nil, err
		}
		return newImagePullStatusReader(ioReader), nil
	}
	return nil, err
}

func (d *dockerImageRuntime) ListImages(ctx context.Context) ([]ImageInfo, error) {
	if err := d.createRuntimeClientIfNecessary(); err != nil {
		return nil, err
	}
	infos, err := d.client.ImageList(ctx, dockertypes.ImageListOptions{All: true})
	if err != nil {
		d.handleRuntimeError(err)
		return nil, err
	}
	return newImageCollectionDocker(infos), nil
}

func newImageCollectionDocker(infos []dockertypes.ImageSummary) []ImageInfo {
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
