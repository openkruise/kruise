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
	"fmt"
	"os"

	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"k8s.io/klog"
)

// Factory is the interface to get container and image runtime service
type Factory interface {
	GetImageRuntime() ImageRuntime
}

type ContainerRuntimeType string

const (
	ContainerRuntimeDocker     = "docker"
	ContainerRuntimeContainerd = "containerd"
	ContainerRuntimePouch      = "pouch"
	ContainerRuntimeOther      = "other"
)

type runtimeConfig struct {
	runtimeType ContainerRuntimeType
	runtimeURI  string
}

type factory struct {
	runtimeConfig
	imageRuntime ImageRuntime
}

func NewFactory(varRunPath string, accountManager daemonutil.ImagePullAccountManager) (Factory, error) {
	cfgs := detectRuntime(varRunPath)
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("not found container runtime sock")
	}

	var err error
	var imageRuntime ImageRuntime
	var cfg runtimeConfig
	for i := range cfgs {
		cfg = cfgs[i]
		switch cfg.runtimeType {
		case ContainerRuntimeDocker:
			imageRuntime, err = NewDockerImageRuntime(cfg.runtimeURI, accountManager)
		case ContainerRuntimePouch:
			imageRuntime, err = NewPouchImageRuntime(cfg.runtimeURI, accountManager)
		case ContainerRuntimeContainerd:
			imageRuntime, err = NewContainerdImageRuntime(cfg.runtimeURI, accountManager)
		case ContainerRuntimeOther:
			err = fmt.Errorf("not found container runtime sock")
		}
		if err != nil {
			klog.Warningf("Failed to new image runtime for %v(%s): %v", cfg.runtimeType, cfg.runtimeURI, err)
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}

	return &factory{
		runtimeConfig: cfg,
		imageRuntime:  imageRuntime,
	}, nil
}

func (f *factory) GetImageRuntime() ImageRuntime {
	return f.imageRuntime
}

func detectRuntime(varRunPath string) []runtimeConfig {
	var err error
	var cfgs []runtimeConfig
	if _, err = os.Stat(fmt.Sprintf("%s/pouchd.sock", varRunPath)); err == nil {
		cfgs = append(cfgs, runtimeConfig{runtimeType: ContainerRuntimePouch, runtimeURI: fmt.Sprintf("unix://%s/pouchd.sock", varRunPath)})
	}
	if _, err = os.Stat(fmt.Sprintf("%s/docker.sock", varRunPath)); err == nil {
		cfgs = append(cfgs, runtimeConfig{runtimeType: ContainerRuntimeDocker, runtimeURI: fmt.Sprintf("unix://%s/docker.sock", varRunPath)})
	}
	if _, err = os.Stat(fmt.Sprintf("%s/pouchcri.sock", varRunPath)); err == nil {
		cfgs = append(cfgs, runtimeConfig{runtimeType: ContainerRuntimeContainerd, runtimeURI: fmt.Sprintf("%s/pouchcri.sock", varRunPath)})
	}
	if _, err = os.Stat(fmt.Sprintf("%s/containerd.sock", varRunPath)); err == nil {
		cfgs = append(cfgs, runtimeConfig{runtimeType: ContainerRuntimeContainerd, runtimeURI: fmt.Sprintf("%s/containerd.sock", varRunPath)})
	}
	if _, err = os.Stat(fmt.Sprintf("%s/containerd/containerd.sock", varRunPath)); err == nil {
		cfgs = append(cfgs, runtimeConfig{runtimeType: ContainerRuntimeContainerd, runtimeURI: fmt.Sprintf("%s/containerd/containerd.sock", varRunPath)})
	}
	return cfgs
}
