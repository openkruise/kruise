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
	"sort"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util/expectations"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/updatesort"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interface for managing pods updating.
type Interface interface {
	Manage(cs *appsv1alpha1.CloneSet,
		updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
		pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
	) error
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
) error {

	if cs.Spec.UpdateStrategy.Paused {
		return nil
	}

	// 1. find currently updated and not-ready count and all pods waiting to update
	var updatedNotReadyCount int
	var waitUpdateIndexes []int
	for i := range pods {
		if err := c.inplaceControl.UpdateCondition(pods[i]); err != nil {
			klog.Errorf("CloneSet %s/%s failed to update pod %s condition for inplace: %v",
				cs.Namespace, cs.Name, pods[i].Name, err)
			return err
		}

		if clonesetutils.GetPodRevision(pods[i]) != updateRevision.Name {
			waitUpdateIndexes = append(waitUpdateIndexes, i)
		} else if !readyForUpdate(pods[i]) {
			updatedNotReadyCount++
		}
	}

	// 2. sort all pods waiting to update
	waitUpdateIndexes = sortUpdateIndexes(cs.Spec.UpdateStrategy, pods, waitUpdateIndexes)

	// 3. calculate max count of pods can update
	needToUpdateCount := calculateUpdateCount(cs.Spec.UpdateStrategy, int(*cs.Spec.Replicas), len(waitUpdateIndexes), updatedNotReadyCount)
	if needToUpdateCount < len(waitUpdateIndexes) {
		waitUpdateIndexes = waitUpdateIndexes[:needToUpdateCount]
	}

	// 4. update pods
	for _, idx := range waitUpdateIndexes {
		pod := pods[idx]
		if err := c.updatePod(cs, updateRevision, revisions, pod, pvcs); err != nil {
			return err
		}
	}

	return nil
}

func sortUpdateIndexes(strategy appsv1alpha1.CloneSetUpdateStrategy, pods []*v1.Pod, waitUpdateIndexes []int) []int {
	// not-ready < ready, unscheduled < scheduled, and pending < running
	sort.Slice(waitUpdateIndexes, func(i, j int) bool {
		return kubecontroller.ActivePods(pods).Less(waitUpdateIndexes[i], waitUpdateIndexes[j])
	})
	if strategy.PriorityStrategy != nil {
		waitUpdateIndexes = updatesort.NewPrioritySorter(strategy.PriorityStrategy).Sort(pods, waitUpdateIndexes)
	}
	if strategy.ScatterStrategy != nil {
		waitUpdateIndexes = updatesort.NewScatterSorter(strategy.ScatterStrategy).Sort(pods, waitUpdateIndexes)
	}
	return waitUpdateIndexes
}

func calculateUpdateCount(strategy appsv1alpha1.CloneSetUpdateStrategy, totalReplicas, notUpdatedCount, updatedNotReadyCount int) int {
	partition := 0
	if strategy.Partition != nil {
		partition = int(*strategy.Partition)
	}
	maxUnavailable, _ := intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(strategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), totalReplicas, true)

	return integer.IntMax(integer.IntMin(
		notUpdatedCount-partition,
		maxUnavailable-updatedNotReadyCount,
	), 0)
}

func (c *realControl) updatePod(cs *appsv1alpha1.CloneSet,
	updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
	pod *v1.Pod, pvcs []*v1.PersistentVolumeClaim,
) error {
	if cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType ||
		cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
		var oldRevision *apps.ControllerRevision
		for _, r := range revisions {
			if r.Name == clonesetutils.GetPodRevision(pod) {
				oldRevision = r
				break
			}
		}

		inplacing, err := c.inplaceControl.UpdateInPlace(pod, oldRevision, updateRevision)
		if inplacing && err == nil {
			c.recorder.Eventf(cs, v1.EventTypeNormal, "SuccessfulUpdatePodInPlace", "successfully update pod %s in-place", pod.Name)
			c.updateExp.ExpectUpdated(clonesetutils.GetControllerKey(cs), updateRevision.Name, pod)
			return nil
		}
		if err != nil {
			c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodInPlace", "failed to update pod %s in-place: %v", pod.Name, err)
			if errors.IsConflict(err) || cs.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
				return err
			}
			// If it failed to in-place update && error is not conflict && podUpdatePolicy is not InPlaceOnly,
			// then we should try to recreate this pod
			klog.Warningf("CloneSet %s/%s failed to in-place update Pod %s, so it will back off to ReCreate", cs.Namespace, cs.Name, pod.Name)
		}
	}

	klog.V(2).Infof("CloneSet %s/%s deleting Pod %s for update %s", cs.Namespace, cs.Name, pod.Name, updateRevision.Name)

	c.scaleExp.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
	if err := c.Delete(context.TODO(), pod); err != nil {
		c.scaleExp.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
		c.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdatePodReCreate",
			"failed to delete pod %s for update: %v", pod.Name, err)
		return err
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
			return err
		}
	}

	c.recorder.Eventf(cs, v1.EventTypeNormal, "SuccessfulUpdatePodReCreate",
		"successfully delete pod %s for update", pod.Name)
	return nil
}

func readyForUpdate(pod *v1.Pod) bool {
	if !clonesetutils.IsRunningAndReady(pod) {
		return false
	}
	c := inplaceupdate.GetCondition(pod)
	if c != nil && c.Status != v1.ConditionTrue {
		return false
	}
	return true
}
