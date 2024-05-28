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
	"sync"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/expectations"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/revision"
)

const (
	// LengthOfInstanceID is the length of instance-id
	LengthOfInstanceID = 5

	// When batching pod creates, initialBatchSize is the size of the initial batch.
	initialBatchSize = 1
)

func (r *realControl) Scale(
	currentCS, updateCS *appsv1alpha1.CloneSet,
	currentRevision, updateRevision string,
	pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim,
) (bool, error) {
	if updateCS.Spec.Replicas == nil {
		return false, fmt.Errorf("spec.Replicas is nil")
	}

	coreControl := clonesetcore.New(updateCS)
	if !coreControl.IsReadyToScale() {
		klog.InfoS("CloneSet skipped scaling for not ready to scale", "cloneSet", klog.KObj(updateCS))
		return false, nil
	}

	// 1. manage pods to delete and in preDelete
	podsSpecifiedToDelete, podsInPreDelete, numToDelete := getPlannedDeletedPods(updateCS, pods)
	if modified, err := r.managePreparingDelete(updateCS, pods, podsInPreDelete, numToDelete); err != nil || modified {
		return modified, err
	}

	// 2. calculate scale numbers
	diffRes := calculateDiffsWithExpectation(updateCS, pods, currentRevision, updateRevision, revision.IsPodUpdate)
	updatedPods, notUpdatedPods := clonesetutils.GroupUpdateAndNotUpdatePods(pods, updateRevision)

	if diffRes.scaleUpNum > diffRes.scaleUpLimit {
		r.recorder.Event(updateCS, v1.EventTypeWarning, "ScaleUpLimited", fmt.Sprintf("scaleUp is limited because of scaleStrategy.maxUnavailable, limit: %d", diffRes.scaleUpLimit))
	}

	// 3. scale out
	if diffRes.scaleUpNum > 0 {
		// total number of this creation
		expectedCreations := diffRes.scaleUpLimit
		// lack number of current version
		expectedCurrentCreations := diffRes.scaleUpNumOldRevision

		klog.V(3).InfoS("CloneSet began to scale out pods, including current revision",
			"cloneSet", klog.KObj(updateCS), "expectedCreations", expectedCreations, "expectedCurrentCreations", expectedCurrentCreations)

		// available instance-id come from free pvc
		availableIDs := getOrGenAvailableIDs(expectedCreations, pods, pvcs)
		// existing pvc names
		existingPVCNames := sets.NewString()
		for _, pvc := range pvcs {
			existingPVCNames.Insert(pvc.Name)
		}

		return r.createPods(expectedCreations, expectedCurrentCreations,
			currentCS, updateCS, currentRevision, updateRevision, availableIDs.List(), existingPVCNames)
	}

	// 4. try to delete pods already in pre-delete
	if len(podsInPreDelete) > 0 {
		klog.V(3).InfoS("CloneSet tried to delete pods in preDelete", "cloneSet", klog.KObj(updateCS), "pods", util.GetPodNames(podsInPreDelete).List())
		if modified, err := r.deletePods(updateCS, podsInPreDelete, pvcs); err != nil || modified {
			return modified, err
		}
	}

	// 5. specified delete
	if podsToDelete := util.DiffPods(podsSpecifiedToDelete, podsInPreDelete); len(podsToDelete) > 0 {
		newPodsToDelete, oldPodsToDelete := clonesetutils.GroupUpdateAndNotUpdatePods(podsToDelete, updateRevision)
		klog.V(3).InfoS("CloneSet tried to delete pods specified", "cloneSet", klog.KObj(updateCS), "deleteReadyLimit", diffRes.deleteReadyLimit,
			"newPods", util.GetPodNames(newPodsToDelete).List(), "oldPods", util.GetPodNames(oldPodsToDelete).List())

		podsCanDelete := make([]*v1.Pod, 0, len(podsToDelete))
		for _, pod := range podsToDelete {
			if !isPodReady(coreControl, pod) {
				podsCanDelete = append(podsCanDelete, pod)
			} else if diffRes.deleteReadyLimit > 0 {
				podsCanDelete = append(podsCanDelete, pod)
				diffRes.deleteReadyLimit--
			}
		}

		if modified, err := r.deletePods(updateCS, podsCanDelete, pvcs); err != nil || modified {
			return modified, err
		}
	}

	// 6. scale in
	if diffRes.scaleDownNum > 0 {
		if numToDelete > 0 {
			klog.V(3).InfoS("CloneSet skipped to scale in for deletion", "cloneSet", klog.KObj(updateCS), "scaleDownNum", diffRes.scaleDownNum,
				"numToDelete", numToDelete, "specifiedToDelete", len(podsSpecifiedToDelete), "preDelete", len(podsInPreDelete))
			return false, nil
		}

		klog.V(3).InfoS("CloneSet began to scale in", "cloneSet", klog.KObj(updateCS), "scaleDownNum", diffRes.scaleDownNum,
			"oldRevision", diffRes.scaleDownNumOldRevision, "deleteReadyLimit", diffRes.deleteReadyLimit)

		podsPreparingToDelete := r.choosePodsToDelete(updateCS, diffRes.scaleDownNum, diffRes.scaleDownNumOldRevision, notUpdatedPods, updatedPods)
		podsToDelete := make([]*v1.Pod, 0, len(podsPreparingToDelete))
		for _, pod := range podsPreparingToDelete {
			if !isPodReady(coreControl, pod) {
				podsToDelete = append(podsToDelete, pod)
			} else if diffRes.deleteReadyLimit > 0 {
				podsToDelete = append(podsToDelete, pod)
				diffRes.deleteReadyLimit--
			}
		}

		return r.deletePods(updateCS, podsToDelete, pvcs)
	}

	return false, nil
}

