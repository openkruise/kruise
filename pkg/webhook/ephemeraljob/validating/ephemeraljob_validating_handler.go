/*
Copyright 2023 The Kruise Authors.

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
	"net/http"
	"unsafe"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// EphemeralJobCreateUpdateHandler handles ImagePullJob
type EphemeralJobCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &EphemeralJobCreateUpdateHandler{}

// Handle handles admission requests.
func (h *EphemeralJobCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	job := &appsv1alpha1.EphemeralJob{}

	err := h.Decoder.Decode(req, job)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := validateEphemeralJobSpec(&job.Spec, field.NewPath("spec")); err != nil {
		klog.Warningf("Error validate EphemeralJob %s/%s: %v", job.Namespace, job.Name, err)
		return admission.Errored(http.StatusBadRequest, err.ToAggregate())
	}

	return admission.ValidationResponse(true, "allowed")
}

func validateEphemeralJobSpec(spec *appsv1alpha1.EphemeralJobSpec, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if spec.Selector != nil {
		if spec.Selector.MatchLabels != nil || spec.Selector.MatchExpressions != nil {
			if _, err := metav1.LabelSelectorAsSelector(spec.Selector); err != nil {
				allErrs = append(allErrs, field.Invalid(path.Child("selector"), spec.Selector, err.Error()))
			}
		}
	}

	var ephemeralContainers []v1.EphemeralContainer
	ecPath := path.Child("template").Child("ephemeralContainers")
	for index, ec := range spec.Template.EphemeralContainers {
		idxPath := ecPath.Index(index)

		// VolumeMount subpaths have the potential to leak resources since they're implemented with bind mounts
		// that aren't cleaned up until the pod exits. Since they also imply that the container is being used
		// as part of the workload, they're disallowed entirely.
		for i, vm := range ec.VolumeMounts {
			if vm.SubPath != "" {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("volumeMounts").Index(i).Child("subPath"), "cannot be set for an Ephemeral Container"))
			}
			if vm.SubPathExpr != "" {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("volumeMounts").Index(i).Child("subPathExpr"), "cannot be set for an Ephemeral Container"))
			}
		}

		// VolumeMount cannot be validated further by ValidatePodEphemeralContainersUpdate method because we do not know the volumes and target container of target Pods
		ec.VolumeMounts, ec.TargetContainerName = nil, ""
		corev1.SetDefaults_Container((*v1.Container)(&ec.EphemeralContainerCommon))
		ephemeralContainers = append(ephemeralContainers, ec)
	}

	// validateEphemeralContainers is a private method in k8s validation package, so we have to use ValidatePodEphemeralContainersUpdate
	// to validate fields of the fields of ephemeral containers
	mockedNewPod := &core.Pod{Spec: core.PodSpec{EphemeralContainers: *(*[]core.EphemeralContainer)(unsafe.Pointer(&ephemeralContainers))}}
	mockedOldPod := &core.Pod{Spec: core.PodSpec{EphemeralContainers: *(*[]core.EphemeralContainer)(unsafe.Pointer(&ephemeralContainers))}}
	return append(allErrs, validation.ValidatePodEphemeralContainersUpdate(mockedNewPod, mockedOldPod, validation.PodValidationOptions{})...)
}
