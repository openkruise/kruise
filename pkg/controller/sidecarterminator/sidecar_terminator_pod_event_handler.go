/*
Copyright 2023 The Kruise Authors.

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

package sidecarterminator

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct{}

func (p *enqueueRequestForPod) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForPod) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForPod) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.handlePodCreate(q, evt.Object)
}
func (p *enqueueRequestForPod) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.handlePodUpdate(q, evt.ObjectOld, evt.ObjectNew)
}

func (p *enqueueRequestForPod) handlePodCreate(q workqueue.RateLimitingInterface, obj runtime.Object) {
	pod := obj.(*corev1.Pod)
	if isInterestingPod(pod) {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			},
		})
	}
}

func (p *enqueueRequestForPod) handlePodUpdate(q workqueue.RateLimitingInterface, old, cur runtime.Object) {
	newPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if oldPod.ResourceVersion == newPod.ResourceVersion {
		return
	}

	if isInterestingPod(newPod) {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: newPod.Namespace,
				Name:      newPod.Name,
			},
		})
	}
}
