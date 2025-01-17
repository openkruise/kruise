/*
Copyright 2022 The Kruise Authors.

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
	"reflect"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/sets"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

const (
	nameMaxLen = 63
)

var (
	validateNameMsg   = "podProbeMarker name must consist of alphanumeric characters or '-'"
	validateNameRegex = regexp.MustCompile(validNameFmt)
	validNameFmt      = `^[a-zA-Z0-9\-]+$`
	// k8s native pod condition type
	k8sNativePodConditions = sets.NewString(string(corev1.PodScheduled), string(corev1.PodInitialized), string(corev1.PodReady),
		string(corev1.ContainersReady), string(pub.KruisePodReadyConditionType), string(pub.InPlaceUpdateReady))
)

// PodProbeMarkerCreateUpdateHandler handles PodProbeMarker
type PodProbeMarkerCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &PodProbeMarkerCreateUpdateHandler{}

// Handle handles admission requests.
func (h *PodProbeMarkerCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.PodProbeMarker{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if !utilfeature.DefaultFeatureGate.Enabled(features.PodProbeMarkerGate) {
		return admission.Errored(http.StatusForbidden, fmt.Errorf("feature-gate %s is not enabled", features.PodProbeMarkerGate))
	}
	if !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) {
		return admission.Errored(http.StatusForbidden, fmt.Errorf("feature-gate %s is not enabled", features.KruiseDaemon))
	}
	var old *appsv1alpha1.PodProbeMarker
	//when Operation is update, decode older object
	if req.AdmissionRequest.Operation == admissionv1.Update {
		old = new(appsv1alpha1.PodProbeMarker)
		if err := h.Decoder.Decode(
			admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Object: req.AdmissionRequest.OldObject}},
			old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}
	allErrs := h.validatingPodProbeMarkerFn(obj, old)
	if len(allErrs) != 0 {
		return admission.Errored(http.StatusBadRequest, allErrs.ToAggregate())
	}
	return admission.ValidationResponse(true, "")
}

func (h *PodProbeMarkerCreateUpdateHandler) validatingPodProbeMarkerFn(obj, old *appsv1alpha1.PodProbeMarker) field.ErrorList {
	//validate ppm.Spec
	allErrs := validatePodProbeMarkerSpec(obj, field.NewPath("spec"))
	// when operation is update, validating whether old and new pps conflict
	if old != nil {
		if !reflect.DeepEqual(old.Spec.Selector, obj.Spec.Selector) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("selector"), obj.Spec.Selector, "selector must be immutable"))
		}
	}
	return allErrs
}

func validatePodProbeMarkerSpec(obj *appsv1alpha1.PodProbeMarker, fldPath *field.Path) field.ErrorList {
	spec := &obj.Spec
	allErrs := field.ErrorList{}
	// selector
	if spec.Selector == nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty Selector is not valid for PodProbeMarker."))
	} else {
		allErrs = append(allErrs, validateSelector(spec.Selector, fldPath.Child("selector"))...)
	}
	// containerProbe
	if len(spec.Probes) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("probes"), spec.Probes, "empty probes is not valid for PodProbeMarker."))
		return allErrs
	}
	uniqueProbe := sets.NewString()
	uniqueConditionType := sets.NewString()
	for _, probe := range spec.Probes {
		if strings.Contains(probe.Name, "#") {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("probes"), probe.Name, "probe name can't contains '#'."))
			return allErrs
		}
		if probe.Name == "" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("probes"), probe.Name, "probe name can't be empty in PodProbeMarker."))
			return allErrs
		}
		if probe.ContainerName == "" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("probes"), probe.ContainerName, "probe containerName can't be empty in PodProbeMarker."))
			return allErrs
		}
		if k8sNativePodConditions.Has(probe.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("probes"), probe.Name, fmt.Sprintf("probe name can't be %s", probe.Name)))
			return allErrs
		}
		if uniqueProbe.Has(probe.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("probes"), probe.Name, "probe name must be unique in PodProbeMarker."))
			return allErrs
		}
		uniqueProbe.Insert(probe.Name)
		allErrs = append(allErrs, validateProbe(&probe.Probe, fldPath.Child("probe"))...)
		if probe.PodConditionType == "" && len(probe.MarkerPolicy) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("probes"), probe, "podConditionType and markerPolicy cannot be empty at the same time"))
			return allErrs
		}
		if probe.PodConditionType != "" && uniqueConditionType.Has(probe.PodConditionType) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("probes"), probe.PodConditionType, fmt.Sprintf("podConditionType %s must be unique in podProbeMarker", probe.PodConditionType)))
			return allErrs
		} else if probe.PodConditionType != "" {
			uniqueConditionType.Insert(probe.PodConditionType)
			allErrs = append(allErrs, metavalidation.ValidateLabelName(probe.PodConditionType, fldPath)...)
		}
		uniquePolicy := sets.NewString()
		for _, policy := range probe.MarkerPolicy {
			if uniquePolicy.Has(string(policy.State)) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("markerPolicy"), policy.State, "marker policy state must be unique."))
				return allErrs
			}
			uniquePolicy.Insert(string(policy.State))
			allErrs = append(allErrs, validateProbeMarkerPolicy(&policy, fldPath.Child("markerPolicy"))...)
		}
	}

	return allErrs
}

func validateSelector(selector *metav1.LabelSelector, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, metavalidation.ValidateLabelSelector(selector, metavalidation.LabelSelectorValidationOptions{}, fldPath)...)
	if len(selector.MatchLabels)+len(selector.MatchExpressions) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, selector, "empty selector is not valid for podProbeMarker."))
	}
	_, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), selector, ""))
	}
	return allErrs
}

func validateProbe(probe *appsv1alpha1.ContainerProbeSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if probe == nil {
		allErrs = append(allErrs, field.Invalid(fldPath, probe, "probe can't be empty in podProbeMarker."))
		return allErrs
	}
	allErrs = append(allErrs, validateHandler(&probe.ProbeHandler, fldPath)...)
	allErrs = append(allErrs, validation.ValidateNonnegativeField(int64(probe.InitialDelaySeconds), fldPath.Child("initialDelaySeconds"))...)
	allErrs = append(allErrs, validation.ValidateNonnegativeField(int64(probe.TimeoutSeconds), fldPath.Child("timeoutSeconds"))...)
	allErrs = append(allErrs, validation.ValidateNonnegativeField(int64(probe.PeriodSeconds), fldPath.Child("periodSeconds"))...)
	allErrs = append(allErrs, validation.ValidateNonnegativeField(int64(probe.SuccessThreshold), fldPath.Child("successThreshold"))...)
	allErrs = append(allErrs, validation.ValidateNonnegativeField(int64(probe.FailureThreshold), fldPath.Child("failureThreshold"))...)
	return allErrs
}

func validateHandler(handler *corev1.ProbeHandler, fldPath *field.Path) field.ErrorList {
	numHandlers := 0
	allErrors := field.ErrorList{}
	if handler.Exec != nil {
		if numHandlers > 0 {
			allErrors = append(allErrors, field.Forbidden(fldPath.Child("exec"), "may not specify more than 1 handler type"))
		} else {
			numHandlers++
			allErrors = append(allErrors, validateExecAction(handler.Exec, fldPath.Child("exec"))...)
		}
	}
	if handler.HTTPGet != nil {
		if numHandlers > 0 {
			allErrors = append(allErrors, field.Forbidden(fldPath.Child("httpGet"), "may not specify more than 1 handler type"))
		} else {
			numHandlers++
			allErrors = append(allErrors, field.Forbidden(fldPath.Child("probe"), "current no support http probe"))
		}
	}
	if handler.TCPSocket != nil {
		if numHandlers > 0 {
			allErrors = append(allErrors, field.Forbidden(fldPath.Child("tcpSocket"), "may not specify more than 1 handler type"))
		} else {
			numHandlers++
			allErrors = append(allErrors, validateTCPSocketAction(handler.TCPSocket, fldPath.Child("tcpSocket"))...)
		}
	}

	if numHandlers == 0 {
		allErrors = append(allErrors, field.Required(fldPath, "must specify a handler type"))
	}
	return allErrors
}

func validateExecAction(exec *corev1.ExecAction, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}
	if len(exec.Command) == 0 {
		allErrors = append(allErrors, field.Required(fldPath.Child("command"), ""))
	}
	return allErrors
}

func validateTCPSocketAction(tcp *corev1.TCPSocketAction, fldPath *field.Path) field.ErrorList {
	return ValidatePortNumOrName(tcp.Port, fldPath.Child("port"))
}

func ValidatePortNumOrName(port intstr.IntOrString, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if port.Type == intstr.Int {
		for _, msg := range validationutil.IsValidPortNum(port.IntValue()) {
			allErrs = append(allErrs, field.Invalid(fldPath, port.IntValue(), msg))
		}
	} else if port.Type == intstr.String {
		for _, msg := range validationutil.IsValidPortName(port.StrVal) {
			allErrs = append(allErrs, field.Invalid(fldPath, port.StrVal, msg))
		}
	} else {
		allErrs = append(allErrs, field.InternalError(fldPath, fmt.Errorf("unknown type: %v", port.Type)))
	}
	return allErrs
}

func validateProbeMarkerPolicy(policy *appsv1alpha1.ProbeMarkerPolicy, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}
	if policy.State != appsv1alpha1.ProbeSucceeded && policy.State != appsv1alpha1.ProbeFailed {
		allErrors = append(allErrors, field.Required(fldPath.Child("state"), "marker policy state must be 'True' or 'False'"))
		return allErrors
	}
	if len(policy.Labels) == 0 && len(policy.Annotations) == 0 {
		allErrors = append(allErrors, field.Required(fldPath, "marker policy annotations or labels can't be both empty"))
		return allErrors
	}
	metadata := metav1.ObjectMeta{Annotations: policy.Annotations, Labels: policy.Labels, Name: "fake-name"}
	allErrors = append(allErrors, genericvalidation.ValidateObjectMeta(&metadata, false, validatePodProbeMarkerName, fldPath)...)

	return allErrors
}

func validatePodProbeMarkerName(name string, prefix bool) (allErrs []string) {
	if !validateNameRegex.MatchString(name) {
		allErrs = append(allErrs, validationutil.RegexError(validateNameMsg, validNameFmt, "example-com"))
	}
	if len(name) > nameMaxLen {
		allErrs = append(allErrs, validationutil.MaxLenError(nameMaxLen))
	}
	return allErrs
}
