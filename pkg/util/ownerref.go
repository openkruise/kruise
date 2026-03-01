/*
Copyright 2022 The Kruise Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func HasOwnerRef(target, owner metav1.Object) bool {
	ownerUID := owner.GetUID()
	for _, ownerRef := range target.GetOwnerReferences() {
		if ownerRef.UID == ownerUID {
			return true
		}
	}
	return false
}

func RemoveOwnerRef(target, owner metav1.Object) bool {
	if !HasOwnerRef(target, owner) {
		return false
	}
	ownerUID := owner.GetUID()
	oldRefs := target.GetOwnerReferences()
	newRefs := make([]metav1.OwnerReference, len(oldRefs)-1)
	skip := 0
	for i := range oldRefs {
		if oldRefs[i].UID == ownerUID {
			skip = -1
		} else {
			newRefs[i+skip] = oldRefs[i]
		}
	}
	target.SetOwnerReferences(newRefs)
	return true
}

func SetOwnerRef(target, owner metav1.Object, gvk schema.GroupVersionKind) bool {
	if HasOwnerRef(target, owner) {
		return false
	}
	ownerRefs := append(
		target.GetOwnerReferences(),
		*metav1.NewControllerRef(owner, gvk),
	)
	target.SetOwnerReferences(ownerRefs)
	return true
}
