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

package criruntime

import (
	"context"
	"fmt"
	"os"
	"time"

	runtimecontainer "github.com/openkruise/kruise/pkg/daemon/criruntime/containerruntime"
	runtimeimage "github.com/openkruise/kruise/pkg/daemon/criruntime/imageruntime"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"google.golang.org/grpc"
	criapi "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog"
	kubeletremote "k8s.io/kubernetes/pkg/kubelet/remote"
	kubeletutil "k8s.io/kubernetes/pkg/kubelet/util"
)

const (
	kubeRuntimeAPIVersion = "0.1.0"
)

// Factory is the interface to get container and image runtime service
type Factory interface {
	GetImageService() runtimeimage.ImageService
	GetContainerService() runtimecontainer.ContainerService
	GetRuntimeService() criapi.RuntimeService
	GetRuntimeServiceByName(runtimeName string) criapi.RuntimeService
}

type ContainerRuntimeType string

const (
	ContainerRuntimeDocker     = "docker"
	ContainerRuntimeContainerd = "containerd"
	ContainerRuntimePouch      = "pouch"
)

type runtimeConfig struct {
	runtimeType      ContainerRuntimeType
	runtimeURI       string
	runtimeRemoteURI string
}

type factory struct {
	impls []*runtimeImpl
}

type runtimeImpl struct {
	cfg              runtimeConfig
	runtimeName      string
	imageService     runtimeimage.ImageService
	containerService runtimecontainer.ContainerService
	runtimeService   criapi.RuntimeService
}

func NewFactory(varRunPath string, accountManager daemonutil.ImagePullAccountManager) (Factory, error) {
	cfgs := detectRuntime(varRunPath)
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("not found container runtime sock")
	}

	var err error
	f := &factory{}

	var cfg runtimeConfig
	for i := range cfgs {
		cfg = cfgs[i]
		var imageService runtimeimage.ImageService
		var containerService runtimecontainer.ContainerService
		var runtimeService criapi.RuntimeService
		var typedVersion *runtimeapi.VersionResponse

		switch cfg.runtimeType {
		case ContainerRuntimeDocker:
			imageService, err = runtimeimage.NewDockerImageService(cfg.runtimeURI, accountManager)
			if err != nil {
				klog.Warningf("Failed to new image service for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
				continue
			}
			containerService, err = runtimecontainer.NewDockerContainerService(cfg.runtimeURI)
			if err != nil {
				klog.Warningf("Failed to new container service for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
				continue
			}
		case ContainerRuntimePouch:
			imageService, err = runtimeimage.NewPouchImageService(cfg.runtimeURI, accountManager)
			if err != nil {
				klog.Warningf("Failed to new image service for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
				continue
			}
			containerService, err = runtimecontainer.NewPouchContainerService(cfg.runtimeURI)
			if err != nil {
				klog.Warningf("Failed to new container service for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
				continue
			}
		case ContainerRuntimeContainerd:
			var conn *grpc.ClientConn
			addr, _, _ := kubeletutil.GetAddressAndDialer(cfg.runtimeRemoteURI)
			conn, err = getContainerdConn(addr)
			if err != nil {
				klog.Warningf("Failed to get connection for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
				continue
			}
			imageService, err = runtimeimage.NewContainerdImageService(conn, accountManager)
			if err != nil {
				klog.Warningf("Failed to new image service for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
				continue
			}
			containerService, err = runtimecontainer.NewContainerdContainerService(conn)
			if err != nil {
				klog.Warningf("Failed to new container service for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
				continue
			}
		}
		if _, err = imageService.ListImages(context.TODO()); err != nil {
			klog.Warningf("Failed to list images for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
			continue
		}

		runtimeService, err = kubeletremote.NewRemoteRuntimeService(cfg.runtimeRemoteURI, time.Second*5)
		if err != nil {
			klog.Warningf("Failed to new runtime service for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
			continue
		}
		typedVersion, err = runtimeService.Version(kubeRuntimeAPIVersion)
		if err != nil {
			klog.Warningf("Failed to get runtime typed version for %v (%s, %s): %v", cfg.runtimeType, cfg.runtimeURI, cfg.runtimeRemoteURI, err)
			continue
		}

		klog.V(2).Infof("Add runtime impl %v, URI: (%s, %s)", typedVersion.RuntimeName, cfg.runtimeURI, cfg.runtimeRemoteURI)
		f.impls = append(f.impls, &runtimeImpl{
			cfg:              cfg,
			runtimeName:      typedVersion.RuntimeName,
			imageService:     imageService,
			containerService: containerService,
			runtimeService:   runtimeService,
		})
	}
	if len(f.impls) == 0 {
		return nil, err
	}

	return f, nil
}

func (f *factory) GetImageService() runtimeimage.ImageService {
	return f.impls[0].imageService
}

func (f *factory) GetContainerService() runtimecontainer.ContainerService {
	return f.impls[0].containerService
}

func (f *factory) GetRuntimeService() criapi.RuntimeService {
	return f.impls[0].runtimeService
}

func (f *factory) GetRuntimeServiceByName(runtimeName string) criapi.RuntimeService {
	for _, impl := range f.impls {
		if impl.runtimeName == runtimeName {
			return impl.runtimeService
		}
	}
	return nil
}

func detectRuntime(varRunPath string) []runtimeConfig {
	var err error
	var cfgs []runtimeConfig

	// pouch
	{
		_, err1 := os.Stat(fmt.Sprintf("%s/pouchd.sock", varRunPath))
		_, err2 := os.Stat(fmt.Sprintf("%s/pouchcri.sock", varRunPath))
		if err1 == nil && err2 == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimePouch,
				runtimeURI:       fmt.Sprintf("unix://%s/pouchd.sock", varRunPath),
				runtimeRemoteURI: fmt.Sprintf("unix://%s/pouchcri.sock", varRunPath),
			})
		} else if err1 == nil && err2 != nil {
			klog.Errorf("%s/pouchd.sock exists, but not found %s/pouchcri.sock", varRunPath, varRunPath)
		} else if err1 != nil && err2 == nil {
			klog.Errorf("%s/pouchdcri.sock exists, but not found %s/pouchd.sock", varRunPath, varRunPath)
		}
	}

	// docker
	{
		_, err1 := os.Stat(fmt.Sprintf("%s/docker.sock", varRunPath))
		_, err2 := os.Stat(fmt.Sprintf("%s/dockershim.sock", varRunPath))
		if err1 == nil && err2 == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeDocker,
				runtimeURI:       fmt.Sprintf("unix://%s/docker.sock", varRunPath),
				runtimeRemoteURI: fmt.Sprintf("unix://%s/dockershim.sock", varRunPath),
			})
		} else if err1 == nil && err2 != nil {
			klog.Errorf("%s/docker.sock exists, but not found %s/dockershim.sock", varRunPath, varRunPath)
		} else if err1 != nil && err2 == nil {
			klog.Errorf("%s/dockershim.sock exists, but not found %s/docker.sock", varRunPath, varRunPath)
		}
	}

	// containerd
	{
		if _, err = os.Stat(fmt.Sprintf("%s/containerd.sock", varRunPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeContainerd,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/containerd.sock", varRunPath),
			})
		}
		if _, err = os.Stat(fmt.Sprintf("%s/containerd/containerd.sock", varRunPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeContainerd,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/containerd/containerd.sock", varRunPath),
			})
		}
	}

	return cfgs
}
