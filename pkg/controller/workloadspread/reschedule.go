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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	wsutil "github.com/openkruise/kruise/pkg/webhook/workloadspread/validating"
)

// rescheduleSubset will delete some unschedulable Pods that still in pending status. Some subsets have no
// sufficient resource can lead to some Pods scheduled failed. WorkloadSpread has multiple subset, so these
// unschedulable Pods should be rescheduled to other subsets.
// Controller will mark the subset contains unschedulable Pods to unschedulable status and Webhook cannot inject
// Pod into this subset by check subset's status. The unschedulable subset' status can be kept for 5 minutes and then
// should be recovered schedulable status to try scheduling Pods again.
// TODO optimize the unschedulable duration of subset.
// return one parameters - unschedulable Pods belongs to this subset.
func (r *ReconcileWorkloadSpread) rescheduleSubset(ws *appsv1alpha1.WorkloadSpread,
	pods []*corev1.Pod,
	subsetStatus, oldSubsetStatus *appsv1alpha1.WorkloadSpreadSubsetStatus) []*corev1.Pod {
	scheduleFailedPods := make([]*corev1.Pod, 0)
	for i := range pods {
		if PodUnscheduledTimeout(ws, pods[i]) {
			scheduleFailedPods = append(scheduleFailedPods, pods[i])
		}
	}
	unschedulable := len(scheduleFailedPods) > 0
	if unschedulable {
		klog.V(3).Infof("Subset (%s) of WorkloadSpread (%s/%s) is unschedulable", subsetStatus.Name, ws.Namespace, ws.Name)
	}

	oldCondition := GetWorkloadSpreadSubsetCondition(oldSubsetStatus, appsv1alpha1.SubsetSchedulable)
	if oldCondition == nil {
		if unschedulable {
			setWorkloadSpreadSubsetCondition(subsetStatus, NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionFalse, "", ""))
		} else {
			setWorkloadSpreadSubsetCondition(subsetStatus, NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionTrue, "", ""))
		}
		return scheduleFailedPods
	}

	// copy old condition to avoid unnecessary update.
	setWorkloadSpreadSubsetCondition(subsetStatus, oldCondition.DeepCopy())
	if unschedulable {
		setWorkloadSpreadSubsetCondition(subsetStatus, NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionFalse, "", ""))
	} else {
		// consider to recover
		if oldCondition.Status == corev1.ConditionFalse {
			expectReschedule := oldCondition.LastTransitionTime.Add(wsutil.MaxScheduledFailedDuration)
			currentTime := time.Now()
			// the duration of unschedule status more than 5 minutes, recover to schedulable.
			if expectReschedule.Before(currentTime) {
				r.recorder.Eventf(ws, corev1.EventTypeNormal,
					"RecoverSchedulable", "Subset %s of WorkloadSpread %s/%s is recovered from unschedulable to schedulable",
					subsetStatus.Name, ws.Namespace, ws.Name)
				setWorkloadSpreadSubsetCondition(subsetStatus, NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionTrue, "", ""))
			} else {
				// less 5 minutes, keep unschedulable.
				durationStore.Push(getWorkloadSpreadKey(ws), expectReschedule.Sub(currentTime))
			}
		}
	}

	return scheduleFailedPods
}

func (r *ReconcileWorkloadSpread) cleanupUnscheduledPods(ws *appsv1alpha1.WorkloadSpread,
	scheduleFailedPodsMap map[string][]*corev1.Pod) error {
	for subsetName, pods := range scheduleFailedPodsMap {
		if err := r.deletePodsForSubset(ws, pods, subsetName); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileWorkloadSpread) deletePodsForSubset(ws *appsv1alpha1.WorkloadSpread,
	pods []*corev1.Pod, subsetName string) error {
	for _, pod := range pods {
		if err := r.Client.Delete(context.TODO(), pod); err != nil {
			r.recorder.Eventf(ws, corev1.EventTypeWarning,
				"DeletePodFailed",
				"Failed to delete unschedulabe Pod %s/%s in Subset %s of WorkloadSpread %s/%s",
				pod.Namespace, pod.Name, subsetName, ws.Namespace, ws.Name)
			return err
		}
		klog.V(3).Infof("WorkloadSpread (%s/%s) delete unschedulabe Pod (%s/%s) in Subset %s successfully",
			ws.Namespace, ws.Name, pod.Namespace, pod.Name, subsetName)
	}
	return nil
}

// PodUnscheduledTimeout return true when Pod was scheduled failed and timeout.
func PodUnscheduledTimeout(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodPending || pod.Spec.NodeName != "" {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse &&
			condition.Reason == corev1.PodReasonUnschedulable {
			currentTime := time.Now()
			rescheduleCriticalSeconds := *ws.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds

			expectSchedule := pod.CreationTimestamp.Add(time.Second * time.Duration(rescheduleCriticalSeconds))
			// schedule timeout
			if expectSchedule.Before(currentTime) {
				return true
			}

			// no timeout, requeue key when expectSchedule is equal to time.Now()
			durationStore.Push(getWorkloadSpreadKey(ws), expectSchedule.Sub(currentTime))

			return false
		}
	}
	return false
}
