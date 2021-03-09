package validating

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/appscode/jsonpatch"
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/intstr"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"

	"github.com/openkruise/kruise/pkg/webhook/util/convertor"
)

var inPlaceUpdateTemplateSpecPatchRexp = regexp.MustCompile("/containers/([0-9]+)/image")

// TODO (rz): break this giant function down further and use unit testing
// ValidateStatefulSetSpec tests if required fields in the StatefulSet spec are set.
func validateStatefulSetSpec(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch spec.PodManagementPolicy {
	case "":
		allErrs = append(allErrs, field.Required(fldPath.Child("podManagementPolicy"), ""))
	case apps.OrderedReadyPodManagement, apps.ParallelPodManagement:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("podManagementPolicy"), spec.PodManagementPolicy, fmt.Sprintf("must be '%s' or '%s'", apps.OrderedReadyPodManagement, apps.ParallelPodManagement)))
	}

	if spec.ReserveOrdinals != nil {
		orders := sets.NewInt(spec.ReserveOrdinals...)
		if orders.Len() != len(spec.ReserveOrdinals) {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.ReserveOrdinals, "reserveOrdinals contains duplicated items"))
		}
		for _, i := range spec.ReserveOrdinals {
			if i < 0 || i >= int(*spec.Replicas)+orders.Len() {
				allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.ReserveOrdinals, fmt.Sprintf("reserveOrdinals contains %d which must be 0 <= order < (%d+%d)",
					i, *spec.Replicas, orders.Len())))
			}
		}
	}

	switch spec.UpdateStrategy.Type {
	case "":
		allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy"), ""))
	case apps.OnDeleteStatefulSetStrategyType:
		if spec.UpdateStrategy.RollingUpdate != nil {
			allErrs = append(
				allErrs,
				field.Invalid(
					fldPath.Child("updateStrategy").Child("rollingUpdate"),
					spec.UpdateStrategy.RollingUpdate,
					fmt.Sprintf("only allowed for updateStrategy '%s'", apps.RollingUpdateStatefulSetStrategyType)))
		}
	case apps.RollingUpdateStatefulSetStrategyType:
		if spec.UpdateStrategy.RollingUpdate != nil {
			allErrs = append(allErrs,
				apivalidation.ValidateNonnegativeField(
					int64(*spec.UpdateStrategy.RollingUpdate.Partition),
					fldPath.Child("updateStrategy").Child("rollingUpdate").Child("partition"))...)
			// validate `minReadySeconds` field's range
			allErrs = append(allErrs,
				apivalidation.ValidateNonnegativeField(
					int64(*spec.UpdateStrategy.RollingUpdate.MinReadySeconds),
					fldPath.Child("updateStrategy").Child("rollingUpdate").Child("minReadySeconds"))...)
			if *spec.UpdateStrategy.RollingUpdate.MinReadySeconds > appsv1beta1.MaxMinReadySeconds {
				allErrs = append(allErrs,
					field.Invalid(fldPath.Child("updateStrategy").Child("rollingUpdate").Child("minReadySeconds"),
						*spec.UpdateStrategy.RollingUpdate.MinReadySeconds,
						"must be no more than 300 seconds"))
			}

			// validate the `maxUnavailable` field
			allErrs = append(allErrs, validateMaxUnavailableField(spec, fldPath)...)

			// validate the `PodUpdatePolicy` related fields
			allErrs = append(allErrs, validatePodUpdatePolicy(spec, fldPath)...)

			if spec.UpdateStrategy.RollingUpdate.UnorderedUpdate != nil {
				if apps.ParallelPodManagement != spec.PodManagementPolicy {
					allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy").
						Child("rollingUpdate").Child("unorderedUpdate"),
						"unorderedUpdate can only work with Parallel PodManagementPolicyType"))
				}
				if err := spec.UpdateStrategy.RollingUpdate.UnorderedUpdate.PriorityStrategy.FieldsValidation(); err != nil {
					allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy").
						Child("rollingUpdate").Child("unorderedUpdate").Child("priorityStrategy"),
						err.Error()))
				}
			}
		}
	default:
		allErrs = append(allErrs,
			field.Invalid(fldPath.Child("updateStrategy"), spec.UpdateStrategy,
				fmt.Sprintf("must be '%s' or '%s'",
					apps.RollingUpdateStatefulSetStrategyType,
					apps.OnDeleteStatefulSetStrategyType)))
	}

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*spec.Replicas), fldPath.Child("replicas"))...)
	if spec.Selector == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector"), ""))
	} else {
		allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(spec.Selector, fldPath.Child("selector"))...)
		if len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is invalid for statefulset"))
		}
	}

	selector, err := metav1.LabelSelectorAsSelector(spec.Selector)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, ""))
	} else {
		coreTemplate, err := convertor.ConvertPodTemplateSpec(&spec.Template)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.Template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
			return allErrs
		}
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForStatefulSet(coreTemplate, selector, fldPath.Child("template"))...)
	}

	if spec.Template.Spec.RestartPolicy != "" && spec.Template.Spec.RestartPolicy != v1.RestartPolicyAlways {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("template", "spec", "restartPolicy"), spec.Template.Spec.RestartPolicy, []string{string(v1.RestartPolicyAlways)}))
	}
	if spec.Template.Spec.ActiveDeadlineSeconds != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("template", "spec", "activeDeadlineSeconds"), "activeDeadlineSeconds in StatefulSet is not Supported"))
	}

	return allErrs
}

