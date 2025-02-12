package validating

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/appscode/jsonpatch"
	apiutil "github.com/openkruise/kruise/pkg/util/api"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/pvc"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/convertor"
)

var inPlaceUpdateTemplateSpecPatchRexp = regexp.MustCompile("/containers/([0-9]+)/image")
var reserveOrdinalRangeRexp = regexp.MustCompile(`^\d+-\d+$`)

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
	if len(spec.ReserveOrdinals) > 0 {
		for i, elem := range spec.ReserveOrdinals {
			if elem.Type == intstr.String {
				if !reserveOrdinalRangeRexp.MatchString(elem.StrVal) {
					allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.ReserveOrdinals,
						fmt.Sprintf("%d th reserve ordinal is not a valid range: %s", i, elem.StrVal)))
				}
				if _, _, err := apiutil.ParseRange(elem.StrVal); err != nil {
					allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.ReserveOrdinals,
						fmt.Sprintf("%d th reserve ordinal is invalid: %s, err = %s", i, elem.StrVal, err)))
				}
			}
			if elem.Type == intstr.Int && elem.IntVal < 0 {
				allErrs = append(allErrs, field.Invalid(fldPath.Root(), spec.ReserveOrdinals,
					fmt.Sprintf("%d th reserve ordinal is negative: %d", i, elem.IntVal)))
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
	statefulSet.Spec.VolumeClaimUpdateStrategy = oldStatefulSet.Spec.VolumeClaimUpdateStrategy

	restoreReserveOrdinals := statefulSet.Spec.ReserveOrdinals
	statefulSet.Spec.ReserveOrdinals = oldStatefulSet.Spec.ReserveOrdinals
	statefulSet.Spec.Lifecycle = oldStatefulSet.Spec.Lifecycle
	statefulSet.Spec.RevisionHistoryLimit = oldStatefulSet.Spec.RevisionHistoryLimit
	statefulSet.Spec.Ordinals = oldStatefulSet.Spec.Ordinals

	if !apiequality.Semantic.DeepEqual(statefulSet.Spec, oldStatefulSet.Spec) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec"), "updates to statefulset spec for fields other than 'replicas', 'ordinals', 'template', 'reserveOrdinals', 'lifecycle', 'revisionHistoryLimit', 'persistentVolumeClaimRetentionPolicy', `volumeClaimTemplates`, `VolumeClaimUpdateStrategy` and 'updateStrategy' are forbidden"))
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

// ValidateVolumeClaimTemplateUpdate tests if only size expand when sc allow expansion.
func ValidateVolumeClaimTemplateUpdate(c client.Client, sts, oldSts *appsv1beta1.StatefulSet) field.ErrorList {
	if sts.Spec.VolumeClaimUpdateStrategy.Type == "" ||
		sts.Spec.VolumeClaimUpdateStrategy.Type == appsv1beta1.OnPVCDeleteVolumeClaimUpdateStrategyType {
		return nil
	}
	if len(sts.Spec.VolumeClaimTemplates) != len(oldSts.Spec.VolumeClaimTemplates) {
		return field.ErrorList{field.Invalid(field.NewPath("spec", "volumeClaimTemplates"), sts.Spec.VolumeClaimTemplates, "volumeClaimTemplate can not be added or deleted when OnRollingUpdate")}
	}

	name2Template := make(map[string]*v1.PersistentVolumeClaim)
	for i := range oldSts.Spec.VolumeClaimTemplates {
		name2Template[oldSts.Spec.VolumeClaimTemplates[i].Name] = &oldSts.Spec.VolumeClaimTemplates[i]
	}

	var err error
	for _, template := range sts.Spec.VolumeClaimTemplates {
		templateIdStr := fmt.Sprintf("volumeClaimTemplates[%v]", template.Name)
		oldTemplate, exist := name2Template[template.Name]
		if !exist {
			return field.ErrorList{field.Forbidden(field.NewPath("spec", templateIdStr, "name"), "volumeClaimTemplate name can not be modified")}
		}

		matched, resizeOnly := pvc.CompareWithCheckFn(oldTemplate, &template, isPVCResize)
		if matched {
			continue
		}
		if !resizeOnly {
			return field.ErrorList{field.Invalid(field.NewPath("spec", templateIdStr), template, "volumeClaimTemplate can not be modified when OnRollingUpdate")}
		}
		// check if sc allow volume expand
		var sc *storagev1.StorageClass
		scName := template.Spec.StorageClassName
		if scName == nil {
			// nil scName means using default storage class
			sc, err = GetDefaultStorageClass(c)
			if err != nil {
				return field.ErrorList{field.Invalid(field.NewPath("spec", templateIdStr, "spec", "storageClassName"), "nil", "can not list storage class")}
			}
			//	if there is no default sc, skip check
			if sc == nil {
				continue
			}
		} else {
			sc = &storagev1.StorageClass{}
			err = c.Get(context.TODO(), client.ObjectKey{Name: *scName}, sc)
			if err != nil || sc == nil {
				return field.ErrorList{field.Invalid(field.NewPath("spec", templateIdStr, "spec", "storageClassName"), *scName, "can not get sc")}
			}
		}
		if sc.AllowVolumeExpansion != nil && !*sc.AllowVolumeExpansion {
			return field.ErrorList{field.Forbidden(field.NewPath("spec", templateIdStr, "spec", "resources", "requests", "storage"),
				fmt.Sprintf("storage class %v does not support volume expansion", sc.Name))}
		}
	}
	return nil
}

const isDefaultStorageClassAnnotation = "storageclass.kubernetes.io/is-default-class"

func GetDefaultStorageClass(c client.Client) (*storagev1.StorageClass, error) {
	// refer to https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1
	// choose the only one or the newest one
	scs := &storagev1.StorageClassList{}
	err := c.List(context.TODO(), scs, &client.ListOptions{})
	if err != nil {
		return nil, err
	}
	var defaultSC *storagev1.StorageClass
	for i, sc := range scs.Items {
		if sc.Annotations[isDefaultStorageClassAnnotation] == "true" {
			if defaultSC == nil || defaultSC.CreationTimestamp.Before(&sc.CreationTimestamp) {
				defaultSC = &scs.Items[i]
			}
		}
	}
	return defaultSC, nil
}

func isPVCResize(claim, template *v1.PersistentVolumeClaim) bool {
	if claim.Spec.Resources.Requests.Storage().Cmp(*template.Spec.Resources.Requests.Storage()) != 0 ||
		claim.Spec.Resources.Limits.Storage().Cmp(*template.Spec.Resources.Limits.Storage()) != 0 {
		return true
	}
	return false
}
