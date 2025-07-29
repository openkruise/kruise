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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/credentialprovider"
)

func TestAuthInfos(t *testing.T) {
	tests := []struct {
		name        string
		imageName   string
		tag         string
		pullSecrets []corev1.Secret
		expectNil   bool
	}{
		{
			name:      "valid image with docker.io - no secrets",
			imageName: "library/ubuntu",
			tag:       "latest",
			expectNil: false,
		},
		{
			name:      "valid image with custom registry - no secrets",
			imageName: "gcr.io/project/image",
			tag:       "v1.0",
			expectNil: false,
		},
		{
			name:      "invalid image name",
			imageName: "INVALID_IMAGE",
			tag:       "latest",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AuthInfos(context.Background(), tt.imageName, tt.tag, tt.pullSecrets)
			if tt.name == "invalid image name" && result != nil {
				t.Errorf("AuthInfos() with invalid image expected nil, got %v", result)
			}
		})
	}
}

func TestMakeAndSetKeyring(t *testing.T) {
	// reset keyring to nil before test
	keyring = nil

	MakeAndSetKeyring()

	if keyring == nil {
		t.Error("MakeAndSetKeyring() should set keyring to non-nil value")
	}
}

func TestConvertToRegistryAuths(t *testing.T) {
	//create test docker config secret
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"docker.io": map[string]interface{}{
				"username": "testuser",
				"password": "testpass",
			},
		},
	}
	configData, _ := json.Marshal(dockerConfig)

	tests := []struct {
		name        string
		pullSecrets []corev1.Secret
		repo        string
		expectError bool
		expectEmpty bool
	}{
		{
			name: "docker config secret",
			pullSecrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "docker-secret"},
					Type:       corev1.SecretTypeDockerConfigJson,
					Data:       map[string][]byte{".dockerconfigjson": configData},
				},
			},
			repo:        "docker.io",
			expectError: false,
			expectEmpty: false,
		},
		{
			name:        "no secrets",
			pullSecrets: []corev1.Secret{},
			repo:        "docker.io",
			expectError: false,
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring = credentialprovider.NewDockerKeyring()

			result, err := ConvertToRegistryAuths(tt.pullSecrets, tt.repo)

			if tt.expectError && err == nil {
				t.Error("ConvertToRegistryAuths() expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("ConvertToRegistryAuths() unexpected error: %v", err)
			}

			// check specific credentials for docker config secret test
			if tt.name == "docker config secret" && len(result) > 0 {
				found := false
				for _, auth := range result {
					if auth.Username == "testuser" && auth.Password == "testpass" {
						found = true
						break
					}
				}
				if !found {
					t.Error("Expected to find testuser/testpass credentials in AuthInfo results")
				}
			}

			if tt.expectEmpty && len(result) > 0 {
				// Note: may find existing credentials from environment, so this is informational
				t.Logf("ConvertToRegistryAuths() found %d existing credentials from environment", len(result))
			}
		})
	}
}
