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
	"context"
	"fmt"
	"sort"
	"time"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/specifieddelete"
	"github.com/openkruise/kruise/pkg/util/updatesort"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *realControl) Update(cs *appsv1alpha1.CloneSet,
	currentRevision, updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
	pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
) error {

	key := clonesetutils.GetControllerKey(cs)
	coreControl := clonesetcore.New(cs)

	// 1. refresh states for all pods
	var modified bool
	for _, pod := range pods {
		patchedState, duration, err := c.refreshPodState(cs, coreControl, pod, updateRevision.Name)
		if err != nil {
			return err
		} else if duration > 0 {
			clonesetutils.DurationStore.Push(key, duration)
		}
		// fix the pod-template-hash label for old pods before v1.1
		patchedHash, err := c.fixPodTemplateHashLabel(cs, pod)
		if err != nil {
			return err
		}
		if patchedState || patchedHash {
			modified = true
		}
	}
	if modified {
		return nil
	}

	if cs.Spec.UpdateStrategy.Paused {
		return nil
	}

	// 2. calculate update diff and the revision to update
	diffRes := calculateDiffsWithExpectation(cs, pods, currentRevision.Name, updateRevision.Name, nil)
	if diffRes.updateNum == 0 {
		return nil
	}

	// 3. find all matched pods can update
	targetRevision := updateRevision
	if diffRes.updateNum < 0 {
		targetRevision = currentRevision
	}
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
			case appspub.LifecycleStatePreparingDelete:
				klog.V(3).InfoS("CloneSet found pod in PreparingDelete state, so skipped updating it",
					"cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
			case appspub.LifecycleStateUpdated:
				klog.V(3).InfoS("CloneSet found pod in Updated state but not in updated revision",
					"cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
				canUpdate = true
			default:
				if gracePeriod, _ := appspub.GetInPlaceUpdateGrace(pod); gracePeriod != "" {
					klog.V(3).InfoS("CloneSet found pod still in grace period, so skipped updating it",
						"cloneSet", klog.KObj(cs), "pod", klog.KObj(pod), "gracePeriod", gracePeriod)
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
	waitUpdateIndexes = limitUpdateIndexes(coreControl, cs.Spec.MinReadySeconds, diffRes, waitUpdateIndexes, pods, targetRevision.Name)

	// 6. update pods
	for _, idx := range waitUpdateIndexes {
		pod := pods[idx]
		// Determine the pub before updating the pod
		if utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetUpdateGate) {
			allowed, _, err := pubcontrol.PodUnavailableBudgetValidatePod(pod, policyv1alpha1.PubUpdateOperation, "kruise-manager", false)
			if err != nil {
				return err
				// pub check does not pass, try again in seconds
			} else if !allowed {
				clonesetutils.DurationStore.Push(key, time.Second)
				return nil
			}
		}
		duration, err := c.updatePod(cs, coreControl, targetRevision, revisions, pod, pvcs)
		if duration > 0 {
			clonesetutils.DurationStore.Push(key, duration)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *realControl) refreshPodState(cs *appsv1alpha1.CloneSet, coreControl clonesetcore.Control, pod *v1.Pod, updateRevision string) (bool, time.Duration, error) {
	opts := coreControl.GetUpdateOptions()
	opts = inplaceupdate.SetOptionsDefaults(opts)

	res := c.inplaceControl.Refresh(pod, opts)
	if res.RefreshErr != nil {
		klog.ErrorS(res.RefreshErr, "CloneSet failed to update pod condition for inplace",
			"cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
		return false, 0, res.RefreshErr
	}

	var state appspub.LifecycleStateType
	switch lifecycle.GetPodLifecycleState(pod) {
	case appspub.LifecycleStatePreparingNormal:
		if cs.Spec.Lifecycle == nil ||
			cs.Spec.Lifecycle.PreNormal == nil ||
			lifecycle.IsPodAllHooked(cs.Spec.Lifecycle.PreNormal, pod) {
			state = appspub.LifecycleStateNormal
		}
	case appspub.LifecycleStatePreparingUpdate:
		// when pod updated to PreparingUpdate state to wait lifecycle blocker to remove,
		// then rollback, do not need update pod inplace since it is the update revision,
		// so just update pod lifecycle state. ref: https://github.com/openkruise/kruise/issues/1156
		if clonesetutils.EqualToRevisionHash("", pod, updateRevision) {
			if cs.Spec.Lifecycle != nil && !lifecycle.IsPodAllHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
				state = appspub.LifecycleStateUpdated
			} else {
				state = appspub.LifecycleStateNormal
			}
		}
	case appspub.LifecycleStateUpdating:
		if opts.CheckPodUpdateCompleted(pod) == nil {
			if cs.Spec.Lifecycle != nil && !lifecycle.IsPodAllHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
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
		var markPodNotReady bool
		if cs.Spec.Lifecycle != nil && cs.Spec.Lifecycle.InPlaceUpdate != nil {
			markPodNotReady = cs.Spec.Lifecycle.InPlaceUpdate.MarkPodNotReady
		}
		if updated, gotPod, err := c.lifecycleControl.UpdatePodLifecycle(pod, state, markPodNotReady); err != nil {
			return false, 0, err
		} else if updated {
			clonesetutils.ResourceVersionExpectations.Expect(gotPod)
			klog.V(3).InfoS("CloneSet updated pod lifecycle", "cloneSet", klog.KObj(cs), "pod", klog.KObj(pod), "newState", state)
			return true, res.DelayDuration, nil
		}
	}

	return false, res.DelayDuration, nil
}

// fix the pod-template-hash label for old pods before v1.1
func (c *realControl) fixPodTemplateHashLabel(cs *appsv1alpha1.CloneSet, pod *v1.Pod) (bool, error) {
	if _, exists := pod.Labels[apps.DefaultDeploymentUniqueLabelKey]; exists {
		return false, nil
	}
	patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`,
		apps.DefaultDeploymentUniqueLabelKey,
		clonesetutils.GetShortHash(pod.Labels[apps.ControllerRevisionHashLabelKey])))
	pod = pod.DeepCopy()
	if err := c.Patch(context.TODO(), pod, client.RawPatch(types.StrategicMergePatchType, patch)); err != nil {
		klog.ErrorS(err, "CloneSet failed to fix pod-template-hash", "cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
		return false, err
	}
	clonesetutils.ResourceVersionExpectations.Expect(pod)
	return true, nil
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
			case "", appspub.LifecycleStatePreparingNormal, appspub.LifecycleStateNormal:
				var err error
				var updated bool
				var gotPod *v1.Pod
				if cs.Spec.Lifecycle != nil && lifecycle.IsPodHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
					markPodNotReady := cs.Spec.Lifecycle.InPlaceUpdate.MarkPodNotReady
					if updated, gotPod, err = c.lifecycleControl.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingUpdate, markPodNotReady); err == nil && updated {
						clonesetutils.ResourceVersionExpectations.Expect(gotPod)
						klog.V(3).InfoS("CloneSet updated pod lifecycle to PreparingUpdate", "cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
					}
					return 0, err
				}
			case appspub.LifecycleStateUpdated:
				var err error
				var updated bool
				var gotPod *v1.Pod
				var inPlaceUpdateHandler *appspub.LifecycleHook
				if cs.Spec.Lifecycle != nil {
					inPlaceUpdateHandler = cs.Spec.Lifecycle.InPlaceUpdate
				}
				if updated, gotPod, err = c.lifecycleControl.UpdatePodLifecycleWithHandler(pod, appspub.LifecycleStatePreparingUpdate, inPlaceUpdateHandler); err == nil && updated {
					clonesetutils.ResourceVersionExpectations.Expect(gotPod)
					klog.V(3).InfoS("CloneSet updated pod lifecycle to PreparingUpdate", "cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
				}
				return 0, err
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
					clonesetutils.ResourceVersionExpectations.Expect(&metav1.ObjectMeta{UID: pod.UID, ResourceVersion: res.NewResourceVersion})
					return res.DelayDuration, nil
				}

				c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodInPlace", "failed to update pod %s in-place(revision %v): %v", pod.Name, updateRevision.Name, res.UpdateErr)
				return res.DelayDuration, res.UpdateErr
			}
		}

		if cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
			return 0, fmt.Errorf("find Pod %s update strategy is InPlaceOnly but can not update in-place", pod.Name)
		}
		klog.InfoS("CloneSet could not update Pod in-place, so it will back off to ReCreate", "cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
	}

	klog.V(2).InfoS("CloneSet started to patch Pod specified-delete for update", "cloneSet", klog.KObj(cs), "pod", klog.KObj(pod), "updateRevision", klog.KObj(updateRevision))

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

// SortUpdateIndexes sorts the given oldRevisionIndexes of Pods to update according to the CloneSet strategy.
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
func limitUpdateIndexes(coreControl clonesetcore.Control, minReadySeconds int32, diffRes expectationDiffs, waitUpdateIndexes []int, pods []*v1.Pod, targetRevisionHash string) []int {
	updateDiff := util.IntAbs(diffRes.updateNum)
	if updateDiff < len(waitUpdateIndexes) {
		waitUpdateIndexes = waitUpdateIndexes[:updateDiff]
	}

	var unavailableCount, targetRevisionUnavailableCount, canUpdateCount int
	for _, p := range pods {
		if !IsPodAvailable(coreControl, p, minReadySeconds) {
			unavailableCount++
			if clonesetutils.EqualToRevisionHash("", p, targetRevisionHash) {
				targetRevisionUnavailableCount++
			}
		}
	}
	for _, i := range waitUpdateIndexes {
		// Make sure unavailable pods in target revision should not be more than maxUnavailable.
		if targetRevisionUnavailableCount+canUpdateCount >= diffRes.updateMaxUnavailable {
			break
		}

		// Make sure unavailable pods in all revisions should not be more than maxUnavailable.
		// Note that update an old pod that already be unavailable will not increase the unavailable number.
		if IsPodAvailable(coreControl, pods[i], minReadySeconds) {
			if unavailableCount >= diffRes.updateMaxUnavailable {
				break
			}
			unavailableCount++
		}
		canUpdateCount++
	}

	if canUpdateCount < len(waitUpdateIndexes) {
		waitUpdateIndexes = waitUpdateIndexes[:canUpdateCount]
	}
	return waitUpdateIndexes
}
