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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var _ handler.TypedEventHandler[*corev1.Pod] = &enqueueRequestForPod{}

type enqueueRequestForPod struct{}

func (p *enqueueRequestForPod) Delete(ctx context.Context, evt event.TypedDeleteEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForPod) Generic(ctx context.Context, evt event.TypedGenericEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForPod) Create(ctx context.Context, evt event.TypedCreateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	p.handlePodCreate(q, evt.Object)
}
func (p *enqueueRequestForPod) Update(ctx context.Context, evt event.TypedUpdateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
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

func isInterestingPod(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil ||
		pod.Status.Phase == corev1.PodPending ||
		pod.Spec.RestartPolicy == corev1.RestartPolicyAlways {
		return false
	}

	sidecars := getSidecar(pod)
	if sidecars.Len() == 0 || containersCompleted(pod, sidecars) {
		return false
	}

	switch pod.Spec.RestartPolicy {
	case corev1.RestartPolicyNever:
		return containersCompleted(pod, getMain(pod))
	case corev1.RestartPolicyOnFailure:
		return containersSucceeded(pod, getMain(pod))
	}
	return false
}

func getMain(pod *corev1.Pod) sets.String {
	mainNames := sets.NewString()
	for i := range pod.Spec.Containers {
		if !isSidecar(pod.Spec.Containers[i]) {
			mainNames.Insert(pod.Spec.Containers[i].Name)
		}
	}
	return mainNames
}

func getSidecar(pod *corev1.Pod) sets.String {
	sidecarNames := sets.NewString()
	for i := range pod.Spec.Containers {
		if isSidecar(pod.Spec.Containers[i]) {
			sidecarNames.Insert(pod.Spec.Containers[i].Name)
		}
	}
	return sidecarNames
}

func isSidecar(container corev1.Container) bool {
	for _, env := range container.Env {
		if env.Name == appsv1alpha1.KruiseTerminateSidecarEnv && strings.EqualFold(env.Value, "true") {
			return true
		} else if env.Name == appsv1alpha1.KruiseTerminateSidecarWithImageEnv && env.Value != "" {
			return true
		}
	}
	return false
}
