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

package ephemeraljob

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestPastActiveDeadline(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		job      *appsv1alpha1.EphemeralJob
		expected bool
	}{
		{
			name: "no active deadline set",
			job: &appsv1alpha1.EphemeralJob{
				Spec: appsv1alpha1.EphemeralJobSpec{},
			},
			expected: false,
		},
		{
			name: "no start time",
			job: &appsv1alpha1.EphemeralJob{
				Spec: appsv1alpha1.EphemeralJobSpec{
					ActiveDeadlineSeconds: ptr(int64(300)),
				},
			},
			expected: false,
		},
		{
			name: "within deadline",
			job: &appsv1alpha1.EphemeralJob{
				Spec: appsv1alpha1.EphemeralJobSpec{
					ActiveDeadlineSeconds: ptr(int64(600)), // 10 minutes
				},
				Status: appsv1alpha1.EphemeralJobStatus{
					StartTime: &metav1.Time{Time: now.Add(-5 * time.Minute)},
				},
			},
			expected: false,
		},
		{
			name: "past deadline",
			job: &appsv1alpha1.EphemeralJob{
				Spec: appsv1alpha1.EphemeralJobSpec{
					ActiveDeadlineSeconds: ptr(int64(300)), // 5 minutes
				},
				Status: appsv1alpha1.EphemeralJobStatus{
					StartTime: &metav1.Time{Time: now.Add(-10 * time.Minute)},
				},
			},
			expected: true,
		},
		{
			name: "at or past deadline (exactly at deadline)",
			job: &appsv1alpha1.EphemeralJob{
				Spec: appsv1alpha1.EphemeralJobSpec{
					ActiveDeadlineSeconds: ptr(int64(300)),
				},
				Status: appsv1alpha1.EphemeralJobStatus{
					StartTime: &metav1.Time{Time: now.Add(-300 * time.Second)},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pastActiveDeadline(tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPodMatchedEphemeralJob(t *testing.T) {
	tests := []struct {
		name        string
		pod         *v1.Pod
		ejob        *appsv1alpha1.EphemeralJob
		expected    bool
		expectError bool
	}{
		{
			name: "namespace mismatch",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			ejob: &appsv1alpha1.EphemeralJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
				},
			},
			expected:    false,
			expectError: false,
		},
		{
			name: "selector matches",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						"app": "test",
					},
				},
			},
			ejob: &appsv1alpha1.EphemeralJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: appsv1alpha1.EphemeralJobSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
				},
			},
			expected:    true,
			expectError: false,
		},
		{
			name: "selector does not match",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						"app": "other",
					},
				},
			},
			ejob: &appsv1alpha1.EphemeralJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: appsv1alpha1.EphemeralJobSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
				},
			},
			expected:    false,
			expectError: false,
		},
		{
			name: "empty selector does not match",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			ejob: &appsv1alpha1.EphemeralJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: appsv1alpha1.EphemeralJobSpec{
					Selector: &metav1.LabelSelector{},
				},
			},
			expected:    false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := podMatchedEphemeralJob(tt.pod, tt.ejob)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAddConditions(t *testing.T) {
	tests := []struct {
		name          string
		conditions    []appsv1alpha1.EphemeralJobCondition
		conditionType appsv1alpha1.EphemeralJobConditionType
		reason        string
		message       string
		expectedLen   int
	}{
		{
			name:          "add to empty conditions",
			conditions:    []appsv1alpha1.EphemeralJobCondition{},
			conditionType: appsv1alpha1.EJobSucceeded,
			reason:        "JobCompleted",
			message:       "All containers succeeded",
			expectedLen:   1,
		},
		{
			name: "update existing condition",
			conditions: []appsv1alpha1.EphemeralJobCondition{
				{
					Type:    appsv1alpha1.EJobSucceeded,
					Status:  v1.ConditionFalse,
					Reason:  "OldReason",
					Message: "Old message",
				},
			},
			conditionType: appsv1alpha1.EJobSucceeded,
			reason:        "NewReason",
			message:       "New message",
			expectedLen:   1,
		},
		{
			name: "add new condition type",
			conditions: []appsv1alpha1.EphemeralJobCondition{
				{
					Type:    appsv1alpha1.EJobSucceeded,
					Status:  v1.ConditionTrue,
					Reason:  "Completed",
					Message: "Done",
				},
			},
			conditionType: appsv1alpha1.EJobFailed,
			reason:        "Failed",
			message:       "Job failed",
			expectedLen:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addConditions(tt.conditions, tt.conditionType, tt.reason, tt.message)

			assert.Equal(t, tt.expectedLen, len(result))

			// Find the condition we added/updated
			var found bool
			for _, cond := range result {
				if cond.Type == tt.conditionType {
					found = true
					assert.Equal(t, v1.ConditionTrue, cond.Status)
					assert.Equal(t, tt.reason, cond.Reason)
					assert.Equal(t, tt.message, cond.Message)
					break
				}
			}
			assert.True(t, found, "condition should be present")
		})
	}
}

