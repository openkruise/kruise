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
	"math"
	"sort"
	"strings"

	"k8s.io/klog/v2"
	"k8s.io/utils/integer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

type nameToReplicas struct {
	SubsetName string
	Replicas   int32
	Specified  bool
}

type subsetInfos []*nameToReplicas

func (n subsetInfos) Get(i int) *nameToReplicas {
	return []*nameToReplicas(n)[i]
}

func (n subsetInfos) Len() int {
	return len(n)
}

func (n subsetInfos) Less(i, j int) bool {
	if n[i].Replicas != n[j].Replicas {
		return n[i].Replicas < n[j].Replicas
	}

	return strings.Compare(n[i].SubsetName, n[j].SubsetName) < 0
}

func (n subsetInfos) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

type ReplicaAllocator interface {
	Alloc(nameToSubset *map[string]*Subset) (*map[string]int32, error)
}

func NewReplicaAllocator(ud *appsv1alpha1.UnitedDeployment) ReplicaAllocator {
	for _, subset := range ud.Spec.Topology.Subsets {
		if subset.MinReplicas != nil || subset.MaxReplicas != nil {
			return &elasticAllocator{ud}
		}
	}
	return &specificAllocator{UnitedDeployment: ud}
}

// RunningReplicas refers to the number of Pods that an unschedulable subset can safely accommodate.
// Exceeding this number may lead to scheduling failures within that subset.
// This value is only effective in the Adaptive scheduling strategy.
func getSubsetRunningReplicas(nameToSubset *map[string]*Subset) map[string]int32 {
	if nameToSubset == nil {
		return nil
	}
	var result = make(map[string]int32)
	for name, subset := range *nameToSubset {
		result[name] = subset.Status.Replicas - subset.Status.UnschedulableStatus.PendingPods
	}
	return result
}

func isSubSetUnschedulable(name string, nameToSubset *map[string]*Subset) (unschedulable bool) {
	if subsetObj, ok := (*nameToSubset)[name]; ok {
		unschedulable = subsetObj.Status.UnschedulableStatus.Unschedulable
	} else {
		// newly created subsets are all schedulable
		unschedulable = false
	}
	return
}

type specificAllocator struct {
	*appsv1alpha1.UnitedDeployment
	subsets *subsetInfos
}

// Alloc returns a mapping from subset to next replicas.
// Next replicas is allocated by realReplicasAllocator, which will consider the current replicas of each subset and
// new replicas indicated from UnitedDeployment.Spec.Topology.Subsets.
func (s *specificAllocator) Alloc(nameToSubset *map[string]*Subset) (*map[string]int32, error) {
	// SortToAllocator to sort all subset by subset.Replicas in order of increment
	s.subsets = getSubsetInfos(nameToSubset, s.UnitedDeployment)
	sort.Sort(s.subsets)

	var expectedReplicas int32 = -1
	if s.Spec.Replicas != nil {
		expectedReplicas = *s.Spec.Replicas
	}

	specifiedReplicas := getSpecifiedSubsetReplicas(expectedReplicas, s.UnitedDeployment)
	klog.V(4).InfoS("UnitedDeployment specifiedReplicas", "unitedDeployment", klog.KObj(s), "specifiedReplicas", specifiedReplicas)
	return s.AllocateReplicas(expectedReplicas, specifiedReplicas)
}

func (s *specificAllocator) validateReplicas(replicas int32, subsetReplicasLimits *map[string]int32) error {
	if subsetReplicasLimits == nil {
		return nil
	}

	var specifiedReplicas int32
	for _, replicas := range *subsetReplicasLimits {
		specifiedReplicas += replicas
	}

	specifiedCount := 0
	for _, subset := range *s.subsets {
		if _, exist := (*subsetReplicasLimits)[subset.SubsetName]; exist {
			specifiedCount++
		}
	}
	klog.V(4).InfoS("SpecifiedCount and subsetsCount", "specifiedCount", specifiedCount, "subsetsCount", len(*s.subsets))
	if replicas != -1 {
		if specifiedReplicas > replicas {
			return fmt.Errorf("specified subsets' replica (%d) is greater than UnitedDeployment replica (%d)",
				specifiedReplicas, replicas)
		} else if specifiedReplicas < replicas && specifiedCount == len(*s.subsets) {
			return fmt.Errorf("specified subsets' replica (%d) is less than UnitedDeployment replica (%d)",
				specifiedReplicas, replicas)
		}
	} else if specifiedCount != len(*s.subsets) {
		return fmt.Errorf("all subsets must be specified when UnitedDeployment replica is unspecified")
	}

	return nil
}

