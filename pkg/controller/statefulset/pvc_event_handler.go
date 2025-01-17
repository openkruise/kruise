/*
Copyright 2024 The Kruise Authors.

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

package statefulset

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type pvcEventHandler struct {
}

var _ handler.TypedEventHandler[*v1.PersistentVolumeClaim] = &pvcEventHandler{}

func (e *pvcEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.PersistentVolumeClaim], q workqueue.RateLimitingInterface) {
}

func (e *pvcEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.PersistentVolumeClaim], q workqueue.RateLimitingInterface) {
	newPVC := evt.ObjectNew
	if len(newPVC.Annotations) == 0 {
		return
	}
	ownedByAstsName, exist := newPVC.Annotations[PVCOwnedByStsAnnotationKey]
	if !exist {
		return
	}

	klog.InfoS("pvc update trigger asts reconcile", "pvc", klog.KObj(newPVC), "sts", ownedByAstsName)
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: newPVC.Namespace,
		Name:      ownedByAstsName,
	}})
	return
}

func (e *pvcEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.PersistentVolumeClaim], q workqueue.RateLimitingInterface) {
}

func (e *pvcEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.PersistentVolumeClaim], q workqueue.RateLimitingInterface) {
}
