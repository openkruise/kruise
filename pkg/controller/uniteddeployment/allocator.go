package uniteddeployment

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/klog"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/uniteddeployment/subset"
)

type nameToReplicas struct {
	Name       string
	SubsetName string
	Replicas   int32
	Partition  int32
	Limited    bool
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
	var tmp *nameToReplicas
	tmp = n[i]
	n[i] = n[j]
	n[j] = tmp
}

// GetAllocatedReplicas returns a mapping from subset to next replicas.
// Next replicas is allocated by replicasAllocator, which will consider the current replicas of each subset and
// new replicas indicated from UnitedDeployment.Spec.Topology.Subsets.
func GetAllocatedReplicas(nameToSubset map[string]*subset.Subset, ud *appsv1alpha1.UnitedDeployment) map[string]int32 {
	replicaLimits, replicasAllocator := getReplicasAllocator(nameToSubset, ud)
	replicasAllocator.AllocateReplicas(*ud.Spec.Replicas, replicaLimits)

	return replicasAllocator.AllocatedReplicas
}

func (n subsetInfos) SortToAllocator() *replicasAllocator {
	sort.Sort(n)
	return &replicasAllocator{&n, map[string]int32{}}
}

type replicasAllocator struct {
	subsets           *subsetInfos
	AllocatedReplicas map[string]int32
}

func (s *replicasAllocator) effectiveReplicasLimitation(replicas int32, subsetReplicasLimits map[string]int32) bool {
	if subsetReplicasLimits == nil {
		return false
	}

	var specifiedReplicas int32
	for _, replicas := range subsetReplicasLimits {
		specifiedReplicas += replicas
	}

	if specifiedReplicas > replicas {
		return false
	}

	limitedCount := 0
	for _, subset := range *s.subsets {
		if _, exist := subsetReplicasLimits[subset.SubsetName]; exist {
			limitedCount++
		}
	}

	if limitedCount != len(subsetReplicasLimits) {
		return false
	}

	return true
}

func getReplicasAllocator(nameToSubset map[string]*subset.Subset, ud *appsv1alpha1.UnitedDeployment) (map[string]int32, *replicasAllocator) {
	nameToSubsetDef := map[string]*appsv1alpha1.Subset{}
	for i := range ud.Spec.Topology.Subsets {
		nameToSubsetDef[ud.Spec.Topology.Subsets[i].Name] = &ud.Spec.Topology.Subsets[i]
	}

	ntr := make([]*nameToReplicas, len(nameToSubset))
	replicaLimits := map[string]int32{}

	idx := 0
	for name, subset := range nameToSubset {
		subsetDef, exist := nameToSubsetDef[name]
		if exist && subsetDef.Replicas != nil {
			if unitLimit, err := ParseSubsetReplicas(*ud.Spec.Replicas, *subsetDef.Replicas); err == nil {
				replicaLimits[name] = unitLimit
			} else {
				klog.Warningf("Fail to consider the replicas of subset %s/%s when parsing replicaLimits during managing replicas of UnitedDeployment %s/%s: %s",
					subset.Namespace, subset.Name, ud.Namespace, ud.Name, err)
			}
		}

		ntr[idx] = &nameToReplicas{Name: subset.Name, SubsetName: name, Replicas: subset.Spec.Replicas}
		idx++
	}
	replicasAllocator := subsetInfos(ntr).SortToAllocator()
	return replicaLimits, replicasAllocator
}

func (s *replicasAllocator) AllocateReplicas(replicas int32, subsetReplicasLimits map[string]int32) {
	if !s.effectiveReplicasLimitation(replicas, subsetReplicasLimits) {
		s.normalAllocate(replicas, 0, s.subsets.Len())
		return
	}

	var specifiedReplicas int32
	for _, subset := range *s.subsets {
		if limit, exist := subsetReplicasLimits[subset.SubsetName]; exist {
			specifiedReplicas += limit
			subset.Replicas = limit
			subset.Limited = true
		}
	}

	index := s.subsets.Len() - 1
	for i := index; i >= 0; i-- {
		if s.subsets.Get(i).Limited {
			tmp := (*s.subsets)[i]
			(*s.subsets)[i] = (*s.subsets)[index]
			(*s.subsets)[index] = tmp
			index--
		}
	}

	// apply the replicas to the left subsets
	s.normalAllocate(replicas, specifiedReplicas, index+1)
	return
}

func (s *replicasAllocator) normalAllocate(replicas, excludedReplicas int32, consideredLen int) {
	var replicasAfterLimit int32
	for _, nts := range *s.subsets {
		replicasAfterLimit += nts.Replicas
	}

	diff := replicas - replicasAfterLimit
	replicas -= excludedReplicas
	if diff > 0 {
		var average int32
		var reminder int32
		var i int
		var leftSubsets int32
		for i = consideredLen - 1; i >= 0; i-- {
			leftSubsets = int32(i) + 1
			average = replicas / leftSubsets
			max := average
			reminder = replicas % leftSubsets
			if reminder > 0 {
				max++
			}
			if max < s.subsets.Get(i).Replicas {
				replicas -= s.subsets.Get(i).Replicas
				continue
			}
			break
		}

		for j := i; j > -1; j-- {
			if reminder > 0 {
				s.subsets.Get(j).Replicas = average + 1
				reminder--
			} else {
				s.subsets.Get(j).Replicas = average
			}
		}

	} else if diff < 0 {
		var average int32
		var reminder int32
		var i int
		var leftSubsets int32
		for i = 0; i < consideredLen; i++ {
			leftSubsets = int32(consideredLen - i)
			average = replicas / leftSubsets
			reminder = replicas % leftSubsets
			if average > s.subsets.Get(i).Replicas {
				replicas -= s.subsets.Get(i).Replicas
				continue
			}
			break
		}

		for j := i; j < consideredLen; j++ {
			if leftSubsets <= reminder {
				s.subsets.Get(j).Replicas = average + 1
			} else {
				s.subsets.Get(j).Replicas = average
				leftSubsets--
			}
		}
	}

	for _, subset := range *s.subsets {
		s.AllocatedReplicas[subset.SubsetName] = subset.Replicas
	}
}

func (s *replicasAllocator) String() string {
	result := ""
	sort.Sort(s.subsets)
	for _, subset := range *s.subsets {
		result = fmt.Sprintf("%s %s -> %d;", result, subset.Name, subset.Replicas)
	}

	return result
}