func getSpecifiedSubsetReplicas(replicas int32, ud *appsv1alpha1.UnitedDeployment) *map[string]int32 {
	replicaLimits := map[string]int32{}
	if ud.Spec.Topology.Subsets == nil {
		return &replicaLimits
	}

	for _, subsetDef := range ud.Spec.Topology.Subsets {
		if subsetDef.Replicas == nil {
			continue
		}

		if specifiedReplicas, err := ParseSubsetReplicas(replicas, *subsetDef.Replicas); err == nil {
			replicaLimits[subsetDef.Name] = specifiedReplicas
		} else {
			klog.ErrorS(err, "Failed to consider the replicas of subset when parsing replicaLimits during managing replicas of UnitedDeployment",
				"subsetName", subsetDef.Name, "unitedDeployment", klog.KObj(ud))
		}
	}

	return &replicaLimits
}

func getSubsetInfos(nameToSubset *map[string]*Subset, ud *appsv1alpha1.UnitedDeployment) *subsetInfos {
	infos := make(subsetInfos, len(ud.Spec.Topology.Subsets))
	for idx, subsetDef := range ud.Spec.Topology.Subsets {
		var replicas int32
		if subset, exist := (*nameToSubset)[subsetDef.Name]; exist {
			replicas = subset.Spec.Replicas
		}
		infos[idx] = &nameToReplicas{SubsetName: subsetDef.Name, Replicas: replicas}
	}

	return &infos
}

// AllocateReplicas will first try to check the specifiedSubsetReplicas is valid or not.
// If valid , normalAllocate will be called. It will apply these specified replicas, then average the rest replicas to left unspecified subsets.
// If not, it will return error
func (s *specificAllocator) AllocateReplicas(replicas int32, specifiedSubsetReplicas *map[string]int32) (
	*map[string]int32, error) {
	if err := s.validateReplicas(replicas, specifiedSubsetReplicas); err != nil {
		return nil, err
	}

	return s.normalAllocate(replicas, specifiedSubsetReplicas), nil
}

func (s *specificAllocator) normalAllocate(expectedReplicas int32, specifiedSubsetReplicas *map[string]int32) *map[string]int32 {
	var specifiedReplicas int32
	specifiedSubsetCount := 0
	// Step 1: apply replicas to specified subsets, and mark them as specified = true.
	for _, subset := range *s.subsets {
		if replicas, exist := (*specifiedSubsetReplicas)[subset.SubsetName]; exist {
			specifiedReplicas += replicas
			subset.Replicas = replicas
			subset.Specified = true
			specifiedSubsetCount++
		}
	}

	// Step 2: averagely allocate the rest replicas to left unspecified subsets.
	leftSubsetCount := len(*s.subsets) - specifiedSubsetCount
	if leftSubsetCount != 0 {
		allocatableReplicas := expectedReplicas - specifiedReplicas
		average := int(allocatableReplicas) / leftSubsetCount
		remainder := int(allocatableReplicas) % leftSubsetCount

		for i := len(*s.subsets) - 1; i >= 0; i-- {
			subset := (*s.subsets)[i]
			if subset.Specified {
				continue
			}

			if remainder > 0 {
				subset.Replicas = int32(average + 1)
				remainder--
			} else {
				subset.Replicas = int32(average)
			}

			leftSubsetCount--

			if leftSubsetCount == 0 {
				break
			}
		}
	}

	return s.toSubsetReplicaMap()
}

func (s *specificAllocator) toSubsetReplicaMap() *map[string]int32 {
	allocatedReplicas := map[string]int32{}
	for _, subset := range *s.subsets {
		allocatedReplicas[subset.SubsetName] = subset.Replicas
	}

	return &allocatedReplicas
}

func (s *specificAllocator) String() string {
	result := ""
	sort.Sort(s.subsets)
	for _, subset := range *s.subsets {
		result = fmt.Sprintf("%s %s -> %d;", result, subset.SubsetName, subset.Replicas)
	}

	return result
}

type elasticAllocator struct {
	*appsv1alpha1.UnitedDeployment
}

