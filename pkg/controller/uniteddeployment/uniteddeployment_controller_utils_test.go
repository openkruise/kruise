package uniteddeployment

import (
	"testing"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckPodStaging(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name                   string
		pod                    *corev1.Pod
		updatedRevision        string
		updatedTime            time.Time
		pendingTimeout         time.Duration
		minReadySeconds        time.Duration
		expectedResult         bool
		expectedNextCheckAfter time.Duration
		expectedLegacyPods     int32
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
			name: "unavailable pod, pending",
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
						appsv1alpha1.ReservedPodLabelKey: "true",
					},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         true,
			expectedNextCheckAfter: 5 * time.Second,
		},
		{
			name: "unavailable pod, ready, but not long enough",
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
						appsv1alpha1.ReservedPodLabelKey: "true",
					},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         true,
			expectedNextCheckAfter: 2 * time.Second,
		},
		{
			name: "unavailable pod, ready, long enough",
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
						appsv1alpha1.ReservedPodLabelKey: "true",
					},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			expectedResult:         false,
			expectedNextCheckAfter: -1,
		},
		{
			name: "Normal pod, running, but with old revision and timeouted",
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
					Labels: map[string]string{
						appsv1alpha1.ControllerRevisionHashLabelKey: "old-revision",
						appsv1alpha1.ReservedPodLabelKey:            "false", // even is marked as not reserved
					},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			updatedRevision:        "new-revision",
			updatedTime:            now.Add(-11 * time.Second),
			expectedResult:         true,
			expectedNextCheckAfter: 5 * time.Second,
			expectedLegacyPods:     1,
		},
		{
			name: "Normal pod, running, but with old revision and not timeouted",
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
					Labels: map[string]string{
						appsv1alpha1.ControllerRevisionHashLabelKey: "old-revision",
					},
				},
			},
			pendingTimeout:         10 * time.Second,
			minReadySeconds:        5 * time.Second,
			updatedRevision:        "new-revision",
			updatedTime:            now.Add(-4 * time.Second),
			expectedResult:         false,
			expectedNextCheckAfter: 6 * time.Second,
			expectedLegacyPods:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subset := &Subset{
				Status: SubsetStatus{UpdatedRevision: tt.updatedRevision},
			}
			updatedCondition := &appsv1alpha1.UnitedDeploymentCondition{
				Type:               appsv1alpha1.UnitedDeploymentUpdated,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(tt.updatedTime),
			}
			result, nextCheckAfter := CheckPodReserved(tt.pod, subset, updatedCondition, tt.pendingTimeout, tt.minReadySeconds, now)
			if result != tt.expectedResult {
				t.Errorf("CheckPodReserved() result = %v, want %v", result, tt.expectedResult)
			}
			if nextCheckAfter != tt.expectedNextCheckAfter {
				t.Errorf("CheckPodReserved() nextCheckAfter = %v, want %v", nextCheckAfter, tt.expectedNextCheckAfter)
			}
			if subset.Status.UnschedulableStatus.TimeoutLegacyPods != tt.expectedLegacyPods {
				t.Errorf("CheckPodReserved() subset.Status.UnschedulableStatus.TimeoutLegacyPods = %v, want %v",
					subset.Status.UnschedulableStatus.TimeoutLegacyPods, tt.expectedLegacyPods)
			}
		})
	}
}
