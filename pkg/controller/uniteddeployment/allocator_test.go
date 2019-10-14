package uniteddeployment

import (
	"testing"
)

func TestScaleReplicas(t *testing.T) {
	subsets := []*nameToReplicas{}
	subsets = append(subsets, createSubset("t1", 1), createSubset("t2", 4), createSubset("t3", 2), createSubset("t4", 2))
	nameToReplicasArray := subsetInfos(subsets)
	allocator := nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(5, map[string]int32{})
	if " t1 -> 1; t3 -> 1; t4 -> 1; t2 -> 2;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets, createSubset("t1", 2))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(0, map[string]int32{})
	if " t1 -> 0;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets, createSubset("t1", 1), createSubset("t2", 4), createSubset("t3", 2), createSubset("t4", 2))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(17, map[string]int32{})
	if " t1 -> 4; t3 -> 4; t4 -> 4; t2 -> 5;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets, createSubset("t1", 2), createSubset("t2", 4), createSubset("t3", 2), createSubset("t4", 2))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(9, map[string]int32{})
	if " t1 -> 2; t3 -> 2; t4 -> 2; t2 -> 3;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}
}

func TestScaleReplicasWithLimit(t *testing.T) {
	subsets := []*nameToReplicas{}
	subsets = append(subsets,
		createSubset("t1", 1),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	nameToReplicasArray := subsetInfos(subsets)
	allocator := nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(27, map[string]int32{
		"t1": 4,
		"t3": 4,
	})
	if " t1 -> 4; t3 -> 4; t4 -> 9; t2 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets,
		createSubset("t1", 2),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(8, map[string]int32{
		"t1": 4,
		"t3": 4,
	})
	if " t2 -> 0; t4 -> 0; t1 -> 4; t3 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets,
		createSubset("t1", 2),
		createSubset("t2", 4),
		createSubset("t3", 2),
		createSubset("t4", 2))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(16, map[string]int32{
		"t1": 4,
		"t2": 4,
		"t3": 4,
		"t4": 4,
	})
	if " t1 -> 4; t2 -> 4; t3 -> 4; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets,
		createSubset("t1", 4),
		createSubset("t2", 2),
		createSubset("t3", 2),
		createSubset("t4", 2))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(10, map[string]int32{
		"t1": 1,
		"t2": 2,
		"t3": 3,
	})
	if " t1 -> 1; t2 -> 2; t3 -> 3; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets,
		createSubset("t1", 4),
		createSubset("t2", 2),
		createSubset("t3", 2),
		createSubset("t4", 2))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(10, map[string]int32{
		"t1": 1,
		"t2": 2,
		"t3": 3,
		"t4": 4,
	})
	if " t1 -> 1; t2 -> 2; t3 -> 3; t4 -> 4;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets,
		createSubset("t1", 10),
		createSubset("t2", 4))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(15, map[string]int32{
		"t1": 10,
		"t2": 11,
	})
	if " t2 -> 5; t1 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}

	subsets = []*nameToReplicas{}
	subsets = append(subsets,
		createSubset("t1", 10),
		createSubset("t2", 4))
	nameToReplicasArray = subsetInfos(subsets)
	allocator = nameToReplicasArray.SortToAllocator()
	allocator.AllocateReplicas(14, map[string]int32{
		"t1": 10,
		"t2": 11,
	})
	if " t2 -> 4; t1 -> 10;" != allocator.String() {
		t.Fatalf("unexpected %s", allocator)
	}
}

func createSubset(name string, replicas int32) *nameToReplicas {
	return &nameToReplicas{
		Name:       name,
		Replicas:   replicas,
		SubsetName: name,
	}
}