func validatePodUpdatePolicy(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	switch spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy {
	case "":
		allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy").Child("rollingUpdate").Child("podUpdatePolicy"), ""))
	case appsv1beta1.RecreatePodUpdateStrategyType:
	case appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, appsv1beta1.InPlaceOnlyPodUpdateStrategyType:
		var containsReadinessGate bool
		for _, r := range spec.Template.Spec.ReadinessGates {
			if r.ConditionType == appspub.InPlaceUpdateReady {
				containsReadinessGate = true
				break
			}
		}
		if !containsReadinessGate {
			allErrs = append(allErrs,
				field.Invalid(fldPath.Child("template").Child("spec").Child("readinessGates"),
					spec.Template.Spec.ReadinessGates,
					fmt.Sprintf("must contains %v when podUpdatePolicy is %v",
						appspub.InPlaceUpdateReady,
						spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy)))
		}
	default:
		allErrs = append(allErrs,
			field.Invalid(fldPath.Child("updateStrategy").Child("rollingUpdate").Child("podUpdatePolicy"),
				spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy,
				fmt.Sprintf("must be '%s', %s or '%s'",
					appsv1beta1.RecreatePodUpdateStrategyType,
					appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
					appsv1beta1.InPlaceOnlyPodUpdateStrategyType)))
	}
	return allErrs
}

func validateMaxUnavailableField(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if spec.UpdateStrategy.RollingUpdate.MaxUnavailable == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy").
			Child("rollingUpdate").Child("maxUnavailable"), ""))
	} else {
		allErrs = append(allErrs,
			appsvalidation.ValidatePositiveIntOrPercent(
				*spec.UpdateStrategy.RollingUpdate.MaxUnavailable,
				fldPath.Child("updateStrategy").Child("rollingUpdate").Child("maxUnavailable"))...)
		if maxUnavailable, err := intstrutil.GetValueFromIntOrPercent(
			intstrutil.ValueOrDefault(spec.UpdateStrategy.RollingUpdate.MaxUnavailable, intstrutil.FromInt(1)),
			1,
			true,
		); err != nil {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("updateStrategy").Child("rollingUpdate").Child("maxUnavailable"),
				spec.UpdateStrategy.RollingUpdate.MaxUnavailable,
				fmt.Sprintf("failed getValueFromIntOrPercent for maxUnavailable: %v", err),
			))
		} else if maxUnavailable < 1 {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("updateStrategy").Child("rollingUpdate").Child("maxUnavailable"),
				spec.UpdateStrategy.RollingUpdate.MaxUnavailable,
				"getValueFromIntOrPercent for maxUnavailable should not be less than 1",
			))
		}
		if apps.ParallelPodManagement != spec.PodManagementPolicy &&
			(spec.UpdateStrategy.RollingUpdate.MaxUnavailable.Type != intstr.Int ||
				spec.UpdateStrategy.RollingUpdate.MaxUnavailable.IntVal != 1) {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("updateStrategy").Child("rollingUpdate").Child("maxUnavailable"),
				spec.UpdateStrategy.RollingUpdate.MaxUnavailable,
				"maxUnavailable can just work with Parallel PodManagementPolicyType",
			))
		}
	}
	return allErrs
}

// ValidateStatefulSet validates a StatefulSet.
func validateStatefulSet(statefulSet *appsv1beta1.StatefulSet) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMeta(&statefulSet.ObjectMeta, true, appsvalidation.ValidateStatefulSetName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateStatefulSetSpec(&statefulSet.Spec, field.NewPath("spec"))...)
	return allErrs
}

// ValidateStatefulSetUpdate tests if required fields in the StatefulSet are set.
func ValidateStatefulSetUpdate(statefulSet, oldStatefulSet *appsv1beta1.StatefulSet) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&statefulSet.ObjectMeta, &oldStatefulSet.ObjectMeta, field.NewPath("metadata"))

	restoreReplicas := statefulSet.Spec.Replicas
	statefulSet.Spec.Replicas = oldStatefulSet.Spec.Replicas

	restoreTemplate := statefulSet.Spec.Template
	statefulSet.Spec.Template = oldStatefulSet.Spec.Template

	restoreStrategy := statefulSet.Spec.UpdateStrategy
	statefulSet.Spec.UpdateStrategy = oldStatefulSet.Spec.UpdateStrategy

	restoreReserveOrdinals := statefulSet.Spec.ReserveOrdinals
	statefulSet.Spec.ReserveOrdinals = oldStatefulSet.Spec.ReserveOrdinals
	statefulSet.Spec.Lifecycle = oldStatefulSet.Spec.Lifecycle

	if !apiequality.Semantic.DeepEqual(statefulSet.Spec, oldStatefulSet.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "updates to statefulset spec for fields other than 'replicas', 'template', 'reserveOrdinals', 'lifecycle' and 'updateStrategy' are forbidden"))
	}
	statefulSet.Spec.Replicas = restoreReplicas
	statefulSet.Spec.Template = restoreTemplate
	statefulSet.Spec.UpdateStrategy = restoreStrategy
	statefulSet.Spec.ReserveOrdinals = restoreReserveOrdinals

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*statefulSet.Spec.Replicas), field.NewPath("spec", "replicas"))...)
	return allErrs
}

func validateTemplateInPlaceOnly(oldTemp, newTemp *v1.PodTemplateSpec) error {
	oldTempJSON, _ := json.Marshal(oldTemp.Spec)
	newTempJSON, _ := json.Marshal(newTemp.Spec)
	patches, err := jsonpatch.CreatePatch(oldTempJSON, newTempJSON)
	if err != nil {
		return fmt.Errorf("failed calculate patches between old/new template spec")
	}

	for _, p := range patches {
		if p.Operation != "replace" || !inPlaceUpdateTemplateSpecPatchRexp.MatchString(p.Path) {
			return fmt.Errorf("%s %s", p.Operation, p.Path)
		}
	}

	return nil
}
