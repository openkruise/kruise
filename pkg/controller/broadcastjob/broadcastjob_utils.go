/*
Copyright 2019 The Kruise Authors.
Copyright 2019 The Kubernetes Authors.

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

package broadcastjob

import (
	"fmt"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// IsJobFinished returns true when finishing job
func IsJobFinished(j *appsv1alpha1.BroadcastJob) bool {
	if j.Spec.CompletionPolicy.Type == appsv1alpha1.Never {
		return false
	}

	for _, c := range j.Status.Conditions {
		if (c.Type == appsv1alpha1.JobComplete || c.Type == appsv1alpha1.JobFailed) && c.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

// filterPods returns list of activePods and number of failed pods, number of succeeded pods
func filterPods(restartLimit int32, pods []*v1.Pod) ([]*v1.Pod, []*v1.Pod, []*v1.Pod) {
	var activePods, succeededPods, failedPods []*v1.Pod
	for _, p := range pods {
		if p.Status.Phase == v1.PodSucceeded {
			succeededPods = append(succeededPods, p)
		} else if p.Status.Phase == v1.PodFailed {
			failedPods = append(failedPods, p)
		} else if p.DeletionTimestamp == nil {
			if isPodFailed(restartLimit, p) {
				failedPods = append(failedPods, p)
			} else {
				activePods = append(activePods, p)
			}
		} else {
			klog.V(4).InfoS("Ignoring inactive pod, deletion scheduled",
				"pod", klog.KObj(p), "phase", p.Status.Phase, "deletionTimestamp", p.DeletionTimestamp)
		}
	}
	return activePods, failedPods, succeededPods
}

// isPodFailed marks the pod as a failed pod, when
// 1. restartPolicy==Never, and exit code is not 0
// 2. restartPolicy==OnFailure, and RestartCount > restartLimit
func isPodFailed(restartLimit int32, pod *v1.Pod) bool {
	if pod.Spec.RestartPolicy != v1.RestartPolicyOnFailure {
		return false
	}

	restartCount := int32(0)
	for i := range pod.Status.InitContainerStatuses {
		stat := pod.Status.InitContainerStatuses[i]
		restartCount += stat.RestartCount
	}
	for i := range pod.Status.ContainerStatuses {
		stat := pod.Status.ContainerStatuses[i]
		restartCount += stat.RestartCount
	}

	return restartCount > restartLimit
}

// pastActiveDeadline checks if job has ActiveDeadlineSeconds field set and if it is exceeded.
func pastActiveDeadline(job *appsv1alpha1.BroadcastJob) bool {
	if job.Spec.CompletionPolicy.ActiveDeadlineSeconds == nil || job.Status.StartTime == nil {
		return false
	}
	now := metav1.Now()
	start := job.Status.StartTime.Time
	duration := now.Time.Sub(start)
	allowedDuration := time.Duration(*job.Spec.CompletionPolicy.ActiveDeadlineSeconds) * time.Second
	return duration >= allowedDuration
}

// pastTTLDeadline checks if job has past the TTLSecondsAfterFinished deadline
func pastTTLDeadline(job *appsv1alpha1.BroadcastJob) (bool, time.Duration) {
	if job.Spec.CompletionPolicy.TTLSecondsAfterFinished == nil || job.Status.CompletionTime == nil {
		return false, -1
	}
	now := metav1.Now()
	finishTime := job.Status.CompletionTime.Time
	duration := now.Time.Sub(finishTime)
	allowedDuration := time.Duration(*job.Spec.CompletionPolicy.TTLSecondsAfterFinished) * time.Second
	return duration >= allowedDuration, allowedDuration - duration
}

func newCondition(conditionType appsv1alpha1.JobConditionType, reason, message string) appsv1alpha1.JobCondition {
	return appsv1alpha1.JobCondition{
		Type:               conditionType,
		Status:             v1.ConditionTrue,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

func asOwner(job *appsv1alpha1.BroadcastJob) *metav1.OwnerReference {
	return metav1.NewControllerRef(job, controllerKind)
}

func validateControllerRef(controllerRef *metav1.OwnerReference) error {
	if controllerRef == nil {
		return fmt.Errorf("controllerRef is nil")
	}
	if len(controllerRef.APIVersion) == 0 {
		return fmt.Errorf("controllerRef has empty APIVersion")
	}
	if len(controllerRef.Kind) == 0 {
		return fmt.Errorf("controllerRef has empty Kind")
	}
	if controllerRef.Controller == nil || !*controllerRef.Controller {
		return fmt.Errorf("controllerRef.Controller is not set to true")
	}
	if controllerRef.BlockOwnerDeletion == nil || !*controllerRef.BlockOwnerDeletion {
		return fmt.Errorf("controllerRef.BlockOwnerDeletion is not set")
	}
	return nil
}

func getAssignedNode(pod *v1.Pod) string {
	if pod.Spec.NodeName != "" {
		return pod.Spec.NodeName
	}
	if pod.Spec.Affinity != nil &&
		pod.Spec.Affinity.NodeAffinity != nil &&
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		terms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		for _, t := range terms {
			for _, req := range t.MatchFields {
				if req.Key == metav1.ObjectNameField && req.Operator == v1.NodeSelectorOpIn && len(req.Values) == 1 {
					return req.Values[0]
				}
			}
		}
	}
	klog.InfoS("Could not find assigned node in Pod", "pod", klog.KObj(pod))
	return ""
}
