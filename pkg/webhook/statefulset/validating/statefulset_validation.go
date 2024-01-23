package validating

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/appscode/jsonpatch"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/convertor"
)

var inPlaceUpdateTemplateSpecPatchRexp = regexp.MustCompile("/containers/([0-9]+)/image")

func validatePodManagementPolicy(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	switch spec.PodManagementPolicy {
	case "":
		allErrs = append(allErrs, field.Required(fldPath.Child("podManagementPolicy"), ""))
	case apps.OrderedReadyPodManagement, apps.ParallelPodManagement:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("podManagementPolicy"), spec.PodManagementPolicy, fmt.Sprintf("must be '%s' or '%s'", apps.OrderedReadyPodManagement, apps.ParallelPodManagement)))
	}
	return allErrs
}

func validateReserveOrdinals(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.ReserveOrdinals != nil {
		orders := sets.NewInt(spec.ReserveOrdinals...)
		if orders.Len() != len(spec.ReserveOrdinals) {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.ReserveOrdinals, "reserveOrdinals contains duplicated items"))
		}
		for _, i := range spec.ReserveOrdinals {
			if i < 0 {
				allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.ReserveOrdinals, fmt.Sprintf("reserveOrdinals contains %d which must be order >= 0", i)))
			}
		}
	}
	return allErrs
}

func validateScaleStrategy(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.ScaleStrategy != nil {
		if spec.ScaleStrategy.MaxUnavailable != nil {
			allErrs = append(allErrs, validateMaxUnavailableField(spec.ScaleStrategy.MaxUnavailable, spec, fldPath.Child("scaleStrategy").Child("maxUnavailable"))...)
		}
	}
	return allErrs
}

func ValidatePersistentVolumeClaimRetentionPolicyType(policy appsv1beta1.PersistentVolumeClaimRetentionPolicyType, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	switch policy {
	case appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType:
	case appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType:
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath, policy, []string{string(appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType), string(appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType)}))
	}
	return allErrs
}

func ValidatePersistentVolumeClaimRetentionPolicy(policy *appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if policy != nil {
		allErrs = append(allErrs, ValidatePersistentVolumeClaimRetentionPolicyType(policy.WhenDeleted, fldPath.Child("whenDeleted"))...)
		allErrs = append(allErrs, ValidatePersistentVolumeClaimRetentionPolicyType(policy.WhenScaled, fldPath.Child("whenScaled"))...)
	}
	return allErrs
}

func validateOnDeleteStatefulSetStrategyType(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.UpdateStrategy.RollingUpdate != nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				fldPath.Child("updateStrategy").Child("rollingUpdate"),
				spec.UpdateStrategy.RollingUpdate,
				fmt.Sprintf("only allowed for updateStrategy '%s'", apps.RollingUpdateStatefulSetStrategyType)))
	}
	return allErrs
}

func validateRollingUpdateStatefulSetStrategyTypeUnorderedUpdate(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

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
	return allErrs
}
func validateRollingUpdateStatefulSetStrategyType(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

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
		if maxUnavailable := spec.UpdateStrategy.RollingUpdate.MaxUnavailable; maxUnavailable == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy").Child("rollingUpdate").Child("maxUnavailable"), ""))
		} else {
			allErrs = append(allErrs, validateMaxUnavailableField(maxUnavailable, spec, fldPath.Child("updateStrategy").Child("rollingUpdate").Child("maxUnavailable"))...)
		}

		// validate the `PodUpdatePolicy` related fields
		allErrs = append(allErrs, validatePodUpdatePolicy(spec, fldPath)...)

		// validate the `spec.UpdateStrategy.RollingUpdate.UnorderedUpdate` related fields
		allErrs = append(allErrs, validateRollingUpdateStatefulSetStrategyTypeUnorderedUpdate(spec, fldPath)...)

	}
	return allErrs
}

func validateUpdateStrategyType(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	switch spec.UpdateStrategy.Type {
	case "":
		allErrs = append(allErrs, field.Required(fldPath.Child("updateStrategy"), ""))
	case apps.OnDeleteStatefulSetStrategyType:
		allErrs = append(allErrs, validateOnDeleteStatefulSetStrategyType(spec, fldPath)...)

	case apps.RollingUpdateStatefulSetStrategyType:
		allErrs = append(allErrs, validateRollingUpdateStatefulSetStrategyType(spec, fldPath)...)
	default:
		allErrs = append(allErrs,
			field.Invalid(fldPath.Child("updateStrategy"), spec.UpdateStrategy,
				fmt.Sprintf("must be '%s' or '%s'",
					apps.RollingUpdateStatefulSetStrategyType,
					apps.OnDeleteStatefulSetStrategyType)))
	}
	return allErrs
}

func validateSpecSelector(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.Selector == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector"), ""))
	} else {
		allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(spec.Selector, unversionedvalidation.LabelSelectorValidationOptions{}, fldPath.Child("selector"))...)
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
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForStatefulSet(coreTemplate, selector, fldPath.Child("template"), webhookutil.DefaultPodValidationOptions)...)
	}
	return allErrs
}

