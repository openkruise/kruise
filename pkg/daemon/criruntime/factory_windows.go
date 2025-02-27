//go:build windows
// +build windows

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
	runtimeimage "github.com/openkruise/kruise/pkg/daemon/criruntime/imageruntime"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
)

var (
	// containerdRemoteURI is the remote URI for containerd.
	// On Windows the default CRI endpoint is npipe://./pipe/containerd-containerd .
	// source: https://kubernetes.io/docs/setup/production-environment/container-runtimes/#containerd
	containerdRemoteURI = `npipe://./pipe/containerd-containerd`
)

// detectRuntime returns containerd runtime config
// Windows node pools support only the containerd runtime for most Kubernetes service providers.
func detectRuntime() (cfgs []runtimeConfig) {
	cfgs = append(cfgs, runtimeConfig{
		runtimeType:      ContainerRuntimeContainerd,
		runtimeRemoteURI: containerdRemoteURI,
	})
	return cfgs
}

func newImageService(cfg runtimeConfig, accountManager daemonutil.ImagePullAccountManager) (runtimeimage.ImageService, error) {
	return runtimeimage.NewCRIImageService(cfg.runtimeRemoteURI, accountManager)
}
