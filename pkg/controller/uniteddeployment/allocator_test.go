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
	allocator.AllocateReplicas(5, &map[string]int32{})
	if " t1 -> 1; t3 -> 1; t4 -> 1; t2 -> 2;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 2),
	}
	allocator = sortToAllocator(infos)
	allocator.AllocateReplicas(0, &map[string]int32{})
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
	allocator.AllocateReplicas(17, &map[string]int32{})
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
	allocator.AllocateReplicas(9, &map[string]int32{})
	if " t1 -> 2; t3 -> 2; t4 -> 2; t2 -> 3;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 0),
		createSubset("t2", 10),
	}
	allocator = sortToAllocator(infos)
	allocator.AllocateReplicas(19, &map[string]int32{})
	if " t1 -> 9; t2 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{
		createSubset("t1", 0),
		createSubset("t2", 10),
	}
	allocator = sortToAllocator(infos)
	allocator.AllocateReplicas(21, &map[string]int32{})
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
	allocator.AllocateReplicas(27, &map[string]int32{
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
	allocator.AllocateReplicas(8, &map[string]int32{
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
	allocator.AllocateReplicas(16, &map[string]int32{
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
	allocator.AllocateReplicas(10, &map[string]int32{
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
	allocator.AllocateReplicas(10, &map[string]int32{
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
	allocator.AllocateReplicas(-1, &map[string]int32{
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
	allocator.AllocateReplicas(14, &map[string]int32{
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
	allocator.AllocateReplicas(14, &map[string]int32{
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
				min := intstr.FromInt(int(cs.minReplicas[index]))
				var max *intstr.IntOrString
				if cs.maxReplicas[index] != -1 {
					m := intstr.FromInt(int(cs.maxReplicas[index]))
					max = &m
				}
				ud.Spec.Topology.Subsets = append(ud.Spec.Topology.Subsets, appsv1alpha1.Subset{
					Name:        fmt.Sprintf("subset-%d", index),
					MinReplicas: &min,
					MaxReplicas: max,
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

func createSubset(name string, replicas int32) *nameToReplicas {
	return &nameToReplicas{
		Replicas:   replicas,
		SubsetName: name,
	}
}
