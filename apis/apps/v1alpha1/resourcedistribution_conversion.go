/*
Copyright 2020 The Kruise Authors.

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

package v1alpha1

import (
	"fmt"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *ResourceDistribution) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.ResourceDistribution:
		dst := dstRaw.(*v1beta1.ResourceDistribution)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Resource = src.Spec.Resource
		dst.Spec.Targets.AllNamespaces = src.Spec.Targets.AllNamespaces
		list := make([]v1beta1.ResourceDistributionNamespace, len(src.Spec.Targets.ExcludedNamespaces.List))
		for list_idx, list_val := range src.Spec.Targets.ExcludedNamespaces.List {
			list[list_idx].Name = list_val.Name
		}
		dst.Spec.Targets.ExcludedNamespaces.List = list
		list = make([]v1beta1.ResourceDistributionNamespace, len(src.Spec.Targets.IncludedNamespaces.List))
		for list_idx, list_val := range src.Spec.Targets.IncludedNamespaces.List {
			list[list_idx].Name = list_val.Name
		}
		dst.Spec.Targets.IncludedNamespaces.List = list
		dst.Spec.Targets.NamespaceLabelSelector = src.Spec.Targets.NamespaceLabelSelector
		dst.Status.Desired = src.Status.Desired
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Failed = src.Status.Failed
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		conditions := make([]v1beta1.ResourceDistributionCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = v1beta1.ResourceDistributionConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = v1beta1.ResourceDistributionConditionStatus(conditions_val.Status)
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].FailedNamespaces = conditions_val.FailedNamespaces
		}
		dst.Status.Conditions = conditions
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *ResourceDistribution) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.ResourceDistribution:
		src := srcRaw.(*v1beta1.ResourceDistribution)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Resource = src.Spec.Resource
		dst.Spec.Targets.AllNamespaces = src.Spec.Targets.AllNamespaces
		list := make([]ResourceDistributionNamespace, len(src.Spec.Targets.ExcludedNamespaces.List))
		for list_idx, list_val := range src.Spec.Targets.ExcludedNamespaces.List {
			list[list_idx].Name = list_val.Name
		}
		dst.Spec.Targets.ExcludedNamespaces.List = list
		list = make([]ResourceDistributionNamespace, len(src.Spec.Targets.IncludedNamespaces.List))
		for list_idx, list_val := range src.Spec.Targets.IncludedNamespaces.List {
			list[list_idx].Name = list_val.Name
		}
		dst.Spec.Targets.IncludedNamespaces.List = list
		dst.Spec.Targets.NamespaceLabelSelector = src.Spec.Targets.NamespaceLabelSelector
		dst.Status.Desired = src.Status.Desired
		dst.Status.Succeeded = src.Status.Succeeded
		dst.Status.Failed = src.Status.Failed
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		conditions := make([]ResourceDistributionCondition, len(src.Status.Conditions))
		for conditions_idx, conditions_val := range src.Status.Conditions {
			conditions[conditions_idx].Type = ResourceDistributionConditionType(conditions_val.Type)
			conditions[conditions_idx].Status = ResourceDistributionConditionStatus(conditions_val.Status)
			conditions[conditions_idx].LastTransitionTime = conditions_val.LastTransitionTime
			conditions[conditions_idx].Reason = conditions_val.Reason
			conditions[conditions_idx].FailedNamespaces = conditions_val.FailedNamespaces
		}
		dst.Status.Conditions = conditions
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
