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
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateFinalizer_AddFinalizer(t *testing.T) {
	tests := []struct {
		name           string
		initialPod     *corev1.Pod
		finalizer      string
		expectedResult []string
		expectError    bool
	}{
		{
			name: "add finalizer to pod without finalizers",
			initialPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			finalizer:      "test.finalizer.io/test",
			expectedResult: []string{"test.finalizer.io/test"},
			expectError:    false,
		},
		{
			name: "add finalizer to pod with existing finalizers",
			initialPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-pod",
					Namespace:  "default",
					Finalizers: []string{"existing.finalizer.io/test"},
				},
			},
			finalizer:      "test.finalizer.io/test",
			expectedResult: []string{"existing.finalizer.io/test", "test.finalizer.io/test"},
			expectError:    false,
		},
		{
			name: "add existing finalizer should not duplicate",
			initialPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-pod",
					Namespace:  "default",
					Finalizers: []string{"test.finalizer.io/test"},
				},
			},
			finalizer:      "test.finalizer.io/test",
			expectedResult: []string{"test.finalizer.io/test"},
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.initialPod).Build()

			err := UpdateFinalizer(fakeClient, tt.initialPod, AddFinalizerOpType, tt.finalizer)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify the finalizer was added correctly
			updatedPod := &corev1.Pod{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{
				Name:      tt.initialPod.Name,
				Namespace: tt.initialPod.Namespace,
			}, updatedPod)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, updatedPod.Finalizers)
		})
	}
}

func TestUpdateFinalizer_RemoveFinalizer(t *testing.T) {
	tests := []struct {
		name           string
		initialPod     *corev1.Pod
		finalizer      string
		expectedResult []string
		expectError    bool
	}{
		{
			name: "remove finalizer from pod with single finalizer",
			initialPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-pod",
					Namespace:  "default",
					Finalizers: []string{"test.finalizer.io/test"},
				},
			},
			finalizer:      "test.finalizer.io/test",
			expectedResult: []string{},
			expectError:    false,
		},
		{
			name: "remove finalizer from pod with multiple finalizers",
			initialPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-pod",
					Namespace:  "default",
					Finalizers: []string{"first.finalizer.io/test", "test.finalizer.io/test", "last.finalizer.io/test"},
				},
			},
			finalizer:      "test.finalizer.io/test",
			expectedResult: []string{"first.finalizer.io/test", "last.finalizer.io/test"},
			expectError:    false,
		},
		{
			name: "remove non-existing finalizer should not error",
			initialPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-pod",
					Namespace:  "default",
					Finalizers: []string{"existing.finalizer.io/test"},
				},
			},
			finalizer:      "non-existing.finalizer.io/test",
			expectedResult: []string{"existing.finalizer.io/test"},
			expectError:    false,
		},
		{
			name: "remove finalizer from pod without finalizers",
			initialPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			finalizer:      "test.finalizer.io/test",
			expectedResult: []string{},
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.initialPod).Build()

			err := UpdateFinalizer(fakeClient, tt.initialPod, RemoveFinalizerOpType, tt.finalizer)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Verify the finalizer was removed correctly
			updatedPod := &corev1.Pod{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{
				Name:      tt.initialPod.Name,
				Namespace: tt.initialPod.Namespace,
			}, updatedPod)
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedResult, updatedPod.Finalizers)
		})
	}
}

func TestUpdateFinalizer_InvalidOperation(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	// Test invalid operation type should panic
	assert.Panics(t, func() {
		_ = UpdateFinalizer(fakeClient, pod, FinalizerOpType("Invalid"), "test.finalizer.io/test")
	})
}

// mockClient is a mock implementation of client.Client for testing error scenarios
type mockClient struct {
	client.Client
	getError    error
	updateError error
	getCount    int
	updateCount int
}

func (m *mockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	m.getCount++
	if m.getError != nil {
		return m.getError
	}
	// For successful gets, we need to populate the object with some data
	if pod, ok := obj.(*corev1.Pod); ok {
		pod.Name = key.Name
		pod.Namespace = key.Namespace
		pod.Finalizers = []string{"existing.finalizer.io/test"}
	}
	return nil
}

func (m *mockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	m.updateCount++
	if m.updateError != nil {
		return m.updateError
	}
	return nil
}

func TestUpdateFinalizer_GetError(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	mockClient := &mockClient{
		getError: errors.New("failed to get object"),
	}

	err := UpdateFinalizer(mockClient, pod, AddFinalizerOpType, "test.finalizer.io/test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get object")
	assert.Equal(t, 1, mockClient.getCount)
	assert.Equal(t, 0, mockClient.updateCount)
}

func TestUpdateFinalizer_UpdateError(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	mockClient := &mockClient{
		updateError: errors.New("failed to update object"),
	}

	err := UpdateFinalizer(mockClient, pod, AddFinalizerOpType, "test.finalizer.io/test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update object")
	assert.Equal(t, 1, mockClient.getCount)
	assert.Equal(t, 1, mockClient.updateCount)
}

func TestUpdateFinalizer_DifferentObjectTypes(t *testing.T) {
	tests := []struct {
		name   string
		object client.Object
	}{
		{
			name: "ConfigMap",
			object: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "default",
				},
			},
		},
		{
			name: "Secret",
			object: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
			},
		},
		{
			name: "Service",
			object: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.object).Build()

			// Test adding finalizer
			err := UpdateFinalizer(fakeClient, tt.object, AddFinalizerOpType, "test.finalizer.io/test")
			assert.NoError(t, err)

			// Verify finalizer was added
			key := client.ObjectKeyFromObject(tt.object)
			updatedObj := tt.object.DeepCopyObject().(client.Object)
			err = fakeClient.Get(context.TODO(), key, updatedObj)
			assert.NoError(t, err)
			assert.Contains(t, updatedObj.GetFinalizers(), "test.finalizer.io/test")

			// Test removing finalizer
			err = UpdateFinalizer(fakeClient, tt.object, RemoveFinalizerOpType, "test.finalizer.io/test")
			assert.NoError(t, err)

			// Verify finalizer was removed
			err = fakeClient.Get(context.TODO(), key, updatedObj)
			assert.NoError(t, err)
			assert.NotContains(t, updatedObj.GetFinalizers(), "test.finalizer.io/test")
		})
	}
}

func TestUpdateFinalizer_EmptyFinalizerName(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	// Test adding empty finalizer name
	err := UpdateFinalizer(fakeClient, pod, AddFinalizerOpType, "")
	assert.NoError(t, err)

	// Verify empty finalizer was added
	updatedPod := &corev1.Pod{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}, updatedPod)
	assert.NoError(t, err)
	assert.Contains(t, updatedPod.Finalizers, "")

	// Test removing empty finalizer name
	err = UpdateFinalizer(fakeClient, pod, RemoveFinalizerOpType, "")
	assert.NoError(t, err)

	// Verify empty finalizer was removed
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}, updatedPod)
	assert.NoError(t, err)
	assert.NotContains(t, updatedPod.Finalizers, "")
}

func TestFinalizerOpType_Constants(t *testing.T) {
	// Test that the constants have the expected values
	assert.Equal(t, FinalizerOpType("Add"), AddFinalizerOpType)
	assert.Equal(t, FinalizerOpType("Remove"), RemoveFinalizerOpType)
}
