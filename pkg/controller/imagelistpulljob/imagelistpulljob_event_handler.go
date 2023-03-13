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

package imagelistpulljob

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/expectations"
)

type imagePullJobEventHandler struct {
	enqueueHandler handler.EnqueueRequestForOwner
}

func isImageListPullJobController(controllerRef *metav1.OwnerReference) bool {
	refGV, err := schema.ParseGroupVersion(controllerRef.APIVersion)
	if err != nil {
		klog.Errorf("Could not parse OwnerReference %v APIVersion: %v", controllerRef, err)
		return false
	}
	return controllerRef.Kind == controllerKind.Kind && refGV.Group == controllerKind.Group
}

func (p *imagePullJobEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	job := evt.Object.(*appsv1alpha1.ImagePullJob)
	if job.DeletionTimestamp != nil {
		p.Delete(event.DeleteEvent{Object: evt.Object}, q)
		return
	}
	if controllerRef := metav1.GetControllerOf(job); controllerRef != nil && isImageListPullJobController(controllerRef) {
		key := types.NamespacedName{Namespace: job.Namespace, Name: controllerRef.Name}.String()
		scaleExpectations.ObserveScale(key, expectations.Create, job.Spec.Image)
		p.enqueueHandler.Create(evt, q)
	}
}

func (p *imagePullJobEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	job := evt.Object.(*appsv1alpha1.ImagePullJob)
	if controllerRef := metav1.GetControllerOf(job); controllerRef != nil && isImageListPullJobController(controllerRef) {
		key := types.NamespacedName{Namespace: job.Namespace, Name: controllerRef.Name}.String()
		scaleExpectations.ObserveScale(key, expectations.Delete, job.Spec.Image)
	}
	p.enqueueHandler.Delete(evt, q)
}

func (p *imagePullJobEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newJob := evt.ObjectNew.(*appsv1alpha1.ImagePullJob)
	resourceVersionExpectations.Expect(newJob)
	p.enqueueHandler.Update(evt, q)
}

func (p *imagePullJobEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

var _ inject.Mapper = &imagePullJobEventHandler{}

func (p *imagePullJobEventHandler) InjectScheme(s *runtime.Scheme) error {
	return p.enqueueHandler.InjectScheme(s)
}

var _ inject.Mapper = &imagePullJobEventHandler{}

func (p *imagePullJobEventHandler) InjectMapper(m meta.RESTMapper) error {
	return p.enqueueHandler.InjectMapper(m)
}
