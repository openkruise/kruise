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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
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
			return &elasticAllocator{ud}
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
		for _, pod := range subset.Spec.SubsetPods {
			if reserved, _ := IsPodMarkedAsReserved(pod); !reserved && pod.Status.Phase == corev1.PodRunning {
				result[name]++
			}
		}
		result[name] = min(subset.Status.ReadyReplicas, result[name])
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
func (ac *elasticAllocator) Alloc(_ map[string]*Subset) (map[string]int32, error) {
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

func (ac *elasticAllocator) validateAndCalculateMinMaxMap(replicas int32) (map[string]int32, map[string]int32, error) {
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
	info := ac.gatherInformation(existingSubsets)
	klog.V(4).InfoS("current allocation", "unitedDeployment", klog.KObj(ac.UnitedDeployment),
		"allocation", info.currentAllocation, "reserved", info.reservedMap, "normal", info.normalMap)
	// manage normal replicas
	var normalReplicasDiff = make(map[string]int32)
	// TODO：扩缩容的时候也有可能调整上下限，因而不能确定一定是向可分配subset中创建 Pod，需要继续设计
	if info.normalReplicas < replicas {
		// scale up
		var diffMinMap, diffMaxMap = make(map[string]int32), make(map[string]int32)
		for _, subset := range info.allocatableSubsets {
			diffMinMap[subset.Name] = max(0, minReplicasMap[subset.Name]-info.currentAllocation[subset.Name])
			diffMaxMap[subset.Name] = max(0, maxReplicasMap[subset.Name]-info.currentAllocation[subset.Name])
		}
		toCreate := replicas - info.normalReplicas
		normalReplicasDiff = allocateByMinMaxMap(toCreate, diffMinMap, diffMaxMap, info.allocatableSubsets)
	} else if info.normalReplicas > replicas {
		// scale down
		totalNormalPodsToDelete := info.normalReplicas - replicas
		for i := len(ac.Spec.Topology.Subsets) - 1; i >= 0; i-- {
			name := ac.Spec.Topology.Subsets[i].Name
			normal := info.normalMap[name]
			minimum := minReplicasMap[name]
			toDelete := min(normal-minimum, totalNormalPodsToDelete)
			normalReplicasDiff[name] = -toDelete
			totalNormalPodsToDelete -= toDelete
		}
	}
	// manage reserved replicas
	var reservedReplicasDiff = make(map[string]int32)
	for _, specSubset := range ac.Spec.Topology.Subsets {
		name := specSubset.Name
		subset := existingSubsets[name]
		if subset == nil || subset.Allocatable() {
			continue
		}
		if normalReplicasDiff[name] < 0 {
			// if a subset has to delete some normal pods,
			// it is obvious that all reserved pods shall be removed.
			reservedReplicasDiff[name] = -info.reservedMap[name] // reduced to 0
			continue
		}
		allocated := info.currentAllocation[name] + normalReplicasDiff[name]
		if allocated > replicas {
			reservedReplicasDiff[name] = replicas - allocated
		}
		minimum := minReplicasMap[name]
		maximum := maxReplicasMap[name]
		if allocated < minimum {
			reservedReplicasDiff[name] += minimum - allocated
		} else if allocated > maximum {
			reservedReplicasDiff[name] -= allocated - maximum
		}
	}
	var nextReplicas = make(map[string]int32)
	for _, subset := range ac.Spec.Topology.Subsets {
		nextReplicas[subset.Name] = info.currentAllocation[subset.Name] + normalReplicasDiff[subset.Name] + reservedReplicasDiff[subset.Name]
	}
	klog.V(4).InfoS("got replicas diff", "unitedDeployment", klog.KObj(ac.UnitedDeployment),
		"replicasDiff", normalReplicasDiff, "reservedReplicasDiff", reservedReplicasDiff, "nextReplicas", nextReplicas)
	return nextReplicas, nil
}

type reservationAllocatorInformation struct {
	currentAllocation  map[string]int32
	allocatableSubsets []appsv1alpha1.Subset
	normalMap          map[string]int32
	reservedMap        map[string]int32
	normalReplicas     int32
}

func (ac *reservationAllocator) gatherInformation(existingSubsets map[string]*Subset) reservationAllocatorInformation {
	info := reservationAllocatorInformation{
		currentAllocation: make(map[string]int32),
		normalMap:         make(map[string]int32),
		reservedMap:       make(map[string]int32),
	}
	allReservedPods := make(map[string]struct{})
	for _, subset := range existingSubsets {
		for name := range subset.Status.UnschedulableStatus.ReservedPods {
			allReservedPods[name] = struct{}{}
		}
	}
	for _, specSubset := range ac.Spec.Topology.Subsets {
		name := specSubset.Name
		subset := existingSubsets[name]
		if subset == nil {
			info.allocatableSubsets = append(info.allocatableSubsets, specSubset)
			continue
		}
		if subset.Allocatable() {
			info.allocatableSubsets = append(info.allocatableSubsets, specSubset)
		}
		info.currentAllocation[name] = subset.Spec.Replicas
		for _, pod := range subset.Spec.SubsetPods {
			if _, ok := allReservedPods[pod.Name]; ok {
				info.reservedMap[name]++
			} else {
				info.normalMap[name]++
			}
		}
		info.normalReplicas += info.normalMap[name]
	}
	return info
}

func (ac *reservationAllocator) allocateStaticReplicas(replicas int32) (map[string]int32, map[string]int32, map[string]int32, error) {
	minReplicasMap, maxReplicasMap, err := calculateRawMinMaxMap(replicas, ac.Spec.Topology.Subsets)
	if err != nil {
		return nil, nil, nil, err
	}
	klog.V(4).InfoS("before allocating static replicas", "unitedDeployment", klog.KObj(ac.UnitedDeployment),
		"minReplicasMap", minReplicasMap, "maxReplicasMap", maxReplicasMap)
	staticAllocation := allocateByMinMaxMap(replicas, minReplicasMap, maxReplicasMap, ac.Spec.Topology.Subsets)
	klog.V(4).InfoS("got static allocation",
		"unitedDeployment", klog.KObj(ac.UnitedDeployment), "nextReplicas", staticAllocation)
	return staticAllocation, minReplicasMap, maxReplicasMap, nil
}

func (ac *reservationAllocator) allocateTemporaryAndReservedReplicas(replicas int32, minReplicasMap, maxReplicasMap,
	staticAllocation map[string]int32, existingSubsets map[string]*Subset, reservedReplicas map[string]int32) map[string]int32 {
	var temporaryPods int32
	for _, subset := range ac.Spec.Topology.Subsets {
		reserved := reservedReplicas[subset.Name]
		allocated := staticAllocation[subset.Name]
		temporaryPods = min(replicas, reserved+temporaryPods)
		minimum := minReplicasMap[subset.Name]
		maximum := maxReplicasMap[subset.Name]
		if es := existingSubsets[subset.Name]; es != nil && !es.Allocatable() {
			minReplicasMap[subset.Name] = 0
			maxReplicasMap[subset.Name] = 0
		} else {
			minReplicasMap[subset.Name] = max(0, minimum-allocated)
			maxReplicasMap[subset.Name] = max(0, maximum-allocated)
		}
	}
	klog.V(4).InfoS("before allocating temporary replicas", "unitedDeployment", klog.KObj(ac.UnitedDeployment),
		"minReplicasMap", minReplicasMap, "maxReplicasMap", maxReplicasMap)
	temporaryAllocation := allocateByMinMaxMap(temporaryPods, minReplicasMap, maxReplicasMap, ac.Spec.Topology.Subsets)
	klog.V(4).InfoS("got temporary allocation",
		"unitedDeployment", klog.KObj(ac.UnitedDeployment), "nextReplicas", temporaryAllocation)
	return temporaryAllocation
}

func (ac *reservationAllocator) allocateReservedReplicas(reservedPods int32, minReplicasMap, maxReplicasMap,
	staticAllocation, reservedReplicas map[string]int32) map[string]int32 {
	for _, subset := range ac.Spec.Topology.Subsets {
		reserved := reservedReplicas[subset.Name]
		allocated := staticAllocation[subset.Name]
		minReplicasMap[subset.Name] = min(reserved, allocated)
		maxReplicasMap[subset.Name] = max(reserved, allocated)
	}
	klog.V(4).InfoS("before allocating reserved replicas", "unitedDeployment", klog.KObj(ac.UnitedDeployment),
		"minReplicasMap", minReplicasMap, "maxReplicasMap", maxReplicasMap)
	reservedAllocation := allocateByMinMaxMap(reservedPods, minReplicasMap, maxReplicasMap, ac.Spec.Topology.Subsets)
	klog.V(4).InfoS("got reserved allocation",
		"unitedDeployment", klog.KObj(ac.UnitedDeployment), "nextReplicas", reservedAllocation)
	return reservedAllocation
}

// The sum of getSubsetReservedReplicas result is the number of temporary pods.
func (ac *reservationAllocator) getSubsetReservedReplicas(existingSubsets map[string]*Subset, staticAllocation map[string]int32) map[string]int32 {
	readyReplicas := getSubsetReadyReplicas(existingSubsets)
	var result = make(map[string]int32)
	for name, subset := range existingSubsets {
		if subset.Allocatable() {
			continue
		}
		if allocation := staticAllocation[name]; allocation > subset.Spec.Replicas {
			// When scaling up, all newly-created pods are considered as reserved.
			// Due to the high risk of their inability to start, it will be necessary to
			// simultaneously create temporary Pods with a higher probability of successful startup.
			result[name] = allocation - readyReplicas[name]
		} else if allocation < subset.Spec.Replicas {
			// it's necessary to take into consideration in advance that
			// when scaling down, reserved pods will be deleted first
			result[name] = max(0, subset.Status.UnschedulableStatus.ReservedPodNum-(subset.Spec.Replicas-allocation))
		} else {
			result[name] = subset.Status.UnschedulableStatus.ReservedPodNum
		}
	}
	return result
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

// minMaxAdjustFunc adjusts the min/max map for each subset based on different adaptive scheduling algorithms.
//type minMaxAdjustFunc func(minReplicasMap, maxReplicasMap map[string]int32, existingSubsets *map[string]*Subset,
//	subsets []appsv1alpha1.Subset) (map[string]int32, map[string]int32)

//func adjustMinMaxMapInDefaultStrategy(minReplicasMap, maxReplicasMap map[string]int32,
//	existingSubsets *map[string]*Subset, subsets []appsv1alpha1.Subset) (map[string]int32, map[string]int32) {
//	readyReplicas := getSubsetReadyReplicas(existingSubsets)
//	for _, subset := range subsets {
//		minReplicas, maxReplicas := minReplicasMap[subset.Name], maxReplicasMap[subset.Name]
//		unschedulable := isSubSetUnschedulable(subset.Name, existingSubsets)
//		if runningReplicas, ok := readyReplicas[subset.Name]; unschedulable && ok {
//			minReplicas = max(runningReplicas, minReplicas)
//			maxReplicas = min(runningReplicas, maxReplicas)
//			klog.V(4).InfoS("adjusted min/max maps for unschedulable subset", "subset", subset.Name,
//				"minReplicas", minReplicasMap[subset.Name], "maxReplicas", maxReplicasMap[subset.Name])
//		}
//		// All healthy pods are permanently allocated to a subset. We have to prevent them from being deleted
//		if runningReplicas := readyReplicas[subset.Name]; !unschedulable && runningReplicas > minReplicas {
//			klog.V(4).InfoS("Assign min(runningReplicas, maxReplicas) to minReplicas to avoid deleting running pods",
//				"subset", subset.Name, "minReplicas", minReplicas, "runningReplicas", runningReplicas, "maxReplicas", maxReplicas)
//			minReplicas = min(runningReplicas, maxReplicas)
//		}
//		minReplicasMap[subset.Name] = minReplicas
//		maxReplicasMap[subset.Name] = maxReplicas
//	}
//	return minReplicasMap, maxReplicasMap
//}

//func adjustMinMaxMapInReservedStrategy(minReplicasMap, maxReplicasMap map[string]int32,
//	existingSubsets *map[string]*Subset, subsets []appsv1alpha1.Subset) (map[string]int32, map[string]int32) {
//	readyReplicas := getSubsetReadyReplicas(existingSubsets)
//	for _, subset := range subsets {
//		minReplicas, maxReplicas := minReplicasMap[subset.Name], maxReplicasMap[subset.Name]
//		unschedulable := isSubSetUnschedulable(subset.Name, existingSubsets)
//		if runningReplicas, ok := readyReplicas[subset.Name]; unschedulable && ok {
//			minReplicasMap[subset.Name] = min(runningReplicas, minReplicas)
//			maxReplicasMap[subset.Name] = min(runningReplicas, maxReplicas)
//			klog.V(4).InfoS("adjusted min/max maps for unschedulable subset", "subset", subset.Name,
//				"runningReplicas", runningReplicas, "minReplicas", minReplicas, "maxReplicas", maxReplicas)
//		}
//	}
//	return minReplicasMap, maxReplicasMap
//}

// replicasAdjustFunc adjusts the next replicas for each subset based on different adaptive scheduling algorithms.
//type replicasAdjustFunc func(expectedReplicas *map[string]int32, existingSubsets *map[string]*Subset, totalReplicas int32,
//	minReplicasMap map[string]int32, subsets []appsv1alpha1.Subset) *map[string]int32

// adjustNextReplicasInReservedStrategy adjusts the next replicas for each subset based on the temporary adaptive scheduling strategy.
// It ensures that subsets marked as unschedulable retain their current replicas and that the total number of replicas does not exceed the specified totalReplicas.
//func adjustNextReplicasInReservedStrategy(expectedReplicas *map[string]int32, existingSubsets *map[string]*Subset,
//	totalReplicas int32, minReplicasMap map[string]int32, subsets []appsv1alpha1.Subset) *map[string]int32 {
//	runningReplicas := getSubsetReadyReplicas(existingSubsets)
//	reservedReplicas := getSubsetReservedReplicas(existingSubsets)
//
//	var totalRunningPods, totalReservedPods int32
//	for i := 0; i < len(subsets); i++ {
//		totalRunningPods += runningReplicas[subsets[i].Name]
//		totalReservedPods += reservedReplicas[subsets[i].Name]
//	}
//
//	// Step 1: Delete redundant (exceeds the total replicas) running pods in the reverse order of the subset.
//	expectedRunningReplicas := min(totalRunningPods, totalReplicas)
//	for i := len(subsets) - 1; i >= 0; i-- {
//		if totalRunningPods <= expectedRunningReplicas {
//			break
//		}
//		name := subsets[i].Name
//		expected := (*expectedReplicas)[name]
//		// The value of toDelete is the smaller of:
//		//  - the maximum number that can be deleted in the subset
//		//  - and the total number that still needs to be deleted.
//		toDelete := min(runningReplicas[name]-expected, totalRunningPods-totalReplicas)
//		runningReplicas[name] -= toDelete
//		totalRunningPods -= toDelete
//	}
//
//	// Step 2: Delete redundant reserved pods in the order of the subset.
//	var countedRunningPods int32
//	for i := 0; i < len(subsets); i++ {
//		name := subsets[i].Name
//		countedRunningPods += runningReplicas[name]
//		if countedRunningPods >= totalReplicas {
//			reservedReplicas[name] = 0
//		}
//	}
//
//	// Step 3: Fulfill subsets to expected, which creates new reserved pods.
//	for name, expected := range *expectedReplicas {
//		(*expectedReplicas)[name] = max(runningReplicas[name]+reservedReplicas[name], expected)
//	}
//	return expectedReplicas
//}

//func calculateMinMaxMapAdaptively(replicas int32, ud *appsv1alpha1.UnitedDeployment, existingSubsets *map[string]*Subset,
//	adjustFunc minMaxAdjustFunc) (map[string]int32, map[string]int32, error) {
//	minReplicasMap, maxReplicasMap, err := calculateRawMinMaxMap(replicas, ud.Spec.Topology.Subsets)
//	if err != nil {
//		return nil, nil, err
//	}
//	klog.V(4).InfoS("raw min/max maps calculated", "minReplicasMap", minReplicasMap, "maxReplicasMap", maxReplicasMap, "unitedDeployment", klog.KObj(ud))
//	minReplicasMap, maxReplicasMap = adjustFunc(minReplicasMap, maxReplicasMap, existingSubsets, ud.Spec.Topology.Subsets)
//	return minReplicasMap, maxReplicasMap, nil
//}

//func allocateReplicasAdaptively(ud *appsv1alpha1.UnitedDeployment, existingSubsets *map[string]*Subset,
//	preFunc minMaxAdjustFunc, postFunc replicasAdjustFunc) (*map[string]int32, error) {
//	var replicas int32
//	if ud.Spec.Replicas != nil {
//		replicas = *ud.Spec.Replicas
//	}
//	minReplicasMap, maxReplicasMap, err := calculateMinMaxMapAdaptively(replicas, ud, existingSubsets, preFunc)
//	if err != nil {
//		return nil, err
//	}
//	nextReplicas := allocateByMinMaxMap(replicas, minReplicasMap, maxReplicasMap, ud.Spec.Topology.Subsets)
//	klog.V(4).InfoS("got UnitedDeployment next replicas", "unitedDeployment",
//		klog.KObj(ud), "nextReplicas", nextReplicas)
//	if postFunc != nil {
//		nextReplicas = postFunc(nextReplicas, existingSubsets, replicas, minReplicasMap, ud.Spec.Topology.Subsets)
//		klog.V(4).InfoS("adjusted UnitedDeployment next replicas for adaptive strategy",
//			"unitedDeployment", klog.KObj(ud), "nextReplicas", nextReplicas)
//	}
//	return nextReplicas, nil
//}