func (r *realControl) managePreparingDelete(cs *appsv1alpha1.CloneSet, pods, podsInPreDelete []*v1.Pod, numToDelete int) (bool, error) {
	//  We do not allow regret once the pod enter PreparingDelete state if MarkPodNotReady is set.
	// Actually, there is a bug cased by this transformation from PreparingDelete to Normal,
	// i.e., Lifecycle Updated Hook may be lost if the pod was transformed from Updating state
	// to PreparingDelete.
	if lifecycle.IsLifecycleMarkPodNotReady(cs.Spec.Lifecycle) {
		return false, nil
	}

	diff := int(*cs.Spec.Replicas) - len(pods) + numToDelete
	var modified bool
	for _, pod := range podsInPreDelete {
		if diff <= 0 {
			return modified, nil
		}
		if isSpecifiedDelete(cs, pod) {
			continue
		}

		klog.V(3).InfoS("CloneSet canceled deletion of pod and patch lifecycle from PreparingDelete to PreparingNormal",
			"cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
		if updated, gotPod, err := r.lifecycleControl.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingNormal, false); err != nil {
			return modified, err
		} else if updated {
			modified = true
			clonesetutils.ResourceVersionExpectations.Expect(gotPod)
		}
		diff--
	}
	return modified, nil
}

