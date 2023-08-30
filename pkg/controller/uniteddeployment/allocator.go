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
	"strings"

	"k8s.io/klog/v2"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
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

// GetAllocatedReplicas returns a mapping from subset to next replicas.
// Next replicas is allocated by replicasAllocator, which will consider the current replicas of each subset and
// new replicas indicated from UnitedDeployment.Spec.Topology.Subsets.
func GetAllocatedReplicas(nameToSubset *map[string]*Subset, ud *appsv1beta1.UnitedDeployment) (*map[string]int32, error) {
	subsetInfos := getSubsetInfos(nameToSubset, ud)

	var expectedReplicas int32 = -1
	if ud.Spec.Replicas != nil {
		expectedReplicas = *ud.Spec.Replicas
	}

	specifiedReplicas := getSpecifiedSubsetReplicas(expectedReplicas, ud)
	klog.V(4).Infof("UnitedDeployment %s/%s specifiedReplicas: %v", ud.Namespace, ud.Name, specifiedReplicas)
	// call SortToAllocator to sort all subset by subset.Replicas in order of increment
	return subsetInfos.SortToAllocator().AllocateReplicas(expectedReplicas, specifiedReplicas)
}

func (n subsetInfos) SortToAllocator() *replicasAllocator {
	sort.Sort(n)
	return &replicasAllocator{subsets: &n}
}

type replicasAllocator struct {
	subsets *subsetInfos
}

func (s *replicasAllocator) validateReplicas(replicas int32, subsetReplicasLimits *map[string]int32) error {
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
	klog.V(4).Infof("specifiedCount: %d, len(*s.subsets): %d", specifiedCount, len(*s.subsets))
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

func getSpecifiedSubsetReplicas(replicas int32, ud *appsv1beta1.UnitedDeployment) *map[string]int32 {
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
			klog.Warningf("Fail to consider the replicas of subset %s when parsing replicaLimits during managing replicas of UnitedDeployment %s/%s: %s",
				subsetDef.Name, ud.Namespace, ud.Name, err)
		}
	}

	return &replicaLimits
}

func getSubsetInfos(nameToSubset *map[string]*Subset, ud *appsv1beta1.UnitedDeployment) *subsetInfos {
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
func (s *replicasAllocator) AllocateReplicas(replicas int32, specifiedSubsetReplicas *map[string]int32) (
	*map[string]int32, error) {
	if err := s.validateReplicas(replicas, specifiedSubsetReplicas); err != nil {
		return nil, err
	}

	return s.normalAllocate(replicas, specifiedSubsetReplicas), nil
}

func (s *replicasAllocator) normalAllocate(expectedReplicas int32, specifiedSubsetReplicas *map[string]int32) *map[string]int32 {
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

func (s *replicasAllocator) toSubsetReplicaMap() *map[string]int32 {
	allocatedReplicas := map[string]int32{}
	for _, subset := range *s.subsets {
		allocatedReplicas[subset.SubsetName] = subset.Replicas
	}

	return &allocatedReplicas
}

func (s *replicasAllocator) String() string {
	result := ""
	sort.Sort(s.subsets)
	for _, subset := range *s.subsets {
		result = fmt.Sprintf("%s %s -> %d;", result, subset.SubsetName, subset.Replicas)
	}

	return result
}
