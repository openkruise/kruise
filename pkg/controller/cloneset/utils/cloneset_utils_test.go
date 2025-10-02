package utils

import (
	"math"
	"testing"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestHasProgressDeadline(t *testing.T) {
	tests := []struct {
		name     string
		cs       *appsv1alpha1.CloneSet
		expected bool
	}{
		{
			name: "Has ProgressDeadlineSeconds",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					ProgressDeadlineSeconds: ptr.To(int32(600)),
				},
			},
			expected: true,
		},
		{
			name:     "No ProgressDeadlineSeconds",
			cs:       &appsv1alpha1.CloneSet{Spec: appsv1alpha1.CloneSetSpec{}},
			expected: false,
		},
		{
			name: "ProgressDeadlineSeconds with MaxInt32",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					ProgressDeadlineSeconds: ptr.To(int32(math.MaxInt32)),
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasProgressDeadline(tt.cs); got != tt.expected {
				t.Errorf("HasProgressDeadline() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetCloneSetCondition(t *testing.T) {
	condType := appsv1alpha1.CloneSetConditionTypeProgressing
	condition := appsv1alpha1.CloneSetCondition{
		Type:   condType,
		Status: v1.ConditionTrue,
	}

	tests := []struct {
		name       string
		status     appsv1alpha1.CloneSetStatus
		condType   appsv1alpha1.CloneSetConditionType
		wantExist  bool
		wantStatus v1.ConditionStatus
	}{
		{
			name: "Condition exists",
			status: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{condition},
			},
			condType:   condType,
			wantExist:  true,
			wantStatus: v1.ConditionTrue,
		},
		{
			name: "Condition not exists",
			status: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{},
			},
			condType:  condType,
			wantExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetCloneSetCondition(tt.status, tt.condType)
			if tt.wantExist && got == nil {
				t.Errorf("GetCloneSetCondition() = nil, want non-nil")
			}
			if !tt.wantExist && got != nil {
				t.Errorf("GetCloneSetCondition() = %v, want nil", got)
			}
			if got != nil && got.Status != tt.wantStatus {
				t.Errorf("GetCloneSetCondition().Status = %v, want %v", got.Status, tt.wantStatus)
			}
		})
	}
}

func TestSetCloneSetCondition(t *testing.T) {
	now := time.Now()
	condType := appsv1alpha1.CloneSetConditionTypeProgressing

	tests := []struct {
		name           string
		initialStatus  appsv1alpha1.CloneSetStatus
		newCondition   *appsv1alpha1.CloneSetCondition
		expectedStatus appsv1alpha1.CloneSetStatus
	}{
		{
			name: "Add new condition",
			initialStatus: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{},
			},
			newCondition: NewCloneSetCondition(condType, v1.ConditionTrue, appsv1alpha1.CloneSetAvailable, "test", now),
			expectedStatus: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               condType,
						Status:             v1.ConditionTrue,
						LastUpdateTime:     metav1.NewTime(now),
						LastTransitionTime: metav1.NewTime(now),
						Reason:             string(appsv1alpha1.CloneSetAvailable),
						Message:            "test",
					},
				},
			},
		},
		{
			name: "Update existing condition with different status",
			initialStatus: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               condType,
						Status:             v1.ConditionTrue,
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Minute)),
						LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
						Reason:             "old",
						Message:            "old",
					},
				},
			},
			newCondition: NewCloneSetCondition(condType, v1.ConditionFalse, appsv1alpha1.CloneSetProgressDeadlineExceeded, "new", now.Add(time.Second)),
			expectedStatus: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               condType,
						Status:             v1.ConditionFalse,
						LastUpdateTime:     metav1.NewTime(now.Add(time.Second)),
						LastTransitionTime: metav1.NewTime(now.Add(time.Second)),
						Reason:             string(appsv1alpha1.CloneSetProgressDeadlineExceeded),
						Message:            "new",
					},
				},
			},
		},
		{
			name: "Update existing condition with same condition",
			initialStatus: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               condType,
						Status:             v1.ConditionTrue,
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Minute)),
						LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
						Reason:             string(appsv1alpha1.CloneSetAvailable),
						Message:            "",
					},
				},
			},
			newCondition: NewCloneSetCondition(condType, v1.ConditionTrue, appsv1alpha1.CloneSetAvailable, "", now.Add(time.Second)),
			expectedStatus: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               condType,
						Status:             v1.ConditionTrue,
						LastUpdateTime:     metav1.NewTime(now.Add(time.Second)),
						LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
						Reason:             string(appsv1alpha1.CloneSetAvailable),
						Message:            "",
					},
				},
			},
		},
		{
			name: "Update existing condition with same status",
			initialStatus: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               condType,
						Status:             v1.ConditionTrue,
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Minute)),
						LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute)),
						Reason:             string(appsv1alpha1.CloneSetProgressPartitionAvailable),
						Message:            "",
					},
				},
			},
			newCondition: NewCloneSetCondition(condType, v1.ConditionTrue, appsv1alpha1.CloneSetAvailable, "", now.Add(time.Second)),
			expectedStatus: appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:               condType,
						Status:             v1.ConditionTrue,
						LastUpdateTime:     metav1.NewTime(now.Add(time.Second)),
						LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute)),
						Reason:             string(appsv1alpha1.CloneSetAvailable),
						Message:            "",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetCloneSetCondition(&tt.initialStatus, *tt.newCondition)
			if len(tt.initialStatus.Conditions) != len(tt.expectedStatus.Conditions) {
				t.Errorf("Condition count mismatch: got %d, want %d", len(tt.initialStatus.Conditions), len(tt.expectedStatus.Conditions))
			}
			if len(tt.initialStatus.Conditions) > 0 {
				got := tt.initialStatus.Conditions[0]
				want := tt.expectedStatus.Conditions[0]
				if got.Type != want.Type || got.Status != want.Status || got.Reason != want.Reason || got.Message != want.Message {
					t.Errorf("Condition mismatch: got %+v, want %+v", got, want)
				}
			}
		})
	}
}