func (r *realControl) createPods(
	expectedCreations, expectedCurrentCreations int,
	currentCS, updateCS *appsv1alpha1.CloneSet,
	currentRevision, updateRevision string,
	availableIDs []string, existingPVCNames sets.String,
) (bool, error) {
	// new all pods need to create
	coreControl := clonesetcore.New(updateCS)
	newPods, err := coreControl.NewVersionedPods(currentCS, updateCS, currentRevision, updateRevision,
		expectedCreations, expectedCurrentCreations, availableIDs)
	if err != nil {
		return false, err
	}

	podsCreationChan := make(chan *v1.Pod, len(newPods))
	for _, p := range newPods {
		clonesetutils.ScaleExpectations.ExpectScale(clonesetutils.GetControllerKey(updateCS), expectations.Create, p.Name)
		podsCreationChan <- p
	}

	var created int64
	successPodNames := sync.Map{}
	_, err = clonesetutils.DoItSlowly(len(newPods), initialBatchSize, func() error {
		pod := <-podsCreationChan

		cs := updateCS
		if clonesetutils.EqualToRevisionHash("", pod, currentRevision) {
			cs = currentCS
		}
		lifecycle.SetPodLifecycle(appspub.LifecycleStatePreparingNormal)(pod)

		var createErr error
		if createErr = r.createOnePod(cs, pod, existingPVCNames); createErr != nil {
			return createErr
		}

		atomic.AddInt64(&created, 1)

		successPodNames.Store(pod.Name, struct{}{})
		return nil
	})

	// rollback to ignore failure pods because the informer won't observe these pods
	for _, pod := range newPods {
		if _, ok := successPodNames.Load(pod.Name); !ok {
			clonesetutils.ScaleExpectations.ObserveScale(clonesetutils.GetControllerKey(updateCS), expectations.Create, pod.Name)
		}
	}

	if created == 0 {
		return false, err
	}
	return true, err
}

func (r *realControl) createOnePod(cs *appsv1alpha1.CloneSet, pod *v1.Pod, existingPVCNames sets.String) error {
	claims := clonesetutils.GetPersistentVolumeClaims(cs, pod)
	for _, c := range claims {
		if existingPVCNames.Has(c.Name) {
			continue
		}
		clonesetutils.ScaleExpectations.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Create, c.Name)
		if err := r.Create(context.TODO(), &c); err != nil {
			clonesetutils.ScaleExpectations.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Create, c.Name)
			r.recorder.Eventf(cs, v1.EventTypeWarning, "FailedCreate", "failed to create pvc: %v, pvc: %v", err, util.DumpJSON(c))
			return err
		}
	}

	if err := r.Create(context.TODO(), pod); err != nil {
		r.recorder.Eventf(cs, v1.EventTypeWarning, "FailedCreate", "failed to create pod: %v, pod: %v", err, util.DumpJSON(pod))
		return err
	}

	r.recorder.Eventf(cs, v1.EventTypeNormal, "SuccessfulCreate", "succeed to create pod %s", pod.Name)
	return nil
}

func (r *realControl) deletePods(cs *appsv1alpha1.CloneSet, podsToDelete []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) (bool, error) {
	var modified bool
	for _, pod := range podsToDelete {
		if cs.Spec.Lifecycle != nil && lifecycle.IsPodHooked(cs.Spec.Lifecycle.PreDelete, pod) {
			markPodNotReady := cs.Spec.Lifecycle.PreDelete.MarkPodNotReady
			if updated, gotPod, err := r.lifecycleControl.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingDelete, markPodNotReady); err != nil {
				return false, err
			} else if updated {
				klog.V(3).InfoS("CloneSet scaling update pod lifecycle to PreparingDelete",
					"cloneSet", klog.KObj(cs), "pod", klog.KObj(pod))
				modified = true
				clonesetutils.ResourceVersionExpectations.Expect(gotPod)
			}
			continue
		}

		clonesetutils.ScaleExpectations.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
		if err := r.Delete(context.TODO(), pod); err != nil {
			clonesetutils.ScaleExpectations.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pod.Name)
			r.recorder.Eventf(cs, v1.EventTypeWarning, "FailedDelete", "failed to delete pod %s: %v", pod.Name, err)
			return modified, err
		}
		modified = true
		r.recorder.Event(cs, v1.EventTypeNormal, "SuccessfulDelete", fmt.Sprintf("succeed to delete pod %s", pod.Name))

		// delete pvcs which have the same instance-id
		for _, pvc := range pvcs {
			if pvc.Labels[appsv1alpha1.CloneSetInstanceID] != pod.Labels[appsv1alpha1.CloneSetInstanceID] {
				continue
			}

			clonesetutils.ScaleExpectations.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
			if err := r.Delete(context.TODO(), pvc); err != nil {
				clonesetutils.ScaleExpectations.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
				r.recorder.Eventf(cs, v1.EventTypeWarning, "FailedDelete", "failed to delete pvc %s: %v", pvc.Name, err)
				return modified, err
			}
		}
	}

	return modified, nil
}