func TestNewCondition(t *testing.T) {
	conditionType := appsv1alpha1.EJobSucceeded
	reason := "JobCompleted"
	message := "All tasks finished"

	cond := newCondition(conditionType, reason, message)

	assert.Equal(t, conditionType, cond.Type)
	assert.Equal(t, v1.ConditionTrue, cond.Status)
	assert.Equal(t, reason, cond.Reason)
	assert.Equal(t, message, cond.Message)
	assert.False(t, cond.LastProbeTime.IsZero())
	assert.False(t, cond.LastTransitionTime.IsZero())
}

func TestGetEphemeralContainersMaps(t *testing.T) {
	tests := []struct {
		name          string
		containers    []v1.EphemeralContainer
		expectedLen   int
		expectedEmpty bool
		expectedNames []string
	}{
		{
			name:          "empty containers",
			containers:    []v1.EphemeralContainer{},
			expectedLen:   0,
			expectedEmpty: true,
		},
		{
			name: "single container",
			containers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name: "debug-1",
					},
				},
			},
			expectedLen:   1,
			expectedEmpty: false,
			expectedNames: []string{"debug-1"},
		},
		{
			name: "multiple containers",
			containers: []v1.EphemeralContainer{
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name: "debug-1",
					},
				},
				{
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name: "debug-2",
					},
				},
			},
			expectedLen:   2,
			expectedEmpty: false,
			expectedNames: []string{"debug-1", "debug-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, isEmpty := getEphemeralContainersMaps(tt.containers)

			assert.Equal(t, tt.expectedLen, len(result))
			assert.Equal(t, tt.expectedEmpty, isEmpty)

			for _, name := range tt.expectedNames {
				_, exists := result[name]
				assert.True(t, exists, "container %s should exist in map", name)
			}
		})
	}
}

func TestGetPodEphemeralContainers(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}

	ejob := &appsv1alpha1.EphemeralJob{
		Spec: appsv1alpha1.EphemeralJobSpec{
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name: "debug",
						},
					},
					{
						EphemeralContainerCommon: v1.EphemeralContainerCommon{
							Name: "trace",
						},
					},
				},
			},
		},
	}

	result := getPodEphemeralContainers(pod, ejob)

	assert.Equal(t, 2, len(result))
	assert.Contains(t, result, "test-pod-debug")
	assert.Contains(t, result, "test-pod-trace")
}

