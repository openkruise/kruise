/*
Copyright 2021 The Kruise Authors.

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

package nodeimage

import (
	"context"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utilimagejob "github.com/openkruise/kruise/pkg/util/imagejob"
)

const (
	VirtualKubelet = "virtual-kubelet"
)

type nodeHandler struct {
	client.Reader
}

var _ handler.TypedEventHandler[*v1.Node] = &nodeHandler{}

func (e *nodeHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Node], q workqueue.RateLimitingInterface) {
	node := evt.Object
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
		return
	}
	e.nodeCreateOrUpdate(node, q)
}

func (e *nodeHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Node], q workqueue.RateLimitingInterface) {
}

func (e *nodeHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Node], q workqueue.RateLimitingInterface) {
	node := evt.ObjectNew
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
	} else {
		e.nodeCreateOrUpdate(node, q)
	}
}

func (e *nodeHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Node], q workqueue.RateLimitingInterface) {
	node := evt.Object
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	e.nodeDelete(node, q)
}

func (e *nodeHandler) nodeCreateOrUpdate(node *v1.Node, q workqueue.RateLimitingInterface) {
	nodeImage := &appsv1alpha1.NodeImage{}
	namespacedName := types.NamespacedName{Name: node.Name}
	if err := e.Get(context.TODO(), namespacedName, nodeImage); err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("Node created event for nodeimage", "nodeImageName", node.Name)
			if isReady, delay := getNodeReadyAndDelayTime(node); !isReady {
				klog.InfoS("Skipped to enqueue Node with not NodeImage, for not ready yet", "nodeImageName", node.Name)
				return
			} else if delay > 0 {
				klog.InfoS("Enqueue Node with not NodeImage after delay", "nodeImageName", node.Name, "delay", delay)
				q.AddAfter(reconcile.Request{NamespacedName: namespacedName}, delay)
				return
			}
			klog.InfoS("Enqueue Node with not NodeImage", "nodeImageName", node.Name)
			q.Add(reconcile.Request{NamespacedName: namespacedName})
			return
		}
		klog.ErrorS(err, "Failed to get NodeImage for Node", "nodeImageName", node.Name)
		return
	}
	if reflect.DeepEqual(node.Labels, nodeImage.Labels) {
		return
	}
	klog.InfoS("Node updated labels for NodeImage", "nodeImageName", node.Name)
	q.Add(reconcile.Request{NamespacedName: namespacedName})
}

func (e *nodeHandler) nodeDelete(node *v1.Node, q workqueue.RateLimitingInterface) {
	nodeImage := &appsv1alpha1.NodeImage{}
	namespacedName := types.NamespacedName{Name: node.Name}
	if err := e.Get(context.TODO(), namespacedName, nodeImage); errors.IsNotFound(err) {
		return
	}
	klog.InfoS("Node deleted event for NodeImage", "nodeImageName", node.Name)
	q.Add(reconcile.Request{NamespacedName: namespacedName})
}

var _ handler.TypedEventHandler[*appsv1alpha1.ImagePullJob] = &imagePullJobHandler{}

type imagePullJobHandler struct {
	client.Reader
}

func (e *imagePullJobHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*appsv1alpha1.ImagePullJob], q workqueue.RateLimitingInterface) {
}

func (e *imagePullJobHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*appsv1alpha1.ImagePullJob], q workqueue.RateLimitingInterface) {
}

func (e *imagePullJobHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*appsv1alpha1.ImagePullJob], q workqueue.RateLimitingInterface) {
}

func (e *imagePullJobHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*appsv1alpha1.ImagePullJob], q workqueue.RateLimitingInterface) {
	job := evt.Object
	nodeImageNames := utilimagejob.PopCachedNodeImagesForJob(job)
	for _, name := range nodeImageNames {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: name}})
	}
}
