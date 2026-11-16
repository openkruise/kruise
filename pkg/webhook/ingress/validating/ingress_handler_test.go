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
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

func TestIngressDeletionFollowsFeatureGate(t *testing.T) {
	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected",
			Namespace: "default",
			Labels:    map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways},
		},
	}
	raw, err := json.Marshal(ingress)
	if err != nil {
		t.Fatal(err)
	}
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Delete,
		Kind:      metav1.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
		Name:      ingress.Name,
		Namespace: ingress.Namespace,
		OldObject: runtime.RawExtension{Raw: raw},
	}}
	h := &IngressHandler{Decoder: admission.NewDecoder(clientgoscheme.Scheme)}

	for _, tc := range []struct {
		gate    bool
		allowed bool
	}{
		// The handler is registered even with the gate off, so it has to allow
		// everything then.
		{gate: false, allowed: true},
		{gate: true, allowed: false},
	} {
		func() {
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ResourcesDeletionProtection, tc.gate)()
			resp := h.Handle(context.TODO(), req)
			if resp.Allowed != tc.allowed {
				t.Errorf("gate=%v: expected allowed=%v, got %v (%v)", tc.gate, tc.allowed, resp.Allowed, resp.Result)
			}
		}()
	}
}