func getPlannedDeletedPods(cs *appsv1alpha1.CloneSet, pods []*v1.Pod) ([]*v1.Pod, []*v1.Pod, int) {
	var podsSpecifiedToDelete []*v1.Pod
	var podsInPreDelete []*v1.Pod
	names := sets.NewString()
	for _, pod := range pods {
		if isSpecifiedDelete(cs, pod) {
			names.Insert(pod.Name)
			podsSpecifiedToDelete = append(podsSpecifiedToDelete, pod)
		}
		if lifecycle.GetPodLifecycleState(pod) == appspub.LifecycleStatePreparingDelete {
			names.Insert(pod.Name)
			podsInPreDelete = append(podsInPreDelete, pod)
		}
	}
	return podsSpecifiedToDelete, podsInPreDelete, names.Len()
}

// Get available IDs, if the a PVC exists but the corresponding pod does not exist, then reusing the ID, i.e., reuse the pvc.
// If there is not enough existing available IDs, then generate ID using rand utility.
// More details: if template changes more than container image, controller will delete pod during update, and
// it will keep the pvc to reuse.
func getOrGenAvailableIDs(num int, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) sets.String {
	existingIDs := sets.NewString()
	availableIDs := sets.NewString()
	for _, pvc := range pvcs {
		if id := pvc.Labels[appsv1alpha1.CloneSetInstanceID]; len(id) > 0 {
			existingIDs.Insert(id)
			availableIDs.Insert(id)
		}
	}

	for _, pod := range pods {
		if id := pod.Labels[appsv1alpha1.CloneSetInstanceID]; len(id) > 0 {
			existingIDs.Insert(id)
			availableIDs.Delete(id)
		}
	}

	retIDs := sets.NewString()
	for i := 0; i < num; i++ {
		id := getOrGenInstanceID(existingIDs, availableIDs)
		retIDs.Insert(id)
	}

	return retIDs
}

func getOrGenInstanceID(existingIDs, availableIDs sets.String) string {
	id, _ := availableIDs.PopAny()
	if len(id) == 0 {
		for {
			id = rand.String(LengthOfInstanceID)
			if !existingIDs.Has(id) {
				break
			}
		}
	}
	return id
}

func (r *realControl) choosePodsToDelete(cs *appsv1alpha1.CloneSet, totalDiff int, currentRevDiff int, notUpdatedPods, updatedPods []*v1.Pod) []*v1.Pod {
	coreControl := clonesetcore.New(cs)
	choose := func(pods []*v1.Pod, diff int) []*v1.Pod {
		// No need to sort pods if we are about to delete all of them.
		if diff < len(pods) {
			var ranker clonesetutils.Ranker
			if constraints := coreControl.GetPodSpreadConstraint(); len(constraints) > 0 {
				ranker = clonesetutils.NewSpreadConstraintsRanker(pods, constraints, r.Client)
			} else {
				ranker = clonesetutils.NewSameNodeRanker(pods)
			}
			sort.Sort(clonesetutils.ActivePodsWithRanks{
				Pods:   pods,
				Ranker: ranker,
				AvailableFunc: func(pod *v1.Pod) bool {
					return IsPodAvailable(coreControl, pod, cs.Spec.MinReadySeconds)
				},
			})
		} else if diff > len(pods) {
			klog.InfoS("Diff > len(pods) in choosePodsToDelete func which is not expected")
			return pods
		}
		return pods[:diff]
	}

	var podsToDelete []*v1.Pod
	if currentRevDiff >= totalDiff {
		podsToDelete = choose(notUpdatedPods, totalDiff)
	} else if currentRevDiff > 0 {
		podsToDelete = choose(notUpdatedPods, currentRevDiff)
		podsToDelete = append(podsToDelete, choose(updatedPods, totalDiff-currentRevDiff)...)
	} else {
		podsToDelete = choose(updatedPods, totalDiff)
	}

	return podsToDelete
}