func validateRestartPolicy(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.Template.Spec.RestartPolicy != "" && spec.Template.Spec.RestartPolicy != v1.RestartPolicyAlways {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("template", "spec", "restartPolicy"), spec.Template.Spec.RestartPolicy, []string{string(v1.RestartPolicyAlways)}))
	}
	return allErrs
}

func validateActiveDeadlineSeconds(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.Template.Spec.ActiveDeadlineSeconds != nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("template", "spec", "activeDeadlineSeconds"), "activeDeadlineSeconds in StatefulSet is not Supported"))
	}
	return allErrs
}

// ValidateStatefulSetSpec tests if required fields in the StatefulSet spec are set.
func validateStatefulSetSpec(spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validatePodManagementPolicy(spec, fldPath)...)
	allErrs = append(allErrs, validateReserveOrdinals(spec, fldPath)...)
	allErrs = append(allErrs, validateScaleStrategy(spec, fldPath)...)
	allErrs = append(allErrs, validateUpdateStrategyType(spec, fldPath)...)
	allErrs = append(allErrs, ValidatePersistentVolumeClaimRetentionPolicy(spec.PersistentVolumeClaimRetentionPolicy, fldPath.Child("persistentVolumeClaimRetentionPolicy"))...)

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*spec.Replicas), fldPath.Child("replicas"))...)

	// validate `spec.Selector`
	allErrs = append(allErrs, validateSpecSelector(spec, fldPath)...)

	// validate `spec.Template.Spec.RestartPolicy`
	allErrs = append(allErrs, validateRestartPolicy(spec, fldPath)...)

	// validate `spec.Template.Spec.ActiveDeadlineSeconds`
	allErrs = append(allErrs, validateActiveDeadlineSeconds(spec, fldPath)...)

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

func validateMaxUnavailableField(maxUnavailable *intstr.IntOrString, spec *appsv1beta1.StatefulSetSpec, fldPath *field.Path) field.ErrorList {
	allErrs := appsvalidation.ValidatePositiveIntOrPercent(*maxUnavailable, fldPath)
	if maxUnavailable, err := intstr.GetValueFromIntOrPercent(intstr.ValueOrDefault(maxUnavailable, intstr.FromInt(1)), 1, true); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, maxUnavailable, fmt.Sprintf("getValueFromIntOrPercent error: %v", err)))
	} else if maxUnavailable < 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, maxUnavailable, "should not be less than 1"))
	}
	if apps.ParallelPodManagement != spec.PodManagementPolicy &&
		(maxUnavailable.Type != intstr.Int || maxUnavailable.IntVal != 1) {
		allErrs = append(allErrs, field.Invalid(fldPath, maxUnavailable, "can only work with Parallel PodManagementPolicyType"))
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

	restorePersistentVolumeClaimRetentionPolicy := statefulSet.Spec.PersistentVolumeClaimRetentionPolicy
	statefulSet.Spec.PersistentVolumeClaimRetentionPolicy = oldStatefulSet.Spec.PersistentVolumeClaimRetentionPolicy

	restoreScaleStrategy := statefulSet.Spec.ScaleStrategy
	statefulSet.Spec.ScaleStrategy = oldStatefulSet.Spec.ScaleStrategy

	restorePVCTemplate := statefulSet.Spec.VolumeClaimTemplates
	statefulSet.Spec.VolumeClaimTemplates = oldStatefulSet.Spec.VolumeClaimTemplates

	restoreReserveOrdinals := statefulSet.Spec.ReserveOrdinals
	statefulSet.Spec.ReserveOrdinals = oldStatefulSet.Spec.ReserveOrdinals
	statefulSet.Spec.Lifecycle = oldStatefulSet.Spec.Lifecycle
	statefulSet.Spec.RevisionHistoryLimit = oldStatefulSet.Spec.RevisionHistoryLimit

	if !apiequality.Semantic.DeepEqual(statefulSet.Spec, oldStatefulSet.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "updates to statefulset spec for fields other than 'replicas', 'template', 'reserveOrdinals', 'lifecycle', 'revisionHistoryLimit', 'persistentVolumeClaimRetentionPolicy', `volumeClaimTemplates` and 'updateStrategy' are forbidden"))
	}
	statefulSet.Spec.Replicas = restoreReplicas
	statefulSet.Spec.Template = restoreTemplate
	statefulSet.Spec.UpdateStrategy = restoreStrategy
	statefulSet.Spec.ScaleStrategy = restoreScaleStrategy
	statefulSet.Spec.ReserveOrdinals = restoreReserveOrdinals
	statefulSet.Spec.VolumeClaimTemplates = restorePVCTemplate
	statefulSet.Spec.PersistentVolumeClaimRetentionPolicy = restorePersistentVolumeClaimRetentionPolicy

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*statefulSet.Spec.Replicas), field.NewPath("spec", "replicas"))...)
	allErrs = append(allErrs, ValidatePersistentVolumeClaimRetentionPolicy(statefulSet.Spec.PersistentVolumeClaimRetentionPolicy, field.NewPath("spec", "persistentVolumeClaimRetentionPolicy"))...)
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
