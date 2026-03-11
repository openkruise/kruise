package validating

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	genericvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/convertor"
)

func validatingDaemonSetFn(ctx context.Context, obj *appsv1alpha1.DaemonSet) (bool, string, error) {
	allErrs := validateDaemonSet(obj)
	if len(allErrs) != 0 {
		return false, "", allErrs.ToAggregate()
	}
	return true, "allowed to be admitted", nil
}

func validateDaemonSet(ds *appsv1alpha1.DaemonSet) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&ds.ObjectMeta, true, ValidateDaemonSetName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateDaemonSetSpec(&ds.Spec, field.NewPath("spec"))...)
	return allErrs
}

// ValidateDaemonSetSpec tests if required fields in the DaemonSetSpec are set.
func validateDaemonSetSpec(spec *appsv1alpha1.DaemonSetSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, metavalidation.ValidateLabelSelector(spec.Selector, metavalidation.LabelSelectorValidationOptions{}, fldPath.Child("selector"))...)

	selector, err := metav1.LabelSelectorAsSelector(spec.Selector)
	if err == nil && !selector.Matches(labels.Set(spec.Template.Labels)) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("template", "metadata", "labels"), spec.Template.Labels, "`selector` does not match template `labels`"))
	}
	if spec.Selector != nil && len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is invalid for daemonset"))
	}

	coreTemplate, err := convertor.ConvertPodTemplateSpec(&spec.Template)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.Template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
		return allErrs
	}
	allErrs = append(allErrs, corevalidation.ValidatePodTemplateSpec(coreTemplate, fldPath.Child("template"), webhookutil.DefaultPodValidationOptions)...)

	// RestartPolicy has already been first-order validated as per ValidatePodTemplateSpec().
	if spec.Template.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("template", "spec", "restartPolicy"), spec.Template.Spec.RestartPolicy, []string{string(corev1.RestartPolicyAlways)}))
	}
	if spec.Template.Spec.ActiveDeadlineSeconds != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("template", "spec", "activeDeadlineSeconds"), "activeDeadlineSeconds in DaemonSet is not Supported"))
	}
	allErrs = append(allErrs, corevalidation.ValidateNonnegativeField(int64(spec.MinReadySeconds), fldPath.Child("minReadySeconds"))...)

	allErrs = append(allErrs, validateDaemonSetUpdateStrategy(&spec.UpdateStrategy, fldPath.Child("updateStrategy"))...)
	if spec.RevisionHistoryLimit != nil {
		// zero is a valid RevisionHistoryLimit
		allErrs = append(allErrs, corevalidation.ValidateNonnegativeField(int64(*spec.RevisionHistoryLimit), fldPath.Child("revisionHistoryLimit"))...)
	}

	if spec.Lifecycle != nil {
		if spec.Lifecycle.InPlaceUpdate != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("lifecycle", "inPlaceUpdate"), "inPlaceUpdate hook has not supported yet"))
		}
	}
	return allErrs
}

func validateDaemonSetUpdateStrategy(strategy *appsv1alpha1.DaemonSetUpdateStrategy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch strategy.Type {
	case appsv1alpha1.OnDeleteDaemonSetStrategyType:
	case appsv1alpha1.RollingUpdateDaemonSetStrategyType:
		// Make sure RollingUpdate field isn't nil.
		if strategy.RollingUpdate == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("rollingUpdate"), ""))
			return allErrs
		}
		allErrs = append(allErrs, validateRollingUpdateDaemonSet(strategy.RollingUpdate, fldPath.Child("rollingUpdate"))...)
	default:
		validValues := []string{string(appsv1alpha1.RollingUpdateDaemonSetStrategyType), string(appsv1alpha1.OnDeleteDaemonSetStrategyType)}
		allErrs = append(allErrs, field.NotSupported(fldPath, strategy, validValues))
	}
	return allErrs
}

