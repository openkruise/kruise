package cloneset

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestCheckProgressingDeadline(t *testing.T) {
	// Create a test table
	tests := []struct {
		name            string
		cloneSet        *appsv1alpha1.CloneSet
		status          *appsv1alpha1.CloneSetStatus
		expectedExpired bool
	}{
		{
			name: "deadline specified and exceeded",
			cloneSet: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					ProgressDeadlineSeconds: func() *int32 { i := int32(60); return &i }(),
				},
			},
			status: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               "Progressing",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
					},
				},
			},
			expectedExpired: true,
		},
		{
			name: "deadline specified but not exceeded",
			cloneSet: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					ProgressDeadlineSeconds: func() *int32 { i := int32(60); return &i }(),
				},
			},
			status: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               "Progressing",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(time.Now().Add(-30 * time.Second)),
					},
				},
			},
			expectedExpired: false,
		},
		{
			name: "no deadline specified",
			cloneSet: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					ProgressDeadlineSeconds: nil,
				},
			},
			status: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               "Progressing",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
					},
				},
			},
			expectedExpired: false,
		},
		{
			name: "no progressing condition",
			cloneSet: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					ProgressDeadlineSeconds: func() *int32 { i := int32(60); return &i }(),
				},
			},
			status: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{},
			},
			expectedExpired: false,
		},
		{
			name: "progressing condition not true",
			cloneSet: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					ProgressDeadlineSeconds: func() *int32 { i := int32(60); return &i }(),
				},
			},
			status: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               "Progressing",
						Status:             corev1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
					},
				},
			},
			expectedExpired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expired := checkProgressingDeadline(tt.cloneSet, tt.status)
			if expired != tt.expectedExpired {
				t.Errorf("checkProgressingDeadline() = %v, want %v", expired, tt.expectedExpired)
			}
		})
	}
}

// Implementation of the deadline check function to be tested
func checkProgressingDeadline(cloneSet *appsv1alpha1.CloneSet, status *appsv1alpha1.CloneSetStatus) bool {
	// If deadline is not specified, there's no timeout
	if cloneSet.Spec.ProgressDeadlineSeconds == nil {
		return false
	}

	// Find the progressing condition
	var condition *appsv1alpha1.CloneSetCondition
	for i := range status.Conditions {
		if status.Conditions[i].Type == "Progressing" {
			condition = &status.Conditions[i]
			break
		}
	}

	if condition == nil || condition.Status != corev1.ConditionTrue {
		return false
	}

	// Check if we've exceeded the deadline
	now := time.Now()
	deadline := condition.LastTransitionTime.Add(time.Duration(*cloneSet.Spec.ProgressDeadlineSeconds) * time.Second)
	return now.After(deadline)
}
