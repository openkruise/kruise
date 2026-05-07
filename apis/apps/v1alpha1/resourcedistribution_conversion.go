package v1alpha1

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func (rd *ResourceDistribution) ConvertTo(dst conversion.Hub) error {
	switch target := dst.(type) {
	case *appsv1beta1.ResourceDistribution:
		return convertResourceDistributionToV1beta1(rd, target)
	default:
		return fmt.Errorf("unsupported type %T", target)
	}
}

func (rd *ResourceDistribution) ConvertFrom(src conversion.Hub) error {
	switch source := src.(type) {
	case *appsv1beta1.ResourceDistribution:
		return convertResourceDistributionFromV1beta1(source, rd)
	default:
		return fmt.Errorf("unsupported type %T", source)
	}
}

func convertResourceDistributionToV1beta1(src *ResourceDistribution, dst *appsv1beta1.ResourceDistribution) error {
	dst.ObjectMeta = src.ObjectMeta
	dst.TypeMeta = src.TypeMeta
	dst.APIVersion = appsv1beta1.GroupVersion.String()
	dst.Kind = "ResourceDistribution"

	dst.Spec = appsv1beta1.ResourceDistributionSpec{
		Resource: src.Spec.Resource,
		Targets:  convertResourceDistributionTargetsToV1beta1(src.Spec.Targets),
	}
	dst.Status = convertResourceDistributionStatusToV1beta1(src.Status)
	return nil
}

func convertResourceDistributionFromV1beta1(src *appsv1beta1.ResourceDistribution, dst *ResourceDistribution) error {
	dst.ObjectMeta = src.ObjectMeta
	dst.TypeMeta = src.TypeMeta
	dst.APIVersion = GroupVersion.String()
	dst.Kind = "ResourceDistribution"

	dst.Spec = ResourceDistributionSpec{
		Resource: src.Spec.Resource,
		Targets:  convertResourceDistributionTargetsFromV1beta1(src.Spec.Targets),
	}
	dst.Status = convertResourceDistributionStatusFromV1beta1(src.Status)
	return nil
}

func convertResourceDistributionTargetsToV1beta1(src ResourceDistributionTargets) appsv1beta1.ResourceDistributionTargets {
	return appsv1beta1.ResourceDistributionTargets{
		AllNamespaces:          src.AllNamespaces,
		ExcludedNamespaces:     convertResourceDistributionTargetNamespacesToV1beta1(src.ExcludedNamespaces),
		IncludedNamespaces:     convertResourceDistributionTargetNamespacesToV1beta1(src.IncludedNamespaces),
		NamespaceLabelSelector: src.NamespaceLabelSelector,
	}
}

func convertResourceDistributionTargetsFromV1beta1(src appsv1beta1.ResourceDistributionTargets) ResourceDistributionTargets {
	return ResourceDistributionTargets{
		AllNamespaces:          src.AllNamespaces,
		ExcludedNamespaces:     convertResourceDistributionTargetNamespacesFromV1beta1(src.ExcludedNamespaces),
		IncludedNamespaces:     convertResourceDistributionTargetNamespacesFromV1beta1(src.IncludedNamespaces),
		NamespaceLabelSelector: src.NamespaceLabelSelector,
	}
}

func convertResourceDistributionTargetNamespacesToV1beta1(src ResourceDistributionTargetNamespaces) appsv1beta1.ResourceDistributionTargetNamespaces {
	if len(src.List) == 0 {
		return appsv1beta1.ResourceDistributionTargetNamespaces{}
	}
	dst := appsv1beta1.ResourceDistributionTargetNamespaces{List: make([]appsv1beta1.ResourceDistributionNamespace, len(src.List))}
	for index, namespace := range src.List {
		dst.List[index] = appsv1beta1.ResourceDistributionNamespace{Name: namespace.Name}
	}
	return dst
}

func convertResourceDistributionTargetNamespacesFromV1beta1(src appsv1beta1.ResourceDistributionTargetNamespaces) ResourceDistributionTargetNamespaces {
	if len(src.List) == 0 {
		return ResourceDistributionTargetNamespaces{}
	}
	dst := ResourceDistributionTargetNamespaces{List: make([]ResourceDistributionNamespace, len(src.List))}
	for index, namespace := range src.List {
		dst.List[index] = ResourceDistributionNamespace{Name: namespace.Name}
	}
	return dst
}

func convertResourceDistributionStatusToV1beta1(src ResourceDistributionStatus) appsv1beta1.ResourceDistributionStatus {
	dst := appsv1beta1.ResourceDistributionStatus{
		Desired:            src.Desired,
		Succeeded:          src.Succeeded,
		Failed:             src.Failed,
		ObservedGeneration: src.ObservedGeneration,
	}
	if len(src.Conditions) == 0 {
		return dst
	}
	dst.Conditions = make([]appsv1beta1.ResourceDistributionCondition, len(src.Conditions))
	for index, condition := range src.Conditions {
		dst.Conditions[index] = appsv1beta1.ResourceDistributionCondition{
			Type:               appsv1beta1.ResourceDistributionConditionType(condition.Type),
			Status:             appsv1beta1.ResourceDistributionConditionStatus(condition.Status),
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			FailedNamespaces:   append([]string(nil), condition.FailedNamespaces...),
		}
	}
	return dst
}

func convertResourceDistributionStatusFromV1beta1(src appsv1beta1.ResourceDistributionStatus) ResourceDistributionStatus {
	dst := ResourceDistributionStatus{
		Desired:            src.Desired,
		Succeeded:          src.Succeeded,
		Failed:             src.Failed,
		ObservedGeneration: src.ObservedGeneration,
	}
	if len(src.Conditions) == 0 {
		return dst
	}
	dst.Conditions = make([]ResourceDistributionCondition, len(src.Conditions))
	for index, condition := range src.Conditions {
		dst.Conditions[index] = ResourceDistributionCondition{
			Type:               ResourceDistributionConditionType(condition.Type),
			Status:             ResourceDistributionConditionStatus(condition.Status),
			LastTransitionTime: condition.LastTransitionTime,
			Reason:             condition.Reason,
			FailedNamespaces:   append([]string(nil), condition.FailedNamespaces...),
		}
	}
	return dst
}