func validateRollingUpdateDaemonSet(rollingUpdate *appsv1alpha1.RollingUpdateDaemonSet, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	var hasUnavailable, hasSurge bool
	if rollingUpdate.MaxUnavailable != nil && getIntOrPercentValue(*rollingUpdate.MaxUnavailable) != 0 {
		hasUnavailable = true
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*rollingUpdate.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
		allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*rollingUpdate.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
	}
	if rollingUpdate.MaxSurge != nil && getIntOrPercentValue(*rollingUpdate.MaxSurge) != 0 {
		hasSurge = true
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*rollingUpdate.MaxSurge, fldPath.Child("maxSurge"))...)
		allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*rollingUpdate.MaxSurge, fldPath.Child("maxSurge"))...)
	}
	switch {
	case hasUnavailable && hasSurge:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxSurge"), rollingUpdate.MaxSurge, "may not be set when maxUnavailable is non-zero"))
	case !hasUnavailable && !hasSurge:
		allErrs = append(allErrs, field.Required(fldPath.Child("maxUnavailable"), "cannot be 0 when maxSurge is 0"))
	}

	switch rollingUpdate.Type {
	case "", appsv1alpha1.StandardRollingUpdateType:
	case appsv1alpha1.InplaceRollingUpdateType:
		if hasSurge {
			allErrs = append(allErrs, field.Required(fldPath.Child("maxSurge"), "must be 0 for InPlaceIfPossible type"))
		}
	case appsv1alpha1.DeprecatedSurgingRollingUpdateType:
		if hasUnavailable {
			allErrs = append(allErrs, field.Required(fldPath.Child("maxUnavailable"), "must be 0 for Surging type"))
		}
	default:
		validValues := []string{string(appsv1alpha1.StandardRollingUpdateType), string(appsv1alpha1.DeprecatedSurgingRollingUpdateType), string(appsv1alpha1.InplaceRollingUpdateType)}
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("rollingUpdate").Child("type"), rollingUpdate.Type, validValues))
	}

	if rollingUpdate.Partition != nil {
		allErrs = append(allErrs, validateNonnegativeIntOrPercent(*rollingUpdate.Partition, fldPath.Child("rollingUpdate").Child("partition"))...)
		allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*rollingUpdate.Partition, fldPath.Child("rollingUpdate").Child("partition"))...)
	}

	return allErrs
}

func getIntOrPercentValue(intOrStringValue intstr.IntOrString) int {
	value, isPercent := getPercentValue(intOrStringValue)
	if isPercent {
		return value
	}
	return intOrStringValue.IntValue()
}

// validateNonnegativeIntOrPercent tests if a given value is a valid non-negative int or percentage.
func validateNonnegativeIntOrPercent(intOrPercent intstr.IntOrString, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch intOrPercent.Type {
	case intstr.String:
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(intOrPercent, fldPath)...)
	case intstr.Int:
		allErrs = append(allErrs, corevalidation.ValidateNonnegativeField(int64(intOrPercent.IntValue()), fldPath)...)
	}
	return allErrs
}

func getPercentValue(intOrStringValue intstr.IntOrString) (int, bool) {
	if intOrStringValue.Type != intstr.String {
		return 0, false
	}
	if len(validation.IsValidPercent(intOrStringValue.StrVal)) != 0 {
		return 0, false
	}
	value, _ := strconv.Atoi(intOrStringValue.StrVal[:len(intOrStringValue.StrVal)-1])
	return value, true
}

// hasHostPort checks if any container in the pod spec uses HostPort
func hasHostPort(spec *corev1.PodSpec) bool {
	for _, container := range spec.Containers {
		for _, port := range container.Ports {
			if port.HostPort > 0 {
				return true
			}
		}
	}
	for _, container := range spec.InitContainers {
		for _, port := range container.Ports {
			if port.HostPort > 0 {
				return true
			}
		}
	}
	return false
}

// GetDaemonSetWarnings returns warnings for DaemonSet configurations that may cause issues.
// This checks for known problematic combinations like HostPort with maxSurge > 0.
// See: https://github.com/kubernetes/kubernetes/issues/106417
func GetDaemonSetWarnings(spec *corev1.PodSpec, maxSurge *intstr.IntOrString) []string {
	var warnings []string
	if maxSurge != nil && getIntOrPercentValue(*maxSurge) > 0 && hasHostPort(spec) {
		warnings = append(warnings, "spec.updateStrategy.rollingUpdate.maxSurge is set with a HostPort container, which can cause port conflicts when old and new pods run simultaneously during rolling updates. See https://github.com/kubernetes/kubernetes/issues/106417")
	}
	return warnings
}

// V1beta1 validation functions
func validatingDaemonSetFnV1beta1(ctx context.Context, obj *appsv1beta1.DaemonSet) (bool, string, error) {
	allErrs := validateDaemonSetV1beta1(obj)
	if len(allErrs) != 0 {
		return false, "", allErrs.ToAggregate()
	}
	return true, "allowed to be admitted", nil
}

func validateDaemonSetV1beta1(ds *appsv1beta1.DaemonSet) field.ErrorList {
	allErrs := genericvalidation.ValidateObjectMeta(&ds.ObjectMeta, true, ValidateDaemonSetName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateDaemonSetSpecV1beta1(&ds.Spec, field.NewPath("spec"))...)
	return allErrs
}

