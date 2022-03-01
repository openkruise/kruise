/*
Copyright 2021 The Kruise Authors.

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

package workloadspread

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schedulecorev1 "k8s.io/component-helpers/scheduling/corev1"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// NewWorkloadSpreadSubsetCondition creates a new WorkloadSpreadSubset condition.
func NewWorkloadSpreadSubsetCondition(condType appsv1alpha1.WorkloadSpreadSubsetConditionType, status corev1.ConditionStatus, reason, message string) *appsv1alpha1.WorkloadSpreadSubsetCondition {
	return &appsv1alpha1.WorkloadSpreadSubsetCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

// GetWorkloadSpreadSubsetCondition returns the condition with the provided type.
func GetWorkloadSpreadSubsetCondition(status *appsv1alpha1.WorkloadSpreadSubsetStatus, condType appsv1alpha1.WorkloadSpreadSubsetConditionType) *appsv1alpha1.WorkloadSpreadSubsetCondition {
	if status == nil {
		return nil
	}
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// setWorkloadSpreadSubsetCondition updates the WorkloadSpreadSubset to include the provided condition. If the condition that
// we are about to add already exists and has the same status, reason and message then we are not going to update.
func setWorkloadSpreadSubsetCondition(status *appsv1alpha1.WorkloadSpreadSubsetStatus, condition *appsv1alpha1.WorkloadSpreadSubsetCondition) {
	if condition == nil {
		return
	}
	currentCond := GetWorkloadSpreadSubsetCondition(status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}

	if currentCond != nil && currentCond.Status == condition.Status {
		condition.LastTransitionTime = currentCond.LastTransitionTime
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, *condition)
}

// removeWorkloadSpreadSubsetCondition removes the WorkloadSpreadSubset condition with the provided type.
func removeWorkloadSpreadSubsetCondition(status *appsv1alpha1.WorkloadSpreadSubsetStatus, condType appsv1alpha1.WorkloadSpreadSubsetConditionType) {
	status.Conditions = filterOutCondition(status.Conditions, condType)
}

func filterOutCondition(conditions []appsv1alpha1.WorkloadSpreadSubsetCondition, condType appsv1alpha1.WorkloadSpreadSubsetConditionType) []appsv1alpha1.WorkloadSpreadSubsetCondition {
	var newConditions []appsv1alpha1.WorkloadSpreadSubsetCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}

func matchesSubset(pod *corev1.Pod, node *corev1.Node, subset *appsv1alpha1.WorkloadSpreadSubset) (bool, int64, error) {
	matched, err := matchesSubsetRequiredAndToleration(pod, node, subset)
	if err != nil || !matched {
		return false, -1, err
	}

	if len(subset.PreferredNodeSelectorTerms) == 0 {
		return matched, 0, nil
	}

	nodePreferredTerms, _ := nodeaffinity.NewPreferredSchedulingTerms(subset.PreferredNodeSelectorTerms)
	return matched, nodePreferredTerms.Score(node), nil
}

func matchesSubsetRequiredAndToleration(pod *corev1.Pod, node *corev1.Node, subset *appsv1alpha1.WorkloadSpreadSubset) (bool, error) {
	// check toleration
	tolerations := append(pod.Spec.Tolerations, subset.Tolerations...)
	if _, hasUntoleratedTaint := v1helper.FindMatchingUntoleratedTaint(node.Spec.Taints, tolerations, nil); hasUntoleratedTaint {
		return false, nil
	}

	if subset.RequiredNodeSelectorTerm == nil {
		return true, nil
	}

	// check nodeSelectorTerm
	var nodeSelectorTerms []corev1.NodeSelectorTerm
	if pod.Spec.Affinity != nil {
		if pod.Spec.Affinity.NodeAffinity != nil {
			if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				nodeSelectorTerms = append(nodeSelectorTerms, pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms...)
			}
		}
	}
	if subset.RequiredNodeSelectorTerm != nil {
		if len(nodeSelectorTerms) == 0 {
			nodeSelectorTerms = []corev1.NodeSelectorTerm{
				*subset.RequiredNodeSelectorTerm,
			}
		} else {
			for i := range nodeSelectorTerms {
				selectorTerm := &nodeSelectorTerms[i]
				selectorTerm.MatchExpressions = append(selectorTerm.MatchExpressions, subset.RequiredNodeSelectorTerm.MatchExpressions...)
				selectorTerm.MatchFields = append(selectorTerm.MatchFields, subset.RequiredNodeSelectorTerm.MatchFields...)
			}
		}
	}

	return schedulecorev1.MatchNodeSelectorTerms(node, &corev1.NodeSelector{
		NodeSelectorTerms: nodeSelectorTerms,
	})
}
