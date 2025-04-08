/*
Copyright 2019 The Kruise Authors.

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

package uniteddeployment

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

func sortToAllocator(infos subsetInfos) *specificAllocator {
	sort.Sort(infos)
	return &specificAllocator{subsets: &infos}
}

func TestScaleReplicas(t *testing.T) {
	infos := subsetInfos{
		createSubset("t1", 1),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator := sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(5, &map[string]int32{})
	if " t1 -> 1; t3 -> 1; t4 -> 1; t2 -> 2;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(0, &map[string]int32{})
	if " t1 -> 0;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 1),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(17, &map[string]int32{})
	if " t1 -> 4; t3 -> 4; t4 -> 4; t2 -> 5;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 2),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(9, &map[string]int32{})
	if " t1 -> 2; t3 -> 2; t4 -> 2; t2 -> 3;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 0),
		createSubset("t2", 10),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(19, &map[string]int32{})
	if " t1 -> 9; t2 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 0),
		createSubset("t2", 10),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(21, &map[string]int32{})
	if " t1 -> 10; t2 -> 11;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}
}

func TestSpecifyValidReplicas(t *testing.T) {
	infos := subsetInfos{
		createSubset("t1", 1),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator := sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(27, &map[string]int32{
		"t1": 4,
		"t3": 4,
	})
	if " t1 -> 4; t3 -> 4; t4 -> 9; t2 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 2),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(8, &map[string]int32{
		"t1": 4,
		"t3": 4,
	})
	if " t2 -> 0; t4 -> 0; t1 -> 4; t3 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 2),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(16, &map[string]int32{
		"t1": 4,
		"t2": 4,
		"t3": 4,
		"t4": 4,
	})
	if " t1 -> 4; t2 -> 4; t3 -> 4; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 4),
		createSubset("t2", 2),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(10, &map[string]int32{
		"t1": 1,
		"t2": 2,
		"t3": 3,
	})
	if " t1 -> 1; t2 -> 2; t3 -> 3; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 4),
		createSubset("t2", 2),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(10, &map[string]int32{
		"t1": 1,
		"t2": 2,
		"t3": 3,
		"t4": 4,
	})
	if " t1 -> 1; t2 -> 2; t3 -> 3; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 4),
		createSubset("t2", 2),
		createSubset("t3", 2),
		createSubset("t4", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(-1, &map[string]int32{
		"t1": 1,
		"t2": 2,
		"t3": 3,
		"t4": 4,
	})
	if " t1 -> 1; t2 -> 2; t3 -> 3; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}
}

func TestSpecifyInvalidReplicas(t *testing.T) {
	infos := subsetInfos{
		createSubset("t1", 10),
		createSubset("t2", 4),
	}
	allocator := sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(14, &map[string]int32{
		"t1": 6,
		"t2": 6,
	})
	if " t2 -> 4; t1 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 10),
		createSubset("t2", 4),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(14, &map[string]int32{
		"t1": 10,
		"t2": 11,
	})
	if " t2 -> 4; t1 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}
}

func TestCapacityAllocator(t *testing.T) {
	cases := []struct {
		name            string
		replicas        int32
		minReplicas     []int32
		maxReplicas     []int32
		desiredReplicas []int32
	}{
		{
			name:     "sum_all_min_replicas == replicas",
			replicas: 10,
			minReplicas: []int32{
				2, 2, 2, 2, 2,
			},
			maxReplicas: []int32{
				5, 5, 5, 5, -1,
			},
			desiredReplicas: []int32{
				2, 2, 2, 2, 2,
			},
		},
		{
			name:     "sum_all_min_replicas < replicas",
			replicas: 14,
			minReplicas: []int32{
				2, 2, 2, 2, 2,
			},
			maxReplicas: []int32{
				5, 5, 5, 5, -1,
			},
			desiredReplicas: []int32{
				5, 3, 2, 2, 2,
			},
		},
		{
			name:     "sum_all_min_replicas > replicas",
			replicas: 5,
			minReplicas: []int32{
				2, 2, 2, 2, 2,
			},
			maxReplicas: []int32{
				5, 5, 5, 5, -1,
			},
			desiredReplicas: []int32{
				2, 2, 1, 0, 0,
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			ud := appsv1alpha1.UnitedDeployment{}
			ud.Spec.Replicas = pointer.Int32(cs.replicas)
			ud.Spec.Topology.Subsets = []appsv1alpha1.Subset{}
			for index := range cs.minReplicas {
				minReplicas := intstr.FromInt32(cs.minReplicas[index])
				var maxReplicas *intstr.IntOrString
				if cs.maxReplicas[index] != -1 {
					m := intstr.FromInt32(cs.maxReplicas[index])
					maxReplicas = &m
				}
				ud.Spec.Topology.Subsets = append(ud.Spec.Topology.Subsets, appsv1alpha1.Subset{
					Name:        fmt.Sprintf("subset-%d", index),
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
				})
			}

			ca := elasticAllocator{&ud}
			result, err := ca.Alloc(nil)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			for index := range cs.desiredReplicas {
				if (*result)[fmt.Sprintf("subset-%d", index)] != cs.desiredReplicas[index] {
					t.Fatalf("unexpected result %v", result)
				}
			}
		})
	}
}

func TestAdaptiveElasticAllocator(t *testing.T) {
	getUnitedDeploymentAndSubsets := func(totalReplicas, minReplicas, maxReplicas, pendingPods int32) (
		*appsv1alpha1.UnitedDeployment, map[string]*Subset) {
		minR, maxR := intstr.FromInt32(minReplicas), intstr.FromInt32(maxReplicas)
		return &appsv1alpha1.UnitedDeployment{
				Spec: appsv1alpha1.UnitedDeploymentSpec{
					Replicas: &totalReplicas,
					Topology: appsv1alpha1.Topology{
						Subsets: []appsv1alpha1.Subset{
							{
								Name:        "subset-1",
								MinReplicas: &minR,
								MaxReplicas: &maxR,
							},
							{
								Name: "subset-2",
							},
						},
						ScheduleStrategy: appsv1alpha1.UnitedDeploymentScheduleStrategy{
							Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
						},
					},
				},
			}, map[string]*Subset{
				"subset-1": {
					Status: SubsetStatus{
						UnschedulableStatus: SubsetUnschedulableStatus{
							Unschedulable: true,
							PendingPods:   pendingPods,
						},
						Replicas:             maxReplicas,
						UpdatedReadyReplicas: maxReplicas - pendingPods,
						ReadyReplicas:        maxReplicas - pendingPods,
					},
					Spec: SubsetSpec{
						Replicas:   maxReplicas,
						SubsetPods: generateSubsetPods(maxReplicas, pendingPods, 0),
					},
				},
			}
	}
	cases := []struct {
		name                                                     string
		totalReplicas, minReplicas, maxReplicas, unavailablePods int32
		subset1Replicas, subset2Replicas                         int32
	}{
		{
			name:          "5 pending pods > maxReplicas -> 0, 10",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, unavailablePods: 5,
			subset1Replicas: 0, subset2Replicas: 10,
		},
		{
			name:          "4 pending pods = maxReplicas -> 0, 10",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, unavailablePods: 4,
			subset1Replicas: 0, subset2Replicas: 10,
		},
		{
			name:          "3 pending pods < maxReplicas -> 1, 9",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, unavailablePods: 3,
			subset1Replicas: 1, subset2Replicas: 9,
		},
		{
			name:          "2 pending pods = minReplicas -> 2, 8",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, unavailablePods: 2,
			subset1Replicas: 2, subset2Replicas: 8,
		},
		{
			name:          "1 pending pods < minReplicas -> 3, 7",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, unavailablePods: 1,
			subset1Replicas: 3, subset2Replicas: 7,
		},
		{
			name:          "no pending pods -> 4, 6",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, unavailablePods: 0,
			subset1Replicas: 4, subset2Replicas: 6,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ud, subsets := getUnitedDeploymentAndSubsets(
				testCase.totalReplicas, testCase.minReplicas, testCase.maxReplicas, testCase.unavailablePods)
			alloc, err := NewReplicaAllocator(ud).Alloc(&subsets)
			if err != nil {
				t.Fatalf("unexpected getNextReplicas error %v", err)
			} else {
				subset1Replicas, subset2Replicas := (*alloc)["subset-1"], (*alloc)["subset-2"]
				if subset1Replicas != testCase.subset1Replicas || subset2Replicas != testCase.subset2Replicas {
					t.Fatalf("subset1Replicas = %d, subset2Replicas = %d,  test case is %+v",
						subset1Replicas, subset2Replicas, testCase)
				}
			}
		})
	}
}

func TestProtectingRunningPodsAdaptive(t *testing.T) {
	getUnitedDeploymentAndSubsets := func(subset1MinReplicas, subset1MaxReplicas, subset1RunningReplicas, subset2RunningReplicas int32) (
		*appsv1alpha1.UnitedDeployment, map[string]*Subset) {
		minR, maxR := intstr.FromInt32(subset1MinReplicas), intstr.FromInt32(subset1MaxReplicas)
		totalReplicas := subset1RunningReplicas + subset2RunningReplicas
		return &appsv1alpha1.UnitedDeployment{
				Spec: appsv1alpha1.UnitedDeploymentSpec{
					Replicas: &totalReplicas,
					Topology: appsv1alpha1.Topology{
						Subsets: []appsv1alpha1.Subset{
							{
								Name:        "subset-1",
								MinReplicas: &minR,
								MaxReplicas: &maxR,
							},
							{
								Name: "subset-2",
							},
						},
						ScheduleStrategy: appsv1alpha1.UnitedDeploymentScheduleStrategy{
							Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
						},
					},
				},
			}, map[string]*Subset{
				"subset-1": {
					Status: SubsetStatus{
						UpdatedReadyReplicas: subset1RunningReplicas,
						ReadyReplicas:        subset1RunningReplicas,
					},
					Spec: SubsetSpec{
						SubsetPods: generateSubsetPods(subset1RunningReplicas, 0, 0),
					},
				},
				"subset-2": {
					Status: SubsetStatus{
						UpdatedReadyReplicas: subset2RunningReplicas,
						ReadyReplicas:        subset2RunningReplicas,
					},
					Spec: SubsetSpec{
						SubsetPods: generateSubsetPods(subset2RunningReplicas, 0, 0),
					},
				},
			}
	}
	cases := []struct {
		name                                                                                   string
		subset1MinReplicas, subset1MaxReplicas, subset1RunningReplicas, subset2RunningReplicas int32
		subset1Replicas, subset2Replicas                                                       int32
	}{
		{
			name:                   "subset1: [2,4], 1,1 -> 2,0",
			subset1MinReplicas:     2,
			subset1MaxReplicas:     4,
			subset1RunningReplicas: 1,
			subset2RunningReplicas: 1,
			subset1Replicas:        2,
			subset2Replicas:        0,
		},
		{
			name:                   "subset1: [2,4], 2,1 -> 2,1",
			subset1MinReplicas:     2,
			subset1MaxReplicas:     4,
			subset1RunningReplicas: 2,
			subset2RunningReplicas: 1,
			subset1Replicas:        2,
			subset2Replicas:        1,
		},
		{
			name:                   "subset1: [2,4], 1,2 -> 2,1",
			subset1MinReplicas:     2,
			subset1MaxReplicas:     4,
			subset1RunningReplicas: 1,
			subset2RunningReplicas: 2,
			subset1Replicas:        2,
			subset2Replicas:        1,
		},
		{
			name:                   "subset1: [2,4], 0,4 -> 2,2",
			subset1MinReplicas:     2,
			subset1MaxReplicas:     4,
			subset1RunningReplicas: 0,
			subset2RunningReplicas: 4,
			subset1Replicas:        2,
			subset2Replicas:        2,
		},
		{
			name:                   "subset1: [2,4], 3,1 -> 3,1",
			subset1MinReplicas:     2,
			subset1MaxReplicas:     4,
			subset1RunningReplicas: 3,
			subset2RunningReplicas: 1,
			subset1Replicas:        3,
			subset2Replicas:        1,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ud, existingSubsets := getUnitedDeploymentAndSubsets(tt.subset1MinReplicas, tt.subset1MaxReplicas, tt.subset1RunningReplicas, tt.subset2RunningReplicas)
			alloc, err := NewReplicaAllocator(ud).Alloc(&existingSubsets)
			if err != nil {
				t.Fatalf("unexpected getNextReplicas error %v", err)
			} else {
				subset1Replicas, subset2Replicas := (*alloc)["subset-1"], (*alloc)["subset-2"]
				if subset1Replicas != tt.subset1Replicas || subset2Replicas != tt.subset2Replicas {
					t.Logf("subset1Replicas got %d, expect %d, subset1Replicas got %d, expect %d", subset1Replicas, tt.subset1Replicas, subset2Replicas, tt.subset2Replicas)
					t.Fail()
				}
			}
		})
	}
	// invalid inputs
	ud, existingSubsets := getUnitedDeploymentAndSubsets(4, 2, 0, 0)
	_, err := NewReplicaAllocator(ud).Alloc(&existingSubsets)
	if err == nil {
		t.Logf("expected error not happen")
		t.Fail()
	}
}

func generateSubsetPods(total, reserved int32, prefix int) []*corev1.Pod {
	var pods []*corev1.Pod
	for i := int32(0); i < total; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("pod-%d-%d", prefix, i),
			},
		}
		if i < reserved {
			pod.Status.Phase = corev1.PodPending
		} else {
			pod.Status.Phase = corev1.PodRunning
		}
		pods = append(pods, pod)
	}
	return pods
}

// Cases of TestGetTemporaryAdaptiveNext must be aligned with TestRescheduleTemporarily
func TestGetTemporaryAdaptiveNext(t *testing.T) {
	getUnitedDeploymentAndSubsets := func(totalReplicas, minReplicas, maxReplicas int32, reserved []int32, cur []int32) (
		*appsv1alpha1.UnitedDeployment, map[string]*Subset) {
		var minR, maxR *intstr.IntOrString
		if minReplicas != 0 {
			minR = &intstr.IntOrString{Type: intstr.Int, IntVal: minReplicas}
		}
		if maxReplicas != 0 {
			maxR = &intstr.IntOrString{Type: intstr.Int, IntVal: maxReplicas}
		}
		ud := &appsv1alpha1.UnitedDeployment{
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &totalReplicas,
				Topology: appsv1alpha1.Topology{
					ScheduleStrategy: appsv1alpha1.UnitedDeploymentScheduleStrategy{
						Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
						Adaptive: &appsv1alpha1.AdaptiveUnitedDeploymentStrategy{
							ReserveUnschedulablePods: true,
						},
					},
				},
			},
		}
		existingSubsets := map[string]*Subset{}
		for i := range reserved {
			name := fmt.Sprintf("subset-%d", i)
			ud.Spec.Topology.Subsets = append(ud.Spec.Topology.Subsets, appsv1alpha1.Subset{
				Name:        name,
				MinReplicas: minR,
				MaxReplicas: maxR,
			})
			subset := &Subset{
				Status: SubsetStatus{
					UnschedulableStatus: SubsetUnschedulableStatus{
						Unschedulable: reserved[i] != 0,
						ReservedPods:  reserved[i],
					},
					Replicas:             cur[i],
					UpdatedReadyReplicas: cur[i] - reserved[i],
					ReadyReplicas:        cur[i] - reserved[i],
				},
				Spec: SubsetSpec{
					SubsetPods: generateSubsetPods(cur[i], reserved[i], i),
				},
			}
			existingSubsets[name] = subset
		}
		ud.Spec.Topology.Subsets[len(ud.Spec.Topology.Subsets)-1].MaxReplicas = nil
		return ud, existingSubsets
	}
	tests := []struct {
		name        string
		replicas    int32   // total replicas of UnitedDeployment
		minReplicas int32   // min replicas of each subset
		maxReplicas int32   // max replicas of each subset except the last one
		cur         []int32 // last allocated results, equals to last expect
		staging     []int32 // current unavailable pod nums of each subset
		next        []int32 // allocated replicas calculated this time
	}{
		{
			name:        "4 subsets, 2 each, start",
			replicas:    8,
			minReplicas: 1,
			maxReplicas: 2,
			cur:         []int32{0, 0, 0, 0},
			staging:     []int32{0, 0, 0, 0},
			next:        []int32{2, 2, 2, 2},
		},
		{
			name:        "4 subsets, subset 1 and 2 unschedulable detected",
			replicas:    8,
			minReplicas: 1,
			maxReplicas: 2,
			cur:         []int32{2, 2, 2, 2},
			staging:     []int32{0, 2, 2, 0},
			next:        []int32{2, 0, 0, 6},
		},
		{
			name:        "4 subsets, subset 1 and 2 starts each 1 pods",
			replicas:    8,
			minReplicas: 1,
			maxReplicas: 2,
			cur:         []int32{2, 2, 2, 6},
			staging:     []int32{0, 1, 1, 0},
			next:        []int32{2, 1, 1, 4},
		},
		{
			name:        "4 subsets, subset 1 recovered",
			replicas:    8,
			minReplicas: 1,
			maxReplicas: 2,
			cur:         []int32{2, 2, 2, 4},
			staging:     []int32{0, 0, 1, 0},
			next:        []int32{2, 2, 1, 3},
		},
		{
			name:        "4 subsets, all subset recovered",
			replicas:    8,
			minReplicas: 1,
			maxReplicas: 2,
			cur:         []int32{2, 2, 2, 3},
			staging:     []int32{0, 0, 0, 0},
			next:        []int32{2, 2, 2, 2},
		},
		{
			name:        "4 subsets, part of subset 1 and 2 started, scaled to 16 replicas",
			replicas:    16,
			minReplicas: 2,
			maxReplicas: 4,
			cur:         []int32{2, 2, 2, 6},
			staging:     []int32{0, 1, 1, 0},
			next:        []int32{4, 1, 1, 10},
		},
		{
			name:        "4 subsets, part of subset 1 and 2 started, scaled back to 8 replicas",
			replicas:    8,
			minReplicas: 1,
			maxReplicas: 2,
			cur:         []int32{4, 2, 2, 10},
			staging:     []int32{0, 1, 1, 0},
			next:        []int32{2, 1, 1, 4},
		},
		{
			name:        "4 subsets, all of subset 1 and 2 started, already scaled to 16 replicas",
			replicas:    16,
			minReplicas: 2,
			maxReplicas: 4,
			cur:         []int32{4, 2, 2, 10},
			staging:     []int32{0, 0, 0, 0},
			next:        []int32{4, 4, 4, 4},
		},
		{
			name:        "4 subsets, all of subset 1 and 2 started, already scaled to 16 replicas, 4 new pods just created",
			replicas:    16,
			minReplicas: 2,
			maxReplicas: 4,
			cur:         []int32{4, 4, 4, 8},
			staging:     []int32{0, 0, 0, 0},
			next:        []int32{4, 4, 4, 4},
		},
		{
			// this case is same as the previous one in allocate stage, just to align with TestRescheduleTemporarily
			name:        "4 subsets, all pods started, already scaled to 16 replicas",
			replicas:    16,
			minReplicas: 2,
			maxReplicas: 4,
			cur:         []int32{4, 4, 4, 8},
			staging:     []int32{0, 0, 0, 0},
			next:        []int32{4, 4, 4, 4},
		},
		{
			name:     "3 infinity subsets, start",
			replicas: 2,
			cur:      []int32{0, 0, 0},
			staging:  []int32{0, 0, 0},
			next:     []int32{2, 0, 0},
		},
		{
			name:     "3 infinity subsets, start",
			replicas: 2,
			cur:      []int32{0, 0, 0},
			staging:  []int32{0, 0, 0},
			next:     []int32{2, 0, 0},
		},
		{
			name:     "3 infinity subsets, found subset-0 unschedulable",
			replicas: 2,
			cur:      []int32{2, 0, 0},
			staging:  []int32{2, 0, 0},
			next:     []int32{0, 2, 0},
		},
		{
			name:     "3 infinity subsets, found subset-1 unschedulable",
			replicas: 2,
			cur:      []int32{2, 2, 0},
			staging:  []int32{2, 2, 0},
			next:     []int32{0, 0, 2},
		},
		{
			name:     "3 infinity subsets, one of subset-1 started",
			replicas: 2,
			cur:      []int32{2, 2, 2},
			staging:  []int32{2, 1, 0},
			next:     []int32{0, 1, 1},
		},
		{
			name:     "3 infinity subsets, subset-0 recovered",
			replicas: 2,
			cur:      []int32{2, 2, 1},
			staging:  []int32{0, 1, 0},
			next:     []int32{2, 0, 0},
		},
		{
			name:     "3 infinity subsets, too many pods running",
			replicas: 2,
			cur:      []int32{2, 2, 0},
			staging:  []int32{0, 0, 0},
			next:     []int32{2, 0, 0},
		},
		{
			name:     "3 infinity subsets, scaled from 2 to 4 and subset-1 recovered",
			replicas: 4,
			cur:      []int32{2, 2, 4},
			staging:  []int32{0, 2, 0},
			next:     []int32{4, 0, 0},
		},
		{
			name:     "3 infinity subsets, scaled from 2 to 4 and subset-1 recovered, two new pods just created",
			replicas: 4,
			cur:      []int32{4, 2, 2},
			staging:  []int32{0, 2, 0},
			next:     []int32{4, 0, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ud, subsets := getUnitedDeploymentAndSubsets(tt.replicas, tt.minReplicas, tt.maxReplicas, tt.staging, tt.cur)
			ac := elasticAllocator{ud}
			result, err := ac.allocate(tt.replicas, &subsets)
			if err != nil {
				t.Fatalf("unexpected getNextReplicas error %v", err)
			}
			actual := make([]int32, len(tt.next))
			for i := 0; i < len(tt.next); i++ {
				actual[i] = (*result)[fmt.Sprintf("subset-%d", i)]
			}
			if !reflect.DeepEqual(actual, tt.next) {
				t.Fatalf("expected %v, got %v", tt.next, actual)
			}
		})
	}
}

func createSubset(name string, replicas int32) *nameToReplicas {
	return &nameToReplicas{
		Replicas:   replicas,
		SubsetName: name,
	}
}
