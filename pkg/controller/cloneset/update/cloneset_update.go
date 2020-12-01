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

package update

import (
	"fmt"
	"sort"
	"time"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	"github.com/openkruise/kruise/pkg/util/specifieddelete"
	"github.com/openkruise/kruise/pkg/util/updatesort"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interface for managing pods updating.
type Interface interface {
	Manage(cs *appsv1alpha1.CloneSet,
		updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
		pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
	) (time.Duration, error)
}

func New(c client.Client, recorder record.EventRecorder) Interface {
	return &realControl{
		inplaceControl: inplaceupdate.New(c, apps.ControllerRevisionHashLabelKey),
		Client:         c,
		recorder:       recorder,
	}
}

type realControl struct {
	client.Client
	inplaceControl inplaceupdate.Interface
	recorder       record.EventRecorder
}

func (c *realControl) Manage(cs *appsv1alpha1.CloneSet,
	updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
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

	// 2. find currently updated and not-ready count and all pods waiting to update
	var waitUpdateIndexes []int
	for i := range pods {
		if coreControl.IsPodUpdatePaused(pods[i]) {
			continue
		}

		if clonesetutils.GetPodRevision(pods[i]) != updateRevision.Name {
			switch lifecycle.GetPodLifecycleState(pods[i]) {
			case appspub.LifecycleStatePreparingDelete, appspub.LifecycleStateUpdated:
				klog.V(3).Infof("CloneSet %s/%s find pod %s in state %s, so skip to update it",
					cs.Namespace, cs.Name, pods[i].Name, lifecycle.GetPodLifecycleState(pods[i]))
			default:
				if gracePeriod := pods[i].Annotations[appspub.InPlaceUpdateGraceKey]; gracePeriod != "" {
					klog.V(3).Infof("CloneSet %s/%s find pod %s still in grace period %s, so skip to update it",
						cs.Namespace, cs.Name, pods[i].Name, gracePeriod)
				} else {
					waitUpdateIndexes = append(waitUpdateIndexes, i)
				}
			}
		}
	}

	// 3. sort all pods waiting to update
	waitUpdateIndexes = sortUpdateIndexes(coreControl, cs.Spec.UpdateStrategy, pods, waitUpdateIndexes)

	// 4. calculate max count of pods can update
	needToUpdateCount := calculateUpdateCount(coreControl, cs.Spec.UpdateStrategy, cs.Spec.MinReadySeconds, int(*cs.Spec.Replicas), waitUpdateIndexes, pods)
	if needToUpdateCount < len(waitUpdateIndexes) {
		waitUpdateIndexes = waitUpdateIndexes[:needToUpdateCount]
	}

	// 5. update pods
	for _, idx := range waitUpdateIndexes {
		pod := pods[idx]
		if duration, err := c.updatePod(cs, coreControl, updateRevision, revisions, pod, pvcs); err != nil {
			return requeueDuration.Get(), err
		} else if duration > 0 {
			requeueDuration.Update(duration)
		}
	}

	return requeueDuration.Get(), nil
}

func (c *realControl) refreshPodState(cs *appsv1alpha1.CloneSet, coreControl clonesetcore.Control, pod *v1.Pod) (bool, time.Duration, error) {
	opts := coreControl.GetUpdateOptions()
	res := c.inplaceControl.Refresh(pod, opts)
	if res.RefreshErr != nil {
		klog.Errorf("CloneSet %s/%s failed to update pod %s condition for inplace: %v",
			cs.Namespace, cs.Name, pod.Name, res.RefreshErr)
		return false, 0, res.RefreshErr
	}

	var state appspub.LifecycleStateType
	switch lifecycle.GetPodLifecycleState(pod) {
	case appspub.LifecycleStateUpdating:
		checkFunc := inplaceupdate.CheckInPlaceUpdateCompleted
		if opts != nil && opts.CustomizeCheckUpdateCompleted != nil {
			checkFunc = opts.CustomizeCheckUpdateCompleted
		}
		if checkFunc(pod) == nil {
			if cs.Spec.Lifecycle != nil && !lifecycle.IsPodHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
				state = appspub.LifecycleStateUpdated
			} else {
				state = appspub.LifecycleStateNormal
			}
		}
	case appspub.LifecycleStateUpdated:
		if cs.Spec.Lifecycle == nil ||
			cs.Spec.Lifecycle.InPlaceUpdate == nil ||
			lifecycle.IsPodHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
			state = appspub.LifecycleStateNormal
		}
	}

	if state != "" {
		if patched, err := lifecycle.PatchPodLifecycle(c, pod, state); err != nil {
			return false, 0, err
		} else if patched {
			clonesetutils.ResourceVersionExpectations.Expect(pod)
			klog.V(3).Infof("CloneSet %s patch pod %s lifecycle to %s",
				clonesetutils.GetControllerKey(cs), pod.Name, state)
			return true, res.DelayDuration, nil
		}
	}

	return false, res.DelayDuration, nil
}

