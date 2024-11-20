package sidecarset

import (
	"sort"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/updatesort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

type Strategy interface {
	// according to sidecarset's upgrade strategy, select the pods to be upgraded, include the following:
	//1. select which pods can be upgrade, the following:
	//	* pod must be not updated for the latest sidecarSet
	//	* If selector is not nil, this upgrade will only update the selected pods.
	//2. Sort Pods with default sequence
	//3. sort waitUpdateIndexes based on the scatter rules
	//4. calculate max count of pods can update with maxUnavailable
	//5. also return the pods that are not upgradable
	GetNextUpgradePods(control sidecarcontrol.SidecarControl, pods []*corev1.Pod) (upgradePods []*corev1.Pod, notUpgradablePods []*corev1.Pod)
}

type spreadingStrategy struct{}

var (
	globalSpreadingStrategy = &spreadingStrategy{}
)

func NewStrategy() Strategy {
	return globalSpreadingStrategy
}

func (p *spreadingStrategy) GetNextUpgradePods(control sidecarcontrol.SidecarControl, pods []*corev1.Pod) (upgradePods []*corev1.Pod, notUpgradablePods []*corev1.Pod) {
	sidecarset := control.GetSidecarset()
	// wait to upgrade pod index
	var waitUpgradedIndexes []int
	// because SidecarSet in-place update only support upgrading Image, if other fields are changed they will not be upgraded.
	var notUpgradableIndexes []int
	strategy := sidecarset.Spec.UpdateStrategy

	// If selector is not nil, check whether the pods is selected to upgrade
	isSelected := func(pod *corev1.Pod) bool {
		//when selector is nil, always return true
		if strategy.Selector == nil {
			return true
		}
		// if selector failed, always return false
		selector, err := util.ValidatedLabelSelectorAsSelector(strategy.Selector)
		if err != nil {
			klog.ErrorS(err, "SidecarSet rolling selector error", "sidecarSet", klog.KObj(sidecarset))
			return false
		}
		//matched
		if selector.Matches(labels.Set(pod.Labels)) {
			return true
		}
		//Not matched, then return false
		return false
	}

	//1. select which pods can be upgraded, the following:
	//	* pod must be not updated for the latest sidecarSet
	//	* If selector is not nil, this upgrade will only update the selected pods.
	//  * In kubernetes cluster, when inplace update pod, only fields such as image can be updated for the container.
	//  * It is to determine whether there are other fields that have been modified for pod.
	for index, pod := range pods {
		isUpdated := sidecarcontrol.IsPodSidecarUpdated(sidecarset, pod)
		if !isUpdated && isSelected(pod) {
			canUpgrade, consistent := control.IsSidecarSetUpgradable(pod)
			if canUpgrade && consistent {
				waitUpgradedIndexes = append(waitUpgradedIndexes, index)
			} else if !canUpgrade {
				// only image field can be in-place updated, if other fields changed, mark pod as not upgradable
				notUpgradableIndexes = append(notUpgradableIndexes, index)
			}
		}
	}

	klog.V(3).InfoS("SidecarSet's pods status", "sidecarSet", klog.KObj(sidecarset), "matchedPods", len(pods),
		"waitUpdated", len(waitUpgradedIndexes), "notUpgradable", len(notUpgradableIndexes))
	//2. sort Pods with default sequence and scatter
	waitUpgradedIndexes = SortUpdateIndexes(strategy, pods, waitUpgradedIndexes)

	//3. calculate to be upgraded pods number for the time
	needToUpgradeCount := calculateUpgradeCount(control, waitUpgradedIndexes, pods)
	if needToUpgradeCount < len(waitUpgradedIndexes) {
		waitUpgradedIndexes = waitUpgradedIndexes[:needToUpgradeCount]
	}

	//4. injectPods will be upgraded in the following process
	for _, idx := range waitUpgradedIndexes {
		upgradePods = append(upgradePods, pods[idx])
	}
	// 5. pods that are not upgradable will not be skipped in the following process
	for _, idx := range notUpgradableIndexes {
		notUpgradablePods = append(notUpgradablePods, pods[idx])
	}
	return
}

// SortUpdateIndexes sorts the given waitUpdateIndexes of Pods to update according to the SidecarSet update strategy.
func SortUpdateIndexes(strategy appsv1alpha1.SidecarSetUpdateStrategy, pods []*corev1.Pod, waitUpdateIndexes []int) []int {
	//Sort Pods with default sequence
	//	- Unassigned < assigned
	//	- PodPending < PodUnknown < PodRunning
	//	- Not ready < ready
	//	- Been ready for empty time < less time < more time
	//	- Pods with containers with higher restart counts < lower restart counts
	//	- Empty creation time pods < newer pods < older pods
	sort.Slice(waitUpdateIndexes, sidecarcontrol.GetPodsSortFunc(pods, waitUpdateIndexes))

	//sort waitUpdateIndexes based on the priority rules
	if strategy.PriorityStrategy != nil {
		waitUpdateIndexes = updatesort.NewPrioritySorter(strategy.PriorityStrategy).Sort(pods, waitUpdateIndexes)
	}

	//sort waitUpdateIndexes based on the scatter rules
	if strategy.ScatterStrategy != nil {
		// convert regular terms to scatter terms
		// for examples: labelA=* -> labelA=value1, labelA=value2...(labels in pod definition)
		scatter := parseUpdateScatterTerms(strategy.ScatterStrategy, pods)
		waitUpdateIndexes = updatesort.NewScatterSorter(scatter).Sort(pods, waitUpdateIndexes)
	}

	return waitUpdateIndexes
}

func calculateUpgradeCount(coreControl sidecarcontrol.SidecarControl, waitUpdateIndexes []int, pods []*corev1.Pod) int {
	totalReplicas := len(pods)
	sidecarSet := coreControl.GetSidecarset()
	strategy := sidecarSet.Spec.UpdateStrategy

	// default partition = 0, indicates all pods will been upgraded
	var partition int
	if strategy.Partition != nil {
		totalInt32 := int32(totalReplicas)
		partition, _ = util.CalculatePartitionReplicas(strategy.Partition, &totalInt32)
	}
	// indicates the partition pods will not be upgraded for the time
	if len(waitUpdateIndexes)-partition <= 0 {
		return 0
	}
	waitUpdateIndexes = waitUpdateIndexes[:(len(waitUpdateIndexes) - partition)]

	// max unavailable pods number, default is 1
	maxUnavailable := 1
	if strategy.MaxUnavailable != nil {
		maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(strategy.MaxUnavailable, totalReplicas, true)
	}

	var upgradeAndNotReadyCount int
	for _, pod := range pods {
		// 1. sidecar containers have been updated to the latest sidecarSet version, for pod.spec.containers
		// 2. whether pod.spec and pod.status is inconsistent after updating the sidecar containers
		// 3. whether pod is not ready
		if sidecarcontrol.IsPodSidecarUpdated(sidecarSet, pod) && (!coreControl.IsPodStateConsistent(pod, nil) || !coreControl.IsPodReady(pod)) {
			upgradeAndNotReadyCount++
		}
	}
	var needUpgradeCount int
	for _, i := range waitUpdateIndexes {
		// If pod is not ready, then not included in the calculation of maxUnavailable
		if !coreControl.IsPodReady(pods[i]) {
			needUpgradeCount++
			continue
		}
		if upgradeAndNotReadyCount >= maxUnavailable {
			break
		}
		upgradeAndNotReadyCount++
		needUpgradeCount++
	}
	return needUpgradeCount
}

func parseUpdateScatterTerms(scatter appsv1alpha1.UpdateScatterStrategy, pods []*corev1.Pod) appsv1alpha1.UpdateScatterStrategy {
	newScatter := appsv1alpha1.UpdateScatterStrategy{}
	for _, term := range scatter {
		if term.Value != "*" {
			newScatter = insertUpdateScatterTerm(newScatter, term)
			continue
		}
		// convert regular terms to scatter terms
		// examples: labelA=* -> labelA=value1, labelA=value2...
		newTerms := matchScatterTerms(pods, term.Key)
		for _, obj := range newTerms {
			newScatter = insertUpdateScatterTerm(newScatter, obj)
		}
	}
	return newScatter
}

func insertUpdateScatterTerm(scatter appsv1alpha1.UpdateScatterStrategy, term appsv1alpha1.UpdateScatterTerm) appsv1alpha1.UpdateScatterStrategy {
	for _, obj := range scatter {
		//if term already exist, return
		if term.Key == obj.Key && term.Value == obj.Value {
			return scatter
		}
	}
	scatter = append(scatter, term)
	return scatter
}

// convert regular terms to scatter terms
func matchScatterTerms(pods []*corev1.Pod, regularLabel string) []appsv1alpha1.UpdateScatterTerm {
	var terms []appsv1alpha1.UpdateScatterTerm
	for _, pod := range pods {
		labelValue, ok := pod.Labels[regularLabel]
		if !ok {
			continue
		}
		terms = append(terms, appsv1alpha1.UpdateScatterTerm{
			Key:   regularLabel,
			Value: labelValue,
		})
	}
	return terms
}
