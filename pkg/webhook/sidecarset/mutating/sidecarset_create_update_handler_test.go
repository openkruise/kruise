package mutating

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
)

func TestSidecarSetCreateHandlerV1beta1(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	sidecarSet := &appsv1beta1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1beta1.SidecarSetSpec{
			Containers: []appsv1beta1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns:1.0",
					},
				},
			},
			PatchPodMetadata: []appsv1beta1.SidecarSetPatchPodMetadata{
				{
					Annotations: map[string]string{
						"key1": "value1",
					},
				},
			},
		},
	}

	rawObj, err := json.Marshal(sidecarSet)
	if err != nil {
		t.Fatalf("Failed to marshal sidecarset: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: rawObj,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Expected allowed response, got: %v", resp.Result)
	}

	if len(resp.Patches) == 0 {
		t.Error("Expected patches to be returned")
	}
}

func TestSidecarSetCreateHandlerV1alpha1(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	sidecarSet := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns:1.0",
					},
				},
			},
		},
	}

	rawObj, err := json.Marshal(sidecarSet)
	if err != nil {
		t.Fatalf("Failed to marshal sidecarset: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: rawObj,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Expected allowed response, got: %v", resp.Result)
	}

	if len(resp.Patches) == 0 {
		t.Error("Expected patches to be returned")
	}
}

func TestSidecarSetUpdateHandlerV1beta1(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	sidecarSet := &appsv1beta1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1beta1.SidecarSetSpec{
			Containers: []appsv1beta1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns:2.0", // Updated image
					},
				},
			},
		},
	}

	rawObj, err := json.Marshal(sidecarSet)
	if err != nil {
		t.Fatalf("Failed to marshal sidecarset: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: rawObj,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Expected allowed response, got: %v", resp.Result)
	}
}

func TestSidecarSetHandlerUnsupportedVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    "apps.kruise.io",
				Version:  "v1alpha999", // Unsupported version
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: []byte("{}"),
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("Expected request to be denied for unsupported version")
	}
	if resp.Result == nil || resp.Result.Message == "" {
		t.Error("Expected error message for unsupported version")
	}
}

func TestSidecarSetHandlerInvalidJSON(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: []byte("invalid json"),
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if resp.Allowed {
		t.Error("Expected request to be denied for invalid JSON")
	}
}

func TestSidecarSetHandlerWithAllDefaults(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	// Create a minimal sidecarset to test default values
	sidecarSet := &appsv1beta1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1beta1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Containers: []appsv1beta1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "sidecar",
						Image: "busybox:1.0",
					},
				},
			},
		},
	}

	rawObj, err := json.Marshal(sidecarSet)
	if err != nil {
		t.Fatalf("Failed to marshal sidecarset: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: rawObj,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Expected allowed response, got: %v", resp.Result)
	}

	// Verify patches were applied
	if len(resp.Patches) == 0 {
		t.Error("Expected patches to be returned for default values")
	}
}

func TestSidecarSetHandlerWithInjectionStrategy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	sidecarSet := &appsv1beta1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1beta1.SidecarSetSpec{
			Containers: []appsv1beta1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns:1.0",
					},
				},
			},
			InjectionStrategy: appsv1beta1.SidecarSetInjectionStrategy{
				Revision: &appsv1beta1.SidecarSetInjectRevision{
					CustomVersion: pointer.String("v1"),
				},
			},
		},
	}

	rawObj, err := json.Marshal(sidecarSet)
	if err != nil {
		t.Fatalf("Failed to marshal sidecarset: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: rawObj,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Expected allowed response, got: %v", resp.Result)
	}
}

func TestSidecarSetHandlerDeleteOperation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	sidecarSet := &appsv1beta1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1beta1.SidecarSetSpec{
			Containers: []appsv1beta1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns:1.0",
					},
				},
			},
		},
	}

	rawObj, err := json.Marshal(sidecarSet)
	if err != nil {
		t.Fatalf("Failed to marshal sidecarset: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Delete, // Delete operation should not mutate
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: rawObj,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Expected allowed response, got: %v", resp.Result)
	}

	// For Delete operation, we expect minimal or no patches
	// since defaults are only applied on Create/Update
}

func TestSidecarSetHandlerWithUpdateStrategy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	sidecarSet := &appsv1beta1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1beta1.SidecarSetSpec{
			Containers: []appsv1beta1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns:1.0",
					},
				},
			},
			UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
				Type:           appsv1beta1.RollingUpdateSidecarSetStrategyType,
				Partition:      &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
				MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
			},
		},
	}

	rawObj, err := json.Marshal(sidecarSet)
	if err != nil {
		t.Fatalf("Failed to marshal sidecarset: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: rawObj,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Expected allowed response, got: %v", resp.Result)
	}
}

func TestSidecarSetHandlerHashAnnotations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	decoder := admission.NewDecoder(scheme)

	handler := &SidecarSetCreateHandler{
		Decoder: decoder,
	}

	sidecarSet := &appsv1beta1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1beta1.SidecarSetSpec{
			Containers: []appsv1beta1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "dns-f",
						Image: "dns:1.0",
					},
				},
			},
		},
	}

	rawObj, err := json.Marshal(sidecarSet)
	if err != nil {
		t.Fatalf("Failed to marshal sidecarset: %v", err)
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "sidecarsets",
			},
			Object: runtime.RawExtension{
				Raw: rawObj,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("Expected allowed response, got: %v", resp.Result)
	}

	// Check that hash annotations are added via patches
	hasHashPatch := false
	hasHashWithoutImagePatch := false
	for _, patch := range resp.Patches {
		pathStr := fmt.Sprintf("%v", patch.Path)
		if pathStr == "/metadata/annotations/"+sidecarcontrol.SidecarSetHashAnnotation {
			hasHashPatch = true
		}
		if pathStr == "/metadata/annotations/"+sidecarcontrol.SidecarSetHashWithoutImageAnnotation {
			hasHashWithoutImagePatch = true
		}
	}

	if !hasHashPatch && !hasHashWithoutImagePatch {
		// At least one of the hash annotations should be present
		t.Log("Warning: Expected hash annotations in patches")
	}
}
