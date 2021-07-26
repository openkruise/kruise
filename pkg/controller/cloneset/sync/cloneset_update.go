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
	"sort"
	"time"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	"github.com/openkruise/kruise/pkg/util/specifieddelete"
	"github.com/openkruise/kruise/pkg/util/updatesort"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

func (c *realControl) Update(cs *appsv1alpha1.CloneSet,
	currentRevision, updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
	pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
) (time.Duration, error) {

	requeueDuration := requeueduration.Duration{}
	coreControl := clonesetcore.New(cs)

	if cs.Spec.UpdateStrategy.Paused {
		return requeueDuration.Get(), nil
	}

	// 1. refresh states for all pods
	var modified bool
	for _, pod := range pods {
		patched, duration, err := c.refreshPodState(cs, coreControl, pod)
		if err != nil {
			return 0, err
		} else if duration > 0 {
			requeueDuration.Update(duration)
		}
		if patched {
			modified = true
		}
	}
	if modified {
		return requeueDuration.Get(), nil
	}

	// 2. calculate update diff and the revision to update
	diffRes := calculateDiffsWithExpectation(cs, pods, currentRevision.Name, updateRevision.Name)
	if diffRes.updateNum == 0 {
		return requeueDuration.Get(), nil
	}

	// 3. find all matched pods can update
	var waitUpdateIndexes []int
	for i, pod := range pods {
		if coreControl.IsPodUpdatePaused(pod) {
			continue
		}

		var waitUpdate, canUpdate bool
		if diffRes.updateNum > 0 {
			waitUpdate = !clonesetutils.EqualToRevisionHash("", pod, updateRevision.Name)
		} else {
			waitUpdate = clonesetutils.EqualToRevisionHash("", pod, updateRevision.Name)
		}
		if waitUpdate {
			switch lifecycle.GetPodLifecycleState(pod) {
			case appspub.LifecycleStatePreparingDelete, appspub.LifecycleStateUpdated:
				klog.V(3).Infof("CloneSet %s/%s find pod %s in state %s, so skip to update it",
					cs.Namespace, cs.Name, pod.Name, lifecycle.GetPodLifecycleState(pod))
			default:
				if gracePeriod, _ := appspub.GetInPlaceUpdateGrace(pod); gracePeriod != "" {
					klog.V(3).Infof("CloneSet %s/%s find pod %s still in grace period %s, so skip to update it",
						cs.Namespace, cs.Name, pod.Name, gracePeriod)
				} else {
					canUpdate = true
				}
			}
		}
		if canUpdate {
			waitUpdateIndexes = append(waitUpdateIndexes, i)
		}
	}

	// 4. sort all pods waiting to update
	waitUpdateIndexes = SortUpdateIndexes(coreControl, cs.Spec.UpdateStrategy, pods, waitUpdateIndexes)

	// 5. limit max count of pods can update
	waitUpdateIndexes = limitUpdateIndexes(coreControl, cs.Spec.MinReadySeconds, diffRes, waitUpdateIndexes, pods)

	// 6. update pods
	for _, idx := range waitUpdateIndexes {
		pod := pods[idx]
		targetRevision := updateRevision
		if diffRes.updateNum < 0 {
			targetRevision = currentRevision
		}
		duration, err := c.updatePod(cs, coreControl, targetRevision, revisions, pod, pvcs)
		if duration > 0 {
			requeueDuration.Update(duration)
		}
		if err != nil {
			return requeueDuration.Get(), err
		}
	}

	return requeueDuration.Get(), nil
}

func (c *realControl) refreshPodState(cs *appsv1alpha1.CloneSet, coreControl clonesetcore.Control, pod *v1.Pod) (bool, time.Duration, error) {
	opts := coreControl.GetUpdateOptions()
	opts = inplaceupdate.SetOptionsDefaults(opts)

	res := c.inplaceControl.Refresh(pod, opts)
	if res.RefreshErr != nil {
		klog.Errorf("CloneSet %s/%s failed to update pod %s condition for inplace: %v",
			cs.Namespace, cs.Name, pod.Name, res.RefreshErr)
		return false, 0, res.RefreshErr
	}

	var state appspub.LifecycleStateType
	switch lifecycle.GetPodLifecycleState(pod) {
	case appspub.LifecycleStateUpdating:
		if opts.CheckUpdateCompleted(pod) == nil {
			if cs.Spec.Lifecycle != nil && !lifecycle.IsPodHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
				state = appspub.LifecycleStateUpdated
			} else {
				state = appspub.LifecycleStateNormal
			}
		}
	case appspub.LifecycleStateUpdated:
		if cs.Spec.Lifecycle == nil ||
			cs.Spec.Lifecycle.InPlaceUpdate == nil ||
			lifecycle.IsPodAllHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
			state = appspub.LifecycleStateNormal
		}
	}

	if state != "" {
		if updated, err := c.lifecycleControl.UpdatePodLifecycle(pod, state); err != nil {
			return false, 0, err
		} else if updated {
			clonesetutils.ResourceVersionExpectations.Expect(pod)
			klog.V(3).Infof("CloneSet %s update pod %s lifecycle to %s", clonesetutils.GetControllerKey(cs), pod.Name, state)
			return true, res.DelayDuration, nil
		}
	}

	return false, res.DelayDuration, nil
}

