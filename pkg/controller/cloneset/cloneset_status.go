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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	if r.inconsistentStatus(cs, newStatus) {
		klog.Infof("To update CloneSet status for  %s/%s, replicas=%d ready=%d available=%d updated=%d updatedReady=%d, revisions current=%s update=%s",
			cs.Namespace, cs.Name, newStatus.Replicas, newStatus.ReadyReplicas, newStatus.AvailableReplicas, newStatus.UpdatedReplicas, newStatus.UpdatedReadyReplicas, newStatus.CurrentRevision, newStatus.UpdateRevision)
		if err := r.updateStatus(cs, newStatus); err != nil {
			return err
		}
	}

	return clonesetcore.New(cs).ExtraStatusCalculation(newStatus, pods)
}

func (r *realStatusUpdater) updateStatus(cs *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone := &appsv1alpha1.CloneSet{}
		if err := r.Get(context.TODO(), types.NamespacedName{Namespace: cs.Namespace, Name: cs.Name}, clone); err != nil {
			return err
		}
		clone.Status = *newStatus
		clone.Annotations = cs.Annotations
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
		if coreControl.IsPodUpdateReady(pod, cs.Spec.MinReadySeconds) {
			newStatus.AvailableReplicas++
		}
		if clonesetutils.EqualToRevisionHash("", pod, newStatus.UpdateRevision) {
			newStatus.UpdatedReplicas++
		}
		if clonesetutils.EqualToRevisionHash("", pod, newStatus.UpdateRevision) && coreControl.IsPodUpdateReady(pod, 0) {
			newStatus.UpdatedReadyReplicas++
		}
	}
	// Consider the update revision as stable if revisions of all pods are consistent to it, no need to wait all of them ready
	if newStatus.UpdatedReplicas == newStatus.Replicas {
		newStatus.CurrentRevision = newStatus.UpdateRevision
	}

	if newStatus.UpdateRevision == newStatus.CurrentRevision {
		newStatus.ExpectedUpdatedReplicas = *cs.Spec.Replicas
	} else {
		if partition, err := util.CalculatePartitionReplicas(cs.Spec.UpdateStrategy.Partition, cs.Spec.Replicas); err == nil {
			newStatus.ExpectedUpdatedReplicas = *cs.Spec.Replicas - int32(partition)
		}
	}
}