func TestCloneSetProgressing(t *testing.T) {
	now := time.Now()
	cs := &appsv1alpha1.CloneSet{
		Spec: appsv1alpha1.CloneSetSpec{
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Paused: false,
			},
		},
		Status: appsv1alpha1.CloneSetStatus{
			Conditions: []appsv1alpha1.CloneSetCondition{
				{
					Type:               appsv1alpha1.CloneSetConditionTypeProgressing,
					Status:             v1.ConditionTrue,
					LastUpdateTime:     metav1.NewTime(now),
					LastTransitionTime: metav1.NewTime(now),
					Reason:             string(appsv1alpha1.CloneSetAvailable),
				},
			},
			UpdatedReplicas:         1,
			ExpectedUpdatedReplicas: 2,
			ReadyReplicas:           1,
			AvailableReplicas:       1,
		},
	}

	newStatus := &appsv1alpha1.CloneSetStatus{
		UpdatedReplicas:         1,
		ExpectedUpdatedReplicas: 2,
		ReadyReplicas:           2,
		AvailableReplicas:       2,
	}

	tests := []struct {
		name         string
		updateSpec   func(*appsv1alpha1.CloneSet)
		updateStatus func(*appsv1alpha1.CloneSet)
		wantResult   bool
	}{
		{
			name: "Paused strategy",
			updateSpec: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.UpdateStrategy.Paused = true
			},
			wantResult: false,
		},
		{
			name: "No progressing condition",
			updateStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = nil
			},
			wantResult: true,
		},
		{
			name: "Resumed from pause",
			updateStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions[0].Reason = string(appsv1alpha1.CloneSetProgressPaused)
			},
			wantResult: true,
		},
		{
			name: "Status changes",
			updateStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.UpdatedReplicas = 0
			},
			wantResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset test case
			testCS := cs.DeepCopy()
			if tt.updateSpec != nil {
				tt.updateSpec(testCS)
			}
			if tt.updateStatus != nil {
				tt.updateStatus(testCS)
			}

			got := CloneSetProgressing(testCS, newStatus)
			if got != tt.wantResult {
				t.Errorf("CloneSetProgressing() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestCloneSetDeadlineExceeded(t *testing.T) {
	now := time.Now()
	cs := &appsv1alpha1.CloneSet{
		Spec: appsv1alpha1.CloneSetSpec{
			ProgressDeadlineSeconds: ptr.To(int32(600)),
		},
		Status: appsv1alpha1.CloneSetStatus{
			Conditions: []appsv1alpha1.CloneSetCondition{
				{
					Type:           appsv1alpha1.CloneSetConditionTypeProgressing,
					Status:         v1.ConditionTrue,
					LastUpdateTime: metav1.NewTime(now.Add(-700 * time.Second)),
					Reason:         "other",
				},
			},
		},
	}

	tests := []struct {
		name         string
		modifyStatus func(*appsv1alpha1.CloneSet)
		wantTimedOut bool
	}{
		{
			name:         "Deadline exceeded",
			wantTimedOut: true,
		},
		{
			name: "Already exceeded",
			modifyStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions[0].Reason = string(appsv1alpha1.CloneSetProgressDeadlineExceeded)
			},
			wantTimedOut: true,
		},
		{
			name: "Already available",
			modifyStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions[0].Reason = string(appsv1alpha1.CloneSetAvailable)
			},
			wantTimedOut: false,
		},
		{
			name: "Recently updated",
			modifyStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions[0].LastUpdateTime = metav1.NewTime(now)
			},
			wantTimedOut: false,
		},
		{
			name: "No progressing condition",
			modifyStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = nil
			},
			wantTimedOut: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCS := cs.DeepCopy()
			if tt.modifyStatus != nil {
				tt.modifyStatus(testCS)
			}
			if got := CloneSetDeadlineExceeded(testCS, now); got != tt.wantTimedOut {
				t.Errorf("CloneSetDeadlineExceeded() = %v, want %v", got, tt.wantTimedOut)
			}
		})
	}
}

