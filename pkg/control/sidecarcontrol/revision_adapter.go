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

package sidecarcontrol

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	RevisionAdapterImpl = &revisionAdapterImpl{}
)

type revisionAdapterImpl struct{}

func (r *revisionAdapterImpl) EqualToRevisionHash(sidecarSetName string, obj metav1.Object, hash string) bool {
	return GetPodSidecarSetRevision(sidecarSetName, obj) == hash
}

func (r *revisionAdapterImpl) WriteRevisionHash(obj metav1.Object, hash string) {
	// No need to implement yet.
}
