/*
Copyright 2021 The Kruise Authors.

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

package sync

import (
	"fmt"
	"reflect"
	"strconv"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/specifieddelete"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/utils/integer"
)

type expectationDiffs struct {
	// scaleNum is the diff number that should scale
	// '0' means no need to scale
	// positive number means need to scale out
	// negative number means need to scale in
	scaleNum int
	// scaleNumOldRevision is part of the scaleNum number
	// it indicates the scale number of old revision Pods
	scaleNumOldRevision int
	// deleteReadyLimit is the limit number of ready Pods that can be deleted
	// it is limited by maxUnavailable
	deleteReadyLimit int
	// useSurge is the number that temporarily expect to be above the desired replicas
	useSurge int
	// useSurgeOldRevision is part of the useSurge number
	// it indicates the above number of old revision Pods
	useSurgeOldRevision int

	// updateNum is the diff number that should update
	// '0' means no need to update
	// positive number means need to update more Pods to updateRevision
	// negative number means need to update more Pods to currentRevision (rollback)
	updateNum int
	// updateMaxUnavailable is the maximum number of ready Pods that can be updating
	updateMaxUnavailable int
}

func (e expectationDiffs) isEmpty() bool {
	return reflect.DeepEqual(e, expectationDiffs{})
}

// This is the most important algorithm in cloneset-controller.
// It calculates the pod numbers to scaling and updating for current CloneSet.
func calculateDiffsWithExpectation(cs *appsv1alpha1.CloneSet, pods []*v1.Pod, currentRevision, updateRevision string) (res expectationDiffs) {
	coreControl := clonesetcore.New(cs)
	replicas := int(*cs.Spec.Replicas)
	var partition, maxSurge, maxUnavailable int
	if cs.Spec.UpdateStrategy.Partition != nil {
		partition, _ = intstrutil.GetValueFromIntOrPercent(cs.Spec.UpdateStrategy.Partition, replicas, true)
		partition = integer.IntMin(partition, replicas)
	}
	if cs.Spec.UpdateStrategy.MaxSurge != nil {
		maxSurge, _ = intstrutil.GetValueFromIntOrPercent(cs.Spec.UpdateStrategy.MaxSurge, replicas, true)
	}
	maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(cs.Spec.UpdateStrategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), replicas, maxSurge == 0)

	var newRevisionCount, newRevisionActiveCount, oldRevisionCount, oldRevisionActiveCount int
	var notReadyNewRevisionCount, notReadyOldRevisionCount int
	var toDeleteNewRevisionCount, toDeleteOldRevisionCount, preDeletingCount int
	defer func() {
		if res.isEmpty() {
			return
		}
		klog.V(1).Infof("Calculate diffs for CloneSet %s/%s, replicas=%d, partition=%d, maxSurge=%d, maxUnavailable=%d,"+
			" allPods=%d, newRevisionPods=%d, newRevisionActivePods=%d, oldRevisionPods=%d, oldRevisionActivePods=%d,"+
			" notReadyNewRevisionCount=%d, notReadyOldRevisionCount=%d,"+
			" preDeletingCount=%d, toDeleteNewRevisionCount=%d, toDeleteOldRevisionCount=%d."+
			" Result: %+v",
			cs.Namespace, cs.Name, replicas, partition, maxSurge, maxUnavailable,
			len(pods), newRevisionCount, newRevisionActiveCount, oldRevisionCount, oldRevisionActiveCount,
			notReadyNewRevisionCount, notReadyOldRevisionCount,
			preDeletingCount, toDeleteNewRevisionCount, toDeleteOldRevisionCount,
			res)
	}()

	for _, p := range pods {
		if clonesetutils.EqualToRevisionHash("", p, updateRevision) {
			newRevisionCount++

			switch state := lifecycle.GetPodLifecycleState(p); state {
			case appspub.LifecycleStatePreparingDelete:
				preDeletingCount++
			default:
				newRevisionActiveCount++

				if isSpecifiedDelete(cs, p) {
					toDeleteNewRevisionCount++
				} else if !isPodReady(coreControl, p, cs.Spec.MinReadySeconds) {
					notReadyNewRevisionCount++
				}
			}

		} else {
			oldRevisionCount++

			switch state := lifecycle.GetPodLifecycleState(p); state {
			case appspub.LifecycleStatePreparingDelete:
				preDeletingCount++
			default:
				oldRevisionActiveCount++

				if isSpecifiedDelete(cs, p) {
					toDeleteOldRevisionCount++
				} else if !isPodReady(coreControl, p, cs.Spec.MinReadySeconds) {
					notReadyOldRevisionCount++
				}
			}
		}
	}

	updateOldDiff := oldRevisionActiveCount - partition
	updateNewDiff := newRevisionActiveCount - (replicas - partition)
	// If the currentRevision and updateRevision are consistent, Pods can only update to this revision
	// If the CloneSetPartitionRollback is not enabled, Pods can only update to the new revision
	if updateRevision == currentRevision || !utilfeature.DefaultFeatureGate.Enabled(features.CloneSetPartitionRollback) {
		updateOldDiff = integer.IntMax(updateOldDiff, 0)
		updateNewDiff = integer.IntMin(updateNewDiff, 0)
	}

	// calculate the number of surge to use
	if maxSurge > 0 {

		// Use surge for maxUnavailable not satisfied before scaling
		var scaleSurge, scaleOldRevisionSurge int
		if toDeleteCount := toDeleteNewRevisionCount + toDeleteOldRevisionCount; toDeleteCount > 0 {
			scaleSurge = integer.IntMin(integer.IntMax((notReadyNewRevisionCount+notReadyOldRevisionCount+toDeleteCount+preDeletingCount)-maxUnavailable, 0), toDeleteCount)
			if scaleSurge > toDeleteNewRevisionCount {
				scaleOldRevisionSurge = scaleSurge - toDeleteNewRevisionCount
			}
		}

		// Use surge for old and new revision updating
		var updateSurge, updateOldRevisionSurge int
		if util.IsIntPlusAndMinus(updateOldDiff, updateNewDiff) {
			if util.IntAbs(updateOldDiff) <= util.IntAbs(updateNewDiff) {
				updateSurge = util.IntAbs(updateOldDiff)
				if updateOldDiff < 0 {
					updateOldRevisionSurge = updateSurge
				}
			} else {
				updateSurge = util.IntAbs(updateNewDiff)
				if updateNewDiff > 0 {
					updateOldRevisionSurge = updateSurge
				}
			}
		}

		// It is because the controller is designed not to do scale and update in once reconcile
		if scaleSurge >= updateSurge {
			res.useSurge = integer.IntMin(maxSurge, scaleSurge)
			res.useSurgeOldRevision = integer.IntMin(res.useSurge, scaleOldRevisionSurge)
		} else {
			res.useSurge = integer.IntMin(maxSurge, updateSurge)
			res.useSurgeOldRevision = integer.IntMin(res.useSurge, updateOldRevisionSurge)
		}
	}

	res.scaleNum = replicas + res.useSurge - len(pods)
	if res.scaleNum > 0 {
		res.scaleNumOldRevision = integer.IntMax(partition+res.useSurgeOldRevision-oldRevisionCount, 0)
	} else if res.scaleNum < 0 {
		res.scaleNumOldRevision = integer.IntMin(partition+res.useSurgeOldRevision-oldRevisionCount, 0)
	}

	if toDeleteNewRevisionCount > 0 || toDeleteOldRevisionCount > 0 || res.scaleNum < 0 {
		res.deleteReadyLimit = integer.IntMax(maxUnavailable+(len(pods)-replicas)-preDeletingCount-notReadyNewRevisionCount-notReadyOldRevisionCount, 0)
	}

	// The consistency between scale and update will be guaranteed by syncCloneSet and expectations
	if util.IntAbs(updateOldDiff) <= util.IntAbs(updateNewDiff) {
		res.updateNum = updateOldDiff
	} else {
		res.updateNum = 0 - updateNewDiff
	}
	if res.updateNum != 0 {
		res.updateMaxUnavailable = maxUnavailable + len(pods) - replicas
	}

	return
}

func isSpecifiedDelete(cs *appsv1alpha1.CloneSet, pod *v1.Pod) bool {
	if specifieddelete.IsSpecifiedDelete(pod) {
		return true
	}
	for _, name := range cs.Spec.ScaleStrategy.PodsToDelete {
		if name == pod.Name {
			return true
		}
	}
	return false
}

func isPodReady(coreControl clonesetcore.Control, pod *v1.Pod, minReadySeconds int32) bool {
	state := lifecycle.GetPodLifecycleState(pod)
	if state != "" && state != appspub.LifecycleStateNormal {
		return false
	}
	return coreControl.IsPodUpdateReady(pod, minReadySeconds)
}

// ActivePodsWithDeletionCost type allows custom sorting of pods so a controller can pick the best ones to delete.
type ActivePodsWithDeletionCost []*v1.Pod

const (
	// PodDeletionCost can be used to set to an int32 that represent the cost of deleting
	// a pod compared to other pods belonging to the same ReplicaSet. Pods with lower
	// deletion cost are preferred to be deleted before pods with higher deletion cost.
	PodDeletionCost = "controller.kubernetes.io/pod-deletion-cost"
)

func (s ActivePodsWithDeletionCost) Len() int      { return len(s) }
func (s ActivePodsWithDeletionCost) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s ActivePodsWithDeletionCost) Less(i, j int) bool {
	// 1. Unassigned < assigned
	// If only one of the pods is unassigned, the unassigned one is smaller
	if s[i].Spec.NodeName != s[j].Spec.NodeName && (len(s[i].Spec.NodeName) == 0 || len(s[j].Spec.NodeName) == 0) {
		return len(s[i].Spec.NodeName) == 0
	}
	// 2. PodPending < PodUnknown < PodRunning
	podPhaseToOrdinal := map[v1.PodPhase]int{v1.PodPending: 0, v1.PodUnknown: 1, v1.PodRunning: 2}
	if podPhaseToOrdinal[s[i].Status.Phase] != podPhaseToOrdinal[s[j].Status.Phase] {
		return podPhaseToOrdinal[s[i].Status.Phase] < podPhaseToOrdinal[s[j].Status.Phase]
	}
	// 3. Not ready < ready
	// If only one of the pods is not ready, the not ready one is smaller
	if podutil.IsPodReady(s[i]) != podutil.IsPodReady(s[j]) {
		return !podutil.IsPodReady(s[i])
	}

	// 4. higher pod-deletion-cost < lower pod-deletion cost
	pi, _ := getDeletionCostFromPodAnnotations(s[i].Annotations)
	pj, _ := getDeletionCostFromPodAnnotations(s[j].Annotations)
	if pi != pj {
		return pi < pj
	}

	// TODO: take availability into account when we push minReadySeconds information from deployment into pods,
	//       see https://github.com/kubernetes/kubernetes/issues/22065
	// 5. Been ready for empty time < less time < more time
	// If both pods are ready, the latest ready one is smaller
	if podutil.IsPodReady(s[i]) && podutil.IsPodReady(s[j]) {
		readyTime1 := podReadyTime(s[i])
		readyTime2 := podReadyTime(s[j])
		if !readyTime1.Equal(readyTime2) {
			return afterOrZero(readyTime1, readyTime2)
		}
	}
	// 6. Pods with containers with higher restart counts < lower restart counts
	if maxContainerRestarts(s[i]) != maxContainerRestarts(s[j]) {
		return maxContainerRestarts(s[i]) > maxContainerRestarts(s[j])
	}
	// 7. Empty creation time pods < newer pods < older pods
	if !s[i].CreationTimestamp.Equal(&s[j].CreationTimestamp) {
		return afterOrZero(&s[i].CreationTimestamp, &s[j].CreationTimestamp)
	}
	return false
}

// afterOrZero checks if time t1 is after time t2; if one of them
// is zero, the zero time is seen as after non-zero time.
func afterOrZero(t1, t2 *metav1.Time) bool {
	if t1.Time.IsZero() || t2.Time.IsZero() {
		return t1.Time.IsZero()
	}
	return t1.After(t2.Time)
}

func podReadyTime(pod *v1.Pod) *metav1.Time {
	if podutil.IsPodReady(pod) {
		for _, c := range pod.Status.Conditions {
			// we only care about pod ready conditions
			if c.Type == v1.PodReady && c.Status == v1.ConditionTrue {
				return &c.LastTransitionTime
			}
		}
	}
	return &metav1.Time{}
}

func maxContainerRestarts(pod *v1.Pod) int {
	maxRestarts := 0
	for _, c := range pod.Status.ContainerStatuses {
		maxRestarts = integer.IntMax(maxRestarts, int(c.RestartCount))
	}
	return maxRestarts
}

// getDeletionCostFromPodAnnotations returns the integer value of pod-deletion-cost. Returns 0
// if not set or the value is invalid.
func getDeletionCostFromPodAnnotations(annotations map[string]string) (int32, error) {
	if value, exist := annotations[PodDeletionCost]; exist {
		// values that start with plus sign (e.g, "+10") or leading zeros (e.g., "008") are not valid.
		if !validFirstDigit(value) {
			return 0, fmt.Errorf("invalid value %q", value)
		}

		i, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			// make sure we default to 0 on error.
			return 0, err
		}
		return int32(i), nil
	}
	return 0, nil
}

func validFirstDigit(str string) bool {
	if len(str) == 0 {
		return false
	}
	return str[0] == '-' || (str[0] == '0' && str == "0") || (str[0] >= '1' && str[0] <= '9')
}
