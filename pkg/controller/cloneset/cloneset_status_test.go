package cloneset

import (
	"math"
	"testing"
	"time"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/clock"
	testingclock "k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
)

func TestSyncProgressingStatus(t *testing.T) {
	progressDeadlineSeconds := ptr.To(int32(10))

	timeFn := func(sec int64, nsec int64) time.Time { return time.Unix(sec, nsec) }

	oldTime := metav1.NewTime(time.Date(2025, 7, 20, 11, 0, 0, 0, time.Local))
	newStatus := func(replicas, readyReplicas, availableReplicas, updatedReplicas, updatedReadyReplicas,
		updatedAvailableReplicas, expectedUpdatedReplicas int32, currentRevision, updateRevision string) *appsv1beta1.CloneSetStatus {
		return &appsv1beta1.CloneSetStatus{
			Replicas:                 replicas,
			ReadyReplicas:            readyReplicas,
			AvailableReplicas:        availableReplicas,
			UpdatedReplicas:          updatedReplicas,
			UpdatedReadyReplicas:     updatedReadyReplicas,
			UpdatedAvailableReplicas: updatedAvailableReplicas,
			ExpectedUpdatedReplicas:  expectedUpdatedReplicas,
			CurrentRevision:          currentRevision,
			UpdateRevision:           updateRevision,
		}
	}

	tests := []struct {
		name          string
		cs            *appsv1beta1.CloneSet
		timer         clock.Clock
		newStatus     *appsv1beta1.CloneSetStatus
		wantCond      *appsv1beta1.CloneSetCondition
		expectEnqueue time.Duration
	}{
		{
			name: "legacy cs starts with nil pds, should remove the condition",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: nil, Replicas: ptr.To(int32(10))},
				Status: appsv1beta1.CloneSetStatus{
					Conditions:      nil,
					CurrentRevision: "1",
					UpdateRevision:  "1",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus:     newStatus(5, 4, 3, 2, 1, 1, 2, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "legacy cs starts with MaxInt32 pds, should remove the condition",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: ptr.To(int32(math.MaxInt32)), Replicas: ptr.To(int32(10))},
				Status: appsv1beta1.CloneSetStatus{
					Conditions:      nil,
					CurrentRevision: "1",
					UpdateRevision:  "1",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus:     newStatus(5, 4, 3, 2, 1, 1, 2, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "legacy cs starts deploying",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: progressDeadlineSeconds, Replicas: ptr.To(int32(10))},
				Status: appsv1beta1.CloneSetStatus{
					Conditions:      nil,
					CurrentRevision: "1",
					UpdateRevision:  "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus: newStatus(5, 4, 3, 2, 1, 1, 2, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is progressing",
				LastUpdateTime:     metav1.Now(),
				LastTransitionTime: metav1.Now(),
			},
			expectEnqueue: 11 * time.Second,
		},
		{
			name: "legacy cs starts paused",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: progressDeadlineSeconds, Replicas: ptr.To(int32(10)), UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{Paused: true}},
				Status: appsv1beta1.CloneSetStatus{
					Conditions:      nil,
					CurrentRevision: "1",
					UpdateRevision:  "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus: newStatus(5, 4, 3, 2, 1, 1, 2, "1", "1"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPaused),
				Message:            "CloneSet is paused",
				LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "legacy cs is available",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: progressDeadlineSeconds, Replicas: ptr.To(int32(10))},
				Status: appsv1beta1.CloneSetStatus{
					Conditions:      nil,
					CurrentRevision: "1",
					UpdateRevision:  "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus: newStatus(10, 10, 10, 10, 10, 10, 10, "1", "1"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetAvailable),
				Message:            "CloneSet is available",
				LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "legacy cs is scaling",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{Replicas: ptr.To(int32(1)), ProgressDeadlineSeconds: progressDeadlineSeconds},
				Status: appsv1beta1.CloneSetStatus{
					Replicas:                 1,
					ReadyReplicas:            1,
					AvailableReplicas:        1,
					UpdatedReplicas:          1,
					UpdatedReadyReplicas:     1,
					UpdatedAvailableReplicas: 0,
					ExpectedUpdatedReplicas:  1,
					UpdateRevision:           "2",
					CurrentRevision:          "2",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus: newStatus(10, 9, 8, 10, 9, 8, 10, "2", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is progressing",
				LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
			},
			expectEnqueue: 11 * time.Second,
		},
		{
			name: "legacy cs is paused",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{Replicas: ptr.To(int32(1)), ProgressDeadlineSeconds: progressDeadlineSeconds, UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{Paused: true}},
				Status: appsv1beta1.CloneSetStatus{
					Replicas:                 10,
					ReadyReplicas:            0,
					AvailableReplicas:        0,
					UpdatedReplicas:          1,
					UpdatedReadyReplicas:     0,
					UpdatedAvailableReplicas: 0,
					ExpectedUpdatedReplicas:  6,
					CurrentRevision:          "1",
					UpdateRevision:           "2",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus: newStatus(10, 8, 7, 6, 5, 5, 6, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPaused),
				Message:            "CloneSet is paused",
				LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "legacy cs is available, and CloneSet is paused due to partition available",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(10)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy:          appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("90%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Replicas:                 10,
					ReadyReplicas:            9,
					AvailableReplicas:        9,
					UpdatedReplicas:          1,
					UpdatedReadyReplicas:     0,
					UpdatedAvailableReplicas: 0,
					ExpectedUpdatedReplicas:  1,
					CurrentRevision:          "1",
					UpdateRevision:           "2",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus: newStatus(10, 10, 10, 1, 1, 1, 1, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
				Message:            "CloneSet has been paused due to partition ready",
				LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with CloneSetAvailable condition, and pds is updated to nil",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: nil, Replicas: ptr.To(int32(10))},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetAvailable),
							Message:            "CloneSet is available",
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					CurrentRevision: "1",
					UpdateRevision:  "1",
					Replicas:        5,
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus:     newStatus(5, 0, 0, 0, 0, 0, 0, "1", "1"),
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with CloneSetAvailable condition, and pds is updated to maxInt32",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: ptr.To(int32(math.MaxInt32)), Replicas: ptr.To(int32(10))},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetAvailable),
							Message:            "CloneSet is available",
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					CurrentRevision: "1",
					UpdateRevision:  "1",
					Replicas:        5,
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(0, 0)),
			newStatus:     newStatus(5, 0, 0, 0, 0, 0, 0, "1", "1"),
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with CloneSetAvailable condition, and CloneSet scales up replicas",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: progressDeadlineSeconds, Replicas: ptr.To(int32(10))},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetAvailable),
							Message:            "CloneSet is available",
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					CurrentRevision: "1",
					UpdateRevision:  "1",
					Replicas:        5,
				},
			},
			newStatus: newStatus(5, 5, 5, 5, 5, 5, 10, "1", "1"),
			timer:     testingclock.NewFakeClock(time.Unix(0, 0)),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is progressing",
				LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with CloneSetAvailable condition, and CloneSet scales up replicas, timeout",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: progressDeadlineSeconds, Replicas: ptr.To(int32(10))},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Status:             v1.ConditionTrue,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							Message:            "CloneSet is progressing",
							LastUpdateTime:     oldTime,
							LastTransitionTime: oldTime,
						},
					},
					CurrentRevision: "1",
					UpdateRevision:  "1",
					Replicas:        5,
				},
			},
			newStatus: &appsv1beta1.CloneSetStatus{
				Replicas:                 10,
				ReadyReplicas:            8,
				AvailableReplicas:        8,
				UpdatedReplicas:          10,
				UpdatedReadyReplicas:     8,
				UpdatedAvailableReplicas: 8,
				ExpectedUpdatedReplicas:  10,
				CurrentRevision:          "1",
				UpdateRevision:           "1",
				Conditions: []appsv1beta1.CloneSetCondition{
					{
						Type:               appsv1beta1.CloneSetConditionTypeProgressing,
						Status:             v1.ConditionTrue,
						Reason:             string(appsv1beta1.CloneSetProgressUpdated),
						Message:            "CloneSet is progressing",
						LastUpdateTime:     oldTime,
						LastTransitionTime: oldTime,
					},
				},
			},
			timer: testingclock.NewFakeClock(time.Unix(0, 0)),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionFalse,
				Reason:             string(appsv1beta1.CloneSetProgressDeadlineExceeded),
				Message:            "CloneSet revision 1 has timed out progressing",
				LastUpdateTime:     metav1.Now(),
				LastTransitionTime: metav1.Now(),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with CloneSetAvailable condition, and CloneSet reconciles with a new revision",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{ProgressDeadlineSeconds: progressDeadlineSeconds},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetAvailable),
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					CurrentRevision: "1",
					UpdateRevision:  "1",
					Replicas:        1,
				},
			},
			newStatus: newStatus(1, 0, 0, 0, 0, 0, 1, "1", "2"),
			timer:     testingclock.NewFakeClock(time.Unix(5, 0)),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is progressing",
				LastUpdateTime:     metav1.NewTime(timeFn(5, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(5, 0)),
			},
			expectEnqueue: 11 * time.Second,
		},
		{
			name: "startup with Updated condition, and pds is updated to nil",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:       ptr.To(int32(10)),
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("50%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  5,
					CurrentRevision:          "1",
					UpdateRevision:           "2",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(5, 0)),
			newStatus:     newStatus(10, 10, 10, 5, 5, 5, 5, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Updated condition, and pds is updated to maxInt32",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(10)),
					ProgressDeadlineSeconds: ptr.To(int32(math.MaxInt32)),
					UpdateStrategy:          appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("50%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  5,
					CurrentRevision:          "1",
					UpdateRevision:           "2",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(5, 0)),
			newStatus:     newStatus(10, 10, 10, 5, 5, 5, 5, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Updated condition, and CloneSet is paused due to partition available",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(10)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy:          appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("50%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  5,
					CurrentRevision:          "1",
					UpdateRevision:           "2",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(5, 0)),
			newStatus: newStatus(10, 10, 10, 5, 5, 5, 5, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
				Message:            "CloneSet has been paused due to partition ready",
				LastUpdateTime:     metav1.NewTime(timeFn(5, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(5, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Updated condition, and CloneSet is scaled down from 10 to 6, partition available",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(6)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy:          appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("10%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            9,
					AvailableReplicas:        9,
					UpdatedReplicas:          8,
					UpdatedReadyReplicas:     8,
					UpdatedAvailableReplicas: 8,
					ExpectedUpdatedReplicas:  9,
					CurrentRevision:          "1",
					UpdateRevision:           "2",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(5, 0)),
			newStatus: newStatus(10, 9, 9, 8, 8, 8, 5, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
				Message:            "CloneSet has been paused due to partition ready",
				LastUpdateTime:     metav1.NewTime(timeFn(5, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(5, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Updated condition, and CloneSet is scaled down from 10 to 6, only ExpectedUpdatedReplicas changed and not timeout",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(6)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy:          appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("10%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							LastUpdateTime:     metav1.Now(),
							LastTransitionTime: metav1.Now(),
							Status:             v1.ConditionTrue,
							Message:            "CloneSet is progressing",
						},
					},
					Replicas:                 10,
					ReadyReplicas:            9,
					AvailableReplicas:        9,
					UpdatedReplicas:          8,
					UpdatedReadyReplicas:     6,
					UpdatedAvailableReplicas: 6,
					ExpectedUpdatedReplicas:  5,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(15, 0)),
			newStatus: newStatus(10, 9, 9, 8, 6, 6, 8, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is progressing",
				LastUpdateTime:     metav1.NewTime(timeFn(9, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(9, 0)),
			},
			expectEnqueue: 5 * time.Second,
		},
		{
			name: "startup with Updated condition, and CloneSet is scaled down from 10 to 6, only ExpectedUpdatedReplicas and timeout",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(6)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy:          appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("10%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							LastUpdateTime:     oldTime,
							LastTransitionTime: oldTime,
						},
					},
					Replicas:                 6,
					ReadyReplicas:            6,
					AvailableReplicas:        6,
					UpdatedReplicas:          4,
					UpdatedReadyReplicas:     4,
					UpdatedAvailableReplicas: 4,
					ExpectedUpdatedReplicas:  6,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer: testingclock.NewFakeClock(time.Unix(15, 0)),
			newStatus: &appsv1beta1.CloneSetStatus{
				Conditions: []appsv1beta1.CloneSetCondition{
					{
						Type:               appsv1beta1.CloneSetConditionTypeProgressing,
						Reason:             string(appsv1beta1.CloneSetProgressUpdated),
						LastUpdateTime:     oldTime,
						LastTransitionTime: oldTime,
					},
				},
				Replicas:                 6,
				ReadyReplicas:            6,
				AvailableReplicas:        6,
				UpdatedReplicas:          4,
				UpdatedReadyReplicas:     4,
				UpdatedAvailableReplicas: 4,
				ExpectedUpdatedReplicas:  6,
				UpdateRevision:           "2",
				CurrentRevision:          "1",
			},
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionFalse,
				Reason:             string(appsv1beta1.CloneSetProgressDeadlineExceeded),
				Message:            "CloneSet revision 2 has timed out progressing",
				LastUpdateTime:     metav1.Now(),
				LastTransitionTime: metav1.Now(),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Updated condition, and CloneSet is scaled up from 10 to 20",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy:          appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("50%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  5,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(5, 0)),
			newStatus: newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is progressing",
				LastUpdateTime:     metav1.NewTime(timeFn(5, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(5, 0)),
			},
			expectEnqueue: 11 * time.Second,
		},
		{
			name: "startup with Updated condition, and CloneSet is timeout",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy:          appsv1beta1.CloneSetUpdateStrategy{Partition: util.GetIntOrStrPointer(intstr.FromString("50%"))},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(10, 0)),
							LastTransitionTime: metav1.NewTime(timeFn(10, 0)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer: testingclock.NewFakeClock(time.Unix(30, 0)),
			newStatus: &appsv1beta1.CloneSetStatus{
				Conditions: []appsv1beta1.CloneSetCondition{
					{
						Type:               appsv1beta1.CloneSetConditionTypeProgressing,
						Reason:             string(appsv1beta1.CloneSetProgressUpdated),
						Status:             v1.ConditionTrue,
						LastUpdateTime:     metav1.NewTime(timeFn(10, 0)),
						LastTransitionTime: metav1.NewTime(timeFn(10, 0)),
					},
				},
				Replicas:                 10,
				ReadyReplicas:            5,
				AvailableReplicas:        5,
				UpdatedReplicas:          5,
				UpdatedReadyReplicas:     3,
				UpdatedAvailableReplicas: 1,
				ExpectedUpdatedReplicas:  10,
				UpdateRevision:           "2",
				CurrentRevision:          "1",
			},
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionFalse,
				Reason:             string(appsv1beta1.CloneSetProgressDeadlineExceeded),
				Message:            "CloneSet revision 2 has timed out progressing",
				LastUpdateTime:     metav1.NewTime(timeFn(30, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(30, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Updated condition, and CloneSet is available",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("0%")),
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 10)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 10)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(9, 0)),
			newStatus: newStatus(20, 20, 20, 20, 20, 20, 20, "2", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetAvailable),
				Message:            "CloneSet is available",
				LastUpdateTime:     metav1.NewTime(timeFn(9, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(9, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Updated condition, and CloneSet is paused",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
						Paused:    true,
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(5, 0)),
							LastTransitionTime: metav1.NewTime(timeFn(5, 0)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus: newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPaused),
				Message:            "CloneSet is paused",
				LastUpdateTime:     metav1.NewTime(timeFn(8, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(8, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Paused condition, and pds is updated to nil",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas: ptr.To(int32(20)),
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
						Paused:    true,
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressPaused),
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
							Message:            "CloneSet is paused",
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus:     newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Paused condition, and pds is updated to maxInt32",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: ptr.To(int32(math.MaxInt32)),
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
						Paused:    true,
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressPaused),
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
							Message:            "CloneSet is paused",
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus:     newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Paused condition, and CloneSet is paused again",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
						Paused:    true,
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressPaused),
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
							Message:            "CloneSet is paused",
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus: newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPaused),
				Message:            "CloneSet is paused",
				LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Paused condition, and CloneSet is resumed",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressPaused),
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(10, 0)),
			newStatus: newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is resumed",
				LastUpdateTime:     metav1.NewTime(timeFn(10, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(10, 0)),
			},
			expectEnqueue: 11 * time.Second,
		},
		{
			name: "startup with DeadlineExceeded condition, and pds is updated to nil",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas: ptr.To(int32(20)),
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressDeadlineExceeded),
							Message:            "CloneSet revision 2 has timed out progressing",
							Status:             v1.ConditionFalse,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(40, 0)),
			newStatus:     newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with DeadlineExceeded condition,and pds is updated to maxInt32",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: ptr.To(int32(math.MaxInt32)),
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressDeadlineExceeded),
							Message:            "CloneSet revision 2 has timed out progressing",
							Status:             v1.ConditionFalse,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(40, 0)),
			newStatus:     newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with DeadlineExceeded condition, and CloneSet is paused",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
						Paused:    true,
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressDeadlineExceeded),
							Message:            "CloneSet revision 2 has timed out progressing",
							Status:             v1.ConditionFalse,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(40, 0)),
			newStatus: newStatus(15, 14, 14, 10, 10, 9, 10, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Reason:             string(appsv1beta1.CloneSetProgressPaused),
				Message:            "CloneSet is paused",
				Status:             v1.ConditionTrue,
				LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with DeadlineExceeded condition, and CloneSet starts a new revision",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("90%")),
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressDeadlineExceeded),
							Message:            "CloneSet revision 2 has timed out progressing",
							Status:             v1.ConditionFalse,
							LastUpdateTime:     metav1.NewTime(timeFn(0, 8)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 8)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(40, 0)),
			newStatus: newStatus(20, 10, 10, 2, 2, 2, 4, "1", "3"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is progressing",
				Status:             v1.ConditionTrue,
				LastUpdateTime:     metav1.NewTime(timeFn(40, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(40, 0)),
			},
			expectEnqueue: 11 * time.Second,
		},
		{
			name: "startup with PartitionAvailable condition, and CloneSet is paused",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(10)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
						Paused:    true,
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Status:             v1.ConditionTrue,
							Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
							Message:            "CloneSet has been paused due to partition ready",
							LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            10,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     5,
					UpdatedAvailableReplicas: 5,
					ExpectedUpdatedReplicas:  5,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus: newStatus(10, 10, 10, 5, 5, 5, 5, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPaused),
				Message:            "CloneSet is paused",
				LastUpdateTime:     metav1.NewTime(timeFn(8, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(8, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with PartitionAvailable condition, and pds is updated to nil",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas: ptr.To(int32(10)),
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
						Paused:    true,
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Status:             v1.ConditionTrue,
							Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
							Message:            "CloneSet has been paused due to partition ready",
							LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            10,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     5,
					UpdatedAvailableReplicas: 5,
					ExpectedUpdatedReplicas:  5,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus:     newStatus(10, 10, 10, 5, 5, 5, 5, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with PartitionAvailable condition, and pds is updated to maxInt32",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(10)),
					ProgressDeadlineSeconds: ptr.To(int32(math.MaxInt32)),
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
						Paused:    true,
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Status:             v1.ConditionTrue,
							Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
							Message:            "CloneSet has been paused due to partition ready",
							LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            10,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     5,
					UpdatedAvailableReplicas: 5,
					ExpectedUpdatedReplicas:  5,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:         testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus:     newStatus(10, 10, 10, 5, 5, 5, 5, "1", "2"),
			wantCond:      nil,
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with PartitionAvailable condition, and CloneSet scales up",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Status:             v1.ConditionTrue,
							Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
							Message:            "CloneSet has been paused due to partition ready",
							LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            10,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     5,
					UpdatedAvailableReplicas: 5,
					ExpectedUpdatedReplicas:  5,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus: newStatus(15, 15, 15, 6, 6, 6, 10, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				Message:            "CloneSet is progressing",
				LastUpdateTime:     metav1.NewTime(timeFn(8, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
			},
			expectEnqueue: 11 * time.Second,
		},
		{
			name: "startup with PartitionAvailable condition, and CloneSet scales up and available again",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(20)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("50%")),
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Status:             v1.ConditionTrue,
							Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
							Message:            "CloneSet has been paused due to partition ready",
							LastUpdateTime:     metav1.NewTime(timeFn(0, 0)),
							LastTransitionTime: metav1.NewTime(timeFn(0, 0)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            10,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     5,
					UpdatedAvailableReplicas: 5,
					ExpectedUpdatedReplicas:  5,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus: newStatus(20, 20, 20, 10, 10, 10, 10, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressPartitionAvailable),
				Message:            "CloneSet has been paused due to partition ready",
				LastUpdateTime:     metav1.NewTime(timeFn(8, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(8, 0)),
			},
			expectEnqueue: time.Duration(-1),
		},
		{
			name: "startup with Updated condition, and CloneSet is still progressing",
			cs: &appsv1beta1.CloneSet{
				Spec: appsv1beta1.CloneSetSpec{
					Replicas:                ptr.To(int32(10)),
					ProgressDeadlineSeconds: progressDeadlineSeconds,
					UpdateStrategy: appsv1beta1.CloneSetUpdateStrategy{
						Partition: util.GetIntOrStrPointer(intstr.FromString("0%")),
					},
				},
				Status: appsv1beta1.CloneSetStatus{
					Conditions: []appsv1beta1.CloneSetCondition{
						{
							Type:               appsv1beta1.CloneSetConditionTypeProgressing,
							Reason:             string(appsv1beta1.CloneSetProgressUpdated),
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.NewTime(timeFn(1, 0)),
							LastTransitionTime: metav1.NewTime(timeFn(1, 0)),
						},
					},
					Replicas:                 10,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          5,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 1,
					ExpectedUpdatedReplicas:  10,
					UpdateRevision:           "2",
					CurrentRevision:          "1",
				},
			},
			timer:     testingclock.NewFakeClock(time.Unix(8, 0)),
			newStatus: newStatus(10, 5, 5, 5, 3, 1, 10, "1", "2"),
			wantCond: &appsv1beta1.CloneSetCondition{
				Type:               appsv1beta1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				Reason:             string(appsv1beta1.CloneSetProgressUpdated),
				LastUpdateTime:     metav1.NewTime(timeFn(1, 0)),
				LastTransitionTime: metav1.NewTime(timeFn(1, 0)),
			},
			expectEnqueue: 4 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &realStatusUpdater{}
			_ = r.calculateProgressingStatus(tt.cs, tt.newStatus)

			var cond *appsv1beta1.CloneSetCondition
			if tt.newStatus != nil {
				cond = clonesetutils.GetCloneSetCondition(*tt.newStatus, appsv1beta1.CloneSetConditionTypeProgressing)
			}
			if tt.wantCond == nil {
				if cond != nil {
					t.Fatalf("expect(%s), but get(%s)", util.DumpJSON(tt.wantCond), util.DumpJSON(cond))
				}
			} else if cond.Status != tt.wantCond.Status || cond.Reason != tt.wantCond.Reason {
				t.Fatalf("expect(%s), but get(%s)", util.DumpJSON(tt.wantCond), util.DumpJSON(cond))
			}
		})
	}
}

func TestHasProgressingConditionChanged(t *testing.T) {
	now := time.Now()
	oldCond := &appsv1beta1.CloneSetCondition{
		Type:               appsv1beta1.CloneSetConditionTypeProgressing,
		Status:             v1.ConditionTrue,
		Reason:             string(appsv1beta1.CloneSetProgressUpdated),
		Message:            "Old message",
		LastUpdateTime:     metav1.NewTime(now),
		LastTransitionTime: metav1.NewTime(now.Add(-time.Minute)),
	}

	newCond := &appsv1beta1.CloneSetCondition{
		Type:               appsv1beta1.CloneSetConditionTypeProgressing,
		Status:             v1.ConditionTrue,
		Reason:             string(appsv1beta1.CloneSetProgressUpdated),
		Message:            "New message",
		LastUpdateTime:     metav1.NewTime(now),
		LastTransitionTime: metav1.NewTime(now),
	}

	tests := []struct {
		name           string
		oldStatus      appsv1beta1.CloneSetStatus
		newStatus      appsv1beta1.CloneSetStatus
		expectedResult bool
	}{
		{
			name:           "Both nil",
			oldStatus:      appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{}},
			newStatus:      appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{}},
			expectedResult: false,
		},
		{
			name:           "Old nil, new exists",
			oldStatus:      appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{}},
			newStatus:      appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*newCond}},
			expectedResult: true,
		},
		{
			name:           "New nil, old exists",
			oldStatus:      appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*newCond}},
			newStatus:      appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{}},
			expectedResult: true,
		},
		{
			name:           "All fields same",
			oldStatus:      appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*oldCond}},
			newStatus:      appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*oldCond}},
			expectedResult: false,
		},
		{
			name:      "Status changed",
			oldStatus: appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*oldCond}},
			newStatus: func() appsv1beta1.CloneSetStatus {
				c := *oldCond
				c.Status = v1.ConditionFalse
				return appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{c}}
			}(),
			expectedResult: true,
		},
		{
			name:      "Reason changed",
			oldStatus: appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*oldCond}},
			newStatus: func() appsv1beta1.CloneSetStatus {
				c := *oldCond
				c.Reason = string(appsv1beta1.CloneSetProgressPaused)
				return appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{c}}
			}(),
			expectedResult: true,
		},
		{
			name:      "Message changed",
			oldStatus: appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*oldCond}},
			newStatus: func() appsv1beta1.CloneSetStatus {
				c := *oldCond
				c.Message = "New message"
				return appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{c}}
			}(),
			expectedResult: false,
		},
		{
			name:      "LastUpdateTime changed",
			oldStatus: appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*oldCond}},
			newStatus: func() appsv1beta1.CloneSetStatus {
				c := *oldCond
				c.LastUpdateTime = metav1.NewTime(now.Add(time.Minute))
				return appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{c}}
			}(),
			expectedResult: false,
		},
		{
			name:      "LastTransitionTime changed",
			oldStatus: appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{*oldCond}},
			newStatus: func() appsv1beta1.CloneSetStatus {
				c := *oldCond
				c.LastTransitionTime = metav1.NewTime(now.Add(time.Minute))
				return appsv1beta1.CloneSetStatus{Conditions: []appsv1beta1.CloneSetCondition{c}}
			}(),
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasProgressingConditionChanged(tt.oldStatus, tt.newStatus); got != tt.expectedResult {
				t.Errorf("hasProgressingConditionChanged() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}

func TestGetRequeueSecondsFromCondition(t *testing.T) {
	now := time.Now()
	condition := &appsv1beta1.CloneSetCondition{
		LastUpdateTime: metav1.NewTime(now.Add(-5 * time.Second)),
	}

	tests := []struct {
		name           string
		condition      *appsv1beta1.CloneSetCondition
		pds            int32
		now            time.Time
		expectedResult time.Duration
	}{
		{
			name:           "Condition nil",
			condition:      nil,
			pds:            10,
			now:            now,
			expectedResult: -1,
		},
		{
			name:           "Deadline not exceeded",
			condition:      condition,
			pds:            10,
			now:            now,
			expectedResult: 6 * time.Second,
		},
		{
			name:           "Deadline exceeded",
			condition:      condition,
			pds:            4,
			now:            now,
			expectedResult: -1 * time.Second,
		},
		{
			name: "Exactly at deadline",
			condition: &appsv1beta1.CloneSetCondition{
				LastUpdateTime: metav1.NewTime(now.Add(-4 * time.Second)),
			},
			pds:            4,
			now:            now,
			expectedResult: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getRequeueSecondsFromCondition(tt.condition, tt.pds, tt.now); got != tt.expectedResult {
				t.Errorf("getRequeueSecondsFromCondition() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}
