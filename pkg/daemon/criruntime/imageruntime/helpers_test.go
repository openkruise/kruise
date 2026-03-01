/*
Copyright 2023 The Kruise Authors.

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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/credentialprovider/plugin"
	"k8s.io/kubernetes/pkg/util/parsers"

	"github.com/openkruise/kruise/pkg/util/secret"
)

func TestMatchRegistryAuths(t *testing.T) {
	cases := []struct {
		name       string
		Image      string
		GetSecrets func() []v1.Secret
		// fix issue https://github.com/openkruise/kruise/issues/1583
		ExpectMinValue int
	}{
		{
			name:  "test1",
			Image: "registry.private.com/app/echoserver:v1",
			GetSecrets: func() []v1.Secret {
				demo := v1.Secret{
					Data: map[string][]byte{
						v1.DockerConfigJsonKey: []byte(`{"auths":{"registry.private.com":{"username":"echoserver","password":"test","auth":"ZWNob3NlcnZlcjp0ZXN0"}}}`),
					},
					Type: v1.SecretTypeDockerConfigJson,
				}
				return []v1.Secret{demo}
			},
			ExpectMinValue: 1,
		},
		{
			name:  "test2",
			Image: "registry.private.com/app/echoserver:v1",
			GetSecrets: func() []v1.Secret {
				demo := v1.Secret{
					Data: map[string][]byte{
						v1.DockerConfigJsonKey: []byte(`{"auths":{"registry.private.com/app":{"username":"echoserver","password":"test","auth":"ZWNob3NlcnZlcjp0ZXN0"}}}`),
					},
					Type: v1.SecretTypeDockerConfigJson,
				}
				return []v1.Secret{demo}
			},
			ExpectMinValue: 1,
		},
		{
			name:  "test3",
			Image: "registry.private.com/app/echoserver@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			GetSecrets: func() []v1.Secret {
				demo := v1.Secret{
					Data: map[string][]byte{
						v1.DockerConfigJsonKey: []byte(`{"auths":{"registry.private.com":{"username":"echoserver","password":"test","auth":"ZWNob3NlcnZlcjp0ZXN0"}}}`),
					},
					Type: v1.SecretTypeDockerConfigJson,
				}
				return []v1.Secret{demo}
			},
			ExpectMinValue: 1,
		},
		{
			name:  "test4",
			Image: "nginx:v1",
			GetSecrets: func() []v1.Secret {
				demo := v1.Secret{
					Data: map[string][]byte{
						v1.DockerConfigJsonKey: []byte(`{"auths":{"docker.io/library/nginx":{"username":"echoserver","password":"test","auth":"ZWNob3NlcnZlcjp0ZXN0"}}}`),
					},
					Type: v1.SecretTypeDockerConfigJson,
				}
				return []v1.Secret{demo}
			},
			ExpectMinValue: 1,
		},
		{
			name:  "test5",
			Image: "registry.private.com/app/echoserver:v1",
			GetSecrets: func() []v1.Secret {
				demo := v1.Secret{
					Data: map[string][]byte{
						v1.DockerConfigJsonKey: []byte(`{"auths":{"registry.private.com/nginx":{"username":"echoserver","password":"test","auth":"ZWNob3NlcnZlcjp0ZXN0"}}}`),
					},
					Type: v1.SecretTypeDockerConfigJson,
				}
				return []v1.Secret{demo}
			},
			ExpectMinValue: 0,
		},
		{
			name:  "test credential plugin if matched",
			Image: "registry.plugin.com/test/echoserver:v1",
			GetSecrets: func() []v1.Secret {
				return []v1.Secret{}
			},
			ExpectMinValue: 1,
		},
	}
	pluginBinDir := "fake_plugin"
	pluginConfigFile := "fake_plugin/plugin-config.yaml"
	// credential plugin is configured for images with "registry.plugin.com" and "registry.private.com",
	// however, only images with "registry.plugin.com" will return a fake credential,
	// other images will be denied by the plugin and an error will be raised,
	// this is to test whether kruise could get expected auths if plugin fails to run
	err := plugin.RegisterCredentialProviderPlugins(pluginConfigFile, pluginBinDir)
	if err != nil {
		klog.ErrorS(err, "Failed to register credential provider plugins")
	}
	secret.MakeAndSetKeyring()
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			repoToPull, _, _, err := parsers.ParseImageName(cs.Image)
			if err != nil {
				t.Fatalf("ParseImageName failed: %s", err.Error())
			}
			infos, err := secret.ConvertToRegistryAuths(cs.GetSecrets(), repoToPull)
			if err != nil {
				t.Fatalf("convertToRegistryAuths failed: %s", err.Error())
			}
			if len(infos) < cs.ExpectMinValue {
				t.Fatalf("convertToRegistryAuths failed")
			}
		})
	}
}

func TestContainsImage(t *testing.T) {
	cases := []struct {
		name       string
		ImageName  string
		Tag        string
		ImageInfos []ImageInfo
		Expect     bool
	}{
		{
			name:      "test_nginx",
			ImageName: "nginx",
			Tag:       "latest",
			ImageInfos: []ImageInfo{{
				RepoTags: []string{"docker.io/library/nginx:latest"},
			},
			},
			Expect: true,
		},
		{
			name:      "test_test/nginx:1.0",
			ImageName: "test/nginx",
			Tag:       "1.0",
			ImageInfos: []ImageInfo{{
				RepoTags: []string{"docker.io/test/nginx:1.0"},
			},
			},
			Expect: true,
		},
		{
			name:      "test_test/nginx:1.0_false",
			ImageName: "test/nginx",
			Tag:       "1.0",
			ImageInfos: []ImageInfo{{
				RepoTags: []string{"docker.io/library/nginx:1.0"},
			},
			},
			Expect: false,
		},

		{
			name:      "test_docker.io/library/nginx",
			ImageName: "docker.io/library/nginx",
			Tag:       "latest",
			ImageInfos: []ImageInfo{{
				RepoTags: []string{"docker.io/library/nginx:latest"},
			},
			},
			Expect: false,
		},
		{
			name:      "test_1.1.1.1:8080/test/nginx",
			ImageName: "1.1.1.1:8080/test/nginx",
			Tag:       "latest",
			ImageInfos: []ImageInfo{{
				RepoTags: []string{"1.1.1.1:8080/test/nginx:latest"},
			},
			},
			Expect: true,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			for _, info := range cs.ImageInfos {
				res := info.ContainsImage(cs.ImageName, cs.Tag)
				if res != cs.Expect {
					t.Fatalf("ContainsImage failed")
				}
			}
		})
	}
}
