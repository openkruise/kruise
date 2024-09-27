package hotstandby

import (
	v1 "k8s.io/api/core/v1"
	"strconv"
)

const (
	// PodHotStandbyEnableKey Pod label key that shows whether hot-standby is enabled
	PodHotStandbyEnableKey = "hotstandby.apps.kruise.io/hot-standby"

	// PodHotStandbyRecoveryKey Pod label key that shows hot-standby pod is recovered to normal
	PodHotStandbyRecoveryKey = "hotstandby.apps.kruise.io/plan-to-recover"

	True = "true"

	False = "false"
)

// IsHotStandbyPod returns if Pod has hot-standby label
func IsHotStandbyPod(pod *v1.Pod) bool {
	if len(pod.Labels) <= 0 {
		return false
	}

	if value, _ := strconv.ParseBool(pod.Labels[PodHotStandbyEnableKey]); value {
		return true
	}

	return false
}

func PutHotStandbyPodLabel(pod *v1.Pod) {
	pod.Labels[PodHotStandbyEnableKey] = True
}

func PutNormalPodLabel(pod *v1.Pod) {
	pod.Labels[PodHotStandbyEnableKey] = False
}

// FilterOutNormalPods returns Pods matched and unmatched the hot-standby label
func FilterOutNormalPods(pods []*v1.Pod) (normal []*v1.Pod) {
	for _, p := range pods {
		if !IsHotStandbyPod(p) {
			normal = append(normal, p)
		}
	}
	return
}

// FilterOutHotStandbyPods returns Pods matched and unmatched the hot-standby label
func FilterOutHotStandbyPods(pods []*v1.Pod) (hotStandby []*v1.Pod) {
	for _, p := range pods {
		if IsHotStandbyPod(p) {
			hotStandby = append(hotStandby, p)
		}
	}
	return
}

// GetHotStandbyPodIndexes returns hot-standby Pod indexes by update num
func GetHotStandbyPodIndexes(updateNum int, waitUpdateIndexes []int) []int {
	if updateNum < len(waitUpdateIndexes) {
		waitUpdateIndexes = waitUpdateIndexes[:updateNum]
	}

	return waitUpdateIndexes
}
