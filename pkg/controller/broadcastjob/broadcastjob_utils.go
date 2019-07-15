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

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
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
func filterPods(pods []*v1.Pod) ([]*v1.Pod, []*v1.Pod, []*v1.Pod) {
	var activePods, succeededPods, failedPods []*v1.Pod
	for _, p := range pods {
		if p.Status.Phase == v1.PodSucceeded {
			succeededPods = append(succeededPods, p)
		} else if p.Status.Phase == v1.PodFailed {
			failedPods = append(failedPods, p)
		} else if p.DeletionTimestamp == nil {
			activePods = append(activePods, p)
		} else {
			klog.V(4).Infof("Ignoring inactive pod %v/%v in state %v, deletion time %v",
				p.Namespace, p.Name, p.Status.Phase, p.DeletionTimestamp)
		}
	}
	return activePods, failedPods, succeededPods
}

// pastBackoffLimitOnFailure checks if container restartCounts sum exceeds BackoffLimit
// this method applies only to pods with restartPolicy == OnFailure
func pastBackoffLimitOnFailure(job *appsv1alpha1.BroadcastJob, pods []*v1.Pod) bool {
	if job.Spec.CompletionPolicy.BackoffLimit == nil {
		// not specify means BackoffLimit is disabled
		return false
	}
	if job.Spec.Template.Spec.RestartPolicy != v1.RestartPolicyOnFailure {
		return false
	}
	result := int32(0)
	for i := range pods {
		po := pods[i]
		if po.Status.Phase != v1.PodRunning {
			continue
		}
		// for only running pods
		for j := range po.Status.InitContainerStatuses {
			stat := po.Status.InitContainerStatuses[j]
			result += stat.RestartCount
		}
		for j := range po.Status.ContainerStatuses {
			stat := po.Status.ContainerStatuses[j]
			result += stat.RestartCount
		}
	}
	return result >= *job.Spec.CompletionPolicy.BackoffLimit
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
func pastTTLDeadline(job *appsv1alpha1.BroadcastJob) bool {
	if job.Spec.CompletionPolicy.TTLSecondsAfterFinished == nil || job.Status.CompletionTime == nil {
		return false
	}
	now := metav1.Now()
	finishTime := job.Status.CompletionTime.Time
	duration := now.Time.Sub(finishTime)
	allowedDuration := time.Duration(*job.Spec.CompletionPolicy.TTLSecondsAfterFinished) * time.Second
	return duration >= allowedDuration
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
	if controllerRef.Controller == nil || *controllerRef.Controller != true {
		return fmt.Errorf("controllerRef.Controller is not set to true")
	}
	if controllerRef.BlockOwnerDeletion == nil || *controllerRef.BlockOwnerDeletion != true {
		return fmt.Errorf("controllerRef.BlockOwnerDeletion is not set")
	}
	return nil
}
