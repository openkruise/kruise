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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

type podEventHandler struct {
	client.Reader
}

var _ handler.TypedEventHandler[*v1.Pod] = &podEventHandler{}

func (e *podEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	e.handle(obj, q)
}

func (e *podEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	obj := evt.ObjectNew
	oldObj := evt.ObjectOld
	if oldObj.DeletionTimestamp == nil && obj.DeletionTimestamp != nil {
		e.handle(obj, q)
		return
	}
	if !reflect.DeepEqual(obj.Status.ContainerStatuses, oldObj.Status.ContainerStatuses) {
		e.handle(obj, q)
		return
	}
}

func (e *podEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	e.handle(obj, q)
}

func (e *podEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
}

func (e *podEventHandler) handle(pod *v1.Pod, q workqueue.RateLimitingInterface) {
	crrList := &appsv1alpha1.ContainerRecreateRequestList{}
	err := e.List(context.TODO(), crrList, client.InNamespace(pod.Namespace), client.MatchingLabels{appsv1alpha1.ContainerRecreateRequestPodUIDKey: string(pod.UID)})
	if err != nil {
		klog.ErrorS(err, "Failed to get CRR List for Pod", "pod", klog.KObj(pod))
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
