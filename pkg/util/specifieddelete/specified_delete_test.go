/*
Copyright 2020 The Kruise Authors.

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

package specifieddelete

import (
	"context"
	"testing"

	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsSpecifiedDelete(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "label not present",
			labels:   map[string]string{"foo": "bar"},
			expected: false,
		},
		{
			name:     "label present",
			labels:   map[string]string{v1alpha1.SpecifiedDeleteKey: "true"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{}
			pod.SetLabels(tt.labels)

			result := IsSpecifiedDelete(pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPatchPodSpecifiedDelete(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: "default",
			Labels:    map[string]string{},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	// Test: patching when label is missing
	patched, err := PatchPodSpecifiedDelete(fakeClient, pod, "true")
	assert.NoError(t, err)
	assert.True(t, patched)

	// Fetch the updated pod
	updatedPod := &corev1.Pod{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      "mypod",
		Namespace: "default",
	}, updatedPod)
	assert.NoError(t, err)
	assert.Equal(t, "true", updatedPod.Labels[v1alpha1.SpecifiedDeleteKey])

	// Test: patching again should return false, since label is now set
	patchedAgain, err := PatchPodSpecifiedDelete(fakeClient, updatedPod, "true")
	assert.NoError(t, err)
	assert.False(t, patchedAgain)
}
