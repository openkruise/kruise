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

package secret

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"testing"

	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/credentialprovider"
)

func init() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "false")
	flag.Set("alsologtostderr", "false")
	klog.SetOutput(io.Discard)

	if tmp, err := os.MkdirTemp("", "docker-config-"); err == nil {
		os.Setenv("DOCKER_CONFIG", tmp)
	}
}

// createDockerConfigSecret correctly creates a Kubernetes secret of type kubernetes.io/dockerconfigjson
func createDockerConfigSecret(name, registry, username, password string) corev1.Secret {
	dockerConfigEntry := credentialprovider.DockerConfigEntry{
		Username: username,
		Password: password,
	}
	dockerConfig := credentialprovider.DockerConfig{
		registry: dockerConfigEntry,
	}
	dockerConfigJSON := credentialprovider.DockerConfigJSON{
		Auths: dockerConfig,
	}
	marshaledJSON, _ := json.Marshal(dockerConfigJSON)
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: marshaledJSON,
		},
	}
}

func TestConvertToRegistryAuths(t *testing.T) {
	testCases := []struct {
		name        string
		secrets     []corev1.Secret
		repo        string
		expectAuths []daemonutil.AuthInfo
		expectErr   bool
	}{
		{
			name: "Matching secret found for docker.io",
			secrets: []corev1.Secret{
				createDockerConfigSecret("my-secret", "https://index.docker.io/v1/", "testuser", "testpass"),
			},
			repo: "index.docker.io",
			expectAuths: []daemonutil.AuthInfo{
				{Username: "testuser", Password: "testpass"},
			},
			expectErr: false,
		},
		{
			name: "No matching secret for repo",
			secrets: []corev1.Secret{
				createDockerConfigSecret("my-secret", "another.registry.com", "testuser", "testpass"),
			},
			repo:      "index.docker.io",
			expectErr: false,
		},
		{
			name: "Secret with invalid data",
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "bad-secret"},
					Type:       corev1.SecretTypeDockerConfigJson,
					Data: map[string][]byte{
						corev1.DockerConfigJsonKey: []byte(`{"invalid-json`),
					},
				},
			},
			repo:      "index.docker.io",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Because the original parse.go uses a global keyring, we must reset it here.
			keyring = nil
			auths, err := ConvertToRegistryAuths(tc.secrets, tc.repo)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tc.expectAuths, auths)
			}
		})
	}
}

func TestAuthInfos(t *testing.T) {
	sharedSecrets := []corev1.Secret{
		createDockerConfigSecret("docker-secret", "https://index.docker.io/v1/", "dockeruser", "dockerpass"),
		createDockerConfigSecret("gcr-secret", "gcr.io", "gcruser", "gcrpass"),
	}

	testCases := []struct {
		name        string
		imageName   string
		tag         string
		secrets     []corev1.Secret
		expectAuths []daemonutil.AuthInfo
	}{
		{
			name:      "Standard docker.io image",
			imageName: "library/ubuntu",
			tag:       "latest",
			secrets:   sharedSecrets,
			expectAuths: []daemonutil.AuthInfo{
				{Username: "dockeruser", Password: "dockerpass"},
			},
		},
		{
			name:      "gcr.io image",
			imageName: "gcr.io/my-project/my-image",
			tag:       "v1",
			secrets:   sharedSecrets,
			expectAuths: []daemonutil.AuthInfo{
				{Username: "gcruser", Password: "gcrpass"},
			},
		},
		{
			name:        "Image with no matching secret",
			imageName:   "quay.io/some/image",
			tag:         "1.0",
			secrets:     []corev1.Secret{createDockerConfigSecret("gcr-secret", "gcr.io", "gcruser", "gcrpass")},
			expectAuths: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Because the original parse.go uses a global keyring, we must reset it here.
			keyring = nil
			auths := AuthInfos(context.TODO(), tc.imageName, tc.tag, tc.secrets)
			assert.ElementsMatch(t, tc.expectAuths, auths)
		})
	}
}
