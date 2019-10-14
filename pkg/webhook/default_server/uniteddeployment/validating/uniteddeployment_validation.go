package validating

import (
	"fmt"
	"regexp"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	apivalidation "k8s.io/kubernetes/pkg/apis/core/validation"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	udctrl "github.com/openkruise/kruise/pkg/controller/uniteddeployment"
)

var inPlaceUpdateTemplateSpecPatchRexp = regexp.MustCompile("/containers/([0-9]+)/image")

// ValidateUnitedDeploymentSpec tests if required fields in the UnitedDeployment spec are set.
func validateUnitedDeploymentSpec(spec *appsv1alpha1.UnitedDeploymentSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*spec.Replicas), fldPath.Child("replicas"))...)
	if spec.Selector == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("selector"), ""))
	} else {
		allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(spec.Selector, fldPath.Child("selector"))...)
		if len(spec.Selector.MatchLabels)+len(spec.Selector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, "empty selector is invalid for statefulset"))
		}
	}

	if spec.Template.StatefulSetTemplate == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("template", "statefulSetTemplate"), ""))
	}

	selector, err := metav1.LabelSelectorAsSelector(spec.Selector)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("selector"), spec.Selector, ""))
	} else {
		allErrs = append(allErrs, validateSubsetTemplate(&spec.Template, &selector, fldPath.Child("template"))...)
	}

	var sumReplicas int32
	var expectedReplicas int32 = 1
	if spec.Replicas != nil {
		expectedReplicas = *spec.Replicas
	}
	subSetNames := sets.String{}
	count := 0
	for _, subset := range spec.Topology.Subsets {
		if len(subset.Name) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("topology", "subset", "name"), ""))
		}

		if subSetNames.Has(subset.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subset", "name"), subset.Name, fmt.Sprintf("duplicated subset name %s", subset.Name)))
		}

		subSetNames.Insert(subset.Name)

		if errs := apimachineryvalidation.NameIsDNSLabel(subset.Name, false); len(errs) > 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subset", "name"), subset.Name, fmt.Sprintf("invalid subset name %s", strings.Join(errs, ", "))))
		}

		if subset.Replicas == nil {
			continue
		}

		replicas, err := udctrl.ParseSubsetReplicas(expectedReplicas, *subset.Replicas)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subset", "replicas"), subset.Replicas, fmt.Sprintf("invalid replicas %s", subset.Replicas.String())))
		} else {
			sumReplicas += replicas
			count++
		}
	}

	// sum of subset replicas may be less than uniteddployment replicas
	if sumReplicas > expectedReplicas {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subset"), sumReplicas, fmt.Sprintf("sum of indicated subset replicas %d should not be greater than UnitedDeployment replicas %d", sumReplicas, expectedReplicas)))
	}

	if count > 0 && count == len(spec.Topology.Subsets) && sumReplicas != expectedReplicas {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subset"), sumReplicas, fmt.Sprintf("if replicas of all subsets are provided, the sum of indicated subset replicas %d should equal UnitedDeployment replicas %d", sumReplicas, expectedReplicas)))
	}

	for subset := range spec.Strategy.Partitions {
		if !subSetNames.Has(subset) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("strategy", "partitions"), spec.Strategy.Partitions, fmt.Sprintf("subset %s does not exist", subset)))
		}
	}

	return allErrs
}

// validateUnitedDeployment validates a UnitedDeployment.
func validateUnitedDeployment(unitedDeployment *appsv1alpha1.UnitedDeployment) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMeta(&unitedDeployment.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, validateUnitedDeploymentSpec(&unitedDeployment.Spec, field.NewPath("spec"))...)
	return allErrs
}

// ValidateUnitedDeploymentUpdate tests if required fields in the UnitedDeployment are set.
func ValidateUnitedDeploymentUpdate(unitedDeployment, oldUnitedDeployment *appsv1alpha1.UnitedDeployment) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&unitedDeployment.ObjectMeta, &oldUnitedDeployment.ObjectMeta, field.NewPath("metadata"))
	allErrs = append(allErrs, validateUnitedDeploymentSpecUpdate(&unitedDeployment.Spec, &oldUnitedDeployment.Spec, field.NewPath("spec"))...)
	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*unitedDeployment.Spec.Replicas), field.NewPath("spec", "replicas"))...)
	return allErrs
}

func convertPodTemplateSpec(template *v1.PodTemplateSpec) (*core.PodTemplateSpec, error) {
	coreTemplate := &core.PodTemplateSpec{}
	if err := corev1.Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec(template.DeepCopy(), coreTemplate, nil); err != nil {
		return nil, err
	}
	return coreTemplate, nil
}

func validateUnitedDeploymentSpecUpdate(spec, oldSpec *appsv1alpha1.UnitedDeploymentSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateSubsetTemplateUpdate(&spec.Template, &oldSpec.Template, fldPath.Child("template"))...)

	return allErrs
}

func validateSubsetTemplateUpdate(template, oldTemplate *appsv1alpha1.SubsetTemplate, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if template.StatefulSetTemplate != nil && oldTemplate.StatefulSetTemplate != nil {
		allErrs = append(allErrs, validateStatefulSetUpdate(template.StatefulSetTemplate, oldTemplate.StatefulSetTemplate, fldPath.Child("statefulSetTemplate"))...)
	}

	return allErrs
}

func validateSubsetTemplate(template *appsv1alpha1.SubsetTemplate, selector *labels.Selector, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if template.StatefulSetTemplate != nil {
		allErrs = append(allErrs, validateStatefulSet(template.StatefulSetTemplate, fldPath.Child("statefulSetTemplate"))...)
		template := template.StatefulSetTemplate.Spec.Template
		coreTemplate, err := convertPodTemplateSpec(&template)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
			return allErrs
		}
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForStatefulSet(coreTemplate, *selector, fldPath.Child("template", "statefulSet"))...)
	}

	return allErrs
}

func validateStatefulSet(statefulSet *appsv1alpha1.StatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if statefulSet.Spec.Replicas != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "replicas"), *statefulSet.Spec.Replicas, "replicas in statefulSetTemplate will not be used"))
	}
	if statefulSet.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType &&
		statefulSet.Spec.UpdateStrategy.RollingUpdate != nil &&
		statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "updateStrategy", "rollingUpdate", "partition"), *statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition, "partition in statefulSetTemplate will not be used"))
	}
	return allErrs
}

func validateStatefulSetUpdate(statefulSet, oldStatefulSet *appsv1alpha1.StatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := apivalidation.ValidateObjectMetaUpdate(&statefulSet.ObjectMeta, &oldStatefulSet.ObjectMeta, fldPath.Child("metadata"))

	restoreReplicas := statefulSet.Spec.Replicas
	statefulSet.Spec.Replicas = oldStatefulSet.Spec.Replicas

	restoreTemplate := statefulSet.Spec.Template
	statefulSet.Spec.Template = oldStatefulSet.Spec.Template

	restoreStrategy := statefulSet.Spec.UpdateStrategy
	statefulSet.Spec.UpdateStrategy = oldStatefulSet.Spec.UpdateStrategy

	if !apiequality.Semantic.DeepEqual(statefulSet.Spec, oldStatefulSet.Spec) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("spec"), "updates to statefulsetTemplate spec for fields other than 'template', and 'updateStrategy' are forbidden"))
	}
	statefulSet.Spec.Replicas = restoreReplicas
	statefulSet.Spec.Template = restoreTemplate
	statefulSet.Spec.UpdateStrategy = restoreStrategy

	allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*statefulSet.Spec.Replicas), fldPath.Child("spec", "replicas"))...)
	return allErrs
}