func TestCloneSetAvailable(t *testing.T) {
	replicas := int32(3)
	cs := &appsv1alpha1.CloneSet{
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1alpha1.CloneSetStatus{
			CurrentRevision:          "1",
			UpdateRevision:           "1",
			Replicas:                 replicas,
			UpdatedReplicas:          replicas,
			UpdatedAvailableReplicas: replicas,
		},
	}
	newStatus := &appsv1alpha1.CloneSetStatus{
		CurrentRevision:          "1",
		UpdateRevision:           "1",
		Replicas:                 replicas,
		UpdatedReplicas:          replicas,
		UpdatedAvailableReplicas: replicas,
	}

	tests := []struct {
		name           string
		modifyStatus   func(*appsv1alpha1.CloneSetStatus)
		expectedResult bool
	}{
		{
			name:           "All conditions met",
			modifyStatus:   func(status *appsv1alpha1.CloneSetStatus) {},
			expectedResult: true,
		},
		{
			name: "CurrentRevision != UpdateRevision",
			modifyStatus: func(status *appsv1alpha1.CloneSetStatus) {
				status.CurrentRevision = "2"
			},
			expectedResult: false,
		},
		{
			name: "Replicas != Spec.Replicas",
			modifyStatus: func(status *appsv1alpha1.CloneSetStatus) {
				status.Replicas = 2
			},
			expectedResult: false,
		},
		{
			name: "UpdatedReplicas != Spec.Replicas",
			modifyStatus: func(status *appsv1alpha1.CloneSetStatus) {
				status.UpdatedReplicas = 2
			},
			expectedResult: false,
		},
		{
			name: "UpdatedAvailableReplicas != Spec.Replicas",
			modifyStatus: func(status *appsv1alpha1.CloneSetStatus) {
				status.UpdatedAvailableReplicas = 2
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testStatus := newStatus.DeepCopy()
			tt.modifyStatus(testStatus)
			if got := CloneSetAvailable(cs, testStatus); got != tt.expectedResult {
				t.Errorf("CloneSetAvailable() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}

func TestCloneSetPartitionAvailable(t *testing.T) {
	cs := &appsv1alpha1.CloneSet{
		Spec: appsv1alpha1.CloneSetSpec{
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Paused: false,
			},
		},
	}
	newStatus := &appsv1alpha1.CloneSetStatus{
		ExpectedUpdatedReplicas:  2,
		UpdatedAvailableReplicas: 2,
	}

	tests := []struct {
		name           string
		modifySpec     func(*appsv1alpha1.CloneSet)
		modifyStatus   func(*appsv1alpha1.CloneSetStatus)
		expectedResult bool
	}{
		{
			name:           "Not paused, expected <= available",
			modifySpec:     func(cs *appsv1alpha1.CloneSet) {},
			modifyStatus:   func(status *appsv1alpha1.CloneSetStatus) {},
			expectedResult: true,
		},
		{
			name: "Paused",
			modifySpec: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.UpdateStrategy.Paused = true
			},
			modifyStatus:   func(status *appsv1alpha1.CloneSetStatus) {},
			expectedResult: false,
		},
		{
			name:           "Expected > Available",
			modifySpec:     func(cs *appsv1alpha1.CloneSet) {},
			modifyStatus:   func(status *appsv1alpha1.CloneSetStatus) { status.ExpectedUpdatedReplicas = 3 },
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCS := cs.DeepCopy()
			tt.modifySpec(testCS)
			testStatus := newStatus.DeepCopy()
			tt.modifyStatus(testStatus)
			if got := CloneSetPartitionAvailable(testCS, testStatus); got != tt.expectedResult {
				t.Errorf("CloneSetPartitionAvailable() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}

func TestCloneSetPaused(t *testing.T) {
	cs := &appsv1alpha1.CloneSet{
		Spec: appsv1alpha1.CloneSetSpec{
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Paused: true,
			},
		},
		Status: appsv1alpha1.CloneSetStatus{
			Conditions: []appsv1alpha1.CloneSetCondition{
				{
					Type:   appsv1alpha1.CloneSetConditionTypeProgressing,
					Reason: string(appsv1alpha1.CloneSetProgressPaused),
				},
			},
		},
	}

	tests := []struct {
		name           string
		modifySpec     func(*appsv1alpha1.CloneSet)
		modifyStatus   func(*appsv1alpha1.CloneSet)
		expectedResult bool
	}{
		{
			name: "Paused with ProgressPaused reason",
			modifySpec: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.UpdateStrategy.Paused = true
			},
			modifyStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions[0].Reason = string(appsv1alpha1.CloneSetProgressPaused)
			},
			expectedResult: false,
		},
		{
			name: "Paused without ProgressPaused reason",
			modifySpec: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.UpdateStrategy.Paused = true
			},
			modifyStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions[0].Reason = "OtherReason"
			},
			expectedResult: true,
		},
		{
			name: "Not paused",
			modifySpec: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.UpdateStrategy.Paused = false
			},
			modifyStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = nil
			},
			expectedResult: false,
		},
		{
			name: "Paused with no conditions",
			modifySpec: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.UpdateStrategy.Paused = true
			},
			modifyStatus: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = nil
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCS := cs.DeepCopy()
			tt.modifySpec(testCS)
			tt.modifyStatus(testCS)
			if got := CloneSetShouldBePaused(testCS); got != tt.expectedResult {
				t.Errorf("CloneSetPaused() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}

func TestCloneSetPartitionAvailableWithZeroExpected(t *testing.T) {
	cs := &appsv1alpha1.CloneSet{
		Spec: appsv1alpha1.CloneSetSpec{
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Paused: false,
			},
		},
	}
	newStatus := &appsv1alpha1.CloneSetStatus{
		ExpectedUpdatedReplicas:  0,
		UpdatedAvailableReplicas: 0,
	}
	if got := CloneSetPartitionAvailable(cs, newStatus); !got {
		t.Errorf("CloneSetPartitionAvailable() = %v, want true when ExpectedUpdatedReplicas is 0", got)
	}
}

func TestRemoveCloneSetCondition(t *testing.T) {
	condType := appsv1alpha1.CloneSetConditionTypeProgressing
	tests := []struct {
		name           string
		initialStatus  *appsv1alpha1.CloneSetStatus
		removeType     appsv1alpha1.CloneSetConditionType
		expectedStatus *appsv1alpha1.CloneSetStatus
	}{
		{
			name:           "Nil Status",
			initialStatus:  nil,
			removeType:     condType,
			expectedStatus: nil,
		},
		{
			name: "Condition exists",
			initialStatus: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:   appsv1alpha1.CloneSetConditionTypeProgressing,
						Status: v1.ConditionTrue,
					},
					{
						Type:   appsv1alpha1.CloneSetConditionFailedUpdate,
						Status: v1.ConditionFalse,
					},
				},
			},
			removeType: condType,
			expectedStatus: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:   appsv1alpha1.CloneSetConditionFailedUpdate,
						Status: v1.ConditionFalse,
					},
				},
			},
		},
		{
			name: "Condition does not exist",
			initialStatus: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:   appsv1alpha1.CloneSetConditionFailedUpdate,
						Status: v1.ConditionFalse,
					},
				},
			},
			removeType: condType,
			expectedStatus: &appsv1alpha1.CloneSetStatus{
				Conditions: []appsv1alpha1.CloneSetCondition{
					{
						Type:   appsv1alpha1.CloneSetConditionFailedUpdate,
						Status: v1.ConditionFalse,
					},
				},
			},
		},
		{
			name: "Empty Conditions",
			initialStatus: &appsv1alpha1.CloneSetStatus{
				Conditions: nil,
			},
			removeType: condType,
			expectedStatus: &appsv1alpha1.CloneSetStatus{
				Conditions: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RemoveCloneSetCondition(tt.initialStatus, tt.removeType)
			if tt.initialStatus == nil {
				if tt.expectedStatus != nil {
					t.Errorf("Expected nil status, got %+v", tt.expectedStatus)
				}
			} else {
				if len(tt.initialStatus.Conditions) != len(tt.expectedStatus.Conditions) {
					t.Errorf("Expected %d conditions, got %d", len(tt.expectedStatus.Conditions), len(tt.initialStatus.Conditions))
					return
				}
				for i := range tt.expectedStatus.Conditions {
					if tt.initialStatus.Conditions[i].Type != tt.expectedStatus.Conditions[i].Type ||
						tt.initialStatus.Conditions[i].Status != tt.expectedStatus.Conditions[i].Status {
						t.Errorf("Condition mismatch: got %+v, want %+v",
							tt.initialStatus.Conditions[i], tt.expectedStatus.Conditions[i])
					}
				}
			}
		})
	}
}

