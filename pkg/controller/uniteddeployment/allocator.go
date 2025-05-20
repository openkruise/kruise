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
	Alloc(existingSubsets map[string]*Subset) (map[string]int32, error)
}

func NewReplicaAllocator(ud *appsv1alpha1.UnitedDeployment) ReplicaAllocator {
	if ud.Spec.Topology.ScheduleStrategy.ShouldReserveUnschedulablePods() {
		return &reservationAllocator{ud}
	}
	if ud.Spec.Topology.ScheduleStrategy.IsAdaptive() {
		return &adaptiveAllocator{ud}
	}
	for _, subset := range ud.Spec.Topology.Subsets {
		if subset.MinReplicas != nil || subset.MaxReplicas != nil {
			return &minMaxAllocator{ud}
		}
	}
	return &specificAllocator{UnitedDeployment: ud}
}

// RunningReplicas refers to the number of Pods that an unschedulable subset can safely accommodate.
// Exceeding this number may lead to scheduling failures within that subset.
// This value is only effective in the Adaptive scheduling strategy.
func getSubsetReadyReplicas(existingSubsets map[string]*Subset) map[string]int32 {
	var result = make(map[string]int32)
	for name, subset := range existingSubsets {
		result[name] = subset.Status.ReadyReplicas
	}
	return result
}

