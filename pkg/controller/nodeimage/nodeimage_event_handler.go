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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utilimagejob "github.com/openkruise/kruise/pkg/util/imagejob"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	VirtualKubelet = "virtual-kubelet"
)

type nodeHandler struct {
	client.Reader
}

var _ handler.EventHandler = &nodeHandler{}

func (e *nodeHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	node := evt.Object.(*v1.Node)
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
		return
	}
	e.nodeCreateOrUpdate(node, q)
}

func (e *nodeHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *nodeHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	node := evt.ObjectNew.(*v1.Node)
	if node.Labels["type"] == VirtualKubelet {
		return
	}
	if node.DeletionTimestamp != nil {
		e.nodeDelete(node, q)
	} else {
		e.nodeCreateOrUpdate(node, q)
	}
}

func (e *nodeHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	node := evt.Object.(*v1.Node)
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
			klog.Infof("Node create event for nodeimage %v", node.Name)
			if isReady, delay := getNodeReadyAndDelayTime(node); !isReady {
				klog.Infof("Skip to enqueue Node %s with not NodeImage, for not ready yet.", node.Name)
				return
			} else if delay > 0 {
				klog.Infof("Enqueue Node %s with not NodeImage after %v.", node.Name, delay)
				q.AddAfter(reconcile.Request{NamespacedName: namespacedName}, delay)
				return
			}
			klog.Infof("Enqueue Node %s with not NodeImage.", node.Name)
			q.Add(reconcile.Request{NamespacedName: namespacedName})
			return
		}
		klog.Errorf("Failed to get NodeImage for Node %s: %v", node.Name, err)
		return
	}
	if reflect.DeepEqual(node.Labels, nodeImage.Labels) {
		return
	}
	klog.Infof("Node update labels for nodeimage %v", node.Name)
	q.Add(reconcile.Request{NamespacedName: namespacedName})
}

func (e *nodeHandler) nodeDelete(node *v1.Node, q workqueue.RateLimitingInterface) {
	nodeImage := &appsv1alpha1.NodeImage{}
	namespacedName := types.NamespacedName{Name: node.Name}
	if err := e.Get(context.TODO(), namespacedName, nodeImage); errors.IsNotFound(err) {
		return
	}
	klog.Infof("Node delete event for nodeimage %v", node.Name)
	q.Add(reconcile.Request{NamespacedName: namespacedName})
}

type imagePullJobHandler struct {
	client.Reader
}

func (e *imagePullJobHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
}

func (e *imagePullJobHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *imagePullJobHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
}

func (e *imagePullJobHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	job := evt.Object.(*appsv1alpha1.ImagePullJob)
	nodeImageNames := utilimagejob.PopCachedNodeImagesForJob(job)
	for _, name := range nodeImageNames {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: name}})
	}
}
