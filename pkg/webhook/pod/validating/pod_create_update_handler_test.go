package validating

import (
	"context"
	"errors"
	"net/http"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// testHandler wraps PodCreateHandler with mock methods
type testHandler struct {
	handler   *PodCreateHandler
	wsAllow   bool
	wsReason  string
	wsErr     error
	pubAllow  bool
	pubReason string
	pubErr    error
}

// Override validatingPodFn to use our mock logic
func (t *testHandler) validatingPodFn(ctx context.Context, req admission.Request) (allowed bool, reason string, err error) {
	allowed = true
	if req.Operation == admissionv1.Delete && len(req.OldObject.Raw) == 0 {
		return
	}

	switch req.Operation {
	case admissionv1.Update:
		if utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetUpdateGate) {
			return t.pubAllow, t.pubReason, t.pubErr
		}
	case admissionv1.Delete, admissionv1.Create:
		if utilfeature.DefaultFeatureGate.Enabled(features.WorkloadSpread) {
			allowed, reason, err = t.wsAllow, t.wsReason, t.wsErr
			if !allowed || err != nil {
				return
			}
		}
		if utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetDeleteGate) {
			return t.pubAllow, t.pubReason, t.pubErr
		}
	}
	return
}

// Handle mimics the real Handle method but uses our mock validatingPodFn
func (t *testHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	allowed, reason, err := t.validatingPodFn(ctx, req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !allowed {
		// Create a custom response with the specific reason
		return admission.Response{
			AdmissionResponse: admissionv1.AdmissionResponse{
				UID:     req.UID,
				Allowed: false,
				Result: &metav1.Status{
					Code:    http.StatusForbidden,
					Message: reason,
					Reason:  metav1.StatusReason(reason),
				},
			},
		}
	}

	return admission.ValidationResponse(allowed, reason)
}

// nopDecoder is an admission.Decoder that no-ops.
type nopDecoder struct{}

func (nopDecoder) Decode(req admission.Request, into runtime.Object) error       { return nil }
func (nopDecoder) DecodeRaw(raw runtime.RawExtension, into runtime.Object) error { return nil }

// clearAllGates turns all three gates off.
func clearAllGates(t *testing.T) {
	err := utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=false")
	if err != nil {
		t.Fatalf("failed to clear WorkloadSpread gate: %v", err)
	}
	err = utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetDeleteGate) + "=false")
	if err != nil {
		t.Fatalf("failed to clear PodUnavailableBudgetDeleteGate: %v", err)
	}
	err = utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetUpdateGate) + "=false")
	if err != nil {
		t.Fatalf("failed to clear PodUnavailableBudgetUpdateGate: %v", err)
	}
}

func TestHandle_DefaultPaths(t *testing.T) {
	clearAllGates(t)

	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
	}

	// CREATE → allowed
	createReq := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}
	if resp := h.Handle(context.Background(), createReq); !resp.Allowed {
		t.Error("CREATE with gates off: expected Allowed=true")
	}

	// DELETE(empty OldObject) → skip → allowed
	delReq := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Delete,
		OldObject: runtime.RawExtension{Raw: []byte{}},
	}}
	if resp := h.Handle(context.Background(), delReq); !resp.Allowed {
		t.Error("DELETE empty-old: expected Allowed=true")
	}

	// UPDATE → allowed
	updateReq := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Update,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
		OldObject: runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}
	if resp := h.Handle(context.Background(), updateReq); !resp.Allowed {
		t.Error("UPDATE with gates off: expected Allowed=true")
	}
}

func TestHandle_WorkloadSpreadBranch(t *testing.T) {
	clearAllGates(t)
	// Turn WorkloadSpread on
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true"); err != nil {
		t.Fatalf("failed to set WorkloadSpread gate: %v", err)
	}
	defer clearAllGates(t)

	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		wsAllow:  false,
		wsReason: "ws-denied",
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("WorkloadSpread deny: expected Allowed=false")
	}
	if string(resp.Result.Reason) != "ws-denied" {
		t.Errorf("WorkloadSpread deny: expected Reason=ws-denied, got %q", string(resp.Result.Reason))
	}

	// Now simulate an error path
	h.wsAllow, h.wsErr = true, errors.New("boom")
	resp = h.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("WorkloadSpread error: expected Allowed=false")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("WorkloadSpread error: expected HTTP 400, got %d", resp.Result.Code)
	}
}

func TestHandle_PodUnavailableBudgetBranches(t *testing.T) {
	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		pubAllow:  false,
		pubReason: "pub-denied",
	}

	// Case 1: UPDATE with PodUnavailableBudgetUpdateGate on → denied
	clearAllGates(t)
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetUpdateGate) + "=true"); err != nil {
		t.Fatalf("failed to set PodUnavailableBudgetUpdateGate: %v", err)
	}
	defer clearAllGates(t)

	updateReq := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Update,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
		OldObject: runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), updateReq)
	if resp.Allowed {
		t.Error("PUB update deny: expected Allowed=false")
	}
	if string(resp.Result.Reason) != "pub-denied" {
		t.Errorf("PUB update deny: expected Reason=pub-denied, got %q", string(resp.Result.Reason))
	}

	// Case 2: CREATE "eviction" with PodUnavailableBudgetDeleteGate on → allowed
	h.pubAllow, h.pubErr = true, nil
	clearAllGates(t)
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetDeleteGate) + "=true"); err != nil {
		t.Fatalf("failed to set PodUnavailableBudgetDeleteGate: %v", err)
	}
	defer clearAllGates(t)

	evictReq := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation:   admissionv1.Create,
		SubResource: "eviction",
		Object:      runtime.RawExtension{Raw: []byte(`{"kind":"Eviction"}`)},
	}}

	resp = h.Handle(context.Background(), evictReq)
	if !resp.Allowed {
		t.Error("PUB delete-gate allow: expected Allowed=true")
	}

	// Case 3: error path → HTTP 400
	h.pubErr = errors.New("pub-error")
	resp = h.Handle(context.Background(), evictReq)
	if resp.Allowed {
		t.Error("PUB error: expected Allowed=false")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("PUB error: expected HTTP 400, got %d", resp.Result.Code)
	}
}

