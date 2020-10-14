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
	"testing"
)

func TestScaleReplicas(t *testing.T) {
	infos := subsetInfos{}
	infos = append(infos,
		createSubset("t1", 1),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	allocator := infos.SortToAllocator()
	allocator.AllocateReplicas(5, &map[string]int32{})
	if " t1 -> 1; t3 -> 1; t4 -> 1; t2 -> 2;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 2))
	infos = subsetInfos(infos)
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(0, &map[string]int32{})
	if " t1 -> 0;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 1),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(17, &map[string]int32{})
	if " t1 -> 4; t3 -> 4; t4 -> 4; t2 -> 5;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 2),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	infos = subsetInfos(infos)
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(9, &map[string]int32{})
	if " t1 -> 2; t3 -> 2; t4 -> 2; t2 -> 3;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 0),
		createSubset("t2", 10))
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(19, &map[string]int32{})
	if " t1 -> 9; t2 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 0),
		createSubset("t2", 10))
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(21, &map[string]int32{})
	if " t1 -> 10; t2 -> 11;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}
}

func TestSpecifyValidReplicas(t *testing.T) {
	infos := subsetInfos{}
	infos = append(infos,
		createSubset("t1", 1),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	allocator := infos.SortToAllocator()
	allocator.AllocateReplicas(27, &map[string]int32{
		"t1": 4,
		"t3": 4,
	})
	if " t1 -> 4; t3 -> 4; t4 -> 9; t2 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 2),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(8, &map[string]int32{
		"t1": 4,
		"t3": 4,
	})
	if " t2 -> 0; t4 -> 0; t1 -> 4; t3 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 2),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(16, &map[string]int32{
		"t1": 4,
		"t2": 4,
		"t3": 4,
		"t4": 4,
	})
	if " t1 -> 4; t2 -> 4; t3 -> 4; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 4),
		createSubset("t2", 2),
		createSubset("t3", 2),
		createSubset("t4", 2))
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(10, &map[string]int32{
		"t1": 1,
		"t2": 2,
		"t3": 3,
	})
	if " t1 -> 1; t2 -> 2; t3 -> 3; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 4),
		createSubset("t2", 2),
		createSubset("t3", 2),
		createSubset("t4", 2))
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(10, &map[string]int32{
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
	infos := subsetInfos{}
	infos = append(infos,
		createSubset("t1", 10),
		createSubset("t2", 4))
	allocator := infos.SortToAllocator()
	allocator.AllocateReplicas(14, &map[string]int32{
		"t1": 6,
		"t2": 6,
	})
	if " t2 -> 4; t1 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	infos = subsetInfos{}
	infos = append(infos,
		createSubset("t1", 10),
		createSubset("t2", 4))
	allocator = infos.SortToAllocator()
	allocator.AllocateReplicas(14, &map[string]int32{
		"t1": 10,
		"t2": 11,
	})
	if " t2 -> 4; t1 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}
}

func createSubset(name string, replicas int32) *nameToReplicas {
	return &nameToReplicas{
		Replicas:   replicas,
		SubsetName: name,
	}
}
