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

package mutating

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

func newCRRTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = appsv1alpha1.AddToScheme(s)
	_ = appsv1beta1.AddToScheme(s)
	return s
}

func newActivePod(name, namespace, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("uid-" + name),
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "nginx:latest",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "app",
					Ready:       true,
					ContainerID: "docker://containerid-app",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
}

func buildCRRHandler(t *testing.T, pod *corev1.Pod) *ContainerRecreateRequestHandler {
	scheme := newCRRTestScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: pod.Spec.NodeName}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod, node).Build()
	decoder := admission.NewDecoder(scheme)
	return &ContainerRecreateRequestHandler{
		Client:  fakeClient,
		Decoder: decoder,
	}
}

func admissionRequest(t *testing.T, op admissionv1.Operation, version string, obj interface{}, oldObj interface{}) admission.Request {
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal object: %v", err)
	}
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: op,
			RequestKind: &metav1.GroupVersionKind{
				Group:   appsv1beta1.GroupVersion.Group,
				Version: version,
				Kind:    "ContainerRecreateRequest",
			},
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  version,
				Resource: "containerrecreaterequests",
			},
			Object: runtime.RawExtension{Raw: raw},
		},
	}
	if oldObj != nil {
		oldRaw, err := json.Marshal(oldObj)
		if err != nil {
			t.Fatalf("marshal old object: %v", err)
		}
		req.OldObject = runtime.RawExtension{Raw: oldRaw}
	}
	return req
}

// ── v1alpha1 create ──────────────────────────────────────────────────────────

func TestCRRHandler_V1alpha1_Create_Valid(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-0", Namespace: "default"},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName:    "pod-0",
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Create, "v1alpha1", crr, nil))
	if !resp.Allowed {
		t.Fatalf("expected Allowed; got %v", resp.Result)
	}
	if len(resp.Patches) == 0 {
		t.Error("expected patches (labels injection), got none")
	}
}

func TestCRRHandler_V1alpha1_Create_EmptyPodName(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-bad", Namespace: "default"},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Create, "v1alpha1", crr, nil))
	if resp.Allowed {
		t.Fatal("expected rejection for empty podName")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.Result.Code)
	}
}

func TestCRRHandler_V1alpha1_Create_EmptyContainers(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-empty", Namespace: "default"},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: "pod-0",
		},
	}
	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Create, "v1alpha1", crr, nil))
	if resp.Allowed {
		t.Fatal("expected rejection for empty containers")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.Result.Code)
	}
}

func TestCRRHandler_V1alpha1_Update_SpecImmutable(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	old := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-0", Namespace: "default"},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName:    "pod-0",
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	updated := old.DeepCopy()
	updated.Spec.Containers = append(updated.Spec.Containers, appsv1alpha1.ContainerRecreateRequestContainer{Name: "sidecar"})

	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Update, "v1alpha1", updated, old))
	if resp.Allowed {
		t.Fatal("expected rejection for spec mutation")
	}
	if resp.Result.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.Result.Code)
	}
}

func TestCRRHandler_V1alpha1_Update_ImmutableLabel(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	old := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crr-0",
			Namespace: "default",
			Labels:    map[string]string{appsv1alpha1.ContainerRecreateRequestPodUIDKey: "uid-pod-0"},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName:    "pod-0",
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	updated := old.DeepCopy()
	updated.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey] = "uid-changed"

	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Update, "v1alpha1", updated, old))
	if resp.Allowed {
		t.Fatal("expected rejection for immutable label change")
	}
	if resp.Result.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.Result.Code)
	}
}

// ── v1beta1 create ───────────────────────────────────────────────────────────

func TestCRRHandler_V1beta1_Create_Valid(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	crr := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-b1", Namespace: "default"},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			PodName:    "pod-0",
			Containers: []appsv1beta1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Create, "v1beta1", crr, nil))
	if !resp.Allowed {
		t.Fatalf("expected Allowed; got %v", resp.Result)
	}
	if len(resp.Patches) == 0 {
		t.Error("expected patches (labels injection), got none")
	}
}

func TestCRRHandler_V1beta1_Create_EmptyPodName(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	crr := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-bad", Namespace: "default"},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			Containers: []appsv1beta1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Create, "v1beta1", crr, nil))
	if resp.Allowed {
		t.Fatal("expected rejection for empty podName")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.Result.Code)
	}
}

func TestCRRHandler_V1beta1_Create_DeadlineTooShort(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	deadline := int64(1)
	crr := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-deadline", Namespace: "default"},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			PodName:               "pod-0",
			Containers:            []appsv1beta1.ContainerRecreateRequestContainer{{Name: "app"}},
			ActiveDeadlineSeconds: &deadline,
		},
	}
	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Create, "v1beta1", crr, nil))
	if resp.Allowed {
		t.Fatal("expected rejection for activeDeadlineSeconds < 3")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.Result.Code)
	}
}

func TestCRRHandler_V1beta1_Update_SpecImmutable(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	old := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-b1", Namespace: "default"},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			PodName:    "pod-0",
			Containers: []appsv1beta1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	updated := old.DeepCopy()
	updated.Spec.Containers = append(updated.Spec.Containers, appsv1beta1.ContainerRecreateRequestContainer{Name: "sidecar"})

	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Update, "v1beta1", updated, old))
	if resp.Allowed {
		t.Fatal("expected rejection for spec mutation")
	}
	if resp.Result.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.Result.Code)
	}
}

