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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ScheduledFailedDuration = 10 * time.Minute

// rescheduleSubset will delete some unscheduled Pods that still in pending status. Some subsets have no
// sufficient resource can lead to some Pods scheduled failed. WorkloadSpread has multiple subset, so some
// Pods scheduled failed should be rescheduled to other subsets.
// controller will mark the subset contains Pods scheduled failed to unscheduable status and Webhook cannot inject
// Pod into this subset by check subset's status. The unscheduled subset can be kept for 10 minutes and then
// should be recovered schedulable status to schedule Pod again.
// TODO optimize the unscheduable duration of subset.
// return two parameters
// 1. new SubsetUnscheduledStatus
// 2. schedule failed Pods belongs to this subset
func rescheduleSubset(ws *appsv1alpha1.WorkloadSpread,
	pods []*corev1.Pod,
	oldSubsetStatus *appsv1alpha1.WorkloadSpreadSubsetStatus) (*appsv1alpha1.SubsetUnscheduledStatus, []*corev1.Pod) {
	subsetUnscheduledStatus := &appsv1alpha1.SubsetUnscheduledStatus{}
	subsetUnscheduledStatus.UnscheduledTime = metav1.Now()
	if oldSubsetStatus != nil {
		subsetUnscheduledStatus.FailedCount = oldSubsetStatus.SubsetUnscheduledStatus.FailedCount
	}

	scheduleFailedPods := make([]*corev1.Pod, 0)
	for i := range pods {
		if PodUnscheduledTimeout(ws, pods[i]) {
			scheduleFailedPods = append(scheduleFailedPods, pods[i])
		}
	}
	unschedulable := len(scheduleFailedPods) > 0

	if unschedulable {
		subsetUnscheduledStatus.Unschedulable = true
		if oldSubsetStatus == nil {
			subsetUnscheduledStatus.FailedCount = 1
		} else if !oldSubsetStatus.SubsetUnscheduledStatus.Unschedulable {
			subsetUnscheduledStatus.FailedCount = oldSubsetStatus.SubsetUnscheduledStatus.FailedCount + 1
		}
	} else {
		if oldSubsetStatus != nil && oldSubsetStatus.SubsetUnscheduledStatus.Unschedulable {
			expectReschedule := oldSubsetStatus.SubsetUnscheduledStatus.UnscheduledTime.Add(ScheduledFailedDuration)
			currentTime := time.Now()
			// the duration of unschedule status more than 10 minutes, recover to schedulable.
			if expectReschedule.Before(currentTime) {
				subsetUnscheduledStatus.Unschedulable = false
			} else {
				// less 10 minutes, keep unschedulable and set UnscheduledTime to the oldStatus's UnscheduledTime.
				subsetUnscheduledStatus.Unschedulable = true
				subsetUnscheduledStatus.UnscheduledTime = oldSubsetStatus.SubsetUnscheduledStatus.UnscheduledTime
				durationStore.Push(getWorkloadSpreadKey(ws), expectReschedule.Sub(currentTime))
			}
		}
	}

	return subsetUnscheduledStatus, scheduleFailedPods
}

func (r *ReconcileWorkloadSpread) cleanupUnscheduledPods(ws *appsv1alpha1.WorkloadSpread,
	scheduleFailedPodsMap map[string][]*corev1.Pod) error {
	for subsetName, pods := range scheduleFailedPodsMap {
		if err := r.deleteUnscheduledPodsForSubset(ws, pods, subsetName); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileWorkloadSpread) deleteUnscheduledPodsForSubset(ws *appsv1alpha1.WorkloadSpread,
	pods []*corev1.Pod, subsetName string) error {
	for _, pod := range pods {
		if err := r.Client.Delete(context.TODO(), pod); err != nil {
			r.recorder.Eventf(ws, corev1.EventTypeWarning,
				"DeletePodFailed",
				"WorkloadSpread %s/%s failed to delete unscheduled Pod %s/%s in subset %s",
				ws.Namespace, ws.Name, pod.Namespace, pod.Name, subsetName)
			return err
		}
		r.recorder.Eventf(ws, corev1.EventTypeNormal, "DeleteUnscheduledPod",
			"WorkloadSpread %s/%s delete unscheduled Pod %s/%s in subset %s successfully",
			ws.Namespace, ws.Name, pod.Namespace, pod.Name, subsetName)
	}
	return nil
}

// PodUnscheduledTimeout return true when Pod was scheduled failed and timeout.
func PodUnscheduledTimeout(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Spec.NodeName != "" || pod.Status.Phase != corev1.PodPending {
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
