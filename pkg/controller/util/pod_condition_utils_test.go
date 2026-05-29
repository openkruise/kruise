/*
Copyright 2025 The Kruise Authors.

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

package util

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestGetMessageKvFromCondition(t *testing.T) {
	tests := []struct {
		name        string
		condition   *v1.PodCondition
		expectedKv  map[string]interface{}
		expectError bool
	}{
		{
			name:        "nil condition",
			condition:   nil,
			expectedKv:  map[string]interface{}{},
			expectError: false,
		},
		{
			name: "empty message",
			condition: &v1.PodCondition{
				Type:    v1.PodReady,
				Status:  v1.ConditionTrue,
				Message: "",
			},
			expectedKv:  map[string]interface{}{},
			expectError: false,
		},
		{
			name: "valid json message",
			condition: &v1.PodCondition{
				Type:    v1.PodReady,
				Status:  v1.ConditionTrue,
				Message: `{"key1":"value1","key2":123}`,
			},
			expectedKv: map[string]interface{}{
				"key1": "value1",
				"key2": float64(123),
			},
			expectError: false,
		},
		{
			name: "invalid json message",
			condition: &v1.PodCondition{
				Type:    v1.PodReady,
				Status:  v1.ConditionTrue,
				Message: `{invalid json}`,
			},
			expectedKv:  nil,
			expectError: true,
		},
		{
			name: "complex nested json",
			condition: &v1.PodCondition{
				Type:    v1.PodScheduled,
				Status:  v1.ConditionFalse,
				Message: `{"reason":"scheduling","details":{"node":"node1","zone":"us-west1"}}`,
			},
			expectedKv: map[string]interface{}{
				"reason": "scheduling",
				"details": map[string]interface{}{
					"node": "node1",
					"zone": "us-west1",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetMessageKvFromCondition(tt.condition)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedKv, result)
			}
		})
	}
}

func TestUpdateMessageKvCondition(t *testing.T) {
	tests := []struct {
		name      string
		kv        map[string]interface{}
		condition *v1.PodCondition
		expected  string
	}{
		{
			name: "simple key-value pairs",
			kv: map[string]interface{}{
				"status": "pending",
				"count":  5,
			},
			condition: &v1.PodCondition{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
			expected: `{"count":5,"status":"pending"}`,
		},
		{
			name: "empty map",
			kv:   map[string]interface{}{},
			condition: &v1.PodCondition{
				Type:   v1.PodScheduled,
				Status: v1.ConditionFalse,
			},
			expected: `{}`,
		},
		{
			name: "nested structure",
			kv: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "test-pod",
					"namespace": "default",
				},
			},
			condition: &v1.PodCondition{
				Type:   v1.ContainersReady,
				Status: v1.ConditionTrue,
			},
			expected: `{"metadata":{"name":"test-pod","namespace":"default"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			UpdateMessageKvCondition(tt.kv, tt.condition)

			var result map[string]interface{}
			err := json.Unmarshal([]byte(tt.condition.Message), &result)
			assert.NoError(t, err)

			var expected map[string]interface{}
			err = json.Unmarshal([]byte(tt.expected), &expected)
			assert.NoError(t, err)

			assert.Equal(t, expected, result)
		})
	}
}

func TestGetTimeBeforePendingTimeout(t *testing.T) {
	currentTime := time.Now()
	timeout := 5 * time.Minute

	tests := []struct {
		name               string
		pod                *v1.Pod
		timeout            time.Duration
		currentTime        time.Time
		expectedTimedout   bool
		expectedNextCheck  time.Duration
		checkNextCheckSign bool // whether to check if nextCheckAfter is positive/negative
	}{
		{
			name: "pod with deletion timestamp",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pod",
					DeletionTimestamp: &metav1.Time{Time: currentTime},
				},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
				},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   false,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
		{
			name: "pod not in pending phase",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
				},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   false,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
		{
			name: "pod already scheduled (has node name)",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
				},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   false,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
		{
			name: "pod pending without schedule failed condition",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pod",
					CreationTimestamp: metav1.Time{Time: currentTime.Add(-2 * time.Minute)},
				},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   false,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
		{
			name: "pod schedule failed but not timeout yet",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pod",
					CreationTimestamp: metav1.Time{Time: currentTime.Add(-2 * time.Minute)},
				},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodScheduled,
							Status: v1.ConditionFalse,
							Reason: v1.PodReasonUnschedulable,
						},
					},
				},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   false,
			checkNextCheckSign: true, // should be positive (3 minutes remaining)
		},
		{
			name: "pod schedule failed and timeout",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pod",
					CreationTimestamp: metav1.Time{Time: currentTime.Add(-10 * time.Minute)},
				},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodScheduled,
							Status: v1.ConditionFalse,
							Reason: v1.PodReasonUnschedulable,
						},
					},
				},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   true,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
		{
			name: "pod schedule failed slightly past timeout",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pod",
					CreationTimestamp: metav1.Time{Time: currentTime.Add(-timeout - time.Second)},
				},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodScheduled,
							Status: v1.ConditionFalse,
							Reason: v1.PodReasonUnschedulable,
						},
					},
				},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   true,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timedout, nextCheckAfter := GetTimeBeforePendingTimeout(tt.pod, tt.timeout, tt.currentTime)

			assert.Equal(t, tt.expectedTimedout, timedout)

			if tt.checkNextCheckSign {
				assert.Greater(t, nextCheckAfter, time.Duration(0), "nextCheckAfter should be positive for pending pods")
			} else {
				assert.Equal(t, tt.expectedNextCheck, nextCheckAfter)
			}
		})
	}
}

func TestGetTimeBeforeUpdateTimeout(t *testing.T) {
	currentTime := time.Now()
	timeout := 10 * time.Minute

	tests := []struct {
		name               string
		pod                *v1.Pod
		updatedCondition   *appsv1alpha1.UnitedDeploymentCondition
		timeout            time.Duration
		currentTime        time.Time
		expectedTimedout   bool
		expectedNextCheck  time.Duration
		checkNextCheckSign bool
	}{
		{
			name: "pod with deletion timestamp",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pod",
					DeletionTimestamp: &metav1.Time{Time: currentTime},
				},
			},
			updatedCondition: &appsv1alpha1.UnitedDeploymentCondition{
				LastTransitionTime: metav1.Time{Time: currentTime.Add(-5 * time.Minute)},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   false,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
		{
			name: "update not timeout yet",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			updatedCondition: &appsv1alpha1.UnitedDeploymentCondition{
				LastTransitionTime: metav1.Time{Time: currentTime.Add(-3 * time.Minute)},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   false,
			checkNextCheckSign: true, // should have 7 minutes remaining
		},
		{
			name: "update timeout",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			updatedCondition: &appsv1alpha1.UnitedDeploymentCondition{
				LastTransitionTime: metav1.Time{Time: currentTime.Add(-15 * time.Minute)},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   true,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
		{
			name: "update slightly past timeout",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			updatedCondition: &appsv1alpha1.UnitedDeploymentCondition{
				LastTransitionTime: metav1.Time{Time: currentTime.Add(-timeout - time.Second)},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   true,
			expectedNextCheck:  -1,
			checkNextCheckSign: false,
		},
		{
			name: "just started update",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			updatedCondition: &appsv1alpha1.UnitedDeploymentCondition{
				LastTransitionTime: metav1.Time{Time: currentTime},
			},
			timeout:            timeout,
			currentTime:        currentTime,
			expectedTimedout:   false,
			checkNextCheckSign: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timedout, nextCheckAfter := GetTimeBeforeUpdateTimeout(tt.pod, tt.updatedCondition, tt.timeout, tt.currentTime)

			assert.Equal(t, tt.expectedTimedout, timedout)

			if tt.checkNextCheckSign {
				assert.Greater(t, nextCheckAfter, time.Duration(0), "nextCheckAfter should be positive for non-timeout cases")
			} else {
				assert.Equal(t, tt.expectedNextCheck, nextCheckAfter)
			}
		})
	}
}
