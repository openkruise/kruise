//go:build !windows
// +build !windows

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
	"flag"
	"fmt"
	"os"

	"k8s.io/klog/v2"
)

const (
	varRunMountPath = "/hostvarrun"
)

var (
	statFunc          = os.Stat
	criSocketFileName = flag.String("socket-file", "", "The name of CRI socket file, and it should be in the mounted /hostvarrun directory.")
)

func detectRuntime() (cfgs []runtimeConfig) {
	var err error

	// firstly check if it is configured from flag
	if criSocketFileName != nil && len(*criSocketFileName) > 0 {
		filePath := fmt.Sprintf("%s/%s", varRunMountPath, *criSocketFileName)
		if _, err = statFunc(filePath); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/%s", varRunMountPath, *criSocketFileName),
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
		if _, err = statFunc(fmt.Sprintf("%s/containerd.sock", varRunMountPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeContainerd,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/containerd.sock", varRunMountPath),
			})
		}
		if _, err = statFunc(fmt.Sprintf("%s/containerd/containerd.sock", varRunMountPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeContainerd,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/containerd/containerd.sock", varRunMountPath),
			})
		}
	}

	// cri-o
	{
		if _, err = statFunc(fmt.Sprintf("%s/crio.sock", varRunMountPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/crio.sock", varRunMountPath),
			})
		}
		if _, err = statFunc(fmt.Sprintf("%s/crio/crio.sock", varRunMountPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/crio/crio.sock", varRunMountPath),
			})
		}
	}

	// cri-docker dockerd as a compliant Container Runtime Interface, detail see https://github.com/Mirantis/cri-dockerd
	{
		if _, err = statFunc(fmt.Sprintf("%s/cri-dockerd.sock", varRunMountPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/cri-dockerd.sock", varRunMountPath),
			})
		}
		// Check if the cri-dockerd runtime socket exists in the expected k3s runtime directory.
		// If found, append it to the runtime configuration list to ensure k3s can use cri-dockerd.
		if _, err = statFunc(fmt.Sprintf("%s/cri-dockerd/cri-dockerd.sock", varRunMountPath)); err == nil {
			cfgs = append(cfgs, runtimeConfig{
				runtimeType:      ContainerRuntimeCommonCRI,
				runtimeRemoteURI: fmt.Sprintf("unix://%s/cri-dockerd/cri-dockerd.sock", varRunMountPath),
			})
		}
	}
	return cfgs
}
