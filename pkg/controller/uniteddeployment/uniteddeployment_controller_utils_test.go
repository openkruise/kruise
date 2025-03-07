package uniteddeployment

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckPodStaging(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name                   string
		pod                    *corev1.Pod
		pendingTimeout         time.Duration
		minReadySeconds        time.Duration
		expectedResult         bool
		expectedNextCheckAfter time.Duration
	}{
		{
			name: "Normal pod, pending, not long enough",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionFalse,
							Reason: corev1.PodReasonUnschedulable,
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: now.Add(-3 * time.Second)},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         false,
			expectedNextCheckAfter: 7 * time.Second,
		},
		{
			name: "Normal pod, pending, long enough",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionFalse,
							Reason: corev1.PodReasonUnschedulable,
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: now.Add(-11 * time.Second)},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         true,
			expectedNextCheckAfter: 5 * time.Second,
		},
		{
			name: "Normal pod, running",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:               corev1.PodReady,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: now.Add(-3 * time.Second)},
						},
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: now.Add(-1 * time.Second)},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         false,
			expectedNextCheckAfter: -1,
		},
		{
			name: "staging pod, pending",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:               corev1.PodReady,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: metav1.Time{Time: now.Add(-3 * time.Second)},
						},
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionFalse,
							Reason: corev1.PodReasonUnschedulable,
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: now.Add(-100 * time.Second)},
					Labels: map[string]string{
						LabelKeyStagingPod: "true",
					},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         true,
			expectedNextCheckAfter: 5 * time.Second,
		},
		{
			name: "staging pod, ready, but not long enough",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:               corev1.PodReady,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: now.Add(-3 * time.Second)},
						},
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: now.Add(-100 * time.Second)},
					Labels: map[string]string{
						LabelKeyStagingPod: "true",
					},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         true,
			expectedNextCheckAfter: 2 * time.Second,
		},
		{
			name: "staging pod, ready, long enough",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{
							Type:               corev1.PodReady,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: now.Add(-6 * time.Second)},
						},
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: now.Add(-100 * time.Second)},
					Labels: map[string]string{
						LabelKeyStagingPod: "true",
					},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         false,
			expectedNextCheckAfter: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, nextCheckAfter := CheckPodStaging(tt.pod, tt.pendingTimeout, tt.minReadySeconds, now)
			if result != tt.expectedResult {
				t.Errorf("CheckPodStaging() result = %v, want %v", result, tt.expectedResult)
			}
			if nextCheckAfter != tt.expectedNextCheckAfter {
				t.Errorf("CheckPodStaging() nextCheckAfter = %v, want %v", nextCheckAfter, tt.expectedNextCheckAfter)
			}
		})
	}
}
