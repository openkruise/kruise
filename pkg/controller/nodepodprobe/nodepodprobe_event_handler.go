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

package nodepodprobe

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

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
	// must be deep copy before update the objection
	new, ok := evt.ObjectNew.(*appsalphav1.NodePodProbe)
	if !ok {
		return
	}
	old, ok := evt.ObjectOld.(*appsalphav1.NodePodProbe)
	if !ok {
		return
	}
	if !reflect.DeepEqual(new.Status, old.Status) {
		p.queue(q, new)
	}
}

func (p *enqueueRequestForNodePodProbe) queue(q workqueue.RateLimitingInterface, npp *appsalphav1.NodePodProbe) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: npp.Name,
		},
	})
}

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	reader client.Reader
}

func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {}

func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(*corev1.Pod)
	if !ok {
		return
	}
	// remove pod probe from nodePodProbe.spec
	if obj.Spec.NodeName == "" {
		return
	}
	npp := &appsalphav1.NodePodProbe{}
	if err := p.reader.Get(context.TODO(), client.ObjectKey{Name: obj.Spec.NodeName}, npp); err != nil {
		if !errors.IsNotFound(err) {
			klog.Errorf("Get NodePodProbe(%s) failed: %s", obj.Spec.NodeName, err)
		}
		return
	}
	for _, probe := range npp.Spec.PodProbes {
		if probe.Namespace == obj.Namespace && probe.Name == obj.Name {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: obj.Spec.NodeName}})
			break
		}
	}
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
	if new.Spec.NodeName != "" && kubecontroller.IsPodActive(old) && !kubecontroller.IsPodActive(new) {
		npp := &appsalphav1.NodePodProbe{}
		if err := p.reader.Get(context.TODO(), client.ObjectKey{Name: new.Spec.NodeName}, npp); err != nil {
			if !errors.IsNotFound(err) {
				klog.Errorf("Get NodePodProbe(%s) failed: %s", new.Spec.NodeName, err)
			}
			return
		}
		for _, probe := range npp.Spec.PodProbes {
			if probe.Namespace == new.Namespace && probe.Name == new.Name {
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: new.Spec.NodeName}})
				break
			}
		}
	}
}

const (
	VirtualKubelet = "virtual-kubelet"
)

type enqueueRequestForNode struct {
	client.Reader
}

var _ handler.EventHandler = &enqueueRequestForNode{}

func (e *enqueueRequestForNode) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	node := evt.Object.(*corev1.Node)
	if node.Labels["type"] == VirtualKubelet {
		return
	}
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
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
	} else {
		e.nodeCreate(node, q)
	}
}

func (e *enqueueRequestForNode) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	node := evt.Object.(*corev1.Node)
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	e.nodeDelete(node, q)
}

func (e *enqueueRequestForNode) nodeCreate(node *corev1.Node, q workqueue.RateLimitingInterface) {
	npp := &appsalphav1.NodePodProbe{}
	if err := e.Get(context.TODO(), client.ObjectKey{Name: node.Name}, npp); err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Node create event for nodePodProbe %v", node.Name)
			namespacedName := types.NamespacedName{Name: node.Name}
			if !isNodeReady(node) {
				klog.Infof("Skip to enqueue Node %s with not nodePodProbe, for not ready yet.", node.Name)
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
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: node.Name}})
}

func isNodeReady(node *corev1.Node) bool {
	_, condition := nodeutil.GetNodeCondition(&node.Status, corev1.NodeReady)
	if condition == nil || condition.Status != corev1.ConditionTrue {
		return false
	}
	return true
}
