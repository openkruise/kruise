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
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
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
	"k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	udctrl "github.com/openkruise/kruise/pkg/controller/uniteddeployment"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/convertor"
)

// validateUnitedDeploymentSpec tests if required fields in the UnitedDeployment spec are set.
func validateUnitedDeploymentSpec(spec *appsv1alpha1.UnitedDeploymentSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if spec.Replicas != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*spec.Replicas), fldPath.Child("replicas"))...)
	}
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
		allErrs = append(allErrs, validateSubsetTemplate(&spec.Template, selector, fldPath.Child("template"))...)
	}

	allErrs = append(allErrs, validateSubsetReplicas(spec.Replicas, spec.Topology.Subsets, fldPath.Child("topology", "subsets"))...)

	subSetNames := sets.String{}
	for i, subset := range spec.Topology.Subsets {
		if len(subset.Name) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("topology", "subsets").Index(i).Child("name"), ""))
		}

		if subSetNames.Has(subset.Name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subsets").Index(i).Child("name"), subset.Name, fmt.Sprintf("duplicated subset name %s", subset.Name)))
		}

		subSetNames.Insert(subset.Name)

		if errs := apimachineryvalidation.NameIsDNSLabel(subset.Name, false); len(errs) > 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subsets").Index(i).Child("name"), subset.Name, fmt.Sprintf("invalid subset name %s", strings.Join(errs, ", "))))
		}

		coreNodeSelectorTerm := &core.NodeSelectorTerm{}
		if err := corev1.Convert_v1_NodeSelectorTerm_To_core_NodeSelectorTerm(subset.NodeSelectorTerm.DeepCopy(), coreNodeSelectorTerm, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subsets").Index(i).Child("nodeSelectorTerm"), subset.NodeSelectorTerm, fmt.Sprintf("Convert_v1_NodeSelectorTerm_To_core_NodeSelectorTerm failed: %v", err)))
		} else {
			allErrs = append(allErrs, apivalidation.ValidateNodeSelectorTerm(*coreNodeSelectorTerm, fldPath.Child("topology", "subsets").Index(i).Child("nodeSelectorTerm"))...)
		}

		if subset.Tolerations != nil {
			var coreTolerations []core.Toleration
			for i, toleration := range subset.Tolerations {
				coreToleration := &core.Toleration{}
				if err := corev1.Convert_v1_Toleration_To_core_Toleration(&toleration, coreToleration, nil); err != nil {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("topology", "subsets").Index(i).Child("tolerations"), subset.Tolerations, fmt.Sprintf("Convert_v1_Toleration_To_core_Toleration failed: %v", err)))
				} else {
					coreTolerations = append(coreTolerations, *coreToleration)
				}
			}
			allErrs = append(allErrs, apivalidation.ValidateTolerations(coreTolerations, fldPath.Child("topology", "subsets").Index(i).Child("tolerations"))...)
		}

		if subset.Replicas == nil {
			continue
		}
	}

	if spec.UpdateStrategy.ManualUpdate != nil {
		for subset := range spec.UpdateStrategy.ManualUpdate.Partitions {
			if !subSetNames.Has(subset) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("updateStrategy", "partitions"), spec.UpdateStrategy.ManualUpdate.Partitions, fmt.Sprintf("subset %s does not exist", subset)))
			}
		}
	}

	return allErrs
}