// Alloc returns a mapping from subset to next replicas.
// Next replicas is allocated by elasticAllocator, which will consider the current minReplicas and maxReplicas
// of each subset and spec.replicas of UnitedDeployment. For example:
// spec.replicas: 5
// subsets:
//   - name: subset-a
//     minReplicas: 2    # will be satisfied with 1st priority
//     maxReplicas: 4    # will be satisfied with 3rd priority
//   - name: subset-b
//     minReplicas: 2    # will be satisfied with 2nd priority
//     maxReplicas: nil  # will be satisfied with 4th priority
//
// the results of map will be: {"subset-a": 3, "subset-b": 2}
func (ac *elasticAllocator) Alloc(nameToSubset *map[string]*Subset) (*map[string]int32, error) {
	replicas := int32(1)
	if ac.Spec.Replicas != nil {
		replicas = *ac.Spec.Replicas
	}

	minReplicasMap, maxReplicasMap, err := ac.validateAndCalculateMinMaxMap(replicas, nameToSubset)
	if err != nil {
		return nil, err
	}
	return ac.alloc(replicas, minReplicasMap, maxReplicasMap), nil
}

func (ac *elasticAllocator) validateAndCalculateMinMaxMap(replicas int32, nameToSubset *map[string]*Subset) (map[string]int32, map[string]int32, error) {
	numSubset := len(ac.Spec.Topology.Subsets)
	minReplicasMap := make(map[string]int32, numSubset)
	maxReplicasMap := make(map[string]int32, numSubset)
	runningReplicasMap := getSubsetRunningReplicas(nameToSubset)
	for index, subset := range ac.Spec.Topology.Subsets {
		minReplicas := int32(0)
		maxReplicas := int32(math.MaxInt32)
		if subset.MinReplicas != nil {
			minReplicas, _ = ParseSubsetReplicas(replicas, *subset.MinReplicas)
		}
		if subset.MaxReplicas != nil {
			maxReplicas, _ = ParseSubsetReplicas(replicas, *subset.MaxReplicas)
		}
		if ac.Spec.Topology.ScheduleStrategy.IsAdaptive() {
			unschedulable := isSubSetUnschedulable(subset.Name, nameToSubset)
			// This means that in the Adaptive scheduling strategy, an unschedulable subset can only be scaled down, not scaled up.
			if runningReplicas, ok := runningReplicasMap[subset.Name]; unschedulable && ok {
				klog.InfoS("Assign min(runningReplicas, minReplicas/maxReplicas) for unschedulable subset",
					"subset", subset.Name)
				minReplicas = integer.Int32Min(runningReplicas, minReplicas)
				maxReplicas = integer.Int32Min(runningReplicas, maxReplicas)
			}
			// To prevent healthy pod from being deleted
			if runningReplicas := runningReplicasMap[subset.Name]; !unschedulable && runningReplicas > minReplicas {
				klog.InfoS("Assign min(runningReplicas, maxReplicas) to minReplicas to avoid deleting running pods",
					"subset", subset.Name, "minReplicas", minReplicas, "runningReplicas", runningReplicas, "maxReplicas", maxReplicas)
				minReplicas = integer.Int32Min(runningReplicas, maxReplicas)
			}
		}

		minReplicasMap[subset.Name] = minReplicas
		maxReplicasMap[subset.Name] = maxReplicas

		if minReplicas > maxReplicas {
			return nil, nil, fmt.Errorf("subset[%d].maxReplicas must be more than or equal to minReplicas", index)
		}
	}
	klog.InfoS("elastic allocate maps calculated", "minReplicasMap", minReplicasMap, "maxReplicasMap", maxReplicasMap)
	return minReplicasMap, maxReplicasMap, nil
}

func (ac *elasticAllocator) alloc(replicas int32, minReplicasMap, maxReplicasMap map[string]int32) *map[string]int32 {
	allocated := int32(0)
	// Step 1: satisfy the minimum replicas of each subset firstly.
	subsetReplicas := make(map[string]int32, len(ac.Spec.Topology.Subsets))
	for _, subset := range ac.Spec.Topology.Subsets {
		minReplicas := minReplicasMap[subset.Name]
		addReplicas := integer.Int32Min(minReplicas, replicas-allocated)
		addReplicas = integer.Int32Max(addReplicas, 0)
		subsetReplicas[subset.Name] = addReplicas
		allocated += addReplicas
	}

	if allocated >= replicas { // no quota to allocate.
		return &subsetReplicas
	}

	// Step 2: satisfy the maximum replicas of each subset.
	for _, subset := range ac.Spec.Topology.Subsets {
		maxReplicas := maxReplicasMap[subset.Name]
		minReplicas := minReplicasMap[subset.Name]
		addReplicas := integer.Int32Min(maxReplicas-minReplicas, replicas-allocated)
		addReplicas = integer.Int32Max(addReplicas, 0)
		subsetReplicas[subset.Name] += addReplicas
		allocated += addReplicas
	}
	return &subsetReplicas
}
