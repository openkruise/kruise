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
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetEmptyObjectWithKey return an empty object with the same namespaced name
func GetEmptyObjectWithKey(object client.Object) client.Object {
	var empty client.Object
	switch object.(type) {
	case *v1.Pod:
		empty = &v1.Pod{}
	case *v1.Service:
		empty = &v1.Service{}
	case *networkingv1.Ingress:
		empty = &networkingv1.Ingress{}
	case *apps.Deployment:
		empty = &apps.Deployment{}
	case *apps.ReplicaSet:
		empty = &apps.ReplicaSet{}
	case *apps.StatefulSet:
		empty = &apps.StatefulSet{}
	case *appsv1alpha1.CloneSet:
		empty = &appsv1alpha1.CloneSet{}
	case *appsv1beta1.StatefulSet:
		empty = &appsv1beta1.StatefulSet{}
	case *appsv1alpha1.DaemonSet:
		empty = &appsv1alpha1.DaemonSet{}
	case *unstructured.Unstructured:
		unstructure := &unstructured.Unstructured{}
		unstructure.SetGroupVersionKind(object.GetObjectKind().GroupVersionKind())
		empty = unstructure
	}
	empty.SetName(object.GetName())
	empty.SetNamespace(object.GetNamespace())
	return empty
}
