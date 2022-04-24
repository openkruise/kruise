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
	"math"
	"reflect"

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
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
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

// This is the most important algorithm in cloneset-controller.
// It calculates the pod numbers to scaling and updating for current CloneSet.
func calculateDiffsWithExpectation(cs *appsv1alpha1.CloneSet, pods []*v1.Pod, currentRevision, updateRevision string) (res expectationDiffs) {
	coreControl := clonesetcore.New(cs)
	replicas := int(*cs.Spec.Replicas)
	var partition, maxSurge, maxUnavailable, scaleMaxUnavailable int
	if cs.Spec.UpdateStrategy.Partition != nil {
		if pValue, err := util.CalculatePartitionReplicas(cs.Spec.UpdateStrategy.Partition, cs.Spec.Replicas); err != nil {
			// TODO: maybe, we should block pod update if partition settings is wrong
			klog.Errorf("CloneSet %s/%s partition value is illegal", cs.Namespace, cs.Name)
		} else {
			partition = pValue
		}
	}
	if cs.Spec.UpdateStrategy.MaxSurge != nil {
		maxSurge, _ = intstrutil.GetValueFromIntOrPercent(cs.Spec.UpdateStrategy.MaxSurge, replicas, true)
	}
	maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(cs.Spec.UpdateStrategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), replicas, maxSurge == 0)
	scaleMaxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(cs.Spec.ScaleStrategy.MaxUnavailable, intstrutil.FromInt(math.MaxInt32)), replicas, true)

	var newRevisionCount, newRevisionActiveCount, oldRevisionCount, oldRevisionActiveCount int
	var unavailableNewRevisionCount, unavailableOldRevisionCount int
	var toDeleteNewRevisionCount, toDeleteOldRevisionCount, preDeletingCount int
	defer func() {
		if res.isEmpty() {
			return
		}
		klog.V(1).Infof("Calculate diffs for CloneSet %s/%s, replicas=%d, partition=%d, maxSurge=%d, maxUnavailable=%d,"+
			" allPods=%d, newRevisionPods=%d, newRevisionActivePods=%d, oldRevisionPods=%d, oldRevisionActivePods=%d,"+
			" unavailableNewRevisionCount=%d, unavailableOldRevisionCount=%d,"+
			" preDeletingCount=%d, toDeleteNewRevisionCount=%d, toDeleteOldRevisionCount=%d."+
			" Result: %+v",
			cs.Namespace, cs.Name, replicas, partition, maxSurge, maxUnavailable,
			len(pods), newRevisionCount, newRevisionActiveCount, oldRevisionCount, oldRevisionActiveCount,
			unavailableNewRevisionCount, unavailableOldRevisionCount,
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
				} else if !isPodAvailable(coreControl, p, cs.Spec.MinReadySeconds) {
					unavailableNewRevisionCount++
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
				} else if !isPodAvailable(coreControl, p, cs.Spec.MinReadySeconds) {
					unavailableOldRevisionCount++
				}
			}
		}
	}

	updateOldDiff := oldRevisionActiveCount - partition
	updateNewDiff := newRevisionActiveCount - (replicas - partition)
	totalUnavailable := preDeletingCount + unavailableNewRevisionCount + unavailableOldRevisionCount
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
			scaleSurge = integer.IntMin(integer.IntMax((unavailableNewRevisionCount+unavailableOldRevisionCount+toDeleteCount+preDeletingCount)-maxUnavailable, 0), toDeleteCount)
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

	if res.scaleNum > 0 {
		res.scaleUpLimit = integer.IntMax(scaleMaxUnavailable-totalUnavailable, 0)
		res.scaleUpLimit = integer.IntMin(res.scaleNum, res.scaleUpLimit)
	}

	if toDeleteNewRevisionCount > 0 || toDeleteOldRevisionCount > 0 || res.scaleNum < 0 {
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
	return isPodAvailable(coreControl, pod, 0)
}

func isPodAvailable(coreControl clonesetcore.Control, pod *v1.Pod, minReadySeconds int32) bool {
	state := lifecycle.GetPodLifecycleState(pod)
	if state != "" && state != appspub.LifecycleStateNormal {
		return false
	}
	return coreControl.IsPodUpdateReady(pod, minReadySeconds)
}
