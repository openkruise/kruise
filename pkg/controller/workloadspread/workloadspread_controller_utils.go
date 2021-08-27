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
