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

var _ handler.TypedEventHandler[*appsalphav1.NodePodProbe] = &enqueueRequestForNodePodProbe{}

type enqueueRequestForNodePodProbe struct{}

func (p *enqueueRequestForNodePodProbe) Create(ctx context.Context, evt event.TypedCreateEvent[*appsalphav1.NodePodProbe], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	p.queue(q, obj)
}

func (p *enqueueRequestForNodePodProbe) Delete(ctx context.Context, evt event.TypedDeleteEvent[*appsalphav1.NodePodProbe], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForNodePodProbe) Generic(ctx context.Context, evt event.TypedGenericEvent[*appsalphav1.NodePodProbe], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForNodePodProbe) Update(ctx context.Context, evt event.TypedUpdateEvent[*appsalphav1.NodePodProbe], q workqueue.RateLimitingInterface) {
	// must be deep copy before update the objection
	new := evt.ObjectNew
	old := evt.ObjectOld
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

var _ handler.TypedEventHandler[*corev1.Pod] = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	reader client.Reader
}

func (p *enqueueRequestForPod) Create(ctx context.Context, evt event.TypedCreateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Delete(ctx context.Context, evt event.TypedDeleteEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	// remove pod probe from nodePodProbe.spec
	if obj.Spec.NodeName == "" {
		return
	}
	npp := &appsalphav1.NodePodProbe{}
	if err := p.reader.Get(context.TODO(), client.ObjectKey{Name: obj.Spec.NodeName}, npp); err != nil {
		if !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to get NodePodProbe", "nodeName", obj.Spec.NodeName)
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

func (p *enqueueRequestForPod) Generic(ctx context.Context, evt event.TypedGenericEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(ctx context.Context, evt event.TypedUpdateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	new := evt.ObjectNew
	old := evt.ObjectOld
	// remove pod probe from nodePodProbe.spec
	if new.Spec.NodeName != "" && kubecontroller.IsPodActive(old) && !kubecontroller.IsPodActive(new) {
		npp := &appsalphav1.NodePodProbe{}
		if err := p.reader.Get(context.TODO(), client.ObjectKey{Name: new.Spec.NodeName}, npp); err != nil {
			if !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to get NodePodProbe", "nodeName", new.Spec.NodeName)
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

var _ handler.TypedEventHandler[*corev1.Node] = &enqueueRequestForNode{}

func (e *enqueueRequestForNode) Create(ctx context.Context, evt event.TypedCreateEvent[*corev1.Node], q workqueue.RateLimitingInterface) {
	node := evt.Object
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
		return
	}
	e.nodeCreate(node, q)
}

func (e *enqueueRequestForNode) Generic(ctx context.Context, evt event.TypedGenericEvent[*corev1.Node], q workqueue.RateLimitingInterface) {
}

func (e *enqueueRequestForNode) Update(ctx context.Context, evt event.TypedUpdateEvent[*corev1.Node], q workqueue.RateLimitingInterface) {
	node := evt.ObjectNew
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
	} else {
		e.nodeCreate(node, q)
	}
}

func (e *enqueueRequestForNode) Delete(ctx context.Context, evt event.TypedDeleteEvent[*corev1.Node], q workqueue.RateLimitingInterface) {
	node := evt.Object
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	e.nodeDelete(node, q)
}

func (e *enqueueRequestForNode) nodeCreate(node *corev1.Node, q workqueue.RateLimitingInterface) {
	npp := &appsalphav1.NodePodProbe{}
	if err := e.Get(context.TODO(), client.ObjectKey{Name: node.Name}, npp); err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("Node created event for nodePodProbe", "nodeName", node.Name)
			namespacedName := types.NamespacedName{Name: node.Name}
			if !isNodeReady(node) {
				klog.InfoS("Skipped to enqueue Node with not nodePodProbe, for not ready yet", "nodeName", node.Name)
				return
			}
			klog.InfoS("Enqueue Node with not nodePodProbe", "nodeName", node.Name)
			q.Add(reconcile.Request{NamespacedName: namespacedName})
			return
		}
		klog.ErrorS(err, "Failed to get nodePodProbe for Node", "nodeName", node.Name)
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
