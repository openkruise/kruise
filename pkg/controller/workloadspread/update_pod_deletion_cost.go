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

package workloadspread

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
)

const (
	// RevisionAnnotation is the revision annotation of a deployment's replica sets which records its rollout sequence
	RevisionAnnotation = "deployment.kubernetes.io/revision"
)

func (r *ReconcileWorkloadSpread) getWorkloadLatestVersion(ws *appsv1alpha1.WorkloadSpread) (string, error) {
	targetRef := ws.Spec.TargetReference
	if targetRef == nil {
		return "", nil
	}

	gvk := schema.FromAPIVersionAndKind(targetRef.APIVersion, targetRef.Kind)
	key := types.NamespacedName{Namespace: ws.Namespace, Name: targetRef.Name}

	object := wsutil.GenerateEmptyWorkloadObject(gvk, key)
	if err := r.Get(context.TODO(), key, object); err != nil {
		return "", client.IgnoreNotFound(err)
	}

	return wsutil.GetWorkloadVersion(r.Client, object)
}

func (r *ReconcileWorkloadSpread) updateDeletionCost(ws *appsv1alpha1.WorkloadSpread,
	versionedPodMap map[string]map[string][]*corev1.Pod,
	workloadReplicas int32) error {
	targetRef := ws.Spec.TargetReference
	if targetRef == nil || !isEffectiveKindForDeletionCost(targetRef) {
		return nil
	}

	latestVersion, err := r.getWorkloadLatestVersion(ws)
	if err != nil {
		klog.ErrorS(err, "Failed to get the latest version for workload in workloadSpread", "workloadSpread", klog.KObj(ws))
		return err
	}

	// To try our best to keep the distribution of workload description during workload rolling:
	// - to the latest version, we hope to scale down the last subset preferentially;
	// - to other old versions, we hope to scale down the first subset preferentially;
	for version, podMap := range versionedPodMap {
		err = r.updateDeletionCostBySubset(ws, podMap, workloadReplicas, version != latestVersion)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileWorkloadSpread) updateDeletionCostBySubset(ws *appsv1alpha1.WorkloadSpread,
	podMap map[string][]*corev1.Pod, workloadReplicas int32, reverseOrder bool) error {
	subsetNum := len(ws.Spec.Subsets)
	subsetIndex := func(index int) int {
		if reverseOrder {
			return subsetNum - index - 1
		}
		return index
	}
	// update Pod's deletion-cost annotation in each subset
	for idx, subset := range ws.Spec.Subsets {
		if err := r.syncSubsetPodDeletionCost(ws, &subset, subsetIndex(idx), podMap[subset.Name], workloadReplicas); err != nil {
			return err
		}
	}
	// update the deletion-cost annotation for such pods that do not match any real subsets.
	// these pods will have the minimum deletion-cost, and will be deleted preferentially.
	if len(podMap[FakeSubsetName]) > 0 {
		if err := r.syncSubsetPodDeletionCost(ws, nil, len(ws.Spec.Subsets), podMap[FakeSubsetName], workloadReplicas); err != nil {
			return err
		}
	}
	return nil
}

// syncSubsetPodDeletionCost calculates the deletion-cost for the Pods belong to subset and update deletion-cost annotation.
// We have two conditions for subset's Pod deletion-cost
//  1. the number of active Pods in this subset <= maxReplicas or maxReplicas = nil, deletion-cost = 100 * (subsets.length - subsetIndex).
//     name         subset-a   subset-b  subset-c
//     maxReplicas    10          10        nil
//     pods number    10          10        10
//     deletion-cost  300         200       100
//     We delete Pods from back subset to front subset. The deletion order is: c -> b -> a.
//  2. the number of active Pods in this subset > maxReplicas
//     two class:
//     (a) the extra Pods more than maxReplicas: deletion-cost = -100 * (subsetIndex + 1) [Priority Deletion],
//     (b) deletion-cost = 100 * (subsets.length - subsetIndex) [Reserve].
//     name         subset-a       subset-b     subset-c
//     maxReplicas    10            10           nil
//     pods number    20            20           20
//     deletion-cost (300,-100)    (200,-200)    100
func (r *ReconcileWorkloadSpread) syncSubsetPodDeletionCost(
	ws *appsv1alpha1.WorkloadSpread,
	subset *appsv1alpha1.WorkloadSpreadSubset,
	subsetIndex int,
	pods []*corev1.Pod,
	workloadReplicas int32) error {
	var err error
	// slice that will contain all Pods that want to set deletion-cost a positive value.
	var positivePods []*corev1.Pod
	// slice that will contain all Pods that want to set deletion-cost a negative value.
	var negativePods []*corev1.Pod

	// count active Pods
	activePods := make([]*corev1.Pod, 0, len(pods))
	for i := range pods {
		if kubecontroller.IsPodActive(pods[i]) {
			activePods = append(activePods, pods[i])
		}
	}
	replicas := len(activePods)

	// First we partition Pods into two lists: positive, negative list.
	if subset == nil {
		// for the scene of FakeSubsetName, where the pods don't match any subset and will be deleted preferentially.
		negativePods = activePods
	} else if subset.MaxReplicas == nil {
		// maxReplicas is nil, which means there is no limit to the number of Pods in this subset.
		positivePods = activePods
	} else {
		subsetMaxReplicas, err := intstr.GetValueFromIntOrPercent(subset.MaxReplicas, int(workloadReplicas), true)
		if err != nil || subsetMaxReplicas < 0 {
			klog.ErrorS(err, "Failed to get maxReplicas value from subset of WorkloadSpread", "subsetName", subset.Name, "workloadSpread", klog.KObj(ws))
			return nil
		}

		if replicas <= subsetMaxReplicas {
			positivePods = activePods
		} else {
			// Pods are classified two class, the one is more healthy and it's size = subsetMaxReplicas, so
			// setting deletion-cost to positive, another one is the left Pods means preferring to delete it,
			// setting deletion-cost to negative， size = replicas - subsetMaxReplicas.
			positivePods = make([]*corev1.Pod, 0, subsetMaxReplicas)
			negativePods = make([]*corev1.Pod, 0, replicas-subsetMaxReplicas)

			// sort Pods according to Pod's condition.
			indexes := sortDeleteIndexes(activePods)
			// partition Pods into negativePods and positivePods by sorted indexes.
			for i := range indexes {
				if i < (replicas - subsetMaxReplicas) {
					negativePods = append(negativePods, activePods[indexes[i]])
				} else {
					positivePods = append(positivePods, activePods[indexes[i]])
				}
			}
		}
	}

	err = r.updateDeletionCostForSubsetPods(ws, subset, positivePods, strconv.Itoa(wsutil.PodDeletionCostPositive*(len(ws.Spec.Subsets)-subsetIndex)))
	if err != nil {
		return err
	}
	return r.updateDeletionCostForSubsetPods(ws, subset, negativePods, strconv.Itoa(wsutil.PodDeletionCostNegative*(subsetIndex+1)))
}

func (r *ReconcileWorkloadSpread) updateDeletionCostForSubsetPods(ws *appsv1alpha1.WorkloadSpread,
	subset *appsv1alpha1.WorkloadSpreadSubset, pods []*corev1.Pod, deletionCostStr string) error {
	for _, pod := range pods {
		if err := r.patchPodDeletionCost(ws, pod, deletionCostStr); err != nil {
			subsetName := FakeSubsetName
			if subset != nil {
				subsetName = subset.Name
			}
			r.recorder.Eventf(ws, corev1.EventTypeWarning,
				"PatchPodDeletionCostFailed",
				"WorkloadSpread %s/%s failed to patch deletion-cost annotation to %d for Pod %s/%s in subset %s",
				ws.Namespace, ws.Name, deletionCostStr, pod.Namespace, pod.Name, subsetName)
			return err
		}
	}
	return nil
}

func (r *ReconcileWorkloadSpread) patchPodDeletionCost(ws *appsv1alpha1.WorkloadSpread,
	pod *corev1.Pod, deletionCostStr string) error {
	clone := pod.DeepCopy()
	annotationKey := wsutil.PodDeletionCostAnnotation
	annotationValue := deletionCostStr

	podAnnotation := pod.GetAnnotations()
	oldValue, exist := podAnnotation[annotationKey]
	// annotation has been set
	if exist && annotationValue == oldValue {
		return nil
	}
	// keep the original setting.
	if !exist && annotationValue == "0" {
		return nil
	}

	body := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, annotationKey, annotationValue)
	if err := r.Patch(context.TODO(), clone, client.RawPatch(types.StrategicMergePatchType, []byte(body))); err != nil {
		return err
	}
	klog.V(3).InfoS("WorkloadSpread patched deletion-cost annotation for Pod successfully", "workloadSpread", klog.KObj(ws), "deletionCost", deletionCostStr, "pod", klog.KObj(pod))
	return nil
}

func sortDeleteIndexes(pods []*corev1.Pod) []int {
	waitDeleteIndexes := make([]int, 0, len(pods))
	for i := 0; i < len(pods); i++ {
		waitDeleteIndexes = append(waitDeleteIndexes, i)
	}

	// Sort Pods with default sequence
	//	- Unassigned < assigned
	//	- PodPending < PodUnknown < PodRunning
	//	- Not ready < ready
	//	- Been ready for empty time < less time < more time
	//	- Pods with containers with higher restart counts < lower restart counts
	//	- Empty creation time pods < newer pods < older pods

	// Using SliceStable to keep equal elements in their original order. It can avoid frequently update.
	sort.SliceStable(waitDeleteIndexes, func(i, j int) bool {
		return kubecontroller.ActivePods(pods).Less(waitDeleteIndexes[i], waitDeleteIndexes[j])
	})

	return waitDeleteIndexes
}

func isEffectiveKindForDeletionCost(targetRef *appsv1alpha1.TargetReference) bool {
	switch targetRef.Kind {
	case controllerKindRS.Kind, controllerKindDep.Kind, controllerKruiseKindCS.Kind:
		return true
	}
	return false
}