func TestCRRHandler_V1beta1_Update_ImmutableLabel(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	old := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crr-b1",
			Namespace: "default",
			Labels:    map[string]string{appsv1beta1.ContainerRecreateRequestNodeNameKey: "node-1"},
		},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			PodName:    "pod-0",
			Containers: []appsv1beta1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	updated := old.DeepCopy()
	updated.Labels[appsv1beta1.ContainerRecreateRequestNodeNameKey] = "node-changed"

	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Update, "v1beta1", updated, old))
	if resp.Allowed {
		t.Fatal("expected rejection for immutable node-name label change")
	}
	if resp.Result.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.Result.Code)
	}
}

func TestCRRHandler_FeatureGateDisabled(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, false)()

	pod := newActivePod("pod-0", "default", "node-1")
	h := buildCRRHandler(t, pod)

	crr := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-fg", Namespace: "default"},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			PodName:    "pod-0",
			Containers: []appsv1beta1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Create, "v1beta1", crr, nil))
	if resp.Allowed {
		t.Fatal("expected rejection when KruiseDaemon feature gate is disabled")
	}
	if resp.Result.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.Result.Code)
	}
}

// TestCRRHandler_V1alpha1_Create_RequestKindV1beta1 verifies routing uses Resource.Version,
// not RequestKind, so v1alpha1 objects are not decoded into v1beta1 types after storage promotion.
func TestCRRHandler_V1alpha1_Create_RequestKindV1beta1(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	pod := newActivePod("test-pod", "default", "node-1")
	h := buildCRRHandler(t, pod)

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-1", Namespace: "default"},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName:    "test-pod",
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	req := admissionRequest(t, admissionv1.Create, "v1alpha1", crr, nil)
	req.RequestKind = &metav1.GroupVersionKind{
		Group:   appsv1beta1.GroupVersion.Group,
		Version: appsv1beta1.GroupVersion.Version,
		Kind:    "ContainerRecreateRequest",
	}

	resp := h.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected allowed, got %d: %s", resp.Result.Code, resp.Result.Message)
	}
}

func TestCRRHandler_V1beta1_Create_PodNotFound(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.KruiseDaemon, true)()

	scheme := newCRRTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	h := &ContainerRecreateRequestHandler{
		Client:  fakeClient,
		Decoder: admission.NewDecoder(scheme),
	}

	crr := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-nopod", Namespace: "default"},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			PodName:    "nonexistent-pod",
			Containers: []appsv1beta1.ContainerRecreateRequestContainer{{Name: "app"}},
		},
	}
	resp := h.Handle(context.Background(), admissionRequest(t, admissionv1.Create, "v1beta1", crr, nil))
	if resp.Allowed {
		t.Fatal("expected rejection when pod does not exist")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.Result.Code)
	}
}

// TestInjectPodIntoContainerRecreateRequestV1alpha1_VirtualKubeletLabel verifies
// that the virtual-kubelet label is propagated from the node to the CRR.
func TestInjectPodIntoContainerRecreateRequestV1alpha1_VirtualKubeletLabel(t *testing.T) {
	basePod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("pod-uid-123"),
		},
		Spec: v1.PodSpec{
			NodeName:   "test-node",
			Containers: []v1.Container{{Name: "main"}},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{Name: "main", ContainerID: "docker://abc123"},
			},
		},
	}

	baseCRR := func() *appsv1alpha1.ContainerRecreateRequest {
		return &appsv1alpha1.ContainerRecreateRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-crr",
				Namespace: "default",
				Labels:    map[string]string{},
			},
			Spec: appsv1alpha1.ContainerRecreateRequestSpec{
				PodName:    "test-pod",
				Containers: []appsv1alpha1.ContainerRecreateRequestContainer{{Name: "main"}},
				Strategy:   &appsv1alpha1.ContainerRecreateRequestStrategy{},
			},
		}
	}

	tests := []struct {
		name        string
		node        *v1.Node
		expectLabel bool
	}{
		{
			name: "node has virtual-kubelet label, CRR should get the label",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: map[string]string{util.VirtualKubeletLabelKey: util.VirtualKubeletLabelValue},
				},
			},
			expectLabel: true,
		},
		{
			name: "node does not have virtual-kubelet label, CRR should not get the label",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: map[string]string{"foo": "bar"},
				},
			},
			expectLabel: false,
		},
		{
			name:        "node not found, should not error and CRR should not get the label",
			node:        nil,
			expectLabel: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crr := baseCRR()
			err := injectPodIntoContainerRecreateRequestV1alpha1(crr, basePod, tt.node)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			labelVal, hasLabel := crr.Labels[util.VirtualKubeletLabelKey]
			if tt.expectLabel {
				if !hasLabel || labelVal != util.VirtualKubeletLabelValue {
					t.Errorf("expected CRR to have label %s=%s, got labels: %v",
						util.VirtualKubeletLabelKey, util.VirtualKubeletLabelValue, crr.Labels)
				}
			} else {
				if hasLabel {
					t.Errorf("expected CRR not to have label %s, got labels: %v",
						util.VirtualKubeletLabelKey, crr.Labels)
				}
			}
		})
	}
}
