/*
Copyright 2025 The Kruise Authors.

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

package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeName(t *testing.T) {
	t.Run("NodeName is set", func(t *testing.T) {
		expectedNodeName := "my-test-node"
		t.Setenv("NODE_NAME", expectedNodeName)
		nodeName, err := NodeName()
		assert.NoError(t, err)
		assert.Equal(t, expectedNodeName, nodeName)
	})

	t.Run("NodeName is not set", func(t *testing.T) {
		_, err := NodeName()
		assert.Error(t, err)
		assert.Equal(t, "not found NODE_NAME in environments", err.Error())
	})
}

func TestParseRegistry(t *testing.T) {
	cases := []struct {
		name      string
		imageName string
		want      string
	}{
		{name: "image with docker.io", imageName: "docker.io/library/ubuntu", want: "docker.io"},
		{name: "image with gcr.io", imageName: "gcr.io/project/image", want: "gcr.io"},
		{name: "image with registry and port", imageName: "myregistry.com:5000/user/image", want: "myregistry.com:5000"},
		{name: "image without registry", imageName: "ubuntu", want: "ubuntu"},
		{name: "empty image name", imageName: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseRegistry(tc.imageName)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseRepositoryTag(t *testing.T) {
	cases := []struct {
		name         string
		repoStr      string
		expectedRepo string
		expectedTag  string
	}{
		{name: "repo with tag", repoStr: "ubuntu:latest", expectedRepo: "ubuntu", expectedTag: "latest"},
		{name: "repo with tag and registry port", repoStr: "localhost.localdomain:5000/samalba/hipache:latest", expectedRepo: "localhost.localdomain:5000/samalba/hipache", expectedTag: "latest"},
		{name: "repo with digest", repoStr: "foo/bar@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb", expectedRepo: "foo/bar", expectedTag: "sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb"},
		{name: "repo with no tag", repoStr: "ubuntu", expectedRepo: "ubuntu", expectedTag: ""},
		{name: "repo with colon in path (invalid tag)", repoStr: "my/repo:v1/path", expectedRepo: "my/repo:v1/path", expectedTag: ""},
		{name: "empty repo string", repoStr: "", expectedRepo: "", expectedTag: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, tag := ParseRepositoryTag(tc.repoStr)
			assert.Equal(t, tc.expectedRepo, repo)
			assert.Equal(t, tc.expectedTag, tag)
		})
	}
}

func TestNormalizeImageRefToNameTag(t *testing.T) {
	digest := "sha256:f2b6de562150a257551639c432c6999337533816574519989a3f244195a63e63"
	cases := []struct {
		name         string
		ref          string
		expectedName string
		expectedTag  string
		expectErr    bool
	}{
		{name: "short name without tag", ref: "ubuntu", expectedName: "ubuntu", expectedTag: "latest", expectErr: false},
		{name: "short name with tag", ref: "ubuntu:20.04", expectedName: "ubuntu", expectedTag: "20.04", expectErr: false},
		{name: "full name with tag", ref: "docker.io/library/ubuntu:latest", expectedName: "ubuntu", expectedTag: "latest", expectErr: false},
		{name: "custom registry with tag", ref: "myregistry:5000/my/image:v1", expectedName: "myregistry:5000/my/image", expectedTag: "v1", expectErr: false},
		{name: "name with digest", ref: fmt.Sprintf("myregistry:5000/my/image@%s", digest), expectedName: "myregistry:5000/my/image", expectedTag: digest, expectErr: false},
		{name: "name with tag and digest", ref: fmt.Sprintf("ubuntu:20.04@%s", digest), expectedName: "ubuntu", expectedTag: digest, expectErr: false},
		{name: "invalid image ref", ref: "INVALID_IMAGE_REF", expectErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, tag, err := NormalizeImageRefToNameTag(tc.ref)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedName, name)
				assert.Equal(t, tc.expectedTag, tag)
			}
		})
	}
}

func TestNormalizeImageRef(t *testing.T) {
	digest := "sha256:f2b6de562150a257551639c432c6999337533816574519989a3f244195a63e63"
	cases := []struct {
		name               string
		ref                string
		expectedNormalized string
		expectErr          bool
	}{
		{name: "short name adds latest", ref: "ubuntu", expectedNormalized: "docker.io/library/ubuntu:latest", expectErr: false},
		{name: "short name with tag", ref: "ubuntu:20.04", expectedNormalized: "docker.io/library/ubuntu:20.04", expectErr: false},
		{name: "name with digest", ref: fmt.Sprintf("ubuntu@%s", digest), expectedNormalized: fmt.Sprintf("docker.io/library/ubuntu@%s", digest), expectErr: false},
		{name: "name with both tag and digest (digest should be preferred)", ref: fmt.Sprintf("ubuntu:20.04@%s", digest), expectedNormalized: fmt.Sprintf("docker.io/library/ubuntu@%s", digest), expectErr: false},
		{name: "invalid reference", ref: "UPPERCASE/image", expectErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			named, err := NormalizeImageRef(tc.ref)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedNormalized, named.String())
			}
		})
	}
}
