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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

type requestCtxKey struct{}

// ctxCheckingClient fails the test when a list does not receive the context of the admission request.
type ctxCheckingClient struct {
	client.Client
	t     *testing.T
	lists int
}

func (c *ctxCheckingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	c.t.Helper()
	c.lists++
	if ctx.Value(requestCtxKey{}) != "request-context" {
		c.t.Errorf("the list did not receive the context of the admission request")
	}
	return c.Client.List(ctx, list, opts...)
}

func TestNamespaceHandlerHandle(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ResourcesDeletionProtection, true)()

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	namespaceJSON := func(protection string) runtime.RawExtension {
		ns := &v1.Namespace{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{Name: "protected", Labels: map[string]string{policyv1alpha1.DeletionProtectionKey: protection}},
		}
		raw, err := json.Marshal(ns)
		if err != nil {
			t.Fatal(err)
		}
		return runtime.RawExtension{Raw: raw}
	}
	newRequest := func(op admissionv1.Operation, old runtime.RawExtension) admission.Request {
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Operation: op, Name: "protected", OldObject: old}}
	}

	tests := []struct {
		name      string
		request   admission.Request
		wantAllow bool
		wantCode  int32
		wantLists int
	}{
		{name: "not a delete", request: newRequest(admissionv1.Update, namespaceJSON("Always")), wantAllow: true},
		{name: "delete without old object", request: newRequest(admissionv1.Delete, runtime.RawExtension{}), wantAllow: true},
		{
			name:     "delete with an undecodable old object",
			request:  newRequest(admissionv1.Delete, runtime.RawExtension{Raw: []byte("{")}),
			wantCode: http.StatusBadRequest,
		},
		{name: "delete forbidden by protection", request: newRequest(admissionv1.Delete, namespaceJSON("Always")), wantCode: http.StatusForbidden},
		{name: "delete allowed without active resources", request: newRequest(admissionv1.Delete, namespaceJSON("Cascading")), wantAllow: true, wantLists: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ctxCheckingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), t: t}
			h := &NamespaceHandler{Client: c, Decoder: admission.NewDecoder(scheme)}

			resp := h.Handle(context.WithValue(context.Background(), requestCtxKey{}, "request-context"), tt.request)

			if resp.Allowed != tt.wantAllow {
				t.Fatalf("expected allowed=%v, got %v (%v)", tt.wantAllow, resp.Allowed, resp.Result)
			}
			if !tt.wantAllow && resp.Result.Code != tt.wantCode {
				t.Fatalf("expected code %d, got %d", tt.wantCode, resp.Result.Code)
			}
			if c.lists != tt.wantLists {
				t.Errorf("expected %d list calls, got %d", tt.wantLists, c.lists)
			}
		})
	}
}
