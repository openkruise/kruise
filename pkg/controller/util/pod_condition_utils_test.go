package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestGetMessageKvFromCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition *v1.PodCondition
		wantKv    map[string]interface{}
		wantErr   bool
	}{
		{
			name:      "nil condition",
			condition: nil,
			wantKv:    map[string]interface{}{},
			wantErr:   false,
		},
		{
			name:      "empty message",
			condition: &v1.PodCondition{Message: ""},
			wantKv:    map[string]interface{}{},
			wantErr:   false,
		},
		{
			name:      "valid JSON message",
			condition: &v1.PodCondition{Message: `{"key":"value","count":42}`},
			wantKv:    map[string]interface{}{"key": "value", "count": float64(42)},
			wantErr:   false,
		},
		{
			name:      "malformed JSON",
			condition: &v1.PodCondition{Message: `{not json}`},
			wantKv:    nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv, err := GetMessageKvFromCondition(tt.condition)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, kv)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantKv, kv)
			}
		})
	}
}

func TestUpdateMessageKvConditionRoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"foo": "bar",
		"num": float64(123),
	}
	condition := &v1.PodCondition{}
	UpdateMessageKvCondition(original, condition)

	assert.NotEmpty(t, condition.Message)

	recovered, err := GetMessageKvFromCondition(condition)
	assert.NoError(t, err)
	assert.Equal(t, original, recovered)
}

func TestGetTimeBeforePendingTimeout(t *testing.T) {
	now := time.Now()
	timeout := 10 * time.Minute
	deletionTime := metav1.Now()

	tests := []struct {
		name               string
		pod                *v1.Pod
		wantTimeouted      bool
		wantPositiveDur    bool
		wantNegativeOneDur bool
	}{
		{
			name: "pod with DeletionTimestamp",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTime},
				Status:     v1.PodStatus{Phase: v1.PodPending},
			},
			wantTimeouted:      false,
			wantNegativeOneDur: true,
		},
		{
			name: "pod not in Pending phase",
			pod: &v1.Pod{
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
			wantTimeouted:      false,
			wantNegativeOneDur: true,
		},
		{
			name: "pod with NodeName set",
			pod: &v1.Pod{
				Spec:   v1.PodSpec{NodeName: "node-1"},
				Status: v1.PodStatus{Phase: v1.PodPending},
			},
			wantTimeouted:      false,
			wantNegativeOneDur: true,
		},
		{
			name: "pending pod without Unschedulable condition",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					Conditions: []v1.PodCondition{
						{Type: v1.PodReady, Status: v1.ConditionFalse},
					},
				},
			},
			wantTimeouted:      false,
			wantNegativeOneDur: true,
		},
		{
			name: "PodScheduled=False but wrong Reason",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-20 * time.Minute))},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodScheduled,
							Status: v1.ConditionFalse,
							Reason: "SomeOtherReason",
						},
					},
				},
			},
			wantTimeouted:      false,
			wantNegativeOneDur: true,
		},
		{
			name: "condition matches, already past deadline",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-20 * time.Minute))},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodScheduled,
							Status: v1.ConditionFalse,
							Reason: string(v1.PodReasonUnschedulable),
						},
					},
				},
			},
			wantTimeouted:      true,
			wantNegativeOneDur: true,
		},
		{
			name: "condition matches, not yet past deadline",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Minute))},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodScheduled,
							Status: v1.ConditionFalse,
							Reason: string(v1.PodReasonUnschedulable),
						},
					},
				},
			},
			wantTimeouted:   false,
			wantPositiveDur: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeouted, dur := GetTimeBeforePendingTimeout(tt.pod, timeout, now)
			assert.Equal(t, tt.wantTimeouted, timeouted)
			if tt.wantNegativeOneDur {
				assert.Equal(t, time.Duration(-1), dur)
			}
			if tt.wantPositiveDur {
				assert.True(t, dur > 0, "expected positive duration, got %v", dur)
			}
		})
	}
}

func TestGetTimeBeforeUpdateTimeout(t *testing.T) {
	now := time.Now()
	timeout := 10 * time.Minute
	deletionTime := metav1.Now()

	tests := []struct {
		name               string
		pod                *v1.Pod
		conditionTime      time.Time
		wantTimeouted      bool
		wantPositiveDur    bool
		wantNegativeOneDur bool
	}{
		{
			name: "DeletionTimestamp set",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletionTime},
			},
			conditionTime:      now.Add(-5 * time.Minute),
			wantTimeouted:      false,
			wantNegativeOneDur: true,
		},
		{
			name:            "before deadline",
			pod:             &v1.Pod{},
			conditionTime:   now.Add(-5 * time.Minute),
			wantTimeouted:   false,
			wantPositiveDur: true,
		},
		{
			name:               "past deadline",
			pod:                &v1.Pod{},
			conditionTime:      now.Add(-20 * time.Minute),
			wantTimeouted:      true,
			wantNegativeOneDur: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := &appsv1alpha1.UnitedDeploymentCondition{
				LastTransitionTime: metav1.NewTime(tt.conditionTime),
			}
			timeouted, dur := GetTimeBeforeUpdateTimeout(tt.pod, condition, timeout, now)
			assert.Equal(t, tt.wantTimeouted, timeouted)
			if tt.wantNegativeOneDur {
				assert.Equal(t, time.Duration(-1), dur)
			}
			if tt.wantPositiveDur {
				assert.True(t, dur > 0, "expected positive duration, got %v", dur)
			}
		})
	}
}
