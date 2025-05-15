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

package cloneset

import (
	"context"
	"fmt"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	"github.com/openkruise/kruise/pkg/controller/cloneset/sync"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
)

// StatusUpdater is interface for updating CloneSet status.
type StatusUpdater interface {
	UpdateCloneSetStatus(cs *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus, pods []*v1.Pod) error
}

func newStatusUpdater(c client.Client) StatusUpdater {
	return &realStatusUpdater{Client: c}
}

type realStatusUpdater struct {
	client.Client
}

func (r *realStatusUpdater) UpdateCloneSetStatus(cs *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus, pods []*v1.Pod) error {
	r.calculateStatus(cs, newStatus, pods)
	if err := clonesetcore.New(cs).ExtraStatusCalculation(newStatus, pods); err != nil {
		return fmt.Errorf("failed to calculate extra status for cloneSet %s/%s: %v", cs.Namespace, cs.Name, err)
	}
	if !r.inconsistentStatus(cs, newStatus) {
		return nil
	}
	// Check for progress deadline exceeded condition
	if cs.Spec.ProgressDeadlineSeconds != nil {
		// Find the Progressing condition
		var progressingCondition *appsv1alpha1.CloneSetCondition
		for i := range newStatus.Conditions {
			if newStatus.Conditions[i].Type == appsv1alpha1.CloneSetConditionTypeProgressing {
				progressingCondition = &newStatus.Conditions[i]
				break
			}
		}
		if progressingCondition == nil {
			// Initialize the Progressing condition if it doesn't exist
			progressingCondition = &appsv1alpha1.CloneSetCondition{
				Type:               appsv1alpha1.CloneSetConditionTypeProgressing,
				Status:             v1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Message:            "CloneSet is progressing.",
			}
			newStatus.Conditions = append(newStatus.Conditions, *progressingCondition)
		}
		// Check if the CloneSet has reached completion
		isCompleted := newStatus.UpdatedReadyReplicas == *cs.Spec.Replicas

		// Check if paused due to partition
		isPaused := false
		if cs.Spec.UpdateStrategy.Partition != nil {
			updatedReplicas := newStatus.UpdatedReplicas
			desiredReplicas := *cs.Spec.Replicas

			// Handle IntOrString type properly
			var targetPartitionValue int32
			if cs.Spec.UpdateStrategy.Partition.Type == intstr.Int {
				targetPartitionValue = int32(cs.Spec.UpdateStrategy.Partition.IntVal)
			} else {
				// For percentage-based partition, calculate the actual number
				// Assuming percentage is of the total replicas
				percentage, _ := strconv.Atoi(cs.Spec.UpdateStrategy.Partition.StrVal)
				targetPartitionValue = int32(float64(desiredReplicas) * float64(percentage) / 100.0)
			}

			if desiredReplicas-updatedReplicas == targetPartitionValue {
				isPaused = true
			}
		}
		if isCompleted || isPaused {
			// Reset any deadline condition if we're complete or paused
			for i := range newStatus.Conditions {
				if newStatus.Conditions[i].Type == appsv1alpha1.CloneSetConditionTypeProgressing {
					if newStatus.Conditions[i].Status != v1.ConditionTrue {
						newStatus.Conditions[i].Status = v1.ConditionTrue
						newStatus.Conditions[i].LastTransitionTime = metav1.Now()
						newStatus.Conditions[i].Reason = ""
						newStatus.Conditions[i].Message = "CloneSet is progressing."
					}
					break
				}
			}
		} else {
			// Check for deadline exceeded
			for i := range newStatus.Conditions {
				if newStatus.Conditions[i].Type == appsv1alpha1.CloneSetConditionTypeProgressing {
					// Get when the condition was last updated
					lastUpdateTime := newStatus.Conditions[i].LastTransitionTime.Time
					deadline := lastUpdateTime.Add(time.Duration(*cs.Spec.ProgressDeadlineSeconds) * time.Second)

					if time.Now().After(deadline) && newStatus.Conditions[i].Status != v1.ConditionFalse {
						newStatus.Conditions[i].Status = v1.ConditionFalse
						newStatus.Conditions[i].LastTransitionTime = metav1.Now()
						newStatus.Conditions[i].Reason = string(appsv1alpha1.CloneSetProgressDeadlineExceeded)
						newStatus.Conditions[i].Message = "CloneSet progress deadline exceeded."
					}
					break
				}
			}
		}
	}
	klog.InfoS("To update CloneSet status", "cloneSet", klog.KObj(cs), "replicas", newStatus.Replicas, "ready", newStatus.ReadyReplicas, "available", newStatus.AvailableReplicas,
		"updated", newStatus.UpdatedReplicas, "updatedReady", newStatus.UpdatedReadyReplicas, "currentRevision", newStatus.CurrentRevision, "updateRevision", newStatus.UpdateRevision)
	return r.updateStatus(cs, newStatus)
}

