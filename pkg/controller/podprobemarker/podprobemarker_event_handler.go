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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
)

var _ handler.EventHandler = &enqueueRequestForPodProbeMarker{}

type enqueueRequestForPodProbeMarker struct{}

func (p *enqueueRequestForPodProbeMarker) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.queue(q, evt.Object)
}

func (p *enqueueRequestForPodProbeMarker) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPodProbeMarker) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPodProbeMarker) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
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

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	reader client.Reader
}

func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {}

func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	new, ok := evt.ObjectNew.(*corev1.Pod)
	if !ok {
		return
	}
	old, ok := evt.ObjectOld.(*corev1.Pod)
	if !ok {
		return
	}
	// add pod probe to nodePodProbe.spec
	oldInitialCondition := util.GetCondition(old, corev1.PodInitialized)
	newInitialCondition := util.GetCondition(new, corev1.PodInitialized)
	if newInitialCondition == nil {
		return
	}
	if !kubecontroller.IsPodActive(new) {
		return
	}
	if ((oldInitialCondition == nil || oldInitialCondition.Status == corev1.ConditionFalse) &&
		newInitialCondition.Status == corev1.ConditionTrue) || old.Status.PodIP != new.Status.PodIP {
		ppms, err := p.getPodProbeMarkerForPod(new)
		if err != nil {
			klog.Errorf("List PodProbeMarker fail: %s", err.Error())
			return
		}
		for _, ppm := range ppms {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ppm.Namespace,
					Name:      ppm.Name,
				},
			})
		}
	}
}

func (p *enqueueRequestForPod) getPodProbeMarkerForPod(pod *corev1.Pod) ([]*appsalphav1.PodProbeMarker, error) {
	ppmList := &appsalphav1.PodProbeMarkerList{}
	if err := p.reader.List(context.TODO(), ppmList, &client.ListOptions{Namespace: pod.Namespace}, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	var ppms []*appsalphav1.PodProbeMarker
	for i := range ppmList.Items {
		ppm := &ppmList.Items[i]
		// This error is irreversible, so continue
		labelSelector, err := util.ValidatedLabelSelectorAsSelector(ppm.Spec.Selector)
		if err != nil {
			continue
		}
		// If a PUB with a nil or empty selector creeps in, it should match nothing, not everything.
		if labelSelector.Empty() || !labelSelector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		ppms = append(ppms, ppm)
	}
	return ppms, nil
}
