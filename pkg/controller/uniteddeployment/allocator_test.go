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
	"sort"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
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
	getUnitedDeploymentAndSubsets := func(totalReplicas, minReplicas, maxReplicas, failedPods int32) (
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
							PendingPods:   failedPods,
						},
						Replicas: maxReplicas,
					},
					Spec: SubsetSpec{Replicas: minReplicas},
				},
				//"subset-2": {
				//	Status: SubsetStatus{},
				//},
			}
	}
	cases := []struct {
		name                                                 string
		totalReplicas, minReplicas, maxReplicas, pendingPods int32
		subset1Replicas, subset2Replicas                     int32
	}{
		{
			name:          "5 pending pods > maxReplicas -> 0, 10",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, pendingPods: 5,
			subset1Replicas: 0, subset2Replicas: 10,
		},
		{
			name:          "4 pending pods = maxReplicas -> 0, 10",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, pendingPods: 4,
			subset1Replicas: 0, subset2Replicas: 10,
		},
		{
			name:          "3 pending pods < maxReplicas -> 1, 9",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, pendingPods: 3,
			subset1Replicas: 1, subset2Replicas: 9,
		},
		{
			name:          "2 pending pods = minReplicas -> 2, 8",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, pendingPods: 2,
			subset1Replicas: 2, subset2Replicas: 8,
		},
		{
			name:          "1 pending pods < minReplicas -> 3, 7",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, pendingPods: 1,
			subset1Replicas: 3, subset2Replicas: 7,
		},
		{
			name:          "no pending pods -> 2, 8",
			totalReplicas: 10, minReplicas: 2, maxReplicas: 4, pendingPods: 0,
			subset1Replicas: 4, subset2Replicas: 6,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ud, subsets := getUnitedDeploymentAndSubsets(
				testCase.totalReplicas, testCase.minReplicas, testCase.maxReplicas, testCase.pendingPods)
			alloc, err := NewReplicaAllocator(ud).Alloc(&subsets)
			if err != nil {
				t.Fatalf("unexpected alloc error %v", err)
			} else {
				subset1Replicas, subset2Replicas := (*alloc)["subset-1"], (*alloc)["subset-2"]
				if subset1Replicas != testCase.subset1Replicas || subset2Replicas != testCase.subset2Replicas {
					t.Fatalf("subset1Replicas = %d, subset1Replicas = %d,  test case is %+v",
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
						Replicas: subset1RunningReplicas,
					},
				},
				"subset-2": {
					Status: SubsetStatus{
						Replicas: subset2RunningReplicas,
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
	for _, c := range cases {
		ud, nameToSubset := getUnitedDeploymentAndSubsets(c.subset1MinReplicas, c.subset1MaxReplicas, c.subset1RunningReplicas, c.subset2RunningReplicas)
		alloc, err := NewReplicaAllocator(ud).Alloc(&nameToSubset)
		if err != nil {
			t.Fatalf("unexpected alloc error %v", err)
		} else {
			subset1Replicas, subset2Replicas := (*alloc)["subset-1"], (*alloc)["subset-2"]
			if subset1Replicas != c.subset1Replicas || subset2Replicas != c.subset2Replicas {
				t.Logf("subset1Replicas got %d, expect %d, subset1Replicas got %d, expect %d", subset1Replicas, c.subset1Replicas, subset2Replicas, c.subset2Replicas)
				t.Fail()
			}
		}
	}
	// invalid inputs
	ud, nameToSubset := getUnitedDeploymentAndSubsets(4, 2, 0, 0)
	_, err := NewReplicaAllocator(ud).Alloc(&nameToSubset)
	if err == nil {
		t.Logf("expected error not happen")
		t.Fail()
	}
}

func createSubset(name string, replicas int32) *nameToReplicas {
	return &nameToReplicas{
		Replicas:   replicas,
		SubsetName: name,
	}
}