func validateSubsetReplicas(expectedReplicas *int32, subsets []appsv1alpha1.Subset, fldPath *field.Path) field.ErrorList {
	var (
		sumReplicas    = int64(0)
		sumMinReplicas = int64(0)
		sumMaxReplicas = int64(0)

		countReplicas    = 0
		countMaxReplicas = 0

		hasReplicasSettings = false
		hasCapacitySettings = false

		err     error
		errList field.ErrorList
	)

	if expectedReplicas == nil {
		expectedReplicas = pointer.Int32(-1)
	}

	for i, subset := range subsets {
		replicas := int32(0)
		if subset.Replicas != nil {
			countReplicas++
			hasReplicasSettings = true
			replicas, err = udctrl.ParseSubsetReplicas(*expectedReplicas, *subset.Replicas)
			if err != nil {
				errList = append(errList, field.Invalid(fldPath.Index(i).Child("replicas"), subset.Replicas, err.Error()))
			}
		}
		sumReplicas += int64(replicas)

		minReplicas := int32(0)
		if subset.MinReplicas != nil {
			hasCapacitySettings = true
			minReplicas, err = udctrl.ParseSubsetReplicas(*expectedReplicas, *subset.MinReplicas)
			if err != nil {
				errList = append(errList, field.Invalid(fldPath.Index(i).Child("minReplicas"), subset.MaxReplicas, err.Error()))
			}
		}
		sumMinReplicas += int64(minReplicas)

		maxReplicas := int32(1000000)
		if subset.MaxReplicas != nil {
			countMaxReplicas++
			hasCapacitySettings = true
			maxReplicas, err = udctrl.ParseSubsetReplicas(*expectedReplicas, *subset.MaxReplicas)
			if err != nil {
				errList = append(errList, field.Invalid(fldPath.Index(i).Child("minReplicas"), subset.MaxReplicas, err.Error()))
			}
		}
		sumMaxReplicas += int64(maxReplicas)

		if minReplicas > maxReplicas {
			errList = append(errList, field.Invalid(fldPath.Index(i).Child("minReplicas"), subset.MaxReplicas,
				fmt.Sprintf("subset[%d].minReplicas must be more than or equal to maxReplicas", i)))
		}
	}

	if hasReplicasSettings && hasCapacitySettings {
		errList = append(errList, field.Invalid(fldPath, subsets, "subset.Replicas and subset.MinReplicas/subset.MaxReplicas are mutually exclusive in a UnitedDeployment"))
		return errList
	}

	if hasCapacitySettings {
		if *expectedReplicas == -1 {
			errList = append(errList, field.Invalid(fldPath, expectedReplicas, "spec.replicas must be not empty if you set subset.minReplicas/maxReplicas"))
		}
		if countMaxReplicas >= len(subsets) {
			errList = append(errList, field.Invalid(fldPath, countMaxReplicas, "at least one subset.maxReplicas must be empty"))
		}
		if sumMinReplicas > sumMaxReplicas {
			errList = append(errList, field.Invalid(fldPath, sumMinReplicas, "sum of indicated subset.minReplicas should not be greater than sum of indicated subset.maxReplicas"))
		}
	} else {
		if *expectedReplicas != -1 {
			// sum of subset replicas may be less than uniteddployment replicas
			if sumReplicas > int64(*expectedReplicas) {
				errList = append(errList, field.Invalid(fldPath, sumReplicas, fmt.Sprintf("sum of indicated subset replicas %d should not be greater than UnitedDeployment replicas %d", sumReplicas, expectedReplicas)))
			}
			if countReplicas > 0 && countReplicas == len(subsets) && sumReplicas != int64(*expectedReplicas) {
				errList = append(errList, field.Invalid(fldPath, sumReplicas, fmt.Sprintf("if replicas of all subsets are provided, the sum of indicated subset replicas %d should equal UnitedDeployment replicas %d", sumReplicas, expectedReplicas)))
			}
		} else if countReplicas != len(subsets) {
			// validate all of subsets replicas are not nil
			errList = append(errList, field.Invalid(fldPath, sumReplicas, "if UnitedDeployment replicas is not provided, replicas of all subsets should be provided"))
		}
	}
	return errList
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
	if unitedDeployment.Spec.Replicas != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*unitedDeployment.Spec.Replicas), field.NewPath("spec", "replicas"))...)
	}
	return allErrs
}

func validateUnitedDeploymentSpecUpdate(spec, oldSpec *appsv1alpha1.UnitedDeploymentSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateSubsetTemplateUpdate(&spec.Template, &oldSpec.Template, fldPath.Child("template"))...)
	allErrs = append(allErrs, validateUnitedDeploymentTopology(&spec.Topology, &oldSpec.Topology, fldPath.Child("topology"))...)

	return allErrs
}

