package scale

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"
	utilpointer "k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetOrGenAvailableIDs(t *testing.T) {
	pods := []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "a"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "b"},
			},
		},
	}

	pvcs := []*v1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "b"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "c"},
			},
		},
	}

	gotIDs := getOrGenAvailableIDs(2, pods, pvcs)
	if gotIDs.Len() != 2 {
		t.Fatalf("expected got 2")
	}

	if !gotIDs.Has("c") {
		t.Fatalf("expected got c")
	}

	gotIDs.Delete("c")
	if id, _ := gotIDs.PopAny(); len(id) != 5 {
		t.Fatalf("expected got random id, but actually %v", id)
	}
}

func TestCalculateDiffs(t *testing.T) {
	intOrStr0 := intstr.FromInt(0)
	intOrStr1 := intstr.FromInt(1)
	intOrStr2 := intstr.FromInt(2)

	cases := []struct {
		name                   string
		cs                     *appsv1alpha1.CloneSet
		revConsistent          bool
		totalPods              int
		notUpdatedPods         int
		expectedTotalDiff      int
		expectedCurrentRevDiff int
	}{
		{
			name: "normal 1",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr0,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          true,
			totalPods:              10,
			notUpdatedPods:         0,
			expectedTotalDiff:      0,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "normal 2",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          true,
			totalPods:              10,
			notUpdatedPods:         0,
			expectedTotalDiff:      0,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "scale out with revConsistent",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          true,
			totalPods:              8,
			notUpdatedPods:         0,
			expectedTotalDiff:      -2,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "scale out without revConsistent 1",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr0,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          false,
			totalPods:              8,
			notUpdatedPods:         0,
			expectedTotalDiff:      -2,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "scale out without revConsistent 2",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          false,
			totalPods:              8,
			notUpdatedPods:         1,
			expectedTotalDiff:      -3,
			expectedCurrentRevDiff: 1,
		},
		{
			name: "scale out without revConsistent 3",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(1),
					},
				},
			},
			revConsistent:          false,
			totalPods:              10,
			notUpdatedPods:         1,
			expectedTotalDiff:      0,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "scale out without revConsistent 4",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          false,
			totalPods:              11,
			notUpdatedPods:         1,
			expectedTotalDiff:      0,
			expectedCurrentRevDiff: 1,
		},
		{
			name: "scale out without revConsistent 5",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(8),
					},
				},
			},
			revConsistent:          false,
			totalPods:              8,
			notUpdatedPods:         6,
			expectedTotalDiff:      -2,
			expectedCurrentRevDiff: -2,
		},
		{
			name: "scale out with maxSurge < currentRevDiff",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(1),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr2,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          false,
			totalPods:              1,
			notUpdatedPods:         1,
			expectedTotalDiff:      -1,
			expectedCurrentRevDiff: 1,
		},
		{
			name: "scale in with revConsistent 1",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr0,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          true,
			totalPods:              11,
			notUpdatedPods:         0,
			expectedTotalDiff:      1,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "scale in with revConsistent 2",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          true,
			totalPods:              11,
			notUpdatedPods:         0,
			expectedTotalDiff:      1,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "scale in without revConsistent 1",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr0,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          false,
			totalPods:              11,
			notUpdatedPods:         0,
			expectedTotalDiff:      1,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "scale in without revConsistent 2",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          false,
			totalPods:              11,
			notUpdatedPods:         0,
			expectedTotalDiff:      1,
			expectedCurrentRevDiff: 0,
		},
		{
			name: "scale in without revConsistent 3",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          false,
			totalPods:              11,
			notUpdatedPods:         1,
			expectedTotalDiff:      0,
			expectedCurrentRevDiff: 1,
		},
		{
			name: "scale in without revConsistent 4",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(0),
					},
				},
			},
			revConsistent:          false,
			totalPods:              12,
			notUpdatedPods:         2,
			expectedTotalDiff:      1,
			expectedCurrentRevDiff: 2,
		},
		{
			name: "scale in without revConsistent 5",
			cs: &appsv1alpha1.CloneSet{
				Spec: appsv1alpha1.CloneSetSpec{
					Replicas: utilpointer.Int32Ptr(10),
					UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
						MaxSurge:  &intOrStr1,
						Partition: utilpointer.Int32Ptr(6),
					},
				},
			},
			revConsistent:          false,
			totalPods:              13,
			notUpdatedPods:         7,
			expectedTotalDiff:      2,
			expectedCurrentRevDiff: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(*testing.T) {
			totalDiff, currentRevDiff := calculateDiffs(tc.cs, tc.revConsistent, tc.totalPods, tc.notUpdatedPods)
			if totalDiff != tc.expectedTotalDiff || currentRevDiff != tc.expectedCurrentRevDiff {
				t.Fatalf("failed calculateDiffs, expected totalDiff %d, got %d, expected currentRevDiff %d, got %d",
					tc.expectedTotalDiff, totalDiff, tc.expectedCurrentRevDiff, currentRevDiff)
			}
		})
	}
}