// ValidateDaemonSetSpec tests if required fields in the DaemonSetSpec are set.
func validateDaemonSetSpecV1beta1(spec *appsv1beta1.DaemonSetSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, metavalidation.ValidateLabelSelector(spec.Selector, metavalidation.LabelSelectorValidationOptions{}, fldPath.Child("selector"))...)

	selector, err := metav1.LabelSelectorAsSelector(spec.Selector)
	if err == nil && !selector.Matches(labels.Set(spec.Template.Labels)) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("template", "metadata", "labels"), spec.Template.Labels, "`selector` does not match template `labels`"))
	}
	if spec.Selector != nil && len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is invalid for daemonset"))
	}

	coreTemplate, err := convertor.ConvertPodTemplateSpec(&spec.Template)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.Template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
		return allErrs
	}
	allErrs = append(allErrs, corevalidation.ValidatePodTemplateSpec(coreTemplate, fldPath.Child("template"), webhookutil.DefaultPodValidationOptions)...)

	// RestartPolicy has already been first-order validated as per ValidatePodTemplateSpec().
	if spec.Template.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("template", "spec", "restartPolicy"), spec.Template.Spec.RestartPolicy, []string{string(corev1.RestartPolicyAlways)}))
	}
	if spec.Template.Spec.ActiveDeadlineSeconds != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("template", "spec", "activeDeadlineSeconds"), "activeDeadlineSeconds in DaemonSet is not Supported"))
	}
	allErrs = append(allErrs, corevalidation.ValidateNonnegativeField(int64(spec.MinReadySeconds), fldPath.Child("minReadySeconds"))...)

	allErrs = append(allErrs, validateDaemonSetUpdateStrategyV1beta1(&spec.UpdateStrategy, fldPath.Child("updateStrategy"))...)
	if spec.RevisionHistoryLimit != nil {
		// zero is a valid RevisionHistoryLimit
		allErrs = append(allErrs, corevalidation.ValidateNonnegativeField(int64(*spec.RevisionHistoryLimit), fldPath.Child("revisionHistoryLimit"))...)
	}

	if spec.Lifecycle != nil {
		if spec.Lifecycle.InPlaceUpdate != nil {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("lifecycle", "inPlaceUpdate"), "inPlaceUpdate hook has not supported yet"))
		}
	}
	return allErrs
}

func validateDaemonSetUpdateStrategyV1beta1(strategy *appsv1beta1.DaemonSetUpdateStrategy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch strategy.Type {
	case appsv1beta1.OnDeleteDaemonSetStrategyType:
	case appsv1beta1.RollingUpdateDaemonSetStrategyType:
		// Make sure RollingUpdate field isn't nil.
		if strategy.RollingUpdate == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("rollingUpdate"), ""))
			return allErrs
		}
		allErrs = append(allErrs, validateRollingUpdateDaemonSetV1beta1(strategy.RollingUpdate, fldPath.Child("rollingUpdate"))...)
	default:
		validValues := []string{string(appsv1beta1.RollingUpdateDaemonSetStrategyType), string(appsv1beta1.OnDeleteDaemonSetStrategyType)}
		allErrs = append(allErrs, field.NotSupported(fldPath, strategy, validValues))
	}
	return allErrs
}

func validateRollingUpdateDaemonSetV1beta1(rollingUpdate *appsv1beta1.RollingUpdateDaemonSet, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	var hasUnavailable, hasSurge bool
	if rollingUpdate.MaxUnavailable != nil && getIntOrPercentValue(*rollingUpdate.MaxUnavailable) != 0 {
		hasUnavailable = true
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*rollingUpdate.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
		allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*rollingUpdate.MaxUnavailable, fldPath.Child("maxUnavailable"))...)
	}
	if rollingUpdate.MaxSurge != nil && getIntOrPercentValue(*rollingUpdate.MaxSurge) != 0 {
		hasSurge = true
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(*rollingUpdate.MaxSurge, fldPath.Child("maxSurge"))...)
		allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*rollingUpdate.MaxSurge, fldPath.Child("maxSurge"))...)
	}
	switch {
	case hasUnavailable && hasSurge:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxSurge"), rollingUpdate.MaxSurge, "may not be set when maxUnavailable is non-zero"))
	case !hasUnavailable && !hasSurge:
		allErrs = append(allErrs, field.Required(fldPath.Child("maxUnavailable"), "cannot be 0 when maxSurge is 0"))
	}

	switch rollingUpdate.Type {
	case "", appsv1beta1.StandardRollingUpdateType:
	case appsv1beta1.InplaceRollingUpdateType:
		if hasSurge {
			allErrs = append(allErrs, field.Required(fldPath.Child("maxSurge"), "must be 0 for InPlaceIfPossible type"))
		}
	default:
		validValues := []string{string(appsv1beta1.StandardRollingUpdateType), string(appsv1beta1.InplaceRollingUpdateType)}
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("rollingUpdate").Child("type"), rollingUpdate.Type, validValues))
	}

	if rollingUpdate.Partition != nil {
		allErrs = append(allErrs, validateNonnegativeIntOrPercent(*rollingUpdate.Partition, fldPath.Child("rollingUpdate").Child("partition"))...)
		allErrs = append(allErrs, appsvalidation.IsNotMoreThan100Percent(*rollingUpdate.Partition, fldPath.Child("rollingUpdate").Child("partition"))...)
	}

	return allErrs
}
