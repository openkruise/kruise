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
	"encoding/json"
	"flag"
	"math"
	"reflect"

	v1 "k8s.io/api/core/v1"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/utils/integer"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/specifieddelete"
)

func init() {
	flag.BoolVar(&scalingExcludePreparingDelete, "cloneset-scaling-exclude-preparing-delete", false,
		"If true, CloneSet Controller will calculate scale number excluding Pods in PreparingDelete state.")
}

var (
	// scalingExcludePreparingDelete indicates whether the controller should calculate scale number excluding Pods in PreparingDelete state.
	scalingExcludePreparingDelete bool
)

type expectationDiffs struct {
	// scaleUpNum is a non-negative integer, which indicates the number that should scale up.
	scaleUpNum int
	// scaleNumOldRevision is a non-negative integer, which indicates the number of old revision Pods that should scale up.
	// It might be bigger than scaleUpNum, but controller will scale up at most scaleUpNum number of Pods.
	scaleUpNumOldRevision int
	// scaleDownNum is a non-negative integer, which indicates the number that should scale down.
	// It has excluded the number of Pods that are already specified to delete.
	scaleDownNum int
	// scaleDownNumOldRevision is a non-negative integer, which indicates the number of old revision Pods that should scale down.
	// It might be bigger than scaleDownNum, but controller will scale down at most scaleDownNum number of Pods.
	// It has excluded the number of old Pods that are already specified to delete.
	scaleDownNumOldRevision int

	// scaleUpLimit is the limit number of creating Pods when scaling up
	// it is limited by scaleStrategy.maxUnavailable
	scaleUpLimit int
	// deleteReadyLimit is the limit number of ready Pods that can be deleted
	// it is limited by UpdateStrategy.maxUnavailable
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

// String implement this to print information in klog
func (e expectationDiffs) String() string {
	b, _ := json.Marshal(e)
	return string(b)
}

type IsPodUpdateFunc func(pod *v1.Pod, updateRevision string) bool

// This is the most important algorithm in cloneset-controller.
// It calculates the pod numbers to scaling and updating for current CloneSet.
func calculateDiffsWithExpectation(cs *appsv1alpha1.CloneSet, pods []*v1.Pod, currentRevision, updateRevision string, isPodUpdate IsPodUpdateFunc) (res expectationDiffs) {
	coreControl := clonesetcore.New(cs)
	replicas := int(*cs.Spec.Replicas)
	var partition, maxSurge, maxUnavailable, scaleMaxUnavailable int
	if cs.Spec.UpdateStrategy.Partition != nil {
		if pValue, err := util.CalculatePartitionReplicas(cs.Spec.UpdateStrategy.Partition, cs.Spec.Replicas); err != nil {
			// TODO: maybe, we should block pod update if partition settings is wrong
			klog.ErrorS(err, "CloneSet partition value was illegal", "cloneSet", klog.KObj(cs))
		} else {
			partition = pValue
		}
	}
	if cs.Spec.UpdateStrategy.MaxSurge != nil {
		maxSurge, _ = intstrutil.GetValueFromIntOrPercent(cs.Spec.UpdateStrategy.MaxSurge, replicas, true)
		if cs.Spec.UpdateStrategy.Paused {
			maxSurge = 0
			klog.V(3).InfoS("Because CloneSet updateStrategy.paused=true, and Set maxSurge=0", "cloneSet", klog.KObj(cs))
		}
	}
	maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(cs.Spec.UpdateStrategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), replicas, maxSurge == 0)
	scaleMaxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(cs.Spec.ScaleStrategy.MaxUnavailable, intstrutil.FromInt(math.MaxInt32)), replicas, true)

	var newRevisionCount, newRevisionActiveCount, oldRevisionCount, oldRevisionActiveCount int
	var unavailableNewRevisionCount, unavailableOldRevisionCount int
	var toDeleteNewRevisionCount, toDeleteOldRevisionCount, preDeletingNewRevisionCount, preDeletingOldRevisionCount int
	defer func() {
		if res.isEmpty() {
			return
		}
		klog.V(1).InfoS("Calculate diffs for CloneSet", "cloneSet", klog.KObj(cs), "replicas", replicas, "partition", partition,
			"maxSurge", maxSurge, "maxUnavailable", maxUnavailable, "allPodCount", len(pods), "newRevisionCount", newRevisionCount,
			"newRevisionActiveCount", newRevisionActiveCount, "oldrevisionCount", oldRevisionCount, "oldRevisionActiveCount", oldRevisionActiveCount,
			"unavailableNewRevisionCount", unavailableNewRevisionCount, "unavailableOldRevisionCount", unavailableOldRevisionCount,
			"preDeletingNewRevisionCount", preDeletingNewRevisionCount, "preDeletingOldRevisionCount", preDeletingOldRevisionCount,
			"toDeleteNewRevisionCount", toDeleteNewRevisionCount, "toDeleteOldRevisionCount", toDeleteOldRevisionCount,
			"enabledPreparingUpdateAsUpdate", utilfeature.DefaultFeatureGate.Enabled(features.PreparingUpdateAsUpdate), "useDefaultIsPodUpdate", isPodUpdate == nil,
			"result", res)
	}()

	// If PreparingUpdateAsUpdate feature gate is enabled:
	// - when scaling, we hope the preparing-update pods should be regarded as update-revision pods,
	//   the isPodUpdate parameter will be IsPodUpdate function in pkg/util/revision/revision.go file;
	// - when updating, we hope the preparing-update pods should be regarded as current-revision pods,
	//   the isPodUpdate parameter will be EqualToRevisionHash function by default;
	if isPodUpdate == nil {
		isPodUpdate = func(pod *v1.Pod, updateRevision string) bool {
			return clonesetutils.EqualToRevisionHash("", pod, updateRevision)
		}
	}

	for _, p := range pods {
		if isPodUpdate(p, updateRevision) {

			newRevisionCount++

			switch state := lifecycle.GetPodLifecycleState(p); state {
			case appspub.LifecycleStatePreparingDelete:
				preDeletingNewRevisionCount++
			default:
				newRevisionActiveCount++

				if isSpecifiedDelete(cs, p) {
					toDeleteNewRevisionCount++
				} else if !IsPodAvailable(coreControl, p, cs.Spec.MinReadySeconds) {
					unavailableNewRevisionCount++
				}
			}

		} else {
			oldRevisionCount++

			switch state := lifecycle.GetPodLifecycleState(p); state {
			case appspub.LifecycleStatePreparingDelete:
				preDeletingOldRevisionCount++
			default:
				oldRevisionActiveCount++

				if isSpecifiedDelete(cs, p) {
					toDeleteOldRevisionCount++
				} else if !IsPodAvailable(coreControl, p, cs.Spec.MinReadySeconds) {
					unavailableOldRevisionCount++
				}
			}
		}
	}

	updateOldDiff := oldRevisionActiveCount - partition
	updateNewDiff := newRevisionActiveCount - (replicas - partition)
	totalUnavailable := preDeletingNewRevisionCount + preDeletingOldRevisionCount + unavailableNewRevisionCount + unavailableOldRevisionCount
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
			scaleSurge = integer.IntMin(integer.IntMax((totalUnavailable+toDeleteCount)-maxUnavailable, 0), toDeleteCount)
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

	// prepare for scale calculation
	currentTotalCount := len(pods)
	currentTotalOldCount := oldRevisionCount
	if shouldScalingExcludePreparingDelete(cs) {
		currentTotalCount = currentTotalCount - preDeletingOldRevisionCount - preDeletingNewRevisionCount
		currentTotalOldCount = currentTotalOldCount - preDeletingOldRevisionCount
	}
	expectedTotalCount := replicas + res.useSurge
	expectedTotalOldCount := partition + res.useSurgeOldRevision

	// scale up
	if num := expectedTotalCount - currentTotalCount; num > 0 {
		res.scaleUpNum = num
		res.scaleUpNumOldRevision = integer.IntMax(expectedTotalOldCount-currentTotalOldCount, 0)

		res.scaleUpLimit = integer.IntMin(res.scaleUpNum, integer.IntMax(scaleMaxUnavailable-totalUnavailable, 0))
	}

	// scale down
	// Note that this should exclude the number of Pods that are already specified to delete.
	if num := currentTotalCount - toDeleteOldRevisionCount - toDeleteNewRevisionCount - expectedTotalCount; num > 0 {
		res.scaleDownNum = num
		res.scaleDownNumOldRevision = integer.IntMax(currentTotalOldCount-toDeleteOldRevisionCount-expectedTotalOldCount, 0)
	}
	if toDeleteNewRevisionCount > 0 || toDeleteOldRevisionCount > 0 || res.scaleDownNum > 0 {
		res.deleteReadyLimit = integer.IntMax(maxUnavailable+(len(pods)-replicas)-totalUnavailable, 0)
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

func isPodReady(coreControl clonesetcore.Control, pod *v1.Pod) bool {
	return IsPodAvailable(coreControl, pod, 0)
}

func IsPodAvailable(coreControl clonesetcore.Control, pod *v1.Pod, minReadySeconds int32) bool {
	state := lifecycle.GetPodLifecycleState(pod)
	if state != "" && state != appspub.LifecycleStateNormal {
		return false
	}
	return coreControl.IsPodUpdateReady(pod, minReadySeconds)
}

func shouldScalingExcludePreparingDelete(cs *appsv1alpha1.CloneSet) bool {
	return scalingExcludePreparingDelete || cs.Labels[appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey] == "true"
}
