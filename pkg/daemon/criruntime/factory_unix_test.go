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
	"os"
	"testing"
	"time"
)

type fileInfo struct {
	name string
}

func (f *fileInfo) Name() string       { return f.name }
func (f *fileInfo) Size() int64        { return 0 }
func (f *fileInfo) Mode() os.FileMode  { return 0644 }
func (f *fileInfo) ModTime() time.Time { return time.Time{} }
func (f *fileInfo) IsDir() bool        { return false }
func (f *fileInfo) Sys() interface{}   { return nil }

func TestDetectRuntimeUnix(t *testing.T) {
	testCases := []struct {
		name              string
		flag              string
		runtimeType       ContainerRuntimeType
		runtimeRemoteURI  string
		statFunc          func(name string) (os.FileInfo, error)
		expectedCfgsCount int
	}{
		{
			name:             "non-existent-socket-with-flag",
			flag:             "non-existent-socket",
			runtimeType:      ContainerRuntimeCommonCRI,
			runtimeRemoteURI: "unix:///hostvarrun/non-existent-socket",
			statFunc: func(name string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 0,
		},
		{
			name:             "crio.sock-with-flag",
			flag:             "crio.sock",
			runtimeType:      ContainerRuntimeCommonCRI,
			runtimeRemoteURI: "unix:///hostvarrun/crio.sock",
			statFunc: func(name string) (os.FileInfo, error) {
				if name == "/hostvarrun/crio.sock" {
					return &fileInfo{name: name}, nil
				}
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 1,
		},
		{
			name:             "containerd.sock",
			flag:             "",
			runtimeType:      ContainerRuntimeContainerd,
			runtimeRemoteURI: "unix:///hostvarrun/containerd.sock",
			statFunc: func(name string) (os.FileInfo, error) {
				if name == "/hostvarrun/containerd.sock" {
					return &fileInfo{name: name}, nil
				}
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 1,
		},
		{
			name:             "containerd/containerd.sock",
			flag:             "",
			runtimeType:      ContainerRuntimeContainerd,
			runtimeRemoteURI: "unix:///hostvarrun/containerd/containerd.sock",
			statFunc: func(name string) (os.FileInfo, error) {
				if name == "/hostvarrun/containerd/containerd.sock" {
					return &fileInfo{name: name}, nil
				}
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 1,
		},
		{
			name:             "crio.sock",
			flag:             "",
			runtimeType:      ContainerRuntimeCommonCRI,
			runtimeRemoteURI: "unix:///hostvarrun/crio.sock",
			statFunc: func(name string) (os.FileInfo, error) {
				if name == "/hostvarrun/crio.sock" {
					return &fileInfo{name: name}, nil
				}
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 1,
		},
		{
			name:             "crio/crio.sock",
			flag:             "",
			runtimeType:      ContainerRuntimeCommonCRI,
			runtimeRemoteURI: "unix:///hostvarrun/crio/crio.sock",
			statFunc: func(name string) (os.FileInfo, error) {
				if name == "/hostvarrun/crio/crio.sock" {
					return &fileInfo{name: name}, nil
				}
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 1,
		},
		{
			name:             "cri-dockerd",
			flag:             "",
			runtimeType:      ContainerRuntimeCommonCRI,
			runtimeRemoteURI: "unix:///hostvarrun/cri-dockerd.sock",
			statFunc: func(name string) (os.FileInfo, error) {
				if name == "/hostvarrun/cri-dockerd.sock" {
					return &fileInfo{name: name}, nil
				}
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 1,
		},
		{
			name:             "cri-dockerd/cri-dockerd.sock",
			flag:             "",
			runtimeType:      ContainerRuntimeCommonCRI,
			runtimeRemoteURI: "unix:///hostvarrun/cri-dockerd/cri-dockerd.sock",
			statFunc: func(name string) (os.FileInfo, error) {
				if name == "/hostvarrun/cri-dockerd/cri-dockerd.sock" {
					return &fileInfo{name: name}, nil
				}
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 1,
		},
		{
			name:             "non-existent-socket-without-flag",
			flag:             "",
			runtimeType:      ContainerRuntimeCommonCRI,
			runtimeRemoteURI: "unix:///hostvarrun/non-existent-socket",
			statFunc: func(name string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			},
			expectedCfgsCount: 0,
		},
	}

	for _, testCase := range testCases {
		flag.Set("socket-file", testCase.flag)
		statFunc = testCase.statFunc
		defer func() { statFunc = os.Stat }()

		cfgs := detectRuntime()
		if len(cfgs) != testCase.expectedCfgsCount {
			t.Fatalf("expected %d runtime config, got %d", testCase.expectedCfgsCount, len(cfgs))
		}
		if testCase.expectedCfgsCount > 0 {
			if cfgs[0].runtimeRemoteURI != testCase.runtimeRemoteURI {
				t.Fatalf("expected runtime remote URI to be %s, got %s", testCase.runtimeRemoteURI, cfgs[0].runtimeRemoteURI)
			}
			if cfgs[0].runtimeType != testCase.runtimeType {
				t.Fatalf("expected runtime type to be %s, got %s", testCase.runtimeType, cfgs[0].runtimeType)
			}
		}
	}
}
