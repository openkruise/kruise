package validating

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"

	"github.com/appscode/jsonpatch"
	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/types"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"
)

var inPlaceUpdateTemplateSpecPatchRexp = regexp.MustCompile("^/containers/([0-9]+)/image$")

func (h *CloneSetCreateUpdateHandler) validateCloneSet(cloneSet *appsv1alpha1.CloneSet) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMeta(&cloneSet.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, h.validateCloneSetSpec(&cloneSet.Spec, &cloneSet.ObjectMeta, field.NewPath("spec"))...)
	return allErrs
}

func (h *CloneSetCreateUpdateHandler) validateCloneSetSpec(spec *appsv1alpha1.CloneSetSpec, metadata *metav1.ObjectMeta, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*spec.Replicas), fldPath.Child("replicas"))...)
	if spec.Selector == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector"), ""))
	} else {
		allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(spec.Selector, fldPath.Child("selector"))...)
		if len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is invalid for cloneset"))
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
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("template", "spec", "activeDeadlineSeconds"), "activeDeadlineSeconds in cloneset is not Supported"))
	}

	allErrs = append(allErrs, h.validateScaleStrategy(&spec.ScaleStrategy, metadata, fldPath.Child("scaleStrategy"))...)
	allErrs = append(allErrs, h.validateUpdateStrategy(&spec.UpdateStrategy, fldPath.Child("updateStrategy"))...)

	return allErrs
}

func (h *CloneSetCreateUpdateHandler) validateScaleStrategy(strategy *appsv1alpha1.CloneSetScaleStrategy, metadata *metav1.ObjectMeta, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if list := util.CheckDuplicate(strategy.PodsToDelete); len(list) > 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("podsToDelete"), strategy.PodsToDelete, fmt.Sprintf("duplicated items %v", list)))
		return allErrs
	}

	for _, podName := range strategy.PodsToDelete {
		pod := &v1.Pod{}
		if err := h.Client.Get(context.TODO(), types.NamespacedName{Namespace: metadata.Namespace, Name: podName}, pod); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("podsToDelete"), podName, fmt.Sprintf("find pod %s failed: %v", podName, err)))
		} else if pod.DeletionTimestamp != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("podsToDelete"), podName, fmt.Sprintf("find pod %s already terminating", podName)))
		} else if owner := metav1.GetControllerOf(pod); owner.UID != metadata.UID {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("podsToDelete"), podName, fmt.Sprintf("find pod %s owner is not this CloneSet", podName)))
		}
	}

	return allErrs
}

func (h *CloneSetCreateUpdateHandler) validateUpdateStrategy(strategy *appsv1alpha1.CloneSetUpdateStrategy, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch strategy.Type {
	case appsv1alpha1.RecreateCloneSetUpdateStrategyType,
		appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
		appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("type"), strategy.Type, fmt.Sprintf("must be '%s', %s or '%s'",
			appsv1alpha1.RecreateCloneSetUpdateStrategyType,
			appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
			appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType)))
	}

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*strategy.Partition), fldPath.Child("partition"))...)

	if err := strategy.PriorityStrategy.FieldsValidation(); err != nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("priorityStrategy"), err.Error()))
	}

	if err := strategy.ScatterStrategy.FieldsValidation(); err != nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("scatterStrategy"), err.Error()))
	}

	if maxUnavailable, err := intstrutil.GetValueFromIntOrPercent(intstrutil.ValueOrDefault(strategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), 1, true); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxUnavailable"), strategy.MaxUnavailable,
			fmt.Sprintf("failed getValueFromIntOrPercent for maxUnavailable: %v", err)))
	} else if maxUnavailable < 1 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("maxUnavailable"), strategy.MaxUnavailable,
			"getValueFromIntOrPercent for maxUnavailable should not be less than 1"))
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

func (h *CloneSetCreateUpdateHandler) validateCloneSetUpdate(cloneSet, oldCloneSet *appsv1alpha1.CloneSet) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&cloneSet.ObjectMeta, &oldCloneSet.ObjectMeta, field.NewPath("metadata"))

	clone := cloneSet.DeepCopy()
	clone.Spec.Replicas = oldCloneSet.Spec.Replicas
	clone.Spec.Template = oldCloneSet.Spec.Template
	clone.Spec.ScaleStrategy = oldCloneSet.Spec.ScaleStrategy
	clone.Spec.UpdateStrategy = oldCloneSet.Spec.UpdateStrategy
	clone.Spec.MinReadySeconds = oldCloneSet.Spec.MinReadySeconds
	if !reflect.DeepEqual(clone.Spec, oldCloneSet.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "updates to cloneset spec for fields other than 'replicas', 'template', 'scaleStrategy', and 'updateStrategy' are forbidden"))
	}

	if cloneSet.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
		if err := validateTemplateInPlaceOnly(&oldCloneSet.Spec.Template, &cloneSet.Spec.Template); err != nil {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("template"),
				"forbid to update fields in template other than image, when updateStrategy type is InPlaceOnly"))
		}
	}

	allErrs = append(allErrs, h.validateCloneSet(cloneSet)...)
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