func TestCloneSetAlreadyTimeoutOrAvailable(t *testing.T) {
	now := time.Now()
	cs := &appsv1alpha1.CloneSet{
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: ptr.To(int32(3)),
		},
		Status: appsv1alpha1.CloneSetStatus{
			UpdateRevision: "revision-1",
		},
	}

	newStatus := &appsv1alpha1.CloneSetStatus{
		UpdateRevision: "revision-1",
	}

	tests := []struct {
		name           string
		modifyCS       func(*appsv1alpha1.CloneSet)
		modifyStatus   func(*appsv1alpha1.CloneSetStatus)
		expectedResult bool
	}{
		{
			name: "Condition is nil",
			modifyCS: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = nil
			},
			modifyStatus:   func(status *appsv1alpha1.CloneSetStatus) {},
			expectedResult: false,
		},
		{
			name: "Update revision changed",
			modifyCS: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = []appsv1alpha1.CloneSetCondition{
					{
						Type:   appsv1alpha1.CloneSetConditionTypeProgressing,
						Status: v1.ConditionTrue,
						Reason: string(appsv1alpha1.CloneSetProgressUpdated),
					},
				}
			},
			modifyStatus: func(status *appsv1alpha1.CloneSetStatus) {
				status.UpdateRevision = "revision-2"
			},
			expectedResult: false,
		},
		{
			name: "Timeout CloneSet",
			modifyCS: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = []appsv1alpha1.CloneSetCondition{
					{
						Type:           appsv1alpha1.CloneSetConditionTypeProgressing,
						Status:         v1.ConditionFalse,
						Reason:         string(appsv1alpha1.CloneSetProgressDeadlineExceeded),
						LastUpdateTime: metav1.NewTime(now),
					},
				}
			},
			modifyStatus:   func(status *appsv1alpha1.CloneSetStatus) {},
			expectedResult: true,
		},
		{
			name: "Available CloneSet",
			modifyCS: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = []appsv1alpha1.CloneSetCondition{
					{
						Type:           appsv1alpha1.CloneSetConditionTypeProgressing,
						Status:         v1.ConditionTrue,
						Reason:         string(appsv1alpha1.CloneSetAvailable),
						LastUpdateTime: metav1.NewTime(now),
					},
				}
			},
			modifyStatus: func(status *appsv1alpha1.CloneSetStatus) {
				status.CurrentRevision = "revision-1"
				status.UpdateRevision = "revision-1"
			},
			expectedResult: true,
		},
		{
			name: "Available CloneSet with different current revision",
			modifyCS: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = []appsv1alpha1.CloneSetCondition{
					{
						Type:           appsv1alpha1.CloneSetConditionTypeProgressing,
						Status:         v1.ConditionTrue,
						Reason:         string(appsv1alpha1.CloneSetAvailable),
						LastUpdateTime: metav1.NewTime(now),
					},
				}
			},
			modifyStatus: func(status *appsv1alpha1.CloneSetStatus) {
				status.CurrentRevision = "revision-2"
				status.UpdateRevision = "revision-1"
			},
			expectedResult: false,
		},
		{
			name: "Other condition reason",
			modifyCS: func(cs *appsv1alpha1.CloneSet) {
				cs.Status.Conditions = []appsv1alpha1.CloneSetCondition{
					{
						Type:           appsv1alpha1.CloneSetConditionTypeProgressing,
						Status:         v1.ConditionTrue,
						Reason:         string(appsv1alpha1.CloneSetProgressUpdated),
						LastUpdateTime: metav1.NewTime(now),
					},
				}
			},
			modifyStatus:   func(status *appsv1alpha1.CloneSetStatus) {},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCS := cs.DeepCopy()
			tt.modifyCS(testCS)
			testStatus := newStatus.DeepCopy()
			tt.modifyStatus(testStatus)

			if got := CloneSetAlreadyTimeoutOrAvailable(testCS, testStatus); got != tt.expectedResult {
				t.Errorf("CloneSetAlreadyTimeoutOrAvailable() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}
