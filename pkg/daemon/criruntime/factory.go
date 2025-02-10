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
	"flag"
	"fmt"
	"os"
	"time"

	oteltrace "go.opentelemetry.io/otel/trace"
	criapi "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
	criremote "k8s.io/kubernetes/pkg/kubelet/cri/remote"
	kubeletutil "k8s.io/kubernetes/pkg/kubelet/util"

	runtimeimage "github.com/openkruise/kruise/pkg/daemon/criruntime/imageruntime"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
)

const (
	kubeRuntimeAPIVersion = "0.1.0"
)

var (
	CRISocketFileName = flag.String("socket-file", "", "The name of CRI socket file, and it should be in the mounted /hostvarrun directory.")
)

// Factory is the interface to get container and image runtime service
type Factory interface {
	GetImageService() runtimeimage.ImageService
	GetRuntimeService() criapi.RuntimeService
	GetRuntimeServiceByName(runtimeName string) criapi.RuntimeService
}

type ContainerRuntimeType string

const (
	ContainerRuntimeContainerd = "containerd"
	ContainerRuntimeCommonCRI  = "common-cri"
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
	cfg            runtimeConfig
	runtimeName    string
	imageService   runtimeimage.ImageService
	runtimeService criapi.RuntimeService
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
		var runtimeService criapi.RuntimeService
		var typedVersion *runtimeapi.VersionResponse

		addr, _, err := kubeletutil.GetAddressAndDialer(cfg.runtimeRemoteURI)
		if err != nil {
			klog.ErrorS(err, "Failed to get address", "runtimeType", cfg.runtimeType, "runtimeURI", cfg.runtimeURI, "runtimeRemoteURI", cfg.runtimeRemoteURI)
			continue
		}
		imageService, err = runtimeimage.NewCRIImageService(addr, accountManager)
		if err != nil {
			klog.ErrorS(err, "Failed to new image service", "runtimeType", cfg.runtimeType, "runtimeURI", cfg.runtimeURI, "runtimeRemoteURI", cfg.runtimeRemoteURI)
			continue
		}

		if _, err = imageService.ListImages(context.TODO()); err != nil {
			klog.ErrorS(err, "Failed to list images", "runtimeType", cfg.runtimeType, "runtimeURI", cfg.runtimeURI, "runtimeRemoteURI", cfg.runtimeRemoteURI)
			continue
		}

		runtimeService, err = criremote.NewRemoteRuntimeService(cfg.runtimeRemoteURI, time.Second*5, oteltrace.NewNoopTracerProvider())
		if err != nil {
			klog.ErrorS(err, "Failed to new runtime service", "runtimeType", cfg.runtimeType, "runtimeURI", cfg.runtimeURI, "runtimeRemoteURI", cfg.runtimeRemoteURI)
			continue
		}
		typedVersion, err = runtimeService.Version(context.TODO(), kubeRuntimeAPIVersion)
		if err != nil {
			klog.ErrorS(err, "Failed to get runtime typed version", "runtimeType", cfg.runtimeType, "runtimeURI", cfg.runtimeURI, "runtimeRemoteURI", cfg.runtimeRemoteURI)
			continue
		}

		klog.V(2).InfoS("Add runtime", "runtimeName", typedVersion.RuntimeName, "runtimeURI", cfg.runtimeURI, "runtimeRemoteURI", cfg.runtimeRemoteURI)
		f.impls = append(f.impls, &runtimeImpl{
			cfg:            cfg,
			runtimeName:    typedVersion.RuntimeName,
			imageService:   imageService,
			runtimeService: runtimeService,
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

func detectRuntime(varRunPath string) (cfgs []runtimeConfig) {
	var err error

	// firstly check if it is configured from flag
	if CRISocketFileName != nil && len(*CRISocketFileName) > 0 {
		filePath := fmt.Sprintf("%s/%s", varRunPath, *CRISocketFileName)
		if _, err = os.Stat(filePath); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/%s", varRunPath, *CRISocketFileName),
			})
			klog.InfoS("Find configured CRI socket with given flag", "filePath", filePath)
		} else {
			klog.ErrorS(err, "Failed to stat the CRI socket with given flag", "filePath", filePath)
		}
		return
	}

	// if the flag is not set, then try to find runtime in the recognized types and paths.

	// containerd, with the same behavior of pullImage as commonCRI
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

	// cri-o
	{
		if _, err = os.Stat(fmt.Sprintf("%s/crio.sock", varRunPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/crio.sock", varRunPath),
			})
		}
		if _, err = os.Stat(fmt.Sprintf("%s/crio/crio.sock", varRunPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/crio/crio.sock", varRunPath),
			})
		}
	}

	// cri-docker dockerd as a compliant Container Runtime Interface, detail see https://github.com/Mirantis/cri-dockerd
	{
		if _, err = os.Stat(fmt.Sprintf("%s/cri-dockerd.sock", varRunPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/cri-dockerd.sock", varRunPath),
			})
		}
		// Check if the cri-dockerd runtime socket exists in the expected k3s runtime directory.
		// If found, append it to the runtime configuration list to ensure k3s can use cri-dockerd.
		if _, err = os.Stat(fmt.Sprintf("%s/cri-dockerd/cri-dockerd.sock", varRunPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/cri-dockerd/cri-dockerd.sock", varRunPath),
			})
		}
	}
	return cfgs
}
