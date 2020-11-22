/*
Copyright 2019 The Kruise Authors.

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
	"fmt"
	"net/http"
	"regexp"

	v1 "k8s.io/api/core/v1"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	validationfield "k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

const (
	sidecarSetNameMaxLen = 63
)

var (
	validateSidecarSetNameMsg   = "sidecarset name must consist of alphanumeric characters or '-'"
	validateSidecarSetNameRegex = regexp.MustCompile(validSidecarSetNameFmt)
	validSidecarSetNameFmt      = `^[a-zA-Z0-9\-]+$`
)

// SidecarSetCreateUpdateHandler handles SidecarSet
type SidecarSetCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

func (h *SidecarSetCreateUpdateHandler) validatingSidecarSetFn(ctx context.Context, obj *appsv1alpha1.SidecarSet) (bool, string, error) {
	allErrs := validateSidecarSet(obj)
	if len(allErrs) != 0 {
		return false, "", allErrs.ToAggregate()
	}
	return true, "allowed to be admitted", nil
}

func validateSidecarSet(obj *appsv1alpha1.SidecarSet) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&obj.ObjectMeta, false, validateSidecarSetName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateSidecarSetSpec(obj, field.NewPath("spec"))...)
	return allErrs
}

func validateSidecarSetName(name string, prefix bool) (allErrs []string) {
	if !validateSidecarSetNameRegex.MatchString(name) {
		allErrs = append(allErrs, validationutil.RegexError(validateSidecarSetNameMsg, validSidecarSetNameFmt, "example-com"))
	}
	if len(name) > sidecarSetNameMaxLen {
		allErrs = append(allErrs, validationutil.MaxLenError(sidecarSetNameMaxLen))
	}
	return allErrs
}

func validateSidecarSetSpec(obj *appsv1alpha1.SidecarSet, fldPath *field.Path) field.ErrorList {
	spec := &obj.Spec
	allErrs := field.ErrorList{}

	if spec.Selector == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector"), "no selector defined for sidecarset"))
	} else {
		allErrs = append(allErrs, metavalidation.ValidateLabelSelector(spec.Selector, fldPath.Child("selector"))...)
		if len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is not valid for sidecarset."))
		}
	}

	allErrs = append(allErrs, validateSidecarSetStrategy(&spec.Strategy, fldPath.Child("strategy"))...)
	vols, vErrs := getCoreVolumes(spec.Volumes, fldPath.Child("volumes"))
	allErrs = append(allErrs, vErrs...)
	allErrs = append(allErrs, validateContainersForSidecarSet(spec.InitContainers, spec.Containers, vols, fldPath.Child("containers"))...)

	return allErrs
}

func validateSidecarSetStrategy(strategy *appsv1alpha1.SidecarSetUpdateStrategy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if strategy.RollingUpdate == nil {
		allErrs = append(allErrs, validationfield.Required(fldPath.Child("rollingUpdate"), ""))
	} else {
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*(strategy.RollingUpdate.MaxUnavailable), fldPath.Child("maxUnavailable"))...)
	}
	return allErrs
}

func getCoreVolumes(volumes []v1.Volume, fldPath *field.Path) ([]core.Volume, field.ErrorList) {
	allErrs := field.ErrorList{}

	var coreVolumes []core.Volume
	for _, volume := range volumes {
		coreVolume := core.Volume{}
		if err := corev1.Convert_v1_Volume_To_core_Volume(&volume, &coreVolume, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), volume, fmt.Sprintf("Convert_v1_Volume_To_core_Volume failed: %v", err)))
			return nil, allErrs
		}
		coreVolumes = append(coreVolumes, coreVolume)
	}

	return coreVolumes, allErrs
}

func validateContainersForSidecarSet(
	initContainers, containers []appsv1alpha1.SidecarContainer, coreVolumes []core.Volume, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	var coreInitContainers []core.Container
	for _, container := range initContainers {
		coreContainer := core.Container{}
		if err := corev1.Convert_v1_Container_To_core_Container(&container.Container, &coreContainer, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), container.Container, fmt.Sprintf("Convert_v1_Container_To_core_Container failed: %v", err)))
			return allErrs
		}
		coreInitContainers = append(coreInitContainers, coreContainer)
	}
	var coreContainers []core.Container
	for _, container := range containers {
		coreContainer := core.Container{}
		if err := corev1.Convert_v1_Container_To_core_Container(&container.Container, &coreContainer, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), container.Container, fmt.Sprintf("Convert_v1_Container_To_core_Container failed: %v", err)))
			return allErrs
		}
		coreContainers = append(coreContainers, coreContainer)
	}

	// hack, use fakePod to reuse unexported 'validateContainers' function
	var fakePod *core.Pod
	if len(coreContainers) == 0 {
		// hack, the ValidatePod requires containers, so create a fake coreContainer
		coreContainers = []core.Container{
			{
				Name:                     "test",
				Image:                    "busybox",
				ImagePullPolicy:          core.PullIfNotPresent,
				TerminationMessagePolicy: core.TerminationMessageReadFile,
			},
		}
	}

	fakePod = &core.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: core.PodSpec{
			DNSPolicy:      core.DNSClusterFirst,
			RestartPolicy:  core.RestartPolicyAlways,
			InitContainers: coreInitContainers,
			Containers:     coreContainers,
			Volumes:        coreVolumes,
		},
	}

	allErrs = append(allErrs, corevalidation.ValidatePod(fakePod)...)

	return allErrs
}

var _ admission.Handler = &SidecarSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *SidecarSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.SidecarSet{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	allowed, reason, err := h.validatingSidecarSetFn(ctx, obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

//var _ inject.Client = &SidecarSetCreateUpdateHandler{}
//
//// InjectClient injects the client into the SidecarSetCreateUpdateHandler
//func (h *SidecarSetCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ admission.DecoderInjector = &SidecarSetCreateUpdateHandler{}

// InjectDecoder injects the decoder into the SidecarSetCreateUpdateHandler
func (h *SidecarSetCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
