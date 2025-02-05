/*
Copyright 2022 The Kruise Authors.

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

package podprobemarker

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/podprobemarker"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.TypedEventHandler[*appsalphav1.PodProbeMarker] = &enqueueRequestForPodProbeMarker{}

type enqueueRequestForPodProbeMarker struct{}

func (p *enqueueRequestForPodProbeMarker) Create(ctx context.Context, evt event.TypedCreateEvent[*appsalphav1.PodProbeMarker], q workqueue.RateLimitingInterface) {
	p.queue(q, evt.Object)
}

func (p *enqueueRequestForPodProbeMarker) Delete(ctx context.Context, evt event.TypedDeleteEvent[*appsalphav1.PodProbeMarker], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPodProbeMarker) Generic(ctx context.Context, evt event.TypedGenericEvent[*appsalphav1.PodProbeMarker], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPodProbeMarker) Update(ctx context.Context, evt event.TypedUpdateEvent[*appsalphav1.PodProbeMarker], q workqueue.RateLimitingInterface) {
	p.queue(q, evt.ObjectNew)
}

func (p *enqueueRequestForPodProbeMarker) queue(q workqueue.RateLimitingInterface, obj runtime.Object) {
	ppm, ok := obj.(*appsalphav1.PodProbeMarker)
	if !ok {
		return
	}
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ppm.Namespace,
			Name:      ppm.Name,
		},
	})
}

var _ handler.TypedEventHandler[*corev1.Pod] = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	reader client.Reader
}

func (p *enqueueRequestForPod) Create(ctx context.Context, evt event.TypedCreateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Delete(ctx context.Context, evt event.TypedDeleteEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Generic(ctx context.Context, evt event.TypedGenericEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(ctx context.Context, evt event.TypedUpdateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	newObj := evt.ObjectNew
	oldObj := evt.ObjectOld
	oldInitialCondition := util.GetCondition(oldObj, corev1.PodInitialized)
	newInitialCondition := util.GetCondition(newObj, corev1.PodInitialized)
	if newObj == nil || !kubecontroller.IsPodActive(newObj) || newInitialCondition == nil ||
		newInitialCondition.Status != corev1.ConditionTrue || newObj.Spec.NodeName == "" {
		return
	}

	isServerlessPod := false
	node := &corev1.Node{}
	if err := p.reader.Get(context.TODO(), client.ObjectKey{Name: newObj.Spec.NodeName}, node); err != nil {
		klog.ErrorS(err, "Failed to get Node", "nodeName", newObj.Spec.NodeName)
		return
	}
	if node.Labels["type"] == VirtualKubelet {
		isServerlessPod = true
	}

	triggerReconcile := false
	diff := sets.NewString()
	// normal pod
	if !isServerlessPod {
		if ((oldInitialCondition == nil || oldInitialCondition.Status == corev1.ConditionFalse) &&
			newInitialCondition.Status == corev1.ConditionTrue) || oldObj.Status.PodIP != newObj.Status.PodIP {
			triggerReconcile = true
		}
	} else if utilfeature.DefaultFeatureGate.Enabled(features.EnablePodProbeMarkerOnServerless) {
		// The serverless pod probe results will patch to the pod condition field,
		// so it needs to be processed by reconcile, which in turn will patch pod labels or annotations.
		diff = diffPodConditions(oldObj.Status.Conditions, newObj.Status.Conditions)
		if diff.Len() > 0 {
			triggerReconcile = true
		}

	}
	if !triggerReconcile {
		return
	}

	ppms, err := podprobemarker.GetPodProbeMarkerForPod(p.reader, newObj)
	if err != nil {
		klog.ErrorS(err, "Failed to List PodProbeMarker")
		return
	}
	for _, ppm := range ppms {
		// normal pod
		if diff.Len() == 0 {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ppm.Namespace,
					Name:      ppm.Name,
				},
			})
			continue
		}

		// only reconcile related ppm
		for _, probe := range ppm.Spec.Probes {
			if diff.Has(probe.PodConditionType) {
				q.Add(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: ppm.Namespace,
						Name:      ppm.Name,
					},
				})
				continue
			}
		}
	}
}

func diffPodConditions(obj1, obj2 []corev1.PodCondition) sets.String {
	diff := sets.NewString()
	// type -> status
	older := map[corev1.PodConditionType]corev1.ConditionStatus{}
	for _, obj := range obj1 {
		older[obj.Type] = obj.Status
	}
	for _, obj := range obj2 {
		status, ok := older[obj.Type]
		if !ok || status != obj.Status {
			diff.Insert(string(obj.Type))
		}
	}
	return diff
}