func (c *realControl) updatePod(cs *appsv1alpha1.CloneSet, coreControl clonesetcore.Control,
	updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
	pod *v1.Pod, pvcs []*v1.PersistentVolumeClaim,
) (time.Duration, error) {

	if cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType ||
		cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
		var oldRevision *apps.ControllerRevision
		for _, r := range revisions {
			if clonesetutils.EqualToRevisionHash("", pod, r.Name) {
				oldRevision = r
				break
			}
		}

		if c.inplaceControl.CanUpdateInPlace(oldRevision, updateRevision, coreControl.GetUpdateOptions()) {
			switch state := lifecycle.GetPodLifecycleState(pod); state {
			case "", appspub.LifecycleStateNormal:
				var err error
				var updated bool
				if cs.Spec.Lifecycle != nil && lifecycle.IsPodHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
					if updated, err = c.lifecycleControl.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingUpdate); err == nil && updated {
						clonesetutils.ResourceVersionExpectations.Expect(pod)
						klog.V(3).Infof("CloneSet %s update pod %s lifecycle to PreparingUpdate",
							clonesetutils.GetControllerKey(cs), pod.Name)
					}
					return 0, err
				}
			case appspub.LifecycleStatePreparingUpdate:
				if cs.Spec.Lifecycle != nil && lifecycle.IsPodHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
					return 0, nil
				}
			case appspub.LifecycleStateUpdating:
			default:
				return 0, fmt.Errorf("not allowed to in-place update pod %s in state %s", pod.Name, state)
			}

			opts := coreControl.GetUpdateOptions()
			opts.AdditionalFuncs = append(opts.AdditionalFuncs, lifecycle.SetPodLifecycle(appspub.LifecycleStateUpdating))
			res := c.inplaceControl.Update(pod, oldRevision, updateRevision, opts)
			if res.InPlaceUpdate {
				if res.UpdateErr == nil {
					c.recorder.Eventf(cs, v1.EventTypeNormal, "SuccessfulUpdatePodInPlace", "successfully update pod %s in-place(revision %v)", pod.Name, updateRevision.Name)
					clonesetutils.UpdateExpectations.ExpectUpdated(clonesetutils.GetControllerKey(cs), updateRevision.Name, pod)
					return res.DelayDuration, nil
				}

				c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodInPlace", "failed to update pod %s in-place(revision %v): %v", pod.Name, updateRevision.Name, res.UpdateErr)
				return res.DelayDuration, res.UpdateErr
			}
		}

		if cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
			return 0, fmt.Errorf("find Pod %s update strategy is InPlaceOnly but can not update in-place", pod.Name)
		}
		klog.Warningf("CloneSet %s/%s can not update Pod %s in-place, so it will back off to ReCreate", cs.Namespace, cs.Name, pod.Name)
	}

	klog.V(2).Infof("CloneSet %s/%s start to patch Pod %s specified-delete for update %s", cs.Namespace, cs.Name, pod.Name, updateRevision.Name)

	if patched, err := specifieddelete.PatchPodSpecifiedDelete(c.Client, pod, "true"); err != nil {
		c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodReCreate",
			"failed to patch pod specified-delete %s for update(revision %s): %v", pod.Name, updateRevision.Name, err)
		return 0, err
	} else if patched {
		clonesetutils.ResourceVersionExpectations.Expect(pod)
	}

	c.recorder.Eventf(cs, v1.EventTypeNormal, "SuccessfulUpdatePodReCreate",
		"successfully patch pod %s specified-delete for update(revision %s)", pod.Name, updateRevision.Name)
	return 0, nil
}

// SortUpdateIndexes sorts the given waitUpdateIndexes of Pods to update according to the CloneSet strategy.
func SortUpdateIndexes(coreControl clonesetcore.Control, strategy appsv1alpha1.CloneSetUpdateStrategy, pods []*v1.Pod, waitUpdateIndexes []int) []int {
	// Sort Pods with default sequence
	sort.Slice(waitUpdateIndexes, coreControl.GetPodsSortFunc(pods, waitUpdateIndexes))

	if strategy.PriorityStrategy != nil {
		waitUpdateIndexes = updatesort.NewPrioritySorter(strategy.PriorityStrategy).Sort(pods, waitUpdateIndexes)
	}
	if strategy.ScatterStrategy != nil {
		waitUpdateIndexes = updatesort.NewScatterSorter(strategy.ScatterStrategy).Sort(pods, waitUpdateIndexes)
	}

	// PreparingUpdate first
	sort.SliceStable(waitUpdateIndexes, func(i, j int) bool {
		preparingUpdateI := lifecycle.GetPodLifecycleState(pods[waitUpdateIndexes[i]]) == appspub.LifecycleStatePreparingUpdate
		preparingUpdateJ := lifecycle.GetPodLifecycleState(pods[waitUpdateIndexes[j]]) == appspub.LifecycleStatePreparingUpdate
		if preparingUpdateI != preparingUpdateJ {
			return preparingUpdateI
		}
		return false
	})
	return waitUpdateIndexes
}

// limitUpdateIndexes limits all pods waiting update by the maxUnavailable policy, and returns the indexes of pods that can finally update
func limitUpdateIndexes(coreControl clonesetcore.Control, minReadySeconds int32, diffRes expectationDiffs, waitUpdateIndexes []int, pods []*v1.Pod) []int {
	updateDiff := util.IntAbs(diffRes.updateNum)
	if updateDiff < len(waitUpdateIndexes) {
		waitUpdateIndexes = waitUpdateIndexes[:updateDiff]
	}

	var notReadyCount, canUpdateCount int
	for _, p := range pods {
		if !isPodAvailable(coreControl, p, minReadySeconds) {
			notReadyCount++
		}
	}
	for _, i := range waitUpdateIndexes {
		if isPodAvailable(coreControl, pods[i], minReadySeconds) {
			if notReadyCount >= diffRes.updateMaxUnavailable {
				break
			}
			notReadyCount++
		}
		canUpdateCount++
	}

	if canUpdateCount < len(waitUpdateIndexes) {
		waitUpdateIndexes = waitUpdateIndexes[:canUpdateCount]
	}
	return waitUpdateIndexes
}