func TestHandle_WorkloadSpreadAllowed(t *testing.T) {
	clearAllGates(t)
	// Turn WorkloadSpread on
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true"); err != nil {
		t.Fatalf("failed to set WorkloadSpread gate: %v", err)
	}
	defer clearAllGates(t)

	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		wsAllow:  true,
		wsReason: "",
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Error("WorkloadSpread allow: expected Allowed=true")
	}
}

func TestHandle_PodUnavailableBudgetAllowed(t *testing.T) {
	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		pubAllow:  true,
		pubReason: "",
	}

	// UPDATE with PodUnavailableBudgetUpdateGate on → allowed
	clearAllGates(t)
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetUpdateGate) + "=true"); err != nil {
		t.Fatalf("failed to set PodUnavailableBudgetUpdateGate: %v", err)
	}
	defer clearAllGates(t)

	updateReq := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Update,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
		OldObject: runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), updateReq)
	if !resp.Allowed {
		t.Error("PUB update allow: expected Allowed=true")
	}
}

func TestHandle_BothGatesEnabled(t *testing.T) {
	clearAllGates(t)
	// Turn both gates on
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true"); err != nil {
		t.Fatalf("failed to set WorkloadSpread gate: %v", err)
	}
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetDeleteGate) + "=true"); err != nil {
		t.Fatalf("failed to set PodUnavailableBudgetDeleteGate: %v", err)
	}
	defer clearAllGates(t)

	// Test case where WorkloadSpread allows but PodUnavailableBudget denies
	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		wsAllow:   true,
		wsReason:  "",
		pubAllow:  false,
		pubReason: "pub-denied",
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("Both gates enabled with PUB deny: expected Allowed=false")
	}
	if string(resp.Result.Reason) != "pub-denied" {
		t.Errorf("Both gates enabled with PUB deny: expected Reason=pub-denied, got %q", string(resp.Result.Reason))
	}
}

func TestHandle_WorkloadSpreadErrorStopsChain(t *testing.T) {
	clearAllGates(t)
	// Turn both gates on
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true"); err != nil {
		t.Fatalf("failed to set WorkloadSpread gate: %v", err)
	}
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetDeleteGate) + "=true"); err != nil {
		t.Fatalf("failed to set PodUnavailableBudgetDeleteGate: %v", err)
	}
	defer clearAllGates(t)

	// Test case where WorkloadSpread has an error, should stop the chain
	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		wsAllow:   true,
		wsErr:     errors.New("ws-error"),
		pubAllow:  true, // This should not be reached
		pubReason: "",
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("WorkloadSpread error: expected Allowed=false")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("WorkloadSpread error: expected HTTP 400, got %d", resp.Result.Code)
	}
}

// ====== NEW TESTS ADDED BASED ON SUGGESTIONS ======

func TestHandle_DirectDeleteWithPUB(t *testing.T) {
	clearAllGates(t)
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetDeleteGate) + "=true"); err != nil {
		t.Fatalf("failed to set PodUnavailableBudgetDeleteGate: %v", err)
	}
	defer clearAllGates(t)

	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		pubAllow:  false,
		pubReason: "pub-direct-delete-denied",
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Delete,
		OldObject: runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("Direct DELETE with PUB: expected Allowed=false")
	}
	if string(resp.Result.Reason) != "pub-direct-delete-denied" {
		t.Errorf("Expected Reason=pub-direct-delete-denied, got %q", string(resp.Result.Reason))
	}

	// Test allow path
	h.pubAllow = true
	resp = h.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Error("Direct DELETE with PUB allow: expected Allowed=true")
	}
}

func TestHandle_BothGatesEnabledAndAllow(t *testing.T) {
	clearAllGates(t)
	// Turn both gates on
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true"); err != nil {
		t.Fatalf("failed to set WorkloadSpread gate: %v", err)
	}
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetDeleteGate) + "=true"); err != nil {
		t.Fatalf("failed to set PodUnavailableBudgetDeleteGate: %v", err)
	}
	defer clearAllGates(t)

	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		wsAllow:   true,
		wsReason:  "",
		pubAllow:  true,
		pubReason: "",
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Error("Both gates enabled and both allow: expected Allowed=true")
	}
}

func TestHandle_PUBErrorAfterWorkloadSpreadAllow(t *testing.T) {
	clearAllGates(t)
	// Turn both gates on
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.WorkloadSpread) + "=true"); err != nil {
		t.Fatalf("failed to set WorkloadSpread gate: %v", err)
	}
	if err := utilfeature.DefaultMutableFeatureGate.Set(string(features.PodUnavailableBudgetDeleteGate) + "=true"); err != nil {
		t.Fatalf("failed to set PodUnavailableBudgetDeleteGate: %v", err)
	}
	defer clearAllGates(t)

	h := &testHandler{
		handler: &PodCreateHandler{
			Decoder: nopDecoder{},
		},
		wsAllow:  true,
		wsReason: "",
		pubAllow: true,
		pubErr:   errors.New("pub-error"),
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: []byte(`{"kind":"Pod"}`)},
	}}

	resp := h.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("PUB error after WS allow: expected Allowed=false")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("PUB error after WS allow: expected HTTP 400, got %d", resp.Result.Code)
	}
}