func (r *realStatusUpdater) updateStatus(cs *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone := &appsv1alpha1.CloneSet{}
		if err := r.Get(context.TODO(), types.NamespacedName{Namespace: cs.Namespace, Name: cs.Name}, clone); err != nil {
			return err
		}
		clone.Status = *newStatus
		return r.Status().Update(context.TODO(), clone)
	})
}

func (r *realStatusUpdater) inconsistentStatus(cs *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus) bool {
	oldStatus := cs.Status
	return newStatus.ObservedGeneration > oldStatus.ObservedGeneration ||
		newStatus.Replicas != oldStatus.Replicas ||
		newStatus.ReadyReplicas != oldStatus.ReadyReplicas ||
		newStatus.AvailableReplicas != oldStatus.AvailableReplicas ||
		newStatus.UpdatedReadyReplicas != oldStatus.UpdatedReadyReplicas ||
		newStatus.UpdatedReplicas != oldStatus.UpdatedReplicas ||
		newStatus.UpdatedAvailableReplicas != oldStatus.UpdatedAvailableReplicas ||
		newStatus.ExpectedUpdatedReplicas != oldStatus.ExpectedUpdatedReplicas ||
		newStatus.UpdateRevision != oldStatus.UpdateRevision ||
		newStatus.CurrentRevision != oldStatus.CurrentRevision ||
		newStatus.LabelSelector != oldStatus.LabelSelector
}

func (r *realStatusUpdater) calculateStatus(cs *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus, pods []*v1.Pod) {
	coreControl := clonesetcore.New(cs)
	for _, pod := range pods {
		newStatus.Replicas++
		if coreControl.IsPodUpdateReady(pod, 0) {
			newStatus.ReadyReplicas++
		}
		if sync.IsPodAvailable(coreControl, pod, cs.Spec.MinReadySeconds) {
			newStatus.AvailableReplicas++
		}
		if clonesetutils.EqualToRevisionHash("", pod, newStatus.UpdateRevision) {
			newStatus.UpdatedReplicas++
		}
		if clonesetutils.EqualToRevisionHash("", pod, newStatus.UpdateRevision) && coreControl.IsPodUpdateReady(pod, 0) {
			newStatus.UpdatedReadyReplicas++
		}
		if clonesetutils.EqualToRevisionHash("", pod, newStatus.UpdateRevision) && sync.IsPodAvailable(coreControl, pod, cs.Spec.MinReadySeconds) {
			newStatus.UpdatedAvailableReplicas++
		}
	}
	// Consider the update revision as stable if revisions of all pods are consistent to it and have the expected number of replicas, no need to wait all of them ready
	if newStatus.UpdatedReplicas == newStatus.Replicas && newStatus.Replicas == *cs.Spec.Replicas {
		newStatus.CurrentRevision = newStatus.UpdateRevision
	}

	if partition, err := util.CalculatePartitionReplicas(cs.Spec.UpdateStrategy.Partition, cs.Spec.Replicas); err == nil {
		newStatus.ExpectedUpdatedReplicas = *cs.Spec.Replicas - int32(partition)
	}
}
