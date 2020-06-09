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
	"context"
	"fmt"
	"sort"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util/expectations"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	"github.com/openkruise/kruise/pkg/util/updatesort"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

func New(c client.Client, recorder record.EventRecorder, scaleExp expectations.ScaleExpectations, updateExp expectations.UpdateExpectations) Interface {
	return &realControl{
		inplaceControl: inplaceupdate.New(c, apps.ControllerRevisionHashLabelKey),
		Client:         c,
		recorder:       recorder,
		scaleExp:       scaleExp,
		updateExp:      updateExp,
	}
}

type realControl struct {
	client.Client
	inplaceControl inplaceupdate.Interface
	recorder       record.EventRecorder
	scaleExp       expectations.ScaleExpectations
	updateExp      expectations.UpdateExpectations
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

	// 1. find currently updated and not-ready count and all pods waiting to update
	var waitUpdateIndexes []int
	for i := range pods {
		if coreControl.IsPodUpdatePaused(pods[i]) {
			continue
		}

		if res := c.inplaceControl.Refresh(pods[i], coreControl.GetUpdateOptions()); res.RefreshErr != nil {
			klog.Errorf("CloneSet %s/%s failed to update pod %s condition for inplace: %v",
				cs.Namespace, cs.Name, pods[i].Name, res.RefreshErr)
			return requeueDuration.Get(), res.RefreshErr
		} else if res.DelayDuration > 0 {
			requeueDuration.Update(res.DelayDuration)
		}

		if clonesetutils.GetPodRevision(pods[i]) != updateRevision.Name {
			waitUpdateIndexes = append(waitUpdateIndexes, i)
		}
	}

	// 2. sort all pods waiting to update
	waitUpdateIndexes = sortUpdateIndexes(coreControl, cs.Spec.UpdateStrategy, pods, waitUpdateIndexes)

	// 3. calculate max count of pods can update
	needToUpdateCount := calculateUpdateCount(coreControl, cs.Spec.UpdateStrategy, cs.Spec.MinReadySeconds, int(*cs.Spec.Replicas), waitUpdateIndexes, pods)
	if needToUpdateCount < len(waitUpdateIndexes) {
		waitUpdateIndexes = waitUpdateIndexes[:needToUpdateCount]
	}

	// 4. update pods
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

func sortUpdateIndexes(coreControl clonesetcore.Control, strategy appsv1alpha1.CloneSetUpdateStrategy, pods []*v1.Pod, waitUpdateIndexes []int) []int {
	// Sort Pods with default sequence
	sort.Slice(waitUpdateIndexes, coreControl.GetPodsSortFunc(pods, waitUpdateIndexes))

	if strategy.PriorityStrategy != nil {
		waitUpdateIndexes = updatesort.NewPrioritySorter(strategy.PriorityStrategy).Sort(pods, waitUpdateIndexes)
	}
	if strategy.ScatterStrategy != nil {
		waitUpdateIndexes = updatesort.NewScatterSorter(strategy.ScatterStrategy).Sort(pods, waitUpdateIndexes)
	}
	return waitUpdateIndexes
}

func calculateUpdateCount(coreControl clonesetcore.Control, strategy appsv1alpha1.CloneSetUpdateStrategy, minReadySeconds int32, totalReplicas int, waitUpdateIndexes []int, pods []*v1.Pod) int {
	partition := 0
	if strategy.Partition != nil {
		partition = int(*strategy.Partition)
	}

	if len(waitUpdateIndexes)-partition <= 0 {
		return 0
	}
	waitUpdateIndexes = waitUpdateIndexes[:(len(waitUpdateIndexes) - partition)]

	maxUnavailable, _ := intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(strategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), totalReplicas, true)
	usedSurge := len(pods) - totalReplicas

	var notReadyCount, updateCount int
	for _, p := range pods {
		if !coreControl.IsPodUpdateReady(p, minReadySeconds) {
			notReadyCount++
		}
	}
	for _, i := range waitUpdateIndexes {
		if coreControl.IsPodUpdateReady(pods[i], minReadySeconds) {
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

		res := c.inplaceControl.Update(pod, oldRevision, updateRevision, coreControl.GetUpdateOptions())

		if res.InPlaceUpdate {
			if res.UpdateErr == nil {
				c.recorder.Eventf(cs, v1.EventTypeNormal, "SuccessfulUpdatePodInPlace", "successfully update pod %s in-place", pod.Name)
				c.updateExp.ExpectUpdated(clonesetutils.GetControllerKey(cs), updateRevision.Name, pod)
				return res.DelayDuration, nil
			}

			c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodInPlace", "failed to update pod %s in-place: %v", pod.Name, res.UpdateErr)
			if errors.IsConflict(res.UpdateErr) || cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
				return res.DelayDuration, res.UpdateErr
			}
			// If it failed to in-place update && error is not conflict && podUpdatePolicy is not InPlaceOnly,
			// then we should try to recreate this pod
			klog.Warningf("CloneSet %s/%s failed to in-place update Pod %s, so it will back off to ReCreate", cs.Namespace, cs.Name, pod.Name)

		} else {
			if cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
				return res.DelayDuration, fmt.Errorf("find Pod %s update strategy is InPlaceOnly but can not update in-place", pod.Name)
			}
			klog.Warningf("CloneSet %s/%s can not update Pod %s in-place, so it will back off to ReCreate", cs.Namespace, cs.Name, pod.Name)
		}
	}

	klog.V(2).Infof("CloneSet %s/%s deleting Pod %s for update %s", cs.Namespace, cs.Name, pod.Name, updateRevision.Name)

	c.scaleExp.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
	if err := c.Delete(context.TODO(), pod); err != nil {
		c.scaleExp.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
		c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodReCreate",
			"failed to delete pod %s for update: %v", pod.Name, err)
		return 0, err
	}

	// TODO(FillZpp): add a strategy controlling if the PVCs of this pod should be deleted
	for _, pvc := range pvcs {
		if pvc.Labels[appsv1alpha1.CloneSetInstanceID] != pod.Labels[appsv1alpha1.CloneSetInstanceID] {
			continue
		}

		c.scaleExp.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
		if err := c.Delete(context.TODO(), pvc); err != nil {
			c.scaleExp.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
			c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedDelete", "failed to delete pvc %s: %v", pvc.Name, err)
			return 0, err
		}
	}

	c.recorder.Eventf(cs, v1.EventTypeNormal, "SuccessfulUpdatePodReCreate",
		"successfully delete pod %s for update", pod.Name)
	return 0, nil
}
