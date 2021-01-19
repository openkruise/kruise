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

package imagepulljob

import (
	"reflect"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	nodeimagesutil "github.com/openkruise/kruise/pkg/util/nodeimages"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type nodeImageEventHandler struct {
	client.Reader
}

var _ handler.EventHandler = &nodeImageEventHandler{}

func (e *nodeImageEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*appsv1alpha1.NodeImage)
	e.handle(obj, q)
}

func (e *nodeImageEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.ObjectNew.(*appsv1alpha1.NodeImage)
	oldObj := evt.ObjectOld.(*appsv1alpha1.NodeImage)
	resourceVersionExpectations.Observe(obj)
	if obj.DeletionTimestamp != nil {
		e.handle(obj, q)
	} else {
		e.handleUpdate(obj, oldObj, q)
	}
}

func (e *nodeImageEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*appsv1alpha1.NodeImage)
	resourceVersionExpectations.Delete(obj)
	e.handle(obj, q)
}

func (e *nodeImageEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *nodeImageEventHandler) handle(nodeImage *appsv1alpha1.NodeImage, q workqueue.RateLimitingInterface) {
	// Get jobs related to this NodeImage
	jobs, err := nodeimagesutil.GetJobsForNodeImage(e.Reader, nodeImage)
	if err != nil {
		klog.Errorf("Failed to get jobs for NodeImage %s: %v", nodeImage.Name, err)
	}
	for _, j := range jobs {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: j.Namespace, Name: j.Name}})
	}
}

func (e *nodeImageEventHandler) handleUpdate(nodeImage, oldNodeImage *appsv1alpha1.NodeImage, q workqueue.RateLimitingInterface) {
	changedImages := sets.NewString()
	oldNodeImage = oldNodeImage.DeepCopy()
	for name, imageSpec := range nodeImage.Spec.Images {
		oldImageSpec := oldNodeImage.Spec.Images[name]
		delete(oldNodeImage.Spec.Images, name)
		if !reflect.DeepEqual(imageSpec, oldImageSpec) {
			changedImages.Insert(name)
		}
	}
	for name := range oldNodeImage.Spec.Images {
		changedImages.Insert(name)
	}
	for name, imageStatus := range nodeImage.Status.ImageStatuses {
		oldImageStatus := oldNodeImage.Status.ImageStatuses[name]
		delete(oldNodeImage.Status.ImageStatuses, name)
		if !reflect.DeepEqual(imageStatus, oldImageStatus) {
			changedImages.Insert(name)
		}
	}
	for name := range oldNodeImage.Status.ImageStatuses {
		changedImages.Insert(name)
	}
	klog.V(5).Infof("Find NodeImage %s updated and only affect images: %v", nodeImage.Name, changedImages.List())

	// Get jobs related to this NodeImage
	jobs, err := nodeimagesutil.GetJobsForNodeImage(e.Reader, nodeImage)
	if err != nil {
		klog.Errorf("Failed to get jobs for NodeImage %s: %v", nodeImage.Name, err)
	}
	for _, j := range jobs {
		var match bool
		for _, cImage := range changedImages.List() {
			if j.Spec.Image == cImage || strings.HasPrefix(j.Spec.Image, cImage+":") {
				match = true
				break
			}
		}
		if match {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: j.Namespace, Name: j.Name}})
		}
	}
}
