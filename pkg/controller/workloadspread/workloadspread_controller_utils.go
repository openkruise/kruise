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
	"encoding/json"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	schedulecorev1 "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"k8s.io/klog/v2"
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

func matchesSubset(pod *corev1.Pod, node *corev1.Node, subset *appsv1alpha1.WorkloadSpreadSubset, missingReplicas int) (bool, int64, error) {
	// necessary condition
	matched, err := matchesSubsetRequiredAndToleration(pod, node, subset)
	if err != nil || !matched {
		return false, -1, err
	}

	// preferredNodeScore is in [0, total_prefer_weight]
	preferredNodeScore := int64(0)
	if subset.PreferredNodeSelectorTerms != nil {
		nodePreferredTerms, _ := nodeaffinity.NewPreferredSchedulingTerms(subset.PreferredNodeSelectorTerms)
		preferredNodeScore = nodePreferredTerms.Score(node)
	}

	// preferredPodScore is in [0, 1]
	preferredPodScore := int64(0)
	if subset.Patch.Raw != nil {
		preferredPodScore = podPreferredScore(subset, pod)
	}

	// we prefer the subset that still has room for more replicas
	quotaScore := int64(0)
	if missingReplicas > 0 {
		quotaScore = int64(1)
	}

	// preferredPodScore is in [0, 1], so it cannot affect preferredNodeScore in the following expression
	preferredScore := preferredNodeScore*100 + preferredPodScore*10 + quotaScore
	return matched, preferredScore, nil
}

func podPreferredScore(subset *appsv1alpha1.WorkloadSpreadSubset, pod *corev1.Pod) int64 {
	podBytes, _ := json.Marshal(pod)
	modified, err := strategicpatch.StrategicMergePatch(podBytes, subset.Patch.Raw, &corev1.Pod{})
	if err != nil {
		klog.ErrorS(err, "Failed to merge patch raw for pod and subset", "pod", klog.KObj(pod), "subsetName", subset.Name)
		return 0
	}
	patchedPod := &corev1.Pod{}
	err = json.Unmarshal(modified, patchedPod)
	if err != nil {
		klog.ErrorS(err, "Failed to unmarshal for pod and subset", "pod", klog.KObj(pod), "subsetName", subset.Name)
		return 0
	}
	// TODO: consider json annotation just like `{"json_key": ["value1", "value2"]}`.
	// currently, we exclude annotations field because annotation may contain some filed we cannot handle.
	// For example, we cannot judge whether the following two annotations are equal via DeepEqual method:
	// example.com/list: '["a", "b", "c"]'
	// example.com/list: '["b", "a", "c"]'
	patchedPod.Annotations = pod.Annotations
	if reflect.DeepEqual(pod, patchedPod) {
		return 1
	}
	return 0
}

func matchesSubsetRequiredAndToleration(pod *corev1.Pod, node *corev1.Node, subset *appsv1alpha1.WorkloadSpreadSubset) (bool, error) {
	// check toleration
	tolerations := append(pod.Spec.Tolerations, subset.Tolerations...)
	if _, hasUntoleratedTaint := schedulecorev1.FindMatchingUntoleratedTaint(node.Spec.Taints, tolerations, nil); hasUntoleratedTaint {
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
