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
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/expectations"
)

var _ handler.TypedEventHandler[*appsv1alpha1.ImagePullJob] = &imagePullJobEventHandler{}

type imagePullJobEventHandler struct {
	enqueueHandler handler.TypedEventHandler[*appsv1alpha1.ImagePullJob]
}

func isImageListPullJobController(controllerRef *metav1.OwnerReference) bool {
	refGV, err := schema.ParseGroupVersion(controllerRef.APIVersion)
	if err != nil {
		klog.ErrorS(err, "Could not parse APIVersion in OwnerReference", "ownerReference", controllerRef)
		return false
	}
	return controllerRef.Kind == controllerKind.Kind && refGV.Group == controllerKind.Group
}

func (p *imagePullJobEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*appsv1alpha1.ImagePullJob], q workqueue.RateLimitingInterface) {
	job := evt.Object
	if job.DeletionTimestamp != nil {
		p.Delete(ctx, event.TypedDeleteEvent[*appsv1alpha1.ImagePullJob]{Object: evt.Object}, q)
		return
	}
	if controllerRef := metav1.GetControllerOf(job); controllerRef != nil && isImageListPullJobController(controllerRef) {
		key := types.NamespacedName{Namespace: job.Namespace, Name: controllerRef.Name}.String()
		scaleExpectations.ObserveScale(key, expectations.Create, job.Spec.Image)
		p.enqueueHandler.Create(ctx, evt, q)
	}
}

func (p *imagePullJobEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*appsv1alpha1.ImagePullJob], q workqueue.RateLimitingInterface) {
	job := evt.Object
	if controllerRef := metav1.GetControllerOf(job); controllerRef != nil && isImageListPullJobController(controllerRef) {
		key := types.NamespacedName{Namespace: job.Namespace, Name: controllerRef.Name}.String()
		scaleExpectations.ObserveScale(key, expectations.Delete, job.Spec.Image)
	}
	p.enqueueHandler.Delete(ctx, evt, q)
}

func (p *imagePullJobEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*appsv1alpha1.ImagePullJob], q workqueue.RateLimitingInterface) {
	newJob := evt.ObjectNew
	resourceVersionExpectations.Expect(newJob)
	p.enqueueHandler.Update(ctx, evt, q)
}

func (p *imagePullJobEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*appsv1alpha1.ImagePullJob], q workqueue.RateLimitingInterface) {
}
