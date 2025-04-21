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
	_, _ = allocator.AllocateReplicas(5, map[string]int32{})
	if " t1 -> 1; t3 -> 1; t4 -> 1; t2 -> 2;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 2),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(0, map[string]int32{})
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
	_, _ = allocator.AllocateReplicas(17, map[string]int32{})
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
	_, _ = allocator.AllocateReplicas(9, map[string]int32{})
	if " t1 -> 2; t3 -> 2; t4 -> 2; t2 -> 3;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 0),
		createSubset("t2", 10),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(19, map[string]int32{})
	if " t1 -> 9; t2 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 0),
		createSubset("t2", 10),
	}
	allocator = sortToAllocator(infos)
	_, _ = allocator.AllocateReplicas(21, map[string]int32{})
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
	_, _ = allocator.AllocateReplicas(27, map[string]int32{
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
	_, _ = allocator.AllocateReplicas(8, map[string]int32{
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
	_, _ = allocator.AllocateReplicas(16, map[string]int32{
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
	_, _ = allocator.AllocateReplicas(10, map[string]int32{
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
	_, _ = allocator.AllocateReplicas(10, map[string]int32{
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
	_, _ = allocator.AllocateReplicas(-1, map[string]int32{
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
	_, _ = allocator.AllocateReplicas(14, map[string]int32{
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
	_, _ = allocator.AllocateReplicas(14, map[string]int32{
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
				if result[fmt.Sprintf("subset-%d", index)] != cs.desiredReplicas[index] {
					t.Fatalf("unexpected result %v", result)
				}
			}
		})
	}
}

func TestDefaultAdaptiveAllocation(t *testing.T) {
	parseInt32 := func(x int32) *intstr.IntOrString {
		if x == -1 {
			return nil
		}
		xx := intstr.FromInt32(x)
		return &xx
	}
	getUnitedDeploymentAndSubsets := func(totalReplicas int32,
		minReplicas, maxReplicas, subsetPods, pendingPods []int32) (
		*appsv1alpha1.UnitedDeployment, map[string]*Subset) {
		ud := &appsv1alpha1.UnitedDeployment{
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Replicas: &totalReplicas,
				Topology: appsv1alpha1.Topology{
					ScheduleStrategy: appsv1alpha1.UnitedDeploymentScheduleStrategy{
						Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
					},
				},
			},
		}
		existingSubsets := map[string]*Subset{}
		for i := range subsetPods {
			subsetName := fmt.Sprintf("subset-%d", i)
			minR, maxR := parseInt32(minReplicas[i]), parseInt32(maxReplicas[i])
			pending := max(0, pendingPods[i])
			ud.Spec.Topology.Subsets = append(ud.Spec.Topology.Subsets, appsv1alpha1.Subset{
				Name:        subsetName,
				MinReplicas: minR,
				MaxReplicas: maxR,
			})
			existingSubsets[subsetName] = &Subset{
				Status: SubsetStatus{
					UnschedulableStatus: SubsetUnschedulableStatus{
						Unschedulable: pendingPods[i] != 0,
						PendingPods:   pending,
					},
					Replicas:             subsetPods[i],
					UpdatedReadyReplicas: subsetPods[i] - pending,
					ReadyReplicas:        subsetPods[i] - pending,
				},
				Spec: SubsetSpec{
					Replicas:   subsetPods[i],
					SubsetPods: generateSubsetPods(subsetPods[i], pending, 0),
				},
			}
		}
		return ud, existingSubsets
	}
	cases := []struct {
		name         string
		specReplicas int32   // ud.spec.replicas
		subsetPods   []int32 // pods in each subset
		minReplicas  []int32 // minReplicas in each subset, -1 means nil
		maxReplicas  []int32 // maxReplicas in each subset, -1 means nil
		pendingPods  []int32 // pendingPods in each subset. set to -1 means no pods but unschedulable
		results      []int32 // result expected
	}{
		{
			name:         "5 pending pods = maxReplicas -> 3, 7",
			specReplicas: 10,
			subsetPods:   []int32{5, 5},
			minReplicas:  []int32{3, -1},
			maxReplicas:  []int32{5, -1},
			pendingPods:  []int32{5, 0},
			results:      []int32{3, 7},
		},
		{
			name:         "4 pending pods < maxReplicas -> 3, 7",
			specReplicas: 10,
			subsetPods:   []int32{5, 5},
			minReplicas:  []int32{3, -1},
			maxReplicas:  []int32{5, -1},
			pendingPods:  []int32{4, 0},
			results:      []int32{3, 7},
		},
		{
			name:         "3 pending pods = minReplicas -> 3, 7",
			specReplicas: 10,
			subsetPods:   []int32{5, 5},
			minReplicas:  []int32{3, -1},
			maxReplicas:  []int32{5, -1},
			pendingPods:  []int32{3, 0},
			results:      []int32{3, 7},
		},
		{
			name:         "2 pending pods < minReplicas -> 3, 7",
			specReplicas: 10,
			subsetPods:   []int32{5, 5},
			minReplicas:  []int32{3, -1},
			maxReplicas:  []int32{5, -1},
			pendingPods:  []int32{2, 0},
			results:      []int32{3, 7},
		},
		{
			name:         "no pending pods -> 5, 5",
			specReplicas: 10,
			subsetPods:   []int32{5, 5},
			minReplicas:  []int32{3, -1},
			maxReplicas:  []int32{5, -1},
			pendingPods:  []int32{0, 0},
			results:      []int32{5, 5},
		},
		{
			name:         "normal scale up 2-> 4",
			specReplicas: 4,
			subsetPods:   []int32{1, 1},
			minReplicas:  []int32{1, -1},
			maxReplicas:  []int32{2, -1},
			pendingPods:  []int32{0, 0},
			results:      []int32{2, 2},
		},
		{
			name:         "normal scale down 4 -> 3",
			specReplicas: 3,
			subsetPods:   []int32{2, 2},
			minReplicas:  []int32{1, -1},
			maxReplicas:  []int32{2, -1},
			pendingPods:  []int32{0, 0},
			results:      []int32{2, 1},
		},
		{
			name:         "normal scale down 3 -> 1",
			specReplicas: 1,
			subsetPods:   []int32{2, 1},
			minReplicas:  []int32{1, -1},
			maxReplicas:  []int32{2, -1},
			pendingPods:  []int32{0, 0},
			results:      []int32{1, 0},
		},
		{
			name:         "scale down with min and pending",
			specReplicas: 3,
			subsetPods:   []int32{1, 3},
			minReplicas:  []int32{1, -1},
			maxReplicas:  []int32{2, -1},
			pendingPods:  []int32{1, 0},
			results:      []int32{1, 2},
		},
		{
			name:         "scale down with only pending",
			specReplicas: 3,
			subsetPods:   []int32{1, 3},
			minReplicas:  []int32{0, -1},
			maxReplicas:  []int32{2, -1},
			pendingPods:  []int32{1, 0},
			results:      []int32{0, 3},
		},
		{
			name:         "complex: start",
			specReplicas: 4,
			subsetPods:   []int32{0, 0, 0, 0},
			minReplicas:  []int32{0, 0, 0, 0},
			maxReplicas:  []int32{2, -1, -1, -1},
			pendingPods:  []int32{0, 0, 0, 0},
			results:      []int32{2, 2, 0, 0},
		},
		{
			name:         "complex: found subset-1 unschedulable",
			specReplicas: 4,
			subsetPods:   []int32{2, 2, 0, 0},
			minReplicas:  []int32{0, 0, 0, 0},
			maxReplicas:  []int32{2, -1, -1, -1},
			pendingPods:  []int32{0, 2, 0, 0},
			results:      []int32{2, 0, 2, 0},
		},
		{
			name:         "complex: found subset-2 unschedulable",
			specReplicas: 4,
			subsetPods:   []int32{2, 0, 2, 0},
			minReplicas:  []int32{0, 0, 0, 0},
			maxReplicas:  []int32{2, -1, -1, -1},
			pendingPods:  []int32{0, -1, 2, 0},
			results:      []int32{2, 0, 0, 2},
		},
		{
			name:         "complex: scale up 4 -> 8 when unschedulable",
			specReplicas: 8,
			subsetPods:   []int32{2, 0, 0, 2},
			minReplicas:  []int32{0, 0, 0, 0},
			maxReplicas:  []int32{2, -1, -1, -1},
			pendingPods:  []int32{0, -1, -1, 0},
			results:      []int32{2, 0, 0, 6},
		},
		{
			name:         "complex: scale up 4 -> 8 after recovered",
			specReplicas: 8,
			subsetPods:   []int32{2, 0, 0, 2},
			minReplicas:  []int32{0, 0, 0, 0},
			maxReplicas:  []int32{2, -1, -1, -1},
			pendingPods:  []int32{0, 0, 0, 0},
			results:      []int32{2, 4, 0, 2},
		},
		{
			name:         "complex: scale down 8 -> 4 when unschedulable",
			specReplicas: 4,
			subsetPods:   []int32{2, 0, 0, 6},
			minReplicas:  []int32{0, 0, 0, 0},
			maxReplicas:  []int32{-1, -1, -1, -1},
			pendingPods:  []int32{0, 0, 0, 0},
			results:      []int32{2, 0, 0, 2},
		},
		{
			name:         "complex: scale down 8 -> 4 after recovered",
			specReplicas: 4,
			subsetPods:   []int32{2, 0, 0, 6},
			minReplicas:  []int32{0, 0, 0, 0},
			maxReplicas:  []int32{-1, -1, -1, -1},
			pendingPods:  []int32{0, 0, 0, 0},
			results:      []int32{2, 0, 0, 2},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ud, subsets := getUnitedDeploymentAndSubsets(tt.specReplicas, tt.minReplicas, tt.maxReplicas,
				tt.subsetPods, tt.pendingPods)
			alloc, err := NewReplicaAllocator(ud).Alloc(subsets)
			if err != nil {
				t.Fatalf("unexpected getNextReplicas error %v", err)
			}
			actual := make([]int32, len(tt.results))
			for i := 0; i < len(tt.results); i++ {
				actual[i] = alloc[fmt.Sprintf("subset-%d", i)]
			}
			if !reflect.DeepEqual(actual, tt.results) {
				t.Errorf("expected %v, but got %v", tt.results, actual)
			}
		})
	}
}

func TestReservedAdaptiveAllocation(t *testing.T) {
	getUnitedDeploymentAndSubsets := func(totalReplicas int32, minReplicas, maxReplicas, reserved, cur, newPods []int32) (
		*appsv1alpha1.UnitedDeployment, map[string]*Subset) {
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
			var minR, maxR *intstr.IntOrString
			if minReplicas != nil && minReplicas[i] >= 0 {
				minR = &intstr.IntOrString{Type: intstr.Int, IntVal: minReplicas[i]}
			}
			if maxReplicas != nil && maxReplicas[i] >= 0 {
				maxR = &intstr.IntOrString{Type: intstr.Int, IntVal: maxReplicas[i]}
			}
			ud.Spec.Topology.Subsets = append(ud.Spec.Topology.Subsets, appsv1alpha1.Subset{
				Name:        name,
				MinReplicas: minR,
				MaxReplicas: maxR,
			})
			var previouslyUnschedulable bool
			if reserved[i] != 0 {
				previouslyUnschedulable = true
				reserved[i] = max(0, reserved[i])
			}
			subset := &Subset{
				Status: SubsetStatus{
					UnschedulableStatus: SubsetUnschedulableStatus{
						Unschedulable:           reserved[i] != 0,
						PreviouslyUnschedulable: previouslyUnschedulable,
						ReservedPodNum:          reserved[i],
					},
					Replicas:             cur[i],
					UpdatedReadyReplicas: cur[i] - reserved[i] - newPods[i],
					ReadyReplicas:        cur[i] - reserved[i] - newPods[i],
				},
				Spec: SubsetSpec{
					SubsetPods: generateSubsetPods(cur[i], reserved[i], i),
					Replicas:   cur[i],
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
		minReplicas []int32 // min replicas of each subset, -1 means nil
		maxReplicas []int32 // max replicas of each subset, -1 means nil
		cur         []int32 // last allocated results, equals to last expect
		reserved    []int32 // current reserved pod nums of each subset, set -1 to make condition unschedulable anyway
		newPods     []int32 // newly created and pending pods caused by replica allocation
		expect      []int32 // final allocated result after reschedule
	}{
		{
			name:        "4 subsets, 2 each, start",
			replicas:    8,
			minReplicas: []int32{1, 1, 1, 1},
			maxReplicas: []int32{2, 2, 2, -1},
			cur:         []int32{0, 0, 0, 0},
			reserved:    []int32{0, 0, 0, 0},
			expect:      []int32{2, 2, 2, 2},
		},
		{
			name:        "4 subsets, subset 1 and 2 unschedulable detected",
			replicas:    8,
			minReplicas: []int32{1, 1, 1, 1},
			maxReplicas: []int32{2, 2, 2, -1},
			cur:         []int32{2, 2, 2, 2},
			reserved:    []int32{0, 2, 2, 0},
			expect:      []int32{2, 2, 2, 6},
		},
		{
			name:        "4 subsets, subset 1 and 2 starts each 1 pods",
			replicas:    8,
			minReplicas: []int32{1, 1, 1, 1},
			maxReplicas: []int32{2, 2, 2, -1},
			cur:         []int32{2, 2, 2, 6},
			reserved:    []int32{0, 1, 1, 0},
			expect:      []int32{2, 2, 2, 4},
		},
		{
			name:        "4 subsets, subset 1 recovered",
			replicas:    8,
			minReplicas: []int32{1, 1, 1, 1},
			maxReplicas: []int32{2, 2, 2, -1},
			cur:         []int32{2, 2, 2, 4},
			reserved:    []int32{0, -1, 1, 0},
			expect:      []int32{2, 2, 2, 3},
		},
		{
			name:        "4 subsets, all subset recovered",
			replicas:    8,
			minReplicas: []int32{1, 1, 1, 1},
			maxReplicas: []int32{2, 2, 2, -1},
			cur:         []int32{2, 2, 2, 3},
			reserved:    []int32{0, 0, -1, 0},
			expect:      []int32{2, 2, 2, 2},
		},
		{
			name:        "4 subsets, part of subset 1 and 2 started, scaled to 16",
			replicas:    16,
			minReplicas: []int32{2, 2, 2, 2},
			maxReplicas: []int32{4, 4, 4, -1},
			cur:         []int32{2, 2, 2, 6},
			reserved:    []int32{0, 1, 1, 0},
			expect:      []int32{4, 2, 2, 10},
		},
		{
			name:        "4 subsets, part of subset 1 and 2 started, scaled back to 8",
			replicas:    8,
			minReplicas: []int32{1, 1, 1, 1},
			maxReplicas: []int32{2, 2, 2, -1},
			cur:         []int32{4, 2, 2, 10},
			reserved:    []int32{0, 1, 1, 0},
			expect:      []int32{2, 2, 2, 4},
		},
		{
			name:        "4 subsets, all of subset 1 and 2 started, already scaled to 16 replicas",
			replicas:    16,
			minReplicas: []int32{2, 2, 2, 2},
			maxReplicas: []int32{4, 4, 4, -1},
			cur:         []int32{4, 2, 2, 10},
			reserved:    []int32{0, -1, -1, 0},
			expect:      []int32{4, 4, 4, 8},
		},
		{
			name:        "4 subsets, scaled to 16, new pods of subset 1 2 already started, verifying new pods",
			replicas:    16,
			minReplicas: []int32{2, 2, 2, 2},
			maxReplicas: []int32{4, 4, 4, -1},
			cur:         []int32{4, 4, 4, 8},
			reserved:    []int32{0, 2, 2, 0},
			expect:      []int32{4, 4, 4, 8},
		},
		{
			name:        "4 subsets, scaled to 16, new pods of subset 1 2 already started, new pods started",
			replicas:    16,
			minReplicas: []int32{2, 2, 2, 2},
			maxReplicas: []int32{4, 4, 4, -1},
			cur:         []int32{4, 4, 4, 8},
			reserved:    []int32{0, -1, -1, 0},
			expect:      []int32{4, 4, 4, 4},
		},
		{
			name:     "3 infinity subsets, start",
			replicas: 2,
			cur:      []int32{0, 0, 0},
			reserved: []int32{0, 0, 0},
			expect:   []int32{2, 0, 0},
		},
		{
			name:     "3 infinity subsets, found subset-0 unschedulable",
			replicas: 2,
			cur:      []int32{2, 0, 0},
			reserved: []int32{2, 0, 0},
			expect:   []int32{2, 2, 0},
		},
		{
			name:     "3 infinity subsets, found subset-1 unschedulable",
			replicas: 2,
			cur:      []int32{2, 2, 0},
			reserved: []int32{2, 2, 0},
			expect:   []int32{2, 2, 2},
		},
		{
			name:     "3 infinity subsets, 2 unschedulable, scale up to 4",
			replicas: 4,
			cur:      []int32{2, 2, 2},
			reserved: []int32{2, 2, 0},
			expect:   []int32{2, 2, 4},
		},
		{
			name:     "3 infinity subsets, scaled, 2 unschedulable, subset-1 recovered",
			replicas: 4,
			cur:      []int32{2, 2, 4},
			reserved: []int32{4, -1, 0},
			expect:   []int32{2, 4, 2},
		},
		{
			name:     "3 infinity subsets, scaled, 2 unschedulable, subset-1 started, waiting verify",
			replicas: 4,
			cur:      []int32{2, 4, 2},
			reserved: []int32{4, 2, 0}, // new pods marked as reserved
			expect:   []int32{2, 4, 2},
		},
		{
			name:     "3 infinity subsets, scaled, 2 unschedulable, subset-1 started after recovery",
			replicas: 4,
			cur:      []int32{2, 4, 2},
			reserved: []int32{4, -1, 0},
			expect:   []int32{2, 4, 0},
		},
		{
			name:     "3 infinity subsets, one of subset-1 started",
			replicas: 2,
			cur:      []int32{2, 2, 2},
			reserved: []int32{2, 1, 0},
			expect:   []int32{2, 2, 1},
		},
		{
			name:     "3 infinity subsets, subset-0 recovered",
			replicas: 2,
			cur:      []int32{2, 2, 1},
			reserved: []int32{0, 1, 0},
			expect:   []int32{2, 0, 0},
		},
		{
			name:     "3 infinity subsets, scale from 4 to 2",
			replicas: 2,
			cur:      []int32{2, 2, 0},
			reserved: []int32{0, 0, 0},
			expect:   []int32{2, 0, 0},
		},
		{
			name:     "3 infinity subsets, scaled from 2 to 4 and subset-1 recovered",
			replicas: 4,
			cur:      []int32{2, 2, 4},
			reserved: []int32{-1, 2, 0},
			expect:   []int32{4, 2, 2},
		},
		{
			name:        "3 subsets, 2 with minReplicas, start",
			replicas:    2,
			minReplicas: []int32{1, 1, 0},
			cur:         []int32{0, 0, 0},
			reserved:    []int32{0, 0, 0},
			expect:      []int32{1, 1, 0},
		},
		{
			name:        "3 subsets, 2 with minReplicas, two subsets unschedulable",
			replicas:    2,
			minReplicas: []int32{1, 1, 0},
			cur:         []int32{1, 1, 0},
			reserved:    []int32{1, 1, 0},
			expect:      []int32{1, 1, 2},
		},
		{
			name:        "3 subsets, 2 with minReplicas, subset-0 recovered",
			replicas:    2,
			minReplicas: []int32{1, 1, 0},
			cur:         []int32{1, 1, 2},
			reserved:    []int32{-1, 1, 0},
			expect:      []int32{2, 1, 1},
		},
		{
			name:        "3 subsets, 2 with minReplicas, subset-0 recovered, verifying new pods ",
			replicas:    2,
			minReplicas: []int32{1, 1, 0},
			cur:         []int32{2, 1, 1},
			reserved:    []int32{1, 1, 0},
			expect:      []int32{2, 1, 1},
		},
		{
			name:        "3 subsets, 2 with minReplicas, subset-0 recovered, new pod started",
			replicas:    2,
			minReplicas: []int32{1, 1, 0},
			cur:         []int32{2, 1, 1},
			reserved:    []int32{0, 1, 0},
			expect:      []int32{2, 1, 0},
		},
		{
			name:        "3 subsets, 2 with minReplicas, subset-1 recovered",
			replicas:    2,
			minReplicas: []int32{1, 1, 0},
			cur:         []int32{2, 1, 0},
			reserved:    []int32{0, -1, 0},
			expect:      []int32{1, 1, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.newPods == nil {
				tt.newPods = make([]int32, len(tt.expect))
			}
			ud, subsets := getUnitedDeploymentAndSubsets(tt.replicas, tt.minReplicas, tt.maxReplicas,
				tt.reserved, tt.cur, tt.newPods)
			ac := NewReplicaAllocator(ud)
			ra, ok := ac.(*reservationAllocator)
			if !ok {
				t.Fatalf("unexpected allocator type %T", ac)
			}

			nextReplicas, err := ra.Alloc(subsets)
			if err != nil {
				t.Fatalf("unexpected Alloc error %v", err)
			}
			actual := make([]int32, len(tt.expect))
			for i := 0; i < len(tt.expect); i++ {
				actual[i] = nextReplicas[fmt.Sprintf("subset-%d", i)]
			}
			if !reflect.DeepEqual(actual, tt.expect) {
				t.Fatalf("expected %v, got %v", tt.expect, actual)
			}
		})
	}
}

func generateSubsetPods(total, pending int32, prefix int) []*corev1.Pod {
	var pods []*corev1.Pod
	for i := int32(0); i < total; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("pod-%d-%d", prefix, i),
			},
		}
		if i < pending {
			pod.Status.Phase = corev1.PodPending
		} else {
			pod.Status.Phase = corev1.PodRunning
		}
		pods = append(pods, pod)
	}
	return pods
}

func createSubset(name string, replicas int32) *nameToReplicas {
	return &nameToReplicas{
		Replicas:   replicas,
		SubsetName: name,
	}
}