func sortUpdateIndexes(coreControl clonesetcore.Control, strategy appsv1alpha1.CloneSetUpdateStrategy, pods []*v1.Pod, waitUpdateIndexes []int) []int {
	// Sort Pods with default sequence
	sort.Slice(waitUpdateIndexes, coreControl.GetPodsSortFunc(pods, waitUpdateIndexes))

	if strategy.PriorityStrategy != nil {
		waitUpdateIndexes = updatesort.NewPrioritySorter(strategy.PriorityStrategy).Sort(pods, waitUpdateIndexes)
	}
	if strategy.ScatterStrategy != nil {
		waitUpdateIndexes = updatesort.NewScatterSorter(strategy.ScatterStrategy).Sort(pods, waitUpdateIndexes)
	}

	// PreparingUpdate first
	sort.Slice(waitUpdateIndexes, func(i, j int) bool {
		preparingUpdateI := lifecycle.GetPodLifecycleState(pods[waitUpdateIndexes[i]]) == appspub.LifecycleStatePreparingUpdate
		preparingUpdateJ := lifecycle.GetPodLifecycleState(pods[waitUpdateIndexes[j]]) == appspub.LifecycleStatePreparingUpdate
		if preparingUpdateI != preparingUpdateJ {
			return preparingUpdateI
		}
		return false
	})
	return waitUpdateIndexes
}

func calculateUpdateCount(coreControl clonesetcore.Control, strategy appsv1alpha1.CloneSetUpdateStrategy, minReadySeconds int32, totalReplicas int, waitUpdateIndexes []int, pods []*v1.Pod) int {
	partition := 0
	if strategy.Partition != nil {
		partition, _ = intstrutil.GetValueFromIntOrPercent(strategy.Partition, totalReplicas, true)
	}

	if len(waitUpdateIndexes)-partition <= 0 {
		return 0
	}
	waitUpdateIndexes = waitUpdateIndexes[:(len(waitUpdateIndexes) - partition)]

	roundUp := true
	if strategy.MaxSurge != nil {
		maxSurge, _ := intstrutil.GetValueFromIntOrPercent(strategy.MaxSurge, totalReplicas, true)
		roundUp = maxSurge == 0
	}
	maxUnavailable, _ := intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(strategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), totalReplicas, roundUp)
	usedSurge := len(pods) - totalReplicas

	var notReadyCount, updateCount int
	for _, p := range pods {
		if !isPodReady(coreControl, p, minReadySeconds) {
			notReadyCount++
		}
	}
	for _, i := range waitUpdateIndexes {
		if isPodReady(coreControl, pods[i], minReadySeconds) {
			if notReadyCount >= (maxUnavailable + usedSurge) {
				break
			} else {
				notReadyCount++
			}
		}
		updateCount++
	}

	return updateCount
}

func isPodReady(coreControl clonesetcore.Control, pod *v1.Pod, minReadySeconds int32) bool {
	state := lifecycle.GetPodLifecycleState(pod)
	if state != "" && state != appspub.LifecycleStateNormal {
		return false
	}
	return coreControl.IsPodUpdateReady(pod, minReadySeconds)
}

func (c *realControl) updatePod(cs *appsv1alpha1.CloneSet, coreControl clonesetcore.Control,
	updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
	pod *v1.Pod, pvcs []*v1.PersistentVolumeClaim,
) (time.Duration, error) {

	if cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType ||
		cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
		var oldRevision *apps.ControllerRevision
		for _, r := range revisions {
			if r.Name == clonesetutils.GetPodRevision(pod) {
				oldRevision = r
				break
			}
		}

		if c.inplaceControl.CanUpdateInPlace(oldRevision, updateRevision, coreControl.GetUpdateOptions()) {
			if cs.Spec.Lifecycle != nil && lifecycle.IsPodHooked(cs.Spec.Lifecycle.InPlaceUpdate, pod) {
				if patched, err := lifecycle.PatchPodLifecycle(c, pod, appspub.LifecycleStatePreparingUpdate); err != nil {
					return 0, err
				} else if patched {
					clonesetutils.ResourceVersionExpectations.Expect(pod)
					klog.V(3).Infof("CloneSet %s patch pod %s lifecycle to PreparingUpdate",
						clonesetutils.GetControllerKey(cs), pod.Name)
				}
				return 0, nil
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

	if patched, err := specifieddelete.PatchPodSpecifiedDelete(c, pod, "true"); err != nil {
		c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodReCreate",
			"failed to patch pod specified-delete %s for update(revision %s): %v", pod.Name, updateRevision.Name, err)
		return 0, err
	} else if patched {
		clonesetutils.ResourceVersionExpectations.Expect(pod)
	}

	//clonesetutils.ScaleExpectations.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
	//if err := c.Delete(context.TODO(), pod); err != nil {
	//	clonesetutils.ScaleExpectations.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
	//	c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodReCreate",
	//		"failed to delete pod %s for update: %v", pod.Name, err)
	//	return 0, err
	//}
	//
	//// TODO(FillZpp): add a strategy controlling if the PVCs of this pod should be deleted
	//for _, pvc := range pvcs {
	//	if pvc.Labels[appsv1alpha1.CloneSetInstanceID] != pod.Labels[appsv1alpha1.CloneSetInstanceID] {
	//		continue
	//	}
	//
	//	clonesetutils.ScaleExpectations.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
	//	if err := c.Delete(context.TODO(), pvc); err != nil {
	//		clonesetutils.ScaleExpectations.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
	//		c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedDelete", "failed to delete pvc %s: %v", pvc.Name, err)
	//		return 0, err
	//	}
	//}

	c.recorder.Eventf(cs, v1.EventTypeNormal, "SuccessfulUpdatePodReCreate",
		"successfully patch pod %s specified-delete for update(revision %s)", pod.Name, updateRevision.Name)
	return 0, nil
}
