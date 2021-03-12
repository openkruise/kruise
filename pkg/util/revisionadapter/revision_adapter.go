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

package revisionadapter

import (
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Interface interface {
	EqualToRevisionHash(controllerKey string, obj metav1.Object, hash string) bool
	WriteRevisionHash(obj metav1.Object, hash string)
}

func NewDefaultImpl() Interface {
	return &defaultImpl{}
}

type defaultImpl struct{}

func (r *defaultImpl) EqualToRevisionHash(_ string, obj metav1.Object, hash string) bool {
	return obj.GetLabels()[apps.ControllerRevisionHashLabelKey] == hash
}

func (r *defaultImpl) WriteRevisionHash(obj metav1.Object, hash string) {
	if obj.GetLabels() == nil {
		obj.SetLabels(make(map[string]string, 1))
	}
	obj.GetLabels()[apps.ControllerRevisionHashLabelKey] = hash
}
