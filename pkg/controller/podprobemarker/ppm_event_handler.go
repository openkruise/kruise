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
	"fmt"
	"reflect"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
			Name:      fmt.Sprintf("%s#%s", ReconPodProbeMarker, ppm.Name),
		},
	})
}

var _ handler.EventHandler = &enqueueRequestForNodePodProbe{}

type enqueueRequestForNodePodProbe struct{}

func (p *enqueueRequestForNodePodProbe) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(*appsalphav1.NodePodProbe)
	if !ok {
		return
	}
	p.queue(q, obj)
}

func (p *enqueueRequestForNodePodProbe) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForNodePodProbe) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForNodePodProbe) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	new, ok := evt.ObjectNew.(*appsalphav1.NodePodProbe)
	if !ok {
		return
	}
	old, ok := evt.ObjectOld.(*appsalphav1.NodePodProbe)
	if !ok {
		return
	}
	if reflect.DeepEqual(new.Status, old.Status) {
		return
	}
	p.queue(q, new)
}

func (p *enqueueRequestForNodePodProbe) queue(q workqueue.RateLimitingInterface, npp *appsalphav1.NodePodProbe) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: npp.Namespace,
			Name:      fmt.Sprintf("%s#%s", ReconNodePodProbe, npp.Name),
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
	// remove pod probe from nodePodProbe.spec
	if old.DeletionTimestamp.IsZero() && !new.DeletionTimestamp.IsZero() {
		if new.Spec.NodeName == "" {
			return
		}
		npp := &appsalphav1.NodePodProbe{}
		if err := p.reader.Get(context.TODO(), client.ObjectKey{Name: new.Spec.NodeName}, npp); err != nil {
			return
		}
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: fmt.Sprintf("%s#%s", ReconNodePodProbe, new.Spec.NodeName)}})
		return
	}

	// add pod probe to nodePodProbe.spec
	oldInitialCondition := util.GetCondition(old, corev1.PodInitialized)
	newInitialCondition := util.GetCondition(new, corev1.PodInitialized)
	if newInitialCondition == nil {
		return
	}
	if (oldInitialCondition == nil || oldInitialCondition.Status == corev1.ConditionFalse) && newInitialCondition.Status == corev1.ConditionTrue {
		ppms, err := p.getPodProbeMarkerForPod(new)
		if err != nil {
			klog.Errorf("List PodProbeMarker fialed: %s", err.Error())
			return
		}
		for _, ppm := range ppms {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ppm.Namespace,
					Name:      fmt.Sprintf("%s#%s", ReconPodProbeMarker, ppm.Name),
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

type enqueueRequestForNode struct {
	client.Reader
}

var _ handler.EventHandler = &enqueueRequestForNode{}

func (e *enqueueRequestForNode) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	node := evt.Object.(*corev1.Node)
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
		return
	}
	e.nodeCreate(node, q)
}

func (e *enqueueRequestForNode) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *enqueueRequestForNode) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	node := evt.ObjectNew.(*corev1.Node)
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
	}
}

func (e *enqueueRequestForNode) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	node := evt.Object.(*corev1.Node)
	e.nodeDelete(node, q)
}

func (e *enqueueRequestForNode) nodeCreate(node *corev1.Node, q workqueue.RateLimitingInterface) {
	npp := &appsalphav1.NodePodProbe{}
	if err := e.Get(context.TODO(), client.ObjectKey{Name: node.Name}, npp); err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Node create event for nodePodProbe %v", node.Name)
			namespacedName := types.NamespacedName{Name: fmt.Sprintf("%s#%s", ReconNodePodProbe, node.Name)}
			if isReady, delay := getNodeReadyAndDelayTime(node); !isReady {
				klog.Infof("Skip to enqueue Node %s with not nodePodProbe, for not ready yet.", node.Name)
				return
			} else if delay > 0 {
				klog.Infof("Enqueue Node %s with not nodePodProbe after %v.", node.Name, delay)
				q.AddAfter(reconcile.Request{NamespacedName: namespacedName}, delay)
				return
			}
			klog.Infof("Enqueue Node %s with not nodePodProbe.", node.Name)
			q.Add(reconcile.Request{NamespacedName: namespacedName})
			return
		}
		klog.Errorf("Failed to get nodePodProbe for Node %s: %v", node.Name, err)
	}
}

func (e *enqueueRequestForNode) nodeDelete(node *corev1.Node, q workqueue.RateLimitingInterface) {
	nodePodProbe := &appsalphav1.NodePodProbe{}
	if err := e.Get(context.TODO(), client.ObjectKey{Name: node.Name}, nodePodProbe); errors.IsNotFound(err) {
		return
	}
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: fmt.Sprintf("%s#%s", ReconNodePodProbe, node.Name)}})
}