func validateUnitedDeploymentTopology(topology, oldTopology *appsv1alpha1.Topology, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if topology == nil || oldTopology == nil {
		return allErrs
	}

	oldSubsets := map[string]*appsv1alpha1.Subset{}
	for i, subset := range oldTopology.Subsets {
		oldSubsets[subset.Name] = &oldTopology.Subsets[i]
	}

	for i, subset := range topology.Subsets {
		if oldSubset, exist := oldSubsets[subset.Name]; exist {
			if !apiequality.Semantic.DeepEqual(oldSubset.NodeSelectorTerm, subset.NodeSelectorTerm) {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("subsets").Index(i).Child("nodeSelectorTerm"), "may not be changed in an update"))
			}
			if !apiequality.Semantic.DeepEqual(oldSubset.Tolerations, subset.Tolerations) {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("subsets").Index(i).Child("tolerations"), "may not be changed in an update"))
			}
		}
	}

	for i, subset := range topology.Subsets {
		if oldSubset, exist := oldSubsets[subset.Name]; exist {
			if !apiequality.Semantic.DeepEqual(oldSubset.Tolerations, subset.Tolerations) {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("subsets").Index(i).Child("tolerations"), "may not be changed in an update"))
			}
		}
	}

	return allErrs
}

func validateSubsetTemplateUpdate(template, oldTemplate *appsv1alpha1.SubsetTemplate, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if template.StatefulSetTemplate != nil && oldTemplate.StatefulSetTemplate != nil {
		allErrs = append(allErrs, validateStatefulSetUpdate(template.StatefulSetTemplate, oldTemplate.StatefulSetTemplate, fldPath.Child("statefulSetTemplate"))...)
	} else if template.AdvancedStatefulSetTemplate != nil && oldTemplate.AdvancedStatefulSetTemplate != nil {
		allErrs = append(allErrs, validateAdvancedStatefulSetUpdate(template.AdvancedStatefulSetTemplate, oldTemplate.AdvancedStatefulSetTemplate, fldPath.Child("advancedStatefulSetTemplate"))...)
	} else if template.DeploymentTemplate != nil && oldTemplate.DeploymentTemplate != nil {
		allErrs = append(allErrs, validateDeploymentUpdate(template.DeploymentTemplate, oldTemplate.DeploymentTemplate, fldPath.Child("deploymentTemplate"))...)
	}

	return allErrs
}

func validateSubsetTemplate(template *appsv1alpha1.SubsetTemplate, selector labels.Selector, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	var templateCount int
	if template.StatefulSetTemplate != nil {
		templateCount++
	}
	if template.AdvancedStatefulSetTemplate != nil {
		templateCount++
	}
	if template.CloneSetTemplate != nil {
		templateCount++
	}
	if template.DeploymentTemplate != nil {
		templateCount++
	}
	if templateCount < 1 {
		allErrs = append(allErrs, field.Required(fldPath, "should provide one of statefulSetTemplate, advancedStatefulSetTemplate, cloneSetTemplate, or deploymentTemplate"))
	} else if templateCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, template, "should provide only one of statefulSetTemplate, advancedStatefulSetTemplate, cloneSetTemplate, or deploymentTemplate"))
	}

	if template.StatefulSetTemplate != nil {
		labels := labels.Set(template.StatefulSetTemplate.Labels)
		if !selector.Matches(labels) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("statefulSetTemplate", "metadata", "labels"), template.StatefulSetTemplate.Labels, "`selector` does not match template `labels`"))
		}
		allErrs = append(allErrs, validateStatefulSet(template.StatefulSetTemplate, fldPath.Child("statefulSetTemplate"))...)
		template := template.StatefulSetTemplate.Spec.Template
		coreTemplate, err := convertor.ConvertPodTemplateSpec(&template)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
			return allErrs
		}
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForStatefulSet(coreTemplate, selector, fldPath.Child("statefulSetTemplate", "spec", "template"), webhookutil.DefaultPodValidationOptions)...)
	} else if template.AdvancedStatefulSetTemplate != nil {
		labels := labels.Set(template.AdvancedStatefulSetTemplate.Labels)
		if !selector.Matches(labels) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("statefulSetTemplate", "metadata", "labels"), template.AdvancedStatefulSetTemplate.Labels, "`selector` does not match template `labels`"))
		}
		allErrs = append(allErrs, validateAdvancedStatefulSet(template.AdvancedStatefulSetTemplate, fldPath.Child("advancedStatefulSetTemplate"))...)
		template := template.AdvancedStatefulSetTemplate.Spec.Template
		coreTemplate, err := convertor.ConvertPodTemplateSpec(&template)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
			return allErrs
		}
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForStatefulSet(coreTemplate, selector, fldPath.Child("advancedStatefulSetTemplate", "spec", "template"), webhookutil.DefaultPodValidationOptions)...)
	} else if template.DeploymentTemplate != nil {
		labels := labels.Set(template.DeploymentTemplate.Labels)
		if !selector.Matches(labels) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("deploymentTemplate", "metadata", "labels"), template.DeploymentTemplate.Labels, "`selector` does not match template `labels`"))
		}
		allErrs = append(allErrs, validateDeployment(template.DeploymentTemplate, fldPath.Child("deploymentTemplate"))...)
		template := template.DeploymentTemplate.Spec.Template
		coreTemplate, err := convertor.ConvertPodTemplateSpec(&template)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), template, fmt.Sprintf("Convert_v1_PodTemplateSpec_To_core_PodTemplateSpec failed: %v", err)))
			return allErrs
		}
		allErrs = append(allErrs, appsvalidation.ValidatePodTemplateSpecForReplicaSet(coreTemplate, nil, selector, 0, fldPath.Child("deploymentTemplate", "spec", "template"), webhookutil.DefaultPodValidationOptions)...)
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

