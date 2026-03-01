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

package util

import (
	"encoding/base64"
	"encoding/json"

	clientset "k8s.io/client-go/kubernetes"
)

type AuthInfo struct {
	Username string
	Password string
}

type AuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`

	// Email is an optional value associated with the username.
	// This field is deprecated and will be removed in a later
	// version of docker.
	Email string `json:"email,omitempty"`

	ServerAddress string `json:"serveraddress,omitempty"`

	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken string `json:"identitytoken,omitempty"`

	// RegistryToken is a bearer token to be sent to a registry
	RegistryToken string `json:"registrytoken,omitempty"`
}

func (i *AuthInfo) EncodeToString() string {
	authConfig := AuthConfig{
		Username: i.Username,
		Password: i.Password,
	}
	encodedJSON, _ := json.Marshal(authConfig)
	return base64.URLEncoding.EncodeToString(encodedJSON)
}

type ImagePullAccountManager interface {
	GetAccountInfo(repo string) (*AuthInfo, error)
}

// NewImagePullAccountManager returns an ImagePullAccountManager, defaults to be nil
func NewImagePullAccountManager(kubeClient clientset.Interface) ImagePullAccountManager {
	return nil
}
