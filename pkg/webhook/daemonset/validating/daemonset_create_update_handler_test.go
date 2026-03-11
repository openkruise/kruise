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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestDaemonSetCreateUpdateHandler_HandleV1alpha1Create(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(1)
	ds := &appsv1alpha1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "default",
		},
		Spec: appsv1alpha1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
		},
	}

	dsBytes, _ := json.Marshal(ds)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1alpha1CreateInvalid(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	// Invalid DaemonSet - selector doesn't match labels
	ds := &appsv1alpha1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "default",
		},
		Spec: appsv1alpha1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "different",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
			},
		},
	}

	dsBytes, _ := json.Marshal(ds)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if resp.Allowed {
		t.Errorf("expected denied, got allowed")
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1alpha1Update(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(1)
	intOrStr1 := intstr.FromInt(1)
	intOrStr2 := intstr.FromInt(2)

	oldDs := &appsv1alpha1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ds",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Spec: appsv1alpha1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
			BurstReplicas: &intOrStr1,
		},
	}

	newDs := oldDs.DeepCopy()
	newDs.ResourceVersion = "2"
	newDs.Spec.BurstReplicas = &intOrStr2

	dsBytes, _ := json.Marshal(newDs)
	oldDsBytes, _ := json.Marshal(oldDs)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
			OldObject: runtime.RawExtension{
				Raw: oldDsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1alpha1UpdateForbidden(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(1)

	oldDs := &appsv1alpha1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ds",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Spec: appsv1alpha1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
		},
	}

	// Try to update forbidden field - selector
	newDs := oldDs.DeepCopy()
	newDs.ResourceVersion = "2"
	newDs.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "different",
		},
	}

	dsBytes, _ := json.Marshal(newDs)
	oldDsBytes, _ := json.Marshal(oldDs)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
			OldObject: runtime.RawExtension{
				Raw: oldDsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if resp.Allowed {
		t.Errorf("expected denied, got allowed")
	}
	if resp.Result.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected status code %d, got %d", http.StatusUnprocessableEntity, resp.Result.Code)
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1beta1Create(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(1)
	ds := &appsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "default",
		},
		Spec: appsv1beta1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
						},
					},
				},
			},
			UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
		},
	}

	dsBytes, _ := json.Marshal(ds)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1beta1Update(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(1)
	intOrStr1 := intstr.FromInt(1)
	intOrStr2 := intstr.FromInt(2)

	oldDs := &appsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ds",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Spec: appsv1beta1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
						},
					},
				},
			},
			UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
			BurstReplicas: &intOrStr1,
		},
	}

	newDs := oldDs.DeepCopy()
	newDs.ResourceVersion = "2"
	newDs.Spec.BurstReplicas = &intOrStr2

	dsBytes, _ := json.Marshal(newDs)
	oldDsBytes, _ := json.Marshal(oldDs)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
			OldObject: runtime.RawExtension{
				Raw: oldDsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
}

func TestDaemonSetCreateUpdateHandler_HandleUnsupportedVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    "apps.kruise.io",
				Version:  "v1gamma1",
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: []byte("{}"),
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if resp.Allowed {
		t.Errorf("expected denied for unsupported version, got allowed")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, resp.Result.Code)
	}
}

func TestDaemonSetCreateUpdateHandler_HandleDecodeError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: []byte("invalid json"),
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if resp.Allowed {
		t.Errorf("expected denied for decode error, got allowed")
	}
	if resp.Result.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, resp.Result.Code)
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1alpha1CreateWithWarnings(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(0)
	maxSurge := intstr.FromInt(1)
	ds := &appsv1alpha1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "default",
		},
		Spec: appsv1alpha1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
							Ports: []corev1.ContainerPort{
								{HostPort: 8080, ContainerPort: 80},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
		},
	}

	dsBytes, _ := json.Marshal(ds)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
	if len(resp.Warnings) == 0 {
		t.Errorf("expected warnings for hostPort with maxSurge, got none")
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1alpha1UpdateWithWarnings(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(0)
	maxSurge := intstr.FromInt(1)

	oldDs := &appsv1alpha1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ds",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Spec: appsv1alpha1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
							Ports: []corev1.ContainerPort{
								{HostPort: 8080, ContainerPort: 80},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
		},
	}

	newDs := oldDs.DeepCopy()
	newDs.ResourceVersion = "2"
	newDs.Spec.Template.Spec.Containers[0].Image = "test:v2"

	dsBytes, _ := json.Marshal(newDs)
	oldDsBytes, _ := json.Marshal(oldDs)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
			OldObject: runtime.RawExtension{
				Raw: oldDsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
	if len(resp.Warnings) == 0 {
		t.Errorf("expected warnings for hostPort with maxSurge, got none")
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1beta1CreateWithWarnings(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(0)
	maxSurge := intstr.FromInt(1)
	ds := &appsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "default",
		},
		Spec: appsv1beta1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
							Ports: []corev1.ContainerPort{
								{HostPort: 8080, ContainerPort: 80},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
		},
	}

	dsBytes, _ := json.Marshal(ds)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
	if len(resp.Warnings) == 0 {
		t.Errorf("expected warnings for hostPort with maxSurge, got none")
	}
}

func TestDaemonSetCreateUpdateHandler_HandleV1beta1UpdateWithWarnings(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)

	maxUnavailable := intstr.FromInt(0)
	maxSurge := intstr.FromInt(1)

	oldDs := &appsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ds",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Spec: appsv1beta1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:v1",
							Ports: []corev1.ContainerPort{
								{HostPort: 8080, ContainerPort: 80},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
		},
	}

	newDs := oldDs.DeepCopy()
	newDs.ResourceVersion = "2"
	newDs.Spec.Template.Spec.Containers[0].Image = "test:v2"

	dsBytes, _ := json.Marshal(newDs)
	oldDsBytes, _ := json.Marshal(oldDs)

	handler := &DaemonSetCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "daemonsets",
			},
			Object: runtime.RawExtension{
				Raw: dsBytes,
			},
			OldObject: runtime.RawExtension{
				Raw: oldDsBytes,
			},
		},
	}

	resp := handler.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("expected allowed, got denied: %v", resp.Result)
	}
	if len(resp.Warnings) == 0 {
		t.Errorf("expected warnings for hostPort with maxSurge, got none")
	}
}
