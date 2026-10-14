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

package deletionprotection

import (
	"context"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

func newRequestCtx() context.Context {
	return context.WithValue(context.Background(), requestCtxKey{}, "request-context")
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func protectedLabels(value string) map[string]string {
	return map[string]string{policyv1alpha1.DeletionProtectionKey: value}
}

func TestValidateNamespaceDeletion(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ResourcesDeletionProtection, true)()

	activePod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "protected", Name: "active"},
		Status:     v1.PodStatus{Phase: v1.PodRunning},
	}
	boundPVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "protected", Name: "bound"},
		Status:     v1.PersistentVolumeClaimStatus{Phase: v1.ClaimBound},
	}
	deletedAt := metav1.Now()

	tests := []struct {
		name      string
		namespace *v1.Namespace
		objects   []client.Object
		wantErr   string
		wantLists int
	}{
		{
			name:      "no protection label",
			namespace: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "protected"}},
		},
		{
			name: "already being deleted",
			namespace: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "protected", Labels: protectedLabels("Always"), DeletionTimestamp: &deletedAt, Finalizers: []string{"f"},
			}},
		},
		{
			name:      "always",
			namespace: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "protected", Labels: protectedLabels("Always")}},
			wantErr:   "forbidden by ResourcesProtectionDeletion",
		},
		{
			name:      "cascading without active resources",
			namespace: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "protected", Labels: protectedLabels("Cascading")}},
			wantLists: 2,
		},
		{
			name:      "cascading with an active pod",
			namespace: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "protected", Labels: protectedLabels("Cascading")}},
			objects:   []client.Object{activePod},
			wantErr:   "active pods 1>0",
			wantLists: 1,
		},
		{
			name:      "cascading with a bound pvc",
			namespace: &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "protected", Labels: protectedLabels("Cascading")}},
			objects:   []client.Object{boundPVC},
			wantErr:   "\"Bound\" status pvc 1>0",
			wantLists: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ctxCheckingClient{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(tt.objects...).Build(), t: t}

			err := ValidateNamespaceDeletion(newRequestCtx(), c, tt.namespace)

			assertError(t, err, tt.wantErr)
			if c.lists != tt.wantLists {
				t.Errorf("expected %d list calls, got %d", tt.wantLists, c.lists)
			}
		})
	}
}

func TestValidateCRDDeletion(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.ResourcesDeletionProtection, true)()

	gvk := schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "FooList"}
	newCR := func(deleting bool) *unstructured.Unstructured {
		cr := &unstructured.Unstructured{}
		cr.SetAPIVersion("example.io/v1")
		cr.SetKind("Foo")
		cr.SetNamespace("ns")
		cr.SetName("foo")
		if deleting {
			now := metav1.Now()
			cr.SetDeletionTimestamp(&now)
			cr.SetFinalizers([]string{"f"})
		}
		return cr
	}
	crd := func(value string) metav1.Object {
		return &metav1.ObjectMeta{Name: "foos.example.io", Labels: protectedLabels(value)}
	}

	tests := []struct {
		name        string
		obj         metav1.Object
		cascadingOn bool
		objects     []client.Object
		wantErr     string
		wantLists   int
	}{
		{name: "no protection label", obj: &metav1.ObjectMeta{Name: "foos.example.io"}},
		{name: "always", obj: crd("Always"), wantErr: "forbidden by ResourcesProtectionDeletion"},
		{name: "cascading needs the feature gate", obj: crd("Cascading"), wantErr: "is not enabled"},
		{name: "cascading without custom resources", obj: crd("Cascading"), cascadingOn: true, wantLists: 1},
		{
			name: "cascading with only deleting custom resources", obj: crd("Cascading"), cascadingOn: true,
			objects: []client.Object{newCR(true)}, wantLists: 1,
		},
		{
			name: "cascading with an active custom resource", obj: crd("Cascading"), cascadingOn: true,
			objects: []client.Object{newCR(false)}, wantErr: "active CRs 1>0", wantLists: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.DeletionProtectionForCRDCascadingGate, tt.cascadingOn)()
			c := &ctxCheckingClient{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(tt.objects...).Build(), t: t}

			err := ValidateCRDDeletion(newRequestCtx(), c, tt.obj, gvk)

			assertError(t, err, tt.wantErr)
			if c.lists != tt.wantLists {
				t.Errorf("expected %d list calls, got %d", tt.wantLists, c.lists)
			}
		})
	}
}

func assertError(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case want != "" && err == nil:
		t.Fatalf("expected an error containing %q", want)
	case want != "" && !strings.Contains(err.Error(), want):
		t.Fatalf("expected an error containing %q, got %v", want, err)
	}
}
