/*
Copyright 2024 The Kruise Authors.

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
	"testing"

	"google.golang.org/grpc"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestJoinImageRef(t *testing.T) {
	const dgst = "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"

	cases := []struct {
		name     string
		image    string
		tag      string
		expected string
	}{{
		name:     "a tag joins with a colon",
		image:    "docker.io/library/nginx",
		tag:      "1.9.1",
		expected: "docker.io/library/nginx:1.9.1",
	}, {
		name:     "a digest joins with an at-sign",
		image:    "docker.io/library/nginx",
		tag:      dgst,
		expected: "docker.io/library/nginx@" + dgst,
	}, {
		name:     "a tag that merely mentions sha256 is still a tag",
		image:    "docker.io/library/nginx",
		tag:      "sha256-rebuild",
		expected: "docker.io/library/nginx:sha256-rebuild",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinImageRef(tc.image, tc.tag); got != tc.expected {
				t.Errorf("joinImageRef(%q, %q) = %q, want %q", tc.image, tc.tag, got, tc.expected)
			}
		})
	}
}

// recordingImageClient records the reference each pull requests. The embedded
// interface supplies the methods these tests do not call.
type recordingImageClient struct {
	runtimeapi.ImageServiceClient
	pulled []string
}

func (c *recordingImageClient) PullImage(_ context.Context, req *runtimeapi.PullImageRequest, _ ...grpc.CallOption) (*runtimeapi.PullImageResponse, error) {
	c.pulled = append(c.pulled, req.GetImage().GetImage())
	return &runtimeapi.PullImageResponse{}, nil
}

// TestPullImageReference covers what the CRI is actually asked to pull, which is
// the reference joinImageRef builds.
func TestPullImageReference(t *testing.T) {
	const (
		imageName = "docker.io/library/nginx"
		dgst      = "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"
	)

	cases := []struct {
		name      string
		tagOrDgst string
		expected  string
	}{{
		name:      "a tag is pulled as name:tag",
		tagOrDgst: "1.9.1",
		expected:  imageName + ":1.9.1",
	}, {
		name:      "a digest is pulled as name@digest",
		tagOrDgst: dgst,
		expected:  imageName + "@" + dgst,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, api := range []string{"v1", "v1alpha2"} {
				t.Run(api, func(t *testing.T) {
					client := &recordingImageClient{}
					service := &commonCRIImageService{}
					pull := service.pullImageV1
					if api == "v1" {
						service.criImageClient = client
					} else {
						service.criImageClientV1alpha2 = client
						pull = service.pullImageV1alpha2
					}

					reader, err := pull(context.Background(), imageName, tc.tagOrDgst, nil, nil)
					if err != nil {
						t.Fatalf("pull %s(%q, %q) failed: %v", api, imageName, tc.tagOrDgst, err)
					}
					reader.Close()

					if len(client.pulled) != 1 || client.pulled[0] != tc.expected {
						t.Errorf("CRI was asked to pull %q, want [%q]", client.pulled, tc.expected)
					}
				})
			}
		})
	}
}
