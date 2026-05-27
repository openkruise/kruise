/*
Copyright 2026 The Kruise Authors.

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

package validating

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// TestMain enables the ResourceDistributionGate feature gate for all handler tests.
func TestMain(m *testing.M) {
	if err := utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.ResourceDistributionGate)); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// newHandlerForTest builds a handler backed by a fake client that has Secrets and
// Namespaces pre-populated so dry-run validation of spec.resource passes.
func newHandlerForTest() *ResourceDistributionCreateUpdateHandler {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-1"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-2"}},
	).Build()

	return &ResourceDistributionCreateUpdateHandler{
		Client:  fakeClient,
		Decoder: admission.NewDecoder(scheme),
	}
}

func newValidV1beta1RD() *appsv1beta1.ResourceDistribution {
	return &appsv1beta1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rd"},
		Spec: appsv1beta1.ResourceDistributionSpec{
			Resource: runtime.RawExtension{Raw: []byte(`{
				"apiVersion":"v1","kind":"Secret",
				"metadata":{"name":"my-secret"},
				"type":"Opaque","data":{"k":"dg=="}
			}`)},
			Targets: appsv1beta1.ResourceDistributionTargets{
				IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-1"}},
				},
			},
		},
	}
}

func newValidV1alpha1RD() *appsv1alpha1.ResourceDistribution {
	return &appsv1alpha1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rd"},
		Spec: appsv1alpha1.ResourceDistributionSpec{
			Resource: runtime.RawExtension{Raw: []byte(`{
				"apiVersion":"v1","kind":"Secret",
				"metadata":{"name":"my-secret"},
				"type":"Opaque","data":{"k":"dg=="}
			}`)},
			Targets: appsv1alpha1.ResourceDistributionTargets{
				IncludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
					List: []appsv1alpha1.ResourceDistributionNamespace{{Name: "ns-1"}},
				},
			},
		},
	}
}

// --- Create tests ---

func TestHandleV1beta1CreateAllowsValidSecret(t *testing.T) {
	h := newHandlerForTest()
	raw, err := json.Marshal(newValidV1beta1RD())
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: appsv1beta1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: raw},
		},
	})

	require.True(t, resp.Allowed)
	require.Equal(t, int32(http.StatusOK), resp.Result.Code)
}

func TestHandleV1alpha1CreateAllowsValidSecret(t *testing.T) {
	h := newHandlerForTest()
	raw, err := json.Marshal(newValidV1alpha1RD())
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource:  metav1.GroupVersionResource{Group: appsv1alpha1.GroupVersion.Group, Version: appsv1alpha1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: raw},
		},
	})

	require.True(t, resp.Allowed)
	require.Equal(t, int32(http.StatusOK), resp.Result.Code)
}

func TestHandleV1beta1CreateRejectsUnsupportedResourceKind(t *testing.T) {
	h := newHandlerForTest()
	rd := newValidV1beta1RD()
	// Pod is not in the supported GK list — handler must reject it
	rd.Spec.Resource = runtime.RawExtension{Raw: []byte(`{
		"apiVersion":"v1","kind":"Pod",
		"metadata":{"name":"some-pod"}
	}`)}
	raw, err := json.Marshal(rd)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: appsv1beta1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: raw},
		},
	})

	require.False(t, resp.Allowed)
	require.Equal(t, int32(http.StatusUnprocessableEntity), resp.Result.Code)
}

func TestHandleV1beta1CreateRejectsInvalidNamespaceName(t *testing.T) {
	h := newHandlerForTest()
	rd := newValidV1beta1RD()
	rd.Spec.Targets.IncludedNamespaces.List = []appsv1beta1.ResourceDistributionNamespace{{Name: "INVALID_NAME"}}
	raw, err := json.Marshal(rd)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: appsv1beta1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: raw},
		},
	})

	require.False(t, resp.Allowed)
	require.Equal(t, int32(http.StatusUnprocessableEntity), resp.Result.Code)
}

func TestHandleV1beta1CreateRejectsConflictingNamespaces(t *testing.T) {
	h := newHandlerForTest()
	rd := newValidV1beta1RD()
	rd.Spec.Targets.IncludedNamespaces.List = []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-1"}}
	rd.Spec.Targets.ExcludedNamespaces = appsv1beta1.ResourceDistributionTargetNamespaces{
		List: []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-1"}},
	}
	raw, err := json.Marshal(rd)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: appsv1beta1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: raw},
		},
	})

	require.False(t, resp.Allowed)
	require.Equal(t, int32(http.StatusUnprocessableEntity), resp.Result.Code)
}

func TestHandleV1beta1CreateIgnoresEmbeddedNamespaceInResource(t *testing.T) {
	h := newHandlerForTest()
	rd := newValidV1beta1RD()
	// namespace inside spec.resource should be silently ignored, not rejected
	rd.Spec.Resource = runtime.RawExtension{Raw: []byte(`{
		"apiVersion":"v1","kind":"ConfigMap",
		"metadata":{"name":"cfg","namespace":"should-be-ignored"},
		"data":{"k":"v"}
	}`)}
	raw, err := json.Marshal(rd)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: appsv1beta1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: raw},
		},
	})

	require.True(t, resp.Allowed)
}

func TestHandleUnsupportedVersionReturns400(t *testing.T) {
	h := newHandlerForTest()
	raw, err := json.Marshal(newValidV1beta1RD())
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: "v9999", Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: raw},
		},
	})

	require.False(t, resp.Allowed)
	require.Equal(t, int32(http.StatusBadRequest), resp.Result.Code)
}

// --- Update tests ---

func TestHandleV1beta1UpdateAllowsAddingTargetNamespace(t *testing.T) {
	h := newHandlerForTest()
	oldObj := newValidV1beta1RD()
	newObj := oldObj.DeepCopy()
	newObj.Spec.Targets.IncludedNamespaces.List = append(
		newObj.Spec.Targets.IncludedNamespaces.List,
		appsv1beta1.ResourceDistributionNamespace{Name: "ns-2"},
	)

	oldRaw, err := json.Marshal(oldObj)
	require.NoError(t, err)
	newRaw, err := json.Marshal(newObj)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: appsv1beta1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	})

	require.True(t, resp.Allowed)
}

func TestHandleV1beta1UpdateRejectsResourceNameChange(t *testing.T) {
	h := newHandlerForTest()
	oldObj := newValidV1beta1RD()
	newObj := oldObj.DeepCopy()
	// change the resource name — must be immutable
	newObj.Spec.Resource = runtime.RawExtension{Raw: []byte(`{
		"apiVersion":"v1","kind":"Secret",
		"metadata":{"name":"DIFFERENT-NAME"},
		"type":"Opaque","data":{"k":"dg=="}
	}`)}

	oldRaw, err := json.Marshal(oldObj)
	require.NoError(t, err)
	newRaw, err := json.Marshal(newObj)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: appsv1beta1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	})

	require.False(t, resp.Allowed)
	require.Equal(t, int32(http.StatusUnprocessableEntity), resp.Result.Code)
}

func TestHandleV1beta1UpdateAllowsAllNamespacesFlag(t *testing.T) {
	h := newHandlerForTest()
	oldObj := newValidV1beta1RD()
	newObj := oldObj.DeepCopy()
	newObj.Spec.Targets = appsv1beta1.ResourceDistributionTargets{AllNamespaces: true}

	oldRaw, err := json.Marshal(oldObj)
	require.NoError(t, err)
	newRaw, err := json.Marshal(newObj)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource:  metav1.GroupVersionResource{Group: appsv1beta1.GroupVersion.Group, Version: appsv1beta1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	})

	require.True(t, resp.Allowed)
}

func TestHandleV1alpha1UpdateAllowsAddingTargetNamespace(t *testing.T) {
	h := newHandlerForTest()
	oldObj := newValidV1alpha1RD()
	newObj := oldObj.DeepCopy()
	newObj.Spec.Targets.IncludedNamespaces.List = append(
		newObj.Spec.Targets.IncludedNamespaces.List,
		appsv1alpha1.ResourceDistributionNamespace{Name: "ns-2"},
	)

	oldRaw, err := json.Marshal(oldObj)
	require.NoError(t, err)
	newRaw, err := json.Marshal(newObj)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource:  metav1.GroupVersionResource{Group: appsv1alpha1.GroupVersion.Group, Version: appsv1alpha1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	})

	require.True(t, resp.Allowed)
}

func TestHandleV1alpha1UpdateRejectsResourceNameChange(t *testing.T) {
	h := newHandlerForTest()
	oldObj := newValidV1alpha1RD()
	newObj := oldObj.DeepCopy()
	newObj.Spec.Resource = runtime.RawExtension{Raw: []byte(`{
		"apiVersion":"v1","kind":"Secret",
		"metadata":{"name":"DIFFERENT-NAME"},
		"type":"Opaque","data":{"k":"dg=="}
	}`)}

	oldRaw, err := json.Marshal(oldObj)
	require.NoError(t, err)
	newRaw, err := json.Marshal(newObj)
	require.NoError(t, err)

	resp := h.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource:  metav1.GroupVersionResource{Group: appsv1alpha1.GroupVersion.Group, Version: appsv1alpha1.GroupVersion.Version, Resource: "resourcedistributions"},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	})

	require.False(t, resp.Allowed)
	require.Equal(t, int32(http.StatusUnprocessableEntity), resp.Result.Code)
}