func validateAdvancedStatefulSet(statefulSet *appsv1alpha1.AdvancedStatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if statefulSet.Spec.Replicas != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "replicas"), *statefulSet.Spec.Replicas, "replicas in advancedStatefulSetTemplate will not be used"))
	}
	if statefulSet.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType &&
		statefulSet.Spec.UpdateStrategy.RollingUpdate != nil &&
		statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "updateStrategy", "rollingUpdate", "partition"), *statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition, "partition in advancedStatefulSetTemplate will not be used"))
	}

	return allErrs
}

func validateDeployment(deployment *appsv1alpha1.DeploymentTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if deployment.Spec.Replicas != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("spec", "replicas"), *deployment.Spec.Replicas, "replicas in deploymentTemplate will not be used"))
	}

	return allErrs
}

func validateStatefulSetUpdate(statefulSet, oldStatefulSet *appsv1alpha1.StatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
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

	if statefulSet.Spec.Replicas != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*statefulSet.Spec.Replicas), fldPath.Child("spec", "replicas"))...)
	}
	return allErrs
}

func validateAdvancedStatefulSetUpdate(statefulSet, oldStatefulSet *appsv1alpha1.AdvancedStatefulSetTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	restoreReplicas := statefulSet.Spec.Replicas
	statefulSet.Spec.Replicas = oldStatefulSet.Spec.Replicas

	restoreTemplate := statefulSet.Spec.Template
	statefulSet.Spec.Template = oldStatefulSet.Spec.Template

	restoreStrategy := statefulSet.Spec.UpdateStrategy
	statefulSet.Spec.UpdateStrategy = oldStatefulSet.Spec.UpdateStrategy

	if !apiequality.Semantic.DeepEqual(statefulSet.Spec, oldStatefulSet.Spec) {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("spec"), "updates to advancedStatefulsetTemplate spec for fields other than 'template', and 'updateStrategy' are forbidden"))
	}
	statefulSet.Spec.Replicas = restoreReplicas
	statefulSet.Spec.Template = restoreTemplate
	statefulSet.Spec.UpdateStrategy = restoreStrategy

	if statefulSet.Spec.Replicas != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*statefulSet.Spec.Replicas), fldPath.Child("spec", "replicas"))...)
	}
	return allErrs
}

func validateDeploymentUpdate(deployment, oldDeployment *appsv1alpha1.DeploymentTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if deployment.Spec.Replicas != nil {
		allErrs = append(allErrs, apivalidation.ValidateNonnegativeField(int64(*deployment.Spec.Replicas), fldPath.Child("spec", "replicas"))...)
	}

	return allErrs
}
