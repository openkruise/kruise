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
	"testing"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestGetEmptyObjectWithKey(t *testing.T) {
	tests := []struct {
		name           string
		inputObject    client.Object
		expectedType   client.Object
		checkNamespace bool
		checkName      bool
	}{
		{
			name: "Pod object",
			inputObject: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			expectedType:   &v1.Pod{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "Service object",
			inputObject: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "kube-system",
				},
			},
			expectedType:   &v1.Service{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "Ingress object",
			inputObject: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "default",
				},
			},
			expectedType:   &networkingv1.Ingress{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "Deployment object",
			inputObject: &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "prod",
				},
			},
			expectedType:   &apps.Deployment{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "ReplicaSet object",
			inputObject: &apps.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs",
					Namespace: "default",
				},
			},
			expectedType:   &apps.ReplicaSet{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "StatefulSet object",
			inputObject: &apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts",
					Namespace: "default",
				},
			},
			expectedType:   &apps.StatefulSet{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "CloneSet object",
			inputObject: &appsv1alpha1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cloneset",
					Namespace: "kruise-system",
				},
			},
			expectedType:   &appsv1alpha1.CloneSet{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "Advanced StatefulSet object",
			inputObject: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-advanced-sts",
					Namespace: "default",
				},
			},
			expectedType:   &appsv1beta1.StatefulSet{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "DaemonSet object",
			inputObject: &appsv1alpha1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds",
					Namespace: "kube-system",
				},
			},
			expectedType:   &appsv1alpha1.DaemonSet{},
			checkNamespace: true,
			checkName:      true,
		},
		{
			name: "Unstructured object with GVK",
			inputObject: func() client.Object {
				u := &unstructured.Unstructured{}
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "custom.io",
					Version: "v1",
					Kind:    "CustomResource",
				})
				u.SetName("custom-resource")
				u.SetNamespace("custom-ns")
				return u
			}(),
			expectedType:   &unstructured.Unstructured{},
			checkNamespace: true,
			checkName:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetEmptyObjectWithKey(tt.inputObject)

			// Check the type matches
			assert.IsType(t, tt.expectedType, result)

			// Check name and namespace are preserved
			if tt.checkName {
				assert.Equal(t, tt.inputObject.GetName(), result.GetName())
			}
			if tt.checkNamespace {
				assert.Equal(t, tt.inputObject.GetNamespace(), result.GetNamespace())
			}

			// For unstructured objects, verify GVK is preserved
			if unstructuredInput, ok := tt.inputObject.(*unstructured.Unstructured); ok {
				unstructuredResult, ok := result.(*unstructured.Unstructured)
				assert.True(t, ok, "result should be unstructured")
				assert.Equal(t, unstructuredInput.GetObjectKind().GroupVersionKind(),
					unstructuredResult.GetObjectKind().GroupVersionKind())
			}
		})
	}
}

func TestGetEmptyObjectWithKey_UnsupportedType(t *testing.T) {
	// Test with an unsupported resource type (not in the switch statement)
	// Current implementation will panic when calling SetName on nil 'empty' variable
	// This test documents the current behavior - consider fixing the implementation
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-configmap",
			Namespace: "default",
		},
	}

	// This will panic in the current implementation
	assert.Panics(t, func() {
		GetEmptyObjectWithKey(configMap)
	}, "GetEmptyObjectWithKey should panic for unsupported types")
}

func TestGetEmptyObjectWithKey_MetadataPreservation(t *testing.T) {
	// Test that only name and namespace are preserved, not other metadata
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
			Annotations: map[string]string{
				"key": "value",
			},
			UID: "12345",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
		},
	}

	result := GetEmptyObjectWithKey(pod)
	resultPod, ok := result.(*v1.Pod)
	assert.True(t, ok)

	// Name and namespace should be preserved
	assert.Equal(t, "test-pod", resultPod.GetName())
	assert.Equal(t, "default", resultPod.GetNamespace())

	// Other metadata should be empty/zero
	assert.Empty(t, resultPod.Labels)
	assert.Empty(t, resultPod.Annotations)
	assert.Empty(t, resultPod.UID)

	// Spec should be empty
	assert.Empty(t, resultPod.Spec.Containers)
}

func TestGetEmptyObjectWithKey_EmptyNamespace(t *testing.T) {
	// Test with a namespaced resource that has no namespace set
	// CloneSet is a namespaced resource, this tests empty namespace handling
	cloneSet := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cloneset",
			// No namespace set
		},
	}

	result := GetEmptyObjectWithKey(cloneSet)
	resultCloneSet, ok := result.(*appsv1alpha1.CloneSet)
	assert.True(t, ok)

	assert.Equal(t, "test-cloneset", resultCloneSet.GetName())
	assert.Empty(t, resultCloneSet.GetNamespace())
}
