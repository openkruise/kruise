package sidecarset

import (
	"sort"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util/updatesort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog"
)

type Strategy interface {
	// according to sidecarset's upgrade strategy, select the pods to be upgraded, include the following:
	//1. select which pods can be upgrade, the following:
	//	* pod must be not updated for the latest sidecarSet
	//	* If selector is not nil, this upgrade will only update the selected pods.
	//2. Sort Pods with default sequence
	//3. sort waitUpdateIndexes based on the scatter rules
	//4. calculate max count of pods can update with maxUnavailable
	GetNextUpgradePods(control sidecarcontrol.SidecarControl, pods []*corev1.Pod) []*corev1.Pod
}

type spreadingStrategy struct{}

var (
	globalSpreadingStrategy = &spreadingStrategy{}
)

func NewStrategy() Strategy {
	return globalSpreadingStrategy
}

func (p *spreadingStrategy) GetNextUpgradePods(control sidecarcontrol.SidecarControl, pods []*corev1.Pod) (upgradePods []*corev1.Pod) {
	sidecarset := control.GetSidecarset()
	// wait to upgrade pod index
	var waitUpgradedIndexes []int
	strategy := sidecarset.Spec.UpdateStrategy

	// If selector is not nil, check whether the pods is selected to upgrade
	isSelected := func(pod *corev1.Pod) bool {
		//when selector is nil, always return ture
		if strategy.Selector == nil {
			return true
		}
		// if selector failed, always return false
		selector, err := metav1.LabelSelectorAsSelector(strategy.Selector)
		if err != nil {
			klog.Errorf("sidecarSet(%s) rolling selector error, err: %v", sidecarset.Name, err)
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
		if !isUpdated && isSelected(pod) && control.IsSidecarSetCanUpgrade(pod) {
			waitUpgradedIndexes = append(waitUpgradedIndexes, index)
		}
	}

	//2. Sort Pods with default sequence
	//	- Unassigned < assigned
	//	- PodPending < PodUnknown < PodRunning
	//	- Not ready < ready
	//	- Been ready for empty time < less time < more time
	//	- Pods with containers with higher restart counts < lower restart counts
	//	- Empty creation time pods < newer pods < older pods
	sort.Slice(waitUpgradedIndexes, sidecarcontrol.GetPodsSortFunc(pods, waitUpgradedIndexes))

	//3. sort waitUpdateIndexes based on the scatter rules
	if strategy.ScatterStrategy != nil {
		// convert regular terms to scatter terms
		// for examples: labelA=* -> labelA=value1, labelA=value2...(labels in pod definition)
		scatter := parseUpdateScatterTerms(strategy.ScatterStrategy, pods)
		waitUpgradedIndexes = updatesort.NewScatterSorter(scatter).Sort(pods, waitUpgradedIndexes)
	}

	//4. calculate to be upgraded pods number for the time
	needToUpgradeCount := calculateUpgradeCount(control, waitUpgradedIndexes, pods)
	if needToUpgradeCount < len(waitUpgradedIndexes) {
		waitUpgradedIndexes = waitUpgradedIndexes[:needToUpgradeCount]
	}

	//5. injectPods will be upgraded in the following process
	for _, idx := range waitUpgradedIndexes {
		upgradePods = append(upgradePods, pods[idx])
	}
	return
}

func calculateUpgradeCount(coreControl sidecarcontrol.SidecarControl, waitUpdateIndexes []int, pods []*corev1.Pod) int {
	totalReplicas := len(pods)
	sidecarSet := coreControl.GetSidecarset()
	strategy := sidecarSet.Spec.UpdateStrategy

	// default partition = 0, indicates all pods will been upgraded
	var partition int
	if strategy.Partition != nil {
		partition, _ = intstrutil.GetValueFromIntOrPercent(strategy.Partition, totalReplicas, false)
	}
	// indicates the partition pods will not be upgraded for the time
	if len(waitUpdateIndexes)-partition <= 0 {
		return 0
	}
	waitUpdateIndexes = waitUpdateIndexes[:(len(waitUpdateIndexes) - partition)]

	// max unavailable pods number, default is 1
	maxUnavailable := 1
	if strategy.MaxUnavailable != nil {
		maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(strategy.MaxUnavailable, totalReplicas, false)
	}

	var upgradeAndNotReadyCount int
	for _, pod := range pods {
		if sidecarcontrol.IsPodSidecarUpdated(sidecarSet, pod) && !coreControl.IsPodConsistentAndReady(pod) {
			upgradeAndNotReadyCount++
		}
	}
	var needUpgradeCount int
	for _, i := range waitUpdateIndexes {
		// If pod is not ready, then not included in the calculation of maxUnavailable
		if !coreControl.IsPodConsistentAndReady(pods[i]) {
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
