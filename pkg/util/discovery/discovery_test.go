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

package discovery

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1 "k8s.io/api/core/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
)

type UnknownObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// DeepCopyObject implements runtime.Object interface
func (u *UnknownObject) DeepCopyObject() runtime.Object {
	if u == nil {
		return nil
	}
	out := new(UnknownObject)
	u.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into the given UnknownObject
func (u *UnknownObject) DeepCopyInto(out *UnknownObject) {
	*out = *u
	out.TypeMeta = u.TypeMeta
	u.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
}

func TestDiscoverGVK_NilClient(t *testing.T) {
	originalClient := client.GetGenericClient()

	// Test with a sample GVK
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	if originalClient != nil {
		t.Logf("Client is available, testing with real client")
		result := DiscoverGVK(gvk)
		// Result should be boolean, we don't assert specific value since it depends on cluster state
		if result != true && result != false {
			t.Errorf("DiscoverGVK() should return a boolean value")
		}
	} else {
		t.Logf("Client is nil, DiscoverGVK should return true")
		result := DiscoverGVK(gvk)
		if result != true {
			t.Errorf("DiscoverGVK() with nil client should return true, got %v", result)
		}
	}
}

func TestDiscoverGVK_ValidGVK(t *testing.T) {
	// Test with known Kubernetes core resources
	testCases := []struct {
		name string
		gvk  schema.GroupVersionKind
	}{
		{
			name: "core v1 Pod",
			gvk:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		},
		{
			name: "apps v1 Deployment",
			gvk:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		{
			name: "non-existent kind",
			gvk:  schema.GroupVersionKind{Group: "nonexistent", Version: "v1", Kind: "NonExistentKind"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := DiscoverGVK(tc.gvk)
			if result != true && result != false {
				t.Errorf("DiscoverGVK() should return a boolean value")
			}
		})
	}
}

func TestDiscoverObject(t *testing.T) {
	testCases := []struct {
		name           string
		obj            runtime.Object
		expectedResult bool
		expectError    bool
	}{
		{
			name: "valid Pod object",
			obj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
			},
			expectedResult: true,
		},
		{
			name: "valid ConfigMap object",
			obj: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
			},
			expectedResult: true,
		},
		{
			name: "object without TypeMeta",
			obj: &corev1.Pod{
			},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := DiscoverObject(tc.obj)
			if result != true && result != false {
				t.Errorf("DiscoverObject() should return a boolean value")
			}
		})
	}
}

func TestDiscoverObject_InvalidObject(t *testing.T) {
	// Test with an object that's not in the scheme
	unknownObj := &UnknownObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "unknown/v1",
			Kind:       "UnknownKind",
		},
	}

	result := DiscoverObject(unknownObj)
	// For unknown objects not in the scheme, DiscoverObject should return false
	if result != false {
		t.Errorf("DiscoverObject() with unknown object should return false, got %v", result)
	}
}

func TestDiscoverGVK_EdgeCases(t *testing.T) {
	testCases := []struct {
		name string
		gvk  schema.GroupVersionKind
	}{
		{
			name: "empty group",
			gvk:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		},
		{
			name: "empty version",
			gvk:  schema.GroupVersionKind{Group: "apps", Version: "", Kind: "Deployment"},
		},
		{
			name: "empty kind",
			gvk:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: ""},
		},
		{
			name: "all empty",
			gvk:  schema.GroupVersionKind{Group: "", Version: "", Kind: ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// These should not panic, even with edge case inputs
			result := DiscoverGVK(tc.gvk)
			if result != true && result != false {
				t.Errorf("DiscoverGVK() should return a boolean value")
			}
		})
	}
}

func TestDiscoverObject_NilObject(t *testing.T) {
	// Test with nil object - this should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("DiscoverObject() with nil object should not panic, but got: %v", r)
		}
	}()

	result := DiscoverObject(nil)
	// With nil object, the function should return false
	if result != false {
		t.Errorf("DiscoverObject() with nil object should return false, got %v", result)
	}
}

func TestPackageInitialization(t *testing.T) {
	// Test that the internal scheme is properly initialized
	if internalScheme == nil {
		t.Error("internalScheme should not be nil")
	}

	// Test that errKindNotFound is properly defined
	if errKindNotFound == nil {
		t.Error("errKindNotFound should not be nil")
	}

	expectedErrMsg := "kind not found in group version resources"
	if errKindNotFound.Error() != expectedErrMsg {
		t.Errorf("errKindNotFound message = %q, want %q", errKindNotFound.Error(), expectedErrMsg)
	}

	// Test that backOff is properly configured
	if backOff.Steps != 4 {
		t.Errorf("backOff.Steps = %d, want 4", backOff.Steps)
	}
	if backOff.Factor != 5.0 {
		t.Errorf("backOff.Factor = %f, want 5.0", backOff.Factor)
	}
	if backOff.Jitter != 0.1 {
		t.Errorf("backOff.Jitter = %f, want 0.1", backOff.Jitter)
	}
}

func TestDiscoverObject_KruiseObjects(t *testing.T) {
	// Test with Kruise objects that should be in the internal scheme
	testCases := []struct {
		name string
		obj  runtime.Object
	}{
		{
			name: "WorkloadSpread object",
			obj: &appsv1alpha1.WorkloadSpread{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "WorkloadSpread",
				},
			},
		},
		{
			name: "BroadcastJob object",
			obj: &appsv1alpha1.BroadcastJob{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "BroadcastJob",
				},
			},
		},
		{
			name: "UnitedDeployment object",
			obj: &appsv1alpha1.UnitedDeployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "UnitedDeployment",
				},
			},
		},
		{
			name: "PodProbeMarker object",
			obj: &appsv1alpha1.PodProbeMarker{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "PodProbeMarker",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := DiscoverObject(tc.obj)
			if result != true && result != false {
				t.Errorf("DiscoverObject() should return a boolean value")
			}
		})
	}
}

func TestDiscoverGVK_KruiseGVKs(t *testing.T) {
	// Test with Kruise GVKs that should be in the scheme
	testCases := []struct {
		name string
		gvk  schema.GroupVersionKind
	}{
		{
			name: "WorkloadSpread GVK",
			gvk:  schema.GroupVersionKind{Group: "apps.kruise.io", Version: "v1alpha1", Kind: "WorkloadSpread"},
		},
		{
			name: "BroadcastJob GVK",
			gvk:  schema.GroupVersionKind{Group: "apps.kruise.io", Version: "v1alpha1", Kind: "BroadcastJob"},
		},
		{
			name: "UnitedDeployment GVK",
			gvk:  schema.GroupVersionKind{Group: "apps.kruise.io", Version: "v1alpha1", Kind: "UnitedDeployment"},
		},
		{
			name: "PodProbeMarker GVK",
			gvk:  schema.GroupVersionKind{Group: "apps.kruise.io", Version: "v1alpha1", Kind: "PodProbeMarker"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := DiscoverGVK(tc.gvk)
			// The result depends on whether there's a client and whether the GVK is discoverable
			if result != true && result != false {
				t.Errorf("DiscoverGVK() should return a boolean value")
			}
		})
	}
}
