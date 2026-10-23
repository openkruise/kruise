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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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

func TestCRDHandlerHandle(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ResourcesDeletionProtection, true)()
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.DeletionProtectionForCRDCascadingGate, true)()

	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := apiextensionsv1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	marshal := func(obj interface{}) runtime.RawExtension {
		raw, err := json.Marshal(obj)
		if err != nil {
			t.Fatal(err)
		}
		return runtime.RawExtension{Raw: raw}
	}
	meta := func(protection string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: "foos.example.io", Labels: map[string]string{policyv1alpha1.DeletionProtectionKey: protection}}
	}
	crdV1 := func(protection string) runtime.RawExtension {
		return marshal(&apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: meta(protection),
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group:    "example.io",
				Names:    apiextensionsv1.CustomResourceDefinitionNames{Kind: "Foo", ListKind: "FooList"},
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1", Served: true, Storage: true}},
			},
		})
	}
	crdV1beta1 := func(protection string) runtime.RawExtension {
		return marshal(&apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: meta(protection),
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:    "example.io",
				Names:    apiextensionsv1beta1.CustomResourceDefinitionNames{Kind: "Foo", ListKind: "FooList"},
				Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{{Name: "v1", Served: true, Storage: true}},
			},
		})
	}
	newRequest := func(op admissionv1.Operation, version string, old runtime.RawExtension) admission.Request {
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: op,
			Name:      "foos.example.io",
			Kind:      metav1.GroupVersionKind{Group: "apiextensions.k8s.io", Version: version, Kind: "CustomResourceDefinition"},
			OldObject: old,
		}}
	}

	tests := []struct {
		name      string
		request   admission.Request
		wantAllow bool
		wantCode  int32
		wantLists int
	}{
		{name: "not a delete", request: newRequest(admissionv1.Update, "v1", crdV1("Always")), wantAllow: true},
		{name: "delete without old object", request: newRequest(admissionv1.Delete, "v1", runtime.RawExtension{}), wantAllow: true},
		{name: "unrecognized version", request: newRequest(admissionv1.Delete, "v2", crdV1("Always")), wantAllow: true},
		{name: "undecodable v1 object", request: newRequest(admissionv1.Delete, "v1", runtime.RawExtension{Raw: []byte("{")}), wantCode: http.StatusBadRequest},
		{name: "undecodable v1beta1 object", request: newRequest(admissionv1.Delete, "v1beta1", runtime.RawExtension{Raw: []byte("{")}), wantCode: http.StatusBadRequest},
		{name: "v1 forbidden by protection", request: newRequest(admissionv1.Delete, "v1", crdV1("Always")), wantCode: http.StatusForbidden},
		{name: "v1beta1 forbidden by protection", request: newRequest(admissionv1.Delete, "v1beta1", crdV1beta1("Always")), wantCode: http.StatusForbidden},
		{name: "v1 allowed without custom resources", request: newRequest(admissionv1.Delete, "v1", crdV1("Cascading")), wantAllow: true, wantLists: 1},
		{name: "v1beta1 allowed without custom resources", request: newRequest(admissionv1.Delete, "v1beta1", crdV1beta1("Cascading")), wantAllow: true, wantLists: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ctxCheckingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), t: t}
			h := &CRDHandler{Client: c, Decoder: admission.NewDecoder(scheme)}

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
