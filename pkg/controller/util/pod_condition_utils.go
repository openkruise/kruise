package util

import (
	"encoding/json"
	"time"

	v1 "k8s.io/api/core/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// using pod condition message to get key-value pairs
func GetMessageKvFromCondition(condition *v1.PodCondition) (map[string]interface{}, error) {
	messageKv := make(map[string]interface{})
	if condition != nil && condition.Message != "" {
		if err := json.Unmarshal([]byte(condition.Message), &messageKv); err != nil {
			return nil, err
		}
	}
	return messageKv, nil
}

// using pod condition message to save key-value pairs
func UpdateMessageKvCondition(kv map[string]interface{}, condition *v1.PodCondition) {
	message, _ := json.Marshal(kv)
	condition.Message = string(message)
}

// getScheduleFailedCondition should be changed later to get the condition from kruise-config.
func getScheduleFailedCondition() v1.PodCondition {
	return v1.PodCondition{
		Type:   v1.PodScheduled,
		Status: v1.ConditionFalse,
		Reason: v1.PodReasonUnschedulable,
	}
}

// GetTimeBeforePendingTimeout return true when Pod was scheduled failed and timeout.
// nextCheckAfter > 0 means the pod is failed to schedule but not timeout yet.
func GetTimeBeforePendingTimeout(pod *v1.Pod, timeout time.Duration, currentTime time.Time) (timeouted bool, nextCheckAfter time.Duration) {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != v1.PodPending || pod.Spec.NodeName != "" {
		return false, -1
	}
	scheduleFailedCondition := getScheduleFailedCondition()
	for _, condition := range pod.Status.Conditions {
		if condition.Type == scheduleFailedCondition.Type && condition.Status == scheduleFailedCondition.Status &&
			condition.Reason == scheduleFailedCondition.Reason {
			expectSchedule := pod.CreationTimestamp.Add(timeout)
			// schedule timeout
			if expectSchedule.Before(currentTime) {
				return true, -1
			}
			return false, expectSchedule.Sub(currentTime)
		}
	}
	return false, -1
}

// GetTimeBeforeUpdateTimeout is used during updating. when the updating lasts longer than timeout, all pods will be considered as timeout.
func GetTimeBeforeUpdateTimeout(pod *v1.Pod, updatedCondition *appsv1alpha1.UnitedDeploymentCondition, timeout time.Duration, currentTime time.Time) (timeouted bool, nextCheckAfter time.Duration) {
	if pod.DeletionTimestamp != nil {
		return false, -1
	}
	expectReschedule := updatedCondition.LastTransitionTime.Add(timeout)
	if expectReschedule.Before(currentTime) {
		return true, -1
	}
	return false, expectReschedule.Sub(currentTime)
}