func TestIsCreatedByEJob(t *testing.T) {
	jobUID := "test-job-uid"

	tests := []struct {
		name      string
		container v1.EphemeralContainer
		expected  bool
	}{
		{
			name: "container created by ejob",
			container: v1.EphemeralContainer{
				EphemeralContainerCommon: v1.EphemeralContainerCommon{
					Env: []v1.EnvVar{
						{
							Name:  appsv1alpha1.EphemeralContainerEnvKey,
							Value: jobUID,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "container not created by ejob",
			container: v1.EphemeralContainer{
				EphemeralContainerCommon: v1.EphemeralContainerCommon{
					Env: []v1.EnvVar{
						{
							Name:  "OTHER_ENV",
							Value: "value",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "container with different job uid",
			container: v1.EphemeralContainer{
				EphemeralContainerCommon: v1.EphemeralContainerCommon{
					Env: []v1.EnvVar{
						{
							Name:  appsv1alpha1.EphemeralContainerEnvKey,
							Value: "different-uid",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "container with no env vars",
			container: v1.EphemeralContainer{
				EphemeralContainerCommon: v1.EphemeralContainerCommon{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCreatedByEJob(jobUID, tt.container)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseEphemeralContainerStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   *v1.ContainerStatus
		expected ephemeralContainerStatusState
	}{
		{
			name: "succeeded container",
			status: &v1.ContainerStatus{
				State: v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{
						ExitCode:   0,
						FinishedAt: metav1.Now(),
					},
				},
			},
			expected: SucceededStatus,
		},
		{
			name: "failed container with non-zero exit code",
			status: &v1.ContainerStatus{
				State: v1.ContainerState{
					Terminated: &v1.ContainerStateTerminated{
						ExitCode: 1,
					},
				},
			},
			expected: FailedStatus,
		},
		{
			name: "running container",
			status: &v1.ContainerStatus{
				State: v1.ContainerState{
					Running: &v1.ContainerStateRunning{
						StartedAt: metav1.Now(),
					},
				},
			},
			expected: RunningStatus,
		},
		{
			name: "waiting container",
			status: &v1.ContainerStatus{
				State: v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{
						Reason: "ContainerCreating",
					},
				},
			},
			expected: WaitingStatus,
		},
		{
			name: "container with RunContainerError",
			status: &v1.ContainerStatus{
				State: v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{
						Reason: "RunContainerError",
					},
				},
			},
			expected: FailedStatus,
		},
		{
			name:     "unknown state",
			status:   &v1.ContainerStatus{},
			expected: UnknownStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEphemeralContainerStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasEphemeralContainerFinalizer(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []string
		expected   bool
	}{
		{
			name:       "empty finalizers",
			finalizers: []string{},
			expected:   false,
		},
		{
			name:       "has ephemeral container finalizer",
			finalizers: []string{EphemeralContainerFinalizer},
			expected:   true,
		},
		{
			name:       "has multiple finalizers including target",
			finalizers: []string{"other.finalizer", EphemeralContainerFinalizer, "another.finalizer"},
			expected:   true,
		},
		{
			name:       "does not have ephemeral container finalizer",
			finalizers: []string{"other.finalizer", "another.finalizer"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasEphemeralContainerFinalizer(tt.finalizers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeleteEphemeralContainerFinalizer(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []string
		finalizer  string
		expected   []string
	}{
		{
			name:       "empty finalizers",
			finalizers: []string{},
			finalizer:  EphemeralContainerFinalizer,
			expected:   []string{},
		},
		{
			name:       "remove existing finalizer",
			finalizers: []string{EphemeralContainerFinalizer},
			finalizer:  EphemeralContainerFinalizer,
			expected:   []string{},
		},
		{
			name:       "remove from multiple finalizers",
			finalizers: []string{"other.finalizer", EphemeralContainerFinalizer, "another.finalizer"},
			finalizer:  EphemeralContainerFinalizer,
			expected:   []string{"other.finalizer", "another.finalizer"},
		},
		{
			name:       "finalizer not present",
			finalizers: []string{"other.finalizer", "another.finalizer"},
			finalizer:  EphemeralContainerFinalizer,
			expected:   []string{"other.finalizer", "another.finalizer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deleteEphemeralContainerFinalizer(tt.finalizers, tt.finalizer)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestTimeNow(t *testing.T) {
	before := time.Now()
	result := timeNow()
	after := time.Now()

	assert.NotNil(t, result)
	assert.True(t, !result.Time.Before(before) && !result.Time.After(after))
}

// Helper function
func ptr[T any](v T) *T {
	return &v
}
