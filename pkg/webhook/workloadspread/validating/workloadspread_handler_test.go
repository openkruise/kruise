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
	"net/http"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// buildWSHandler constructs a WorkloadSpreadCreateUpdateHandler backed by a fake client.
func buildWSHandler(t *testing.T) *WorkloadSpreadCreateUpdateHandler {
	t.Helper()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	decoder := admission.NewDecoder(scheme)
	return &WorkloadSpreadCreateUpdateHandler{
		Client:  fakeClient,
		Decoder: decoder,
	}
}

// wsAdmissionReq builds an admission.Request for Create or Update operations.
func wsAdmissionReq(t *testing.T, op admissionv1.Operation, version string, obj, oldObj interface{}) admission.Request {
	t.Helper()
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal obj: %v", err)
	}
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: op,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  version,
				Resource: "workloadspreads",
			},
			Object: runtime.RawExtension{Raw: raw},
		},
	}
	if oldObj != nil {
		oldRaw, err := json.Marshal(oldObj)
		if err != nil {
			t.Fatalf("marshal oldObj: %v", err)
		}
		req.OldObject = runtime.RawExtension{Raw: oldRaw}
	}
	return req
}

// validV1beta1WS returns a minimal but valid v1beta1 WorkloadSpread.
func validV1beta1WS(name, ns string) *appsv1beta1.WorkloadSpread {
	max := intstr.FromInt32(3)
	return &appsv1beta1.WorkloadSpread{
		TypeMeta:   metav1.TypeMeta{APIVersion: appsv1beta1.GroupVersion.String(), Kind: "WorkloadSpread"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1beta1.WorkloadSpreadSpec{
			TargetReference: &appsv1beta1.TargetReference{
				APIVersion: appsv1alpha1.SchemeGroupVersion.String(),
				Kind:       "CloneSet",
				Name:       "my-cs",
			},
			Subsets: []appsv1beta1.WorkloadSpreadSubset{
				{
					Name:        "zone-a",
					MaxReplicas: &max,
					RequiredNodeSelector: &corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"}},
						},
					},
				},
				{Name: "zone-b"},
			},
		},
	}
}

// validV1alpha1WS returns a minimal but valid v1alpha1 WorkloadSpread.
func validV1alpha1WS(name, ns string) *appsv1alpha1.WorkloadSpread {
	max := intstr.FromInt32(3)
	return &appsv1alpha1.WorkloadSpread{
		TypeMeta:   metav1.TypeMeta{APIVersion: appsv1alpha1.SchemeGroupVersion.String(), Kind: "WorkloadSpread"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1alpha1.WorkloadSpreadSpec{
			TargetReference: &appsv1alpha1.TargetReference{
				APIVersion: appsv1alpha1.SchemeGroupVersion.String(),
				Kind:       "CloneSet",
				Name:       "my-cs",
			},
			Subsets: []appsv1alpha1.WorkloadSpreadSubset{
				{
					Name:        "zone-a",
					MaxReplicas: &max,
					RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"a"}},
						},
					},
				},
				{Name: "zone-b"},
			},
		},
	}
}

// ── feature gate guard ────────────────────────────────────────────────────────

func TestWSHandler_FeatureGateDisabled(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=false")
	defer func() { _ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true") }()

	h := buildWSHandler(t)
	ws := validV1beta1WS("ws", "default")
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Create, "v1beta1", ws, nil))
	if resp.Result.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.Result.Code)
	}
}

// ── v1beta1 Create ────────────────────────────────────────────────────────────

func TestWSHandler_V1beta1_Create_Valid(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	ws := validV1beta1WS("ws", "default")
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Create, "v1beta1", ws, nil))
	if !resp.Allowed {
		t.Fatalf("expected allowed, got: %v", resp.Result)
	}
}

func TestWSHandler_V1beta1_Create_NoTargetRef(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	ws := validV1beta1WS("ws", "default")
	ws.Spec.TargetReference = nil
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Create, "v1beta1", ws, nil))
	if resp.Allowed {
		t.Fatal("expected rejection for missing targetRef")
	}
}

func TestWSHandler_V1beta1_Create_NoSubsets(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	ws := validV1beta1WS("ws", "default")
	ws.Spec.Subsets = nil
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Create, "v1beta1", ws, nil))
	if resp.Allowed {
		t.Fatal("expected rejection for zero subsets")
	}
}

// ── v1alpha1 Create (exercises the decode + ConvertTo path) ──────────────────

func TestWSHandler_V1alpha1_Create_Valid(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	ws := validV1alpha1WS("ws", "default")
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Create, "v1alpha1", ws, nil))
	if !resp.Allowed {
		t.Fatalf("expected allowed for v1alpha1 create, got: %v", resp.Result)
	}
}

func TestWSHandler_V1alpha1_Create_NoTargetRef(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	ws := validV1alpha1WS("ws", "default")
	ws.Spec.TargetReference = nil
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Create, "v1alpha1", ws, nil))
	if resp.Allowed {
		t.Fatal("expected rejection for v1alpha1 create with missing targetRef")
	}
}

// ── v1beta1 Update ────────────────────────────────────────────────────────────

func TestWSHandler_V1beta1_Update_Valid(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	old := validV1beta1WS("ws", "default")
	old.ResourceVersion = "1"
	new := old.DeepCopy()
	// change a non-immutable field
	new.Spec.Subsets[0].MaxReplicas = func() *intstr.IntOrString { v := intstr.FromInt32(5); return &v }()
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Update, "v1beta1", new, old))
	if !resp.Allowed {
		t.Fatalf("expected update allowed, got: %v", resp.Result)
	}
}

func TestWSHandler_V1beta1_Update_TargetRefImmutable(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	old := validV1beta1WS("ws", "default")
	old.ResourceVersion = "1"
	new := old.DeepCopy()
	new.Spec.TargetReference.Name = "different-cs"
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Update, "v1beta1", new, old))
	if resp.Allowed {
		t.Fatal("expected rejection for changing immutable targetRef")
	}
}

// ── v1alpha1 Update (exercises OldObject decode + ConvertTo path) ─────────────

func TestWSHandler_V1alpha1_Update_TargetRefImmutable(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	old := validV1alpha1WS("ws", "default")
	old.ResourceVersion = "1"
	new := old.DeepCopy()
	new.Spec.TargetReference.Name = "different-cs"
	resp := h.Handle(context.Background(), wsAdmissionReq(t, admissionv1.Update, "v1alpha1", new, old))
	if resp.Allowed {
		t.Fatal("expected rejection for v1alpha1 update changing immutable targetRef")
	}
}

// ── malformed body ────────────────────────────────────────────────────────────

func TestWSHandler_V1beta1_Create_BadJSON(t *testing.T) {
	_ = utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true")
	h := buildWSHandler(t)
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  "v1beta1",
				Resource: "workloadspreads",
			},
			Object: runtime.RawExtension{Raw: []byte(`{"not valid json`)},
		},
	}
	resp := h.Handle(context.Background(), req)
	if resp.Allowed {
		t.Fatal("expected rejection for bad JSON body")
	}
}
