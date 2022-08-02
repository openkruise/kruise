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

package containerrecreaterequest

import (
	"context"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type podEventHandler struct {
	client.Reader
}

var _ handler.EventHandler = &podEventHandler{}

func (e *podEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*v1.Pod)
	e.handle(obj, q)
}

func (e *podEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.ObjectNew.(*v1.Pod)
	oldObj := evt.ObjectOld.(*v1.Pod)
	if oldObj.DeletionTimestamp == nil && obj.DeletionTimestamp != nil {
		e.handle(obj, q)
		return
	}
	if !reflect.DeepEqual(obj.Status.ContainerStatuses, oldObj.Status.ContainerStatuses) {
		e.handle(obj, q)
		return
	}
}

func (e *podEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*v1.Pod)
	e.handle(obj, q)
}

func (e *podEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *podEventHandler) handle(pod *v1.Pod, q workqueue.RateLimitingInterface) {
	crrList := &appsv1alpha1.ContainerRecreateRequestList{}
	err := e.List(context.TODO(), crrList, client.InNamespace(pod.Namespace), client.MatchingLabels{appsv1alpha1.ContainerRecreateRequestPodUIDKey: string(pod.UID)})
	if err != nil {
		klog.Errorf("Failed to get CRR List for Pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return
	}
	for i := range crrList.Items {
		crr := &crrList.Items[i]
		if crr.DeletionTimestamp != nil || crr.Status.CompletionTime != nil {
			continue
		}
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: crr.Namespace,
			Name:      crr.Name,
		}})
	}
}
