package validating

import (
	"fmt"

	appsv1alpha1 "github.com/kruiseio/kruise/pkg/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"
)

// ValidateStatefulSetSpec tests if required fields in the StatefulSet spec are set.
func validateStatefulSetSpec(spec *appsv1alpha1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch spec.PodManagementPolicy {
	case "":
		allErrs = append(allErrs, field.Required(fldPath.Child("podManagementPolicy"), ""))
	case apps.OrderedReadyPodManagement, apps.ParallelPodManagement:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("podManagementPolicy"), spec.PodManagementPolicy, fmt.Sprintf("must be '%s' or '%s'", apps.OrderedReadyPodManagement, apps.ParallelPodManagement)))
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

			if spec.UpdateStrategy.RollingUpdate.MaxUnavailable == nil {
				allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy").
					Child("rollingUpdate").Child("maxUnavailable"), ""))
			} else {
				allErrs = append(allErrs,
					appsvalidation.ValidatePositiveIntOrPercent(
						*spec.UpdateStrategy.RollingUpdate.MaxUnavailable,
						fldPath.Child("updateStrategy").Child("rollingUpdate").Child("maxUnavailable"))...)
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

			switch spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy {
			case "":
				allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy").Child("podUpdatePolicy"), ""))
			case appsv1alpha1.RecreatePodUpdateStrategyType, appsv1alpha1.InPlaceIfPossiblePodUpdateStrategyType:
			default:
				allErrs = append(allErrs,
					field.Invalid(fldPath.Child("updateStrategy").Child("podUpdatePolicy"),
						spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy,
						fmt.Sprintf("must be '%s' or '%s'",
							appsv1alpha1.RecreatePodUpdateStrategyType,
							appsv1alpha1.InPlaceIfPossiblePodUpdateStrategyType)))
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
		coreTemplate, err := convertPodTemplateSpec(&spec.Template)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.Template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
			return allErrs
		}
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForStatefulSet(coreTemplate, selector, fldPath.Child("template"))...)
	}

	if spec.Template.Spec.RestartPolicy != v1.RestartPolicyAlways {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("template", "spec", "restartPolicy"), spec.Template.Spec.RestartPolicy, []string{string(v1.RestartPolicyAlways)}))
	}
	if spec.Template.Spec.ActiveDeadlineSeconds != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("template", "spec", "activeDeadlineSeconds"), "activeDeadlineSeconds in StatefulSet is not Supported"))
	}

	return allErrs
}

// ValidateStatefulSet validates a StatefulSet.
func validateStatefulSet(statefulSet *appsv1alpha1.StatefulSet) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMeta(&statefulSet.ObjectMeta, true, appsvalidation.ValidateStatefulSetName, field.NewPath("metadata"))
	allErrs = append(allErrs, validateStatefulSetSpec(&statefulSet.Spec, field.NewPath("spec"))...)
	return allErrs
}

// validateStatefulSetUpdate tests if required fields in the StatefulSet are set.
func ValidateStatefulSetUpdate(statefulSet, oldStatefulSet *appsv1alpha1.StatefulSet) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&statefulSet.ObjectMeta, &oldStatefulSet.ObjectMeta, field.NewPath("metadata"))

	restoreReplicas := statefulSet.Spec.Replicas
	statefulSet.Spec.Replicas = oldStatefulSet.Spec.Replicas

	restoreTemplate := statefulSet.Spec.Template
	statefulSet.Spec.Template = oldStatefulSet.Spec.Template

	restoreStrategy := statefulSet.Spec.UpdateStrategy
	statefulSet.Spec.UpdateStrategy = oldStatefulSet.Spec.UpdateStrategy

	if !apiequality.Semantic.DeepEqual(statefulSet.Spec, oldStatefulSet.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "updates to statefulset spec for fields other than 'replicas', 'template', and 'updateStrategy' are forbidden"))
	}
	statefulSet.Spec.Replicas = restoreReplicas
	statefulSet.Spec.Template = restoreTemplate
	statefulSet.Spec.UpdateStrategy = restoreStrategy

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*statefulSet.Spec.Replicas), field.NewPath("spec", "replicas"))...)
	return allErrs
}

// ValidateStatefulSetStatus validates a StatefulSetStatus.
func validateStatefulSetStatus(status *appsv1alpha1.StatefulSetStatus, fieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(status.Replicas), fieldPath.Child("replicas"))...)
	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(status.ReadyReplicas), fieldPath.Child("readyReplicas"))...)
	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(status.CurrentReplicas), fieldPath.Child("currentReplicas"))...)
	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(status.UpdatedReplicas), fieldPath.Child("updatedReplicas"))...)
	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(status.ObservedGeneration), fieldPath.Child("observedGeneration"))...)
	if status.CollisionCount != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*status.CollisionCount), fieldPath.Child("collisionCount"))...)
	}

	msg := "cannot be greater than status.replicas"
	if status.ReadyReplicas > status.Replicas {
		allErrs = append(allErrs, field.Invalid(fieldPath.Child("readyReplicas"), status.ReadyReplicas, msg))
	}
	if status.CurrentReplicas > status.Replicas {
		allErrs = append(allErrs, field.Invalid(fieldPath.Child("currentReplicas"), status.CurrentReplicas, msg))
	}
	if status.UpdatedReplicas > status.Replicas {
		allErrs = append(allErrs, field.Invalid(fieldPath.Child("updatedReplicas"), status.UpdatedReplicas, msg))
	}

	return allErrs
}

// ValidateStatefulSetStatusUpdate tests if required fields in the StatefulSet are set.
func validateStatefulSetStatusUpdate(statefulSet, oldStatefulSet *appsv1alpha1.StatefulSet) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateStatefulSetStatus(&statefulSet.Status, field.NewPath("status"))...)
	allErrs = append(allErrs, apivalidation.ValidateObjectMetaUpdate(&statefulSet.ObjectMeta, &oldStatefulSet.ObjectMeta, field.NewPath("metadata"))...)
	// TODO: Validate status.
	if apivalidation.IsDecremented(statefulSet.Status.CollisionCount, oldStatefulSet.Status.CollisionCount) {
		value := int32(0)
		if statefulSet.Status.CollisionCount != nil {
			value = *statefulSet.Status.CollisionCount
		}
		allErrs = append(allErrs, field.Invalid(field.NewPath("status").Child("collisionCount"), value, "cannot be decremented"))
	}
	return allErrs
}

func convertPodTemplateSpec(template *v1.PodTemplateSpec) (*core.PodTemplateSpec, error) {
	coreTemplate := &core.PodTemplateSpec{}
	if err := corev1.Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec(template.DeepCopy(), coreTemplate, nil); err != nil {
		return nil, err
	}
	return coreTemplate, nil
}