func isSubSetUnschedulable(name string, existingSubsets map[string]*Subset) (unschedulable bool) {
	if subsetObj, ok := existingSubsets[name]; ok {
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
func (s *specificAllocator) Alloc(existingSubsets map[string]*Subset) (map[string]int32, error) {
	// SortToAllocator to sort all subset by subset.Replicas in order of increment
	s.subsets = getSubsetInfos(existingSubsets, s.UnitedDeployment)
	sort.Sort(s.subsets)

	var expectedReplicas int32 = -1
	if s.Spec.Replicas != nil {
		expectedReplicas = *s.Spec.Replicas
	}

	specifiedReplicas := getSpecifiedSubsetReplicas(expectedReplicas, s.UnitedDeployment)
	klog.V(4).InfoS("UnitedDeployment specifiedReplicas", "unitedDeployment", klog.KObj(s), "specifiedReplicas", specifiedReplicas)
	return s.AllocateReplicas(expectedReplicas, specifiedReplicas)
}

func (s *specificAllocator) validateReplicas(replicas int32, subsetReplicasLimits map[string]int32) error {
	if subsetReplicasLimits == nil {
		return nil
	}

	var specifiedReplicas int32
	for _, replicas := range subsetReplicasLimits {
		specifiedReplicas += replicas
	}

	specifiedCount := 0
	for _, subset := range *s.subsets {
		if _, exist := subsetReplicasLimits[subset.SubsetName]; exist {
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

func getSpecifiedSubsetReplicas(replicas int32, ud *appsv1alpha1.UnitedDeployment) map[string]int32 {
	replicaLimits := map[string]int32{}
	if ud.Spec.Topology.Subsets == nil {
		return replicaLimits
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

	return replicaLimits
}

func getSubsetInfos(existingSubsets map[string]*Subset, ud *appsv1alpha1.UnitedDeployment) *subsetInfos {
	infos := make(subsetInfos, len(ud.Spec.Topology.Subsets))
	for idx, subsetDef := range ud.Spec.Topology.Subsets {
		var replicas int32
		if subset, exist := existingSubsets[subsetDef.Name]; exist {
			replicas = subset.Spec.Replicas
		}
		infos[idx] = &nameToReplicas{SubsetName: subsetDef.Name, Replicas: replicas}
	}

	return &infos
}

// AllocateReplicas will first try to check the specifiedSubsetReplicas is valid or not.
// If valid , normalAllocate will be called. It will apply these specified replicas, then average the rest replicas to left unspecified subsets.
// If not, it will return error
func (s *specificAllocator) AllocateReplicas(replicas int32, specifiedSubsetReplicas map[string]int32) (
	map[string]int32, error) {
	if err := s.validateReplicas(replicas, specifiedSubsetReplicas); err != nil {
		return nil, err
	}

	return s.normalAllocate(replicas, specifiedSubsetReplicas), nil
}

func (s *specificAllocator) normalAllocate(expectedReplicas int32, specifiedSubsetReplicas map[string]int32) map[string]int32 {
	var specifiedReplicas int32
	specifiedSubsetCount := 0
	// Step 1: apply replicas to specified subsets, and mark them as specified = true.
	for _, subset := range *s.subsets {
		if replicas, exist := specifiedSubsetReplicas[subset.SubsetName]; exist {
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

func (s *specificAllocator) toSubsetReplicaMap() map[string]int32 {
	allocatedReplicas := map[string]int32{}
	for _, subset := range *s.subsets {
		allocatedReplicas[subset.SubsetName] = subset.Replicas
	}
	return allocatedReplicas
}

func (s *specificAllocator) String() string {
	result := ""
	sort.Sort(s.subsets)
	for _, subset := range *s.subsets {
		result = fmt.Sprintf("%s %s -> %d;", result, subset.SubsetName, subset.Replicas)
	}

	return result
}

type minMaxAllocator struct {
	*appsv1alpha1.UnitedDeployment
}

// Alloc returns a mapping from subset to next replicas.
// Next replicas is allocated by minMaxAllocator, which will consider the current minReplicas and maxReplicas
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
func (ac *minMaxAllocator) Alloc(_ map[string]*Subset) (map[string]int32, error) {
	var replicas int32
	if ac.Spec.Replicas != nil {
		replicas = *ac.Spec.Replicas
	}
	minReplicasMap, maxReplicasMap, err := ac.validateAndCalculateMinMaxMap(replicas)
	if err != nil {
		return nil, err
	}
	nextReplicas := allocateByMinMaxMap(replicas, minReplicasMap, maxReplicasMap, ac.Spec.Topology.Subsets)
	klog.V(4).InfoS("Got UnitedDeployment next replicas", "unitedDeployment", klog.KObj(ac.UnitedDeployment), "nextReplicas", nextReplicas)
	return nextReplicas, nil
}

func (ac *minMaxAllocator) validateAndCalculateMinMaxMap(replicas int32) (map[string]int32, map[string]int32, error) {
	numSubset := len(ac.Spec.Topology.Subsets)
	minReplicasMap := make(map[string]int32, numSubset)
	maxReplicasMap := make(map[string]int32, numSubset)
	for index, subset := range ac.Spec.Topology.Subsets {
		minReplicas := int32(0)
		maxReplicas := int32(math.MaxInt32)
		if subset.MinReplicas != nil {
			minReplicas, _ = ParseSubsetReplicas(replicas, *subset.MinReplicas)
		}
		if subset.MaxReplicas != nil {
			maxReplicas, _ = ParseSubsetReplicas(replicas, *subset.MaxReplicas)
		}
		minReplicasMap[subset.Name] = minReplicas
		maxReplicasMap[subset.Name] = maxReplicas

		if minReplicas > maxReplicas {
			return nil, nil, fmt.Errorf("subset[%d].maxReplicas(%d) must be more than or equal to minReplicas(%d)", index, maxReplicas, minReplicas)
		}
	}
	klog.InfoS("elastic allocate maps calculated", "minReplicasMap", minReplicasMap, "maxReplicasMap", maxReplicasMap, "unitedDeployment", klog.KObj(ac.UnitedDeployment))
	return minReplicasMap, maxReplicasMap, nil
}

// adaptiveAllocator is the allocator for default adaptive strategy
type adaptiveAllocator struct {
	*appsv1alpha1.UnitedDeployment
}

func (ac *adaptiveAllocator) Alloc(existingSubsets map[string]*Subset) (map[string]int32, error) {
	var replicas int32
	if ac.Spec.Replicas != nil {
		replicas = *ac.Spec.Replicas
	}
	minReplicasMap, maxReplicasMap, err := calculateRawMinMaxMap(replicas, ac.Spec.Topology.Subsets)
	if err != nil {
		return nil, err
	}
	klog.V(4).InfoS("raw min/max maps calculated", "unitedDeployment", klog.KObj(ac.UnitedDeployment),
		"minReplicasMap", minReplicasMap, "maxReplicasMap", maxReplicasMap)
	readyReplicas := getSubsetReadyReplicas(existingSubsets)
	for _, subset := range ac.Spec.Topology.Subsets {
		minReplicas, maxReplicas := minReplicasMap[subset.Name], maxReplicasMap[subset.Name]
		unschedulable := isSubSetUnschedulable(subset.Name, existingSubsets)
		if runningReplicas, ok := readyReplicas[subset.Name]; unschedulable && ok {
			minReplicas = max(runningReplicas, minReplicas)
			maxReplicas = min(runningReplicas, maxReplicas)
			klog.V(4).InfoS("adjusted min/max maps for unschedulable subset", "subset", subset.Name,
				"minReplicas", minReplicasMap[subset.Name], "maxReplicas", maxReplicasMap[subset.Name])
		}
		// All healthy pods are permanently allocated to a subset. We have to prevent them from being deleted
		if runningReplicas := readyReplicas[subset.Name]; !unschedulable && runningReplicas > minReplicas {
			klog.V(4).InfoS("adjusted minReplicas to avoid deleting running pods",
				"subset", subset.Name, "minReplicas", minReplicas, "runningReplicas", runningReplicas, "maxReplicas", maxReplicas)
			minReplicas = min(runningReplicas, maxReplicas)
		}
		minReplicasMap[subset.Name] = minReplicas
		maxReplicasMap[subset.Name] = maxReplicas
	}
	nextReplicas := allocateByMinMaxMap(replicas, minReplicasMap, maxReplicasMap, ac.Spec.Topology.Subsets)
	klog.V(4).InfoS("got UnitedDeployment next replicas", "unitedDeployment",
		klog.KObj(ac.UnitedDeployment), "nextReplicas", nextReplicas)
	return nextReplicas, nil
}

func allocateByMinMaxMap(replicas int32, minReplicasMap, maxReplicasMap map[string]int32, subsets []appsv1alpha1.Subset) map[string]int32 {
	allocated := int32(0)
	// Step 1: satisfy the minimum replicas of each subset firstly.
	subsetReplicas := make(map[string]int32, len(subsets))
	for _, subset := range subsets {
		minReplicas := minReplicasMap[subset.Name]
		addReplicas := min(minReplicas, replicas-allocated)
		addReplicas = max(addReplicas, 0)
		subsetReplicas[subset.Name] = addReplicas
		allocated += addReplicas
	}

	if allocated >= replicas { // no quota to allocate.
		return subsetReplicas
	}

	// Step 2: satisfy the maximum replicas of each subset.
	for _, subset := range subsets {
		maxReplicas := maxReplicasMap[subset.Name]
		minReplicas := minReplicasMap[subset.Name]
		addReplicas := min(maxReplicas-minReplicas, replicas-allocated)
		addReplicas = max(addReplicas, 0)
		subsetReplicas[subset.Name] += addReplicas
		allocated += addReplicas
	}
	return subsetReplicas
}

// reservationAllocator is an allocator for reservation adaptive strategy
type reservationAllocator struct {
	*appsv1alpha1.UnitedDeployment
}

func (ac *reservationAllocator) Alloc(existingSubsets map[string]*Subset) (map[string]int32, error) {
	var replicas int32
	if ac.Spec.Replicas != nil {
		replicas = *ac.Spec.Replicas
	}
	minReplicasMap, maxReplicasMap, err := calculateRawMinMaxMap(replicas, ac.Spec.Topology.Subsets)
	if err != nil {
		return nil, err
	}
	var subsets = make([]string, 0, len(ac.Spec.Topology.Subsets))
	for _, subset := range ac.Spec.Topology.Subsets {
		subsets = append(subsets, subset.Name)
	}
	nextReplicas := allocateByMinMaxMapAndReservation(replicas, minReplicasMap, maxReplicasMap, existingSubsets, subsets)
	klog.V(4).InfoS("got UnitedDeployment next replicas", "unitedDeployment",
		klog.KObj(ac.UnitedDeployment), "nextReplicas", nextReplicas)
	return nextReplicas, nil
}

type reservationAllocatorSubsetRecord struct {
	// maxAllocatable indicates the maximum number of replicas that can be safely allocated in a subset based on current information.
	// Specifically, if this value is non-negative, it means that allocating the corresponding number of additional replicas
	// in the subset is expected to succeed; if it is negative, it indicates that there are already -maxAllocatable reserved Pods
	// in the subset that are expected to or have already failed scheduling.
	//
	// Under normal circumstances, this value is initialized to MaxInt32, meaning that an arbitrarily large number of replicas
	// is expected to be schedulable. However, when there are reserved Pods or the subset has already been marked as unschedulable,
	// this value is initialized to the number of ready Pods in the subset.
	maxAllocatable int32

	// allocated replicas of the subset, which will be set to the value of the workload's spec.replicas after reconciliation.
	allocated int32
}

// allocate allocates `to` replicas to a subset and returns how many replicas is really allocated.
//
// Under normal circumstances, if X replicas are allocated to a subset, the number of remaining unassigned replicas
// should obviously decrease by X. However, if they are assigned to an unschedulable subset that only has Y healthy Pods,
// which is stored in field maxAllocatable, then starting from the (Y+1)th replica, they will certainly be reserved Pods
// that cannot be scheduled. Therefore, when X (X > Y) replicas are allocated to such an unschedulable subset, the number
// of remaining unassigned replicas can only decrease by Y. This means that the X-Y reserved replicas, which are expected
// to fail to start, will be redundantly assigned in other subsets to create temporary Pods as their substitutes.
// As the reserved Pods gradually recover, Y will gradually increase, and the number of temporary Pods, X-Y, will gradually
// decrease until all work is handed back to the recovered reserved Pods.
//
// Following are some examples (Assume there are 5 replicas remaining to be allocated in total):
//
//  1. Trying to allocate 3 replicas to a subset with 0 healthy Pods and 2 reserved Pods (typical unschedulable):
//     allocate(3, 5), -> 0,
//     the number of remaining unassigned replicas is decreased by 0, 3-0=3 temporary pods will be created.
//
//  2. Trying to allocate 3 replicas to a subset with 3 healthy Pods and no reserved Pods (schedulable):
//     allocate(3, 5), -> 3,
//     the number of remaining unassigned replicas is decreased by 3, 3-3=0 temporary pods will be created.
//
//  3. Trying to allocate 3 replicas to a subset with 1 healthy Pod and 1 reserved Pod:
//     allocate(3, 5), -> 1,
//     the number of remaining unassigned replicas is decreased by 1, 3-1=2 temporary pod will be created
//
//     - param to: will scale up the subset to the value, which should be greater than allocated.
//     - param rest: rest replicas can be allocated
//     - return allocated: the number of replicas that should be decreased. if not decreased, temporary pods will be created.
func (ar *reservationAllocatorSubsetRecord) allocate(to, rest int32) (allocated int32) {
	toAdd := min(to-ar.allocated, rest)
	ar.allocated += toAdd
	ar.maxAllocatable -= toAdd
	if ar.maxAllocatable < 0 {
		// Since maxAllocatable is negative after allocation, it indicates that a total of |maxAllocatable| replicas
		// are expected to fail scheduling (they will be marked as reserved Pods directly). Replicas expected to fail
		// scheduling should not be subtracted from the total replica count, but should instead be reallocated to other subsets.
		toAdd += ar.maxAllocatable // -= |ar.maxAllocatable|
		return max(0, toAdd)
	} else {
		// Since maxAllocatable remains non-negative, it indicates that the allocated toAdd replicas are still within
		// the expected capacity for successful scheduling, and thus the total replica count can be reduced accordingly.
		return toAdd
	}
}

func exportReservationRecords(records map[string]*reservationAllocatorSubsetRecord) map[string]int32 {
	var nextReplicas = make(map[string]int32, len(records))
	for name, record := range records {
		nextReplicas[name] = record.allocated
	}
	return nextReplicas
}

// allocateByMinMaxMapAndReservation allocates replicas to each subset through a unique allocate method
// (refer to the allocate method of reservationAllocatorSubsetRecord).
// During allocation, each subset is first assigned a number of replicas equal to minReplicas in sequence.
// Then, each subset is allocated up to maxReplicas (schedulable) or its allocated number (unschedulable) in sequence again.
func allocateByMinMaxMapAndReservation(replicas int32, minReplicasMap, maxReplicasMap map[string]int32,
	existingSubsets map[string]*Subset, subsetNames []string) map[string]int32 {
	// Prepare for the records
	records := make(map[string]*reservationAllocatorSubsetRecord, len(subsetNames))
	for _, name := range subsetNames {
		if subset := existingSubsets[name]; subset == nil || subset.Allocatable() {
			records[name] = &reservationAllocatorSubsetRecord{maxAllocatable: math.MaxInt32}
		} else {
			records[name] = &reservationAllocatorSubsetRecord{maxAllocatable: subset.Spec.Replicas - subset.Status.UnschedulableStatus.ReservedPods}
		}
	}
	// Step 1: satisfy the minimum replicas of each subset firstly.
	for _, name := range subsetNames {
		replicas -= records[name].allocate(minReplicasMap[name], replicas)
		if replicas <= 0 {
			return exportReservationRecords(records)
		}
	}

	// Step 2: allocate the rest replicas into subsets sequentially
	for _, name := range subsetNames {
		subset := existingSubsets[name]
		if subset == nil || !subset.Status.UnschedulableStatus.Unschedulable {
			replicas -= records[name].allocate(maxReplicasMap[name], replicas)
		} else {
			replicas -= records[name].allocate(subset.Spec.Replicas, replicas)
		}
		if replicas <= 0 {
			return exportReservationRecords(records)
		}
	}

	return exportReservationRecords(records)
}

func calculateRawMinMaxMap(replicas int32, subsets []appsv1alpha1.Subset) (map[string]int32, map[string]int32, error) {
	numSubset := len(subsets)
	minReplicasMap := make(map[string]int32, numSubset)
	maxReplicasMap := make(map[string]int32, numSubset)
	for index, subset := range subsets {
		minReplicas, maxReplicas := parseMinMaxReplicas(replicas, subset)
		if minReplicas > maxReplicas {
			return nil, nil, fmt.Errorf("subset[%d].maxReplicas(%d) must be more than or equal to minReplicas(%d)", index, maxReplicas, minReplicas)
		}
		minReplicasMap[subset.Name] = minReplicas
		maxReplicasMap[subset.Name] = maxReplicas
	}
	return minReplicasMap, maxReplicasMap, nil
}

func parseMinMaxReplicas(replicas int32, subset appsv1alpha1.Subset) (int32, int32) {
	minReplicas := int32(0)
	maxReplicas := int32(math.MaxInt32)
	if subset.MinReplicas != nil {
		minReplicas, _ = ParseSubsetReplicas(replicas, *subset.MinReplicas)
	}
	if subset.MaxReplicas != nil {
		maxReplicas, _ = ParseSubsetReplicas(replicas, *subset.MaxReplicas)
	}
	return minReplicas, maxReplicas
}
