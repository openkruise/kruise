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
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
)

// syncSubsetPodDeletionCost calculates the deletion-cost for the Pods belong to subset and update deletion-cost annotation.
// We have three types for subset's Pod deletion-cost
// 1. the number of active Pods in this subset <= maxReplicas, deletion-cost = 100. indicating the priority of Pods
//    in this subset is more higher than other Pods in workload.
// 2. subset's maxReplicas is nil, deletion-cost = 0.
// 3. the number of active Pods in this subset > maxReplicas, two class: (a) deletion-cost = -100, (b) deletion-cost = +100.
//    indicating we prefer deleting the Pod have -100 deletion-cost in this subset in order to control the instance of subset
//    meeting up the desired maxReplicas number.
func (r *ReconcileWorkloadSpread) syncSubsetPodDeletionCost(
	ws *appsv1alpha1.WorkloadSpread,
	subset *appsv1alpha1.WorkloadSpreadSubset,
	pods []*corev1.Pod,
	workloadReplicas int32) error {
	var err error
	// slice that will contain all Pods that want to set deletion-cost a positive value.
	var positivePods []*corev1.Pod
	// slice that will contain all Pods that want to set deletion-cost a negative value.
	var negativePods []*corev1.Pod
	// slice that will contain all Pods that setting deletion-cost to 0.
	var zeroPods []*corev1.Pod

	// count active Pods
	activePods := make([]*corev1.Pod, 0, len(pods))
	for i := range pods {
		if kubecontroller.IsPodActive(pods[i]) {
			activePods = append(activePods, pods[i])
		}
	}
	replicas := len(activePods)

	// First we partition Pods into three lists: positive, negative and zero list.
	if subset.MaxReplicas == nil {
		// maxReplicas is nil, which means there is no limit to the number of Pods in this subset.
		zeroPods = activePods
	} else {
		subsetMaxReplicas, err := intstr.GetValueFromIntOrPercent(subset.MaxReplicas, int(workloadReplicas), true)
		if err != nil || subsetMaxReplicas < 0 {
			klog.Errorf("failed to get maxReplicas value from subset (%s) of WorkloadSpread (%s/%s)",
				subset.Name, ws.Namespace, ws.Name)
			return nil
		}

		if replicas <= subsetMaxReplicas {
			positivePods = activePods
		} else {
			// Pods are classified two class, the one is more healthy and it's size = subsetMaxReplicas, so
			// setting deletion-cost to "100", another one is the left Pods means preferring to delete it,
			// setting deletion-cost to "-100"， size = replicas - subsetMaxReplicas.
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

	err = r.updateDeletionCostForSubsetPods(ws, subset, positivePods, wsutil.PodDeletionCostPositive)
	if err != nil {
		return err
	}
	err = r.updateDeletionCostForSubsetPods(ws, subset, negativePods, wsutil.PodDeletionCostNegative)
	if err != nil {
		return err
	}
	err = r.updateDeletionCostForSubsetPods(ws, subset, zeroPods, wsutil.PodDeletionCostDefault)
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileWorkloadSpread) updateDeletionCostForSubsetPods(ws *appsv1alpha1.WorkloadSpread,
	subset *appsv1alpha1.WorkloadSpreadSubset, pods []*corev1.Pod, deletionCostStr string) error {
	for _, pod := range pods {
		if err := r.updatePodDeletionCost(ws, pod, deletionCostStr); err != nil {
			r.recorder.Eventf(ws, corev1.EventTypeWarning,
				"UpdatePodDeletionCostFailed",
				"WorkloadSpread %s/%s failed to update deletion-cost annotation to %d for Pod %s/%s in subset %s",
				ws.Namespace, ws.Name, deletionCostStr, pod.Namespace, pod.Name, subset.Name)
			return err
		}
	}
	return nil
}

func (r *ReconcileWorkloadSpread) updatePodDeletionCost(ws *appsv1alpha1.WorkloadSpread,
	pod *corev1.Pod, deletionCostStr string) error {
	clone := pod.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		updated := updatePodAnnotation(clone, wsutil.PodDeletionCostAnnotation, deletionCostStr)
		if !updated {
			return nil
		}

		updateErr := r.Update(context.TODO(), clone)
		if updateErr == nil {
			klog.V(3).Infof("WorkloadSpread (%s/%s) updated deletion-cost annotation to %s for Pod (%s/%s) successfully",
				ws.Namespace, ws.Name, deletionCostStr, pod.Namespace, pod.Name)
			return nil
		}

		key := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
		if err := r.Get(context.TODO(), key, clone); err != nil {
			klog.Errorf("error getting Pod (%s/%s) from client", pod.Namespace, pod.Name)
			return err
		}

		return updateErr
	})
	return err
}

func updatePodAnnotation(pod *corev1.Pod, annotationKey, annotationValue string) bool {
	podAnnotation := pod.GetAnnotations()
	oldValue, exist := podAnnotation[annotationKey]
	// annotation has been set
	if exist && annotationValue == oldValue {
		return false
	}
	// keep the original condition.
	if !exist && annotationValue == "0" {
		return false
	}

	if podAnnotation == nil {
		podAnnotation = make(map[string]string)
	}
	podAnnotation[annotationKey] = annotationValue
	pod.SetAnnotations(podAnnotation)

	return true
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
	sort.Slice(waitDeleteIndexes, func(i, j int) bool {
		return kubecontroller.ActivePods(pods).Less(waitDeleteIndexes[i], waitDeleteIndexes[j])
	})

	return waitDeleteIndexes
}
