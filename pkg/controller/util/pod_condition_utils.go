package util

import (
	"encoding/json"
	"time"

	v1 "k8s.io/api/core/v1"
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

// GetTimeBeforePendingTimeout return true when Pod was scheduled failed and timeout.
// nextCheckAfter > 0 means the pod is failed to schedule but not timeout yet.
func GetTimeBeforePendingTimeout(pod *v1.Pod, timeout time.Duration) (timeouted bool, nextCheckAfter time.Duration) {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != v1.PodPending || pod.Spec.NodeName != "" {
		return false, -1
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodScheduled && condition.Status == v1.ConditionFalse &&
			condition.Reason == v1.PodReasonUnschedulable {
			currentTime := time.Now()
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
