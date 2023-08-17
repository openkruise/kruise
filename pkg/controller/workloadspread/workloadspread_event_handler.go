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

package workloadspread

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/configuration"
	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
)

type EventAction string

const (
	CreateEventAction EventAction = "Create"
	UpdateEventAction EventAction = "Update"
	DeleteEventAction EventAction = "Delete"
)

var _ handler.EventHandler = &podEventHandler{}

type podEventHandler struct{}

func (p *podEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.handlePod(q, evt.Object, CreateEventAction)
}

func (p *podEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldPod := evt.ObjectOld.(*corev1.Pod)
	newPod := evt.ObjectNew.(*corev1.Pod)

	if kubecontroller.IsPodActive(oldPod) && !kubecontroller.IsPodActive(newPod) {
		p.handlePod(q, newPod, UpdateEventAction)
	}
}

func (p *podEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	p.handlePod(q, evt.Object, DeleteEventAction)
}

func (p *podEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {}

func (p *podEventHandler) handlePod(q workqueue.RateLimitingInterface, obj runtime.Object, action EventAction) {
	pod := obj.(*corev1.Pod)
	if value, exist := pod.GetAnnotations()[wsutil.MatchedWorkloadSpreadSubsetAnnotations]; exist {
		injectWorkloadSpread := &wsutil.InjectWorkloadSpread{}
		if err := json.Unmarshal([]byte(value), injectWorkloadSpread); err != nil {
			klog.Errorf("Failed to unmarshal %s to WorkloadSpread", value)
			return
		}
		nsn := types.NamespacedName{Namespace: pod.GetNamespace(), Name: injectWorkloadSpread.Name}
		klog.V(5).Infof("%s Pod (%s/%s) and reconcile WorkloadSpread (%s/%s)",
			action, pod.Namespace, pod.Name, nsn.Namespace, nsn.Name)
		q.Add(reconcile.Request{NamespacedName: nsn})
	}
}

var _ handler.EventHandler = &workloadEventHandler{}

type workloadEventHandler struct {
	client.Reader
}

func (w workloadEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	w.handleWorkload(q, evt.Object, CreateEventAction)
}

func (w workloadEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	var gvk schema.GroupVersionKind
	var oldReplicas int32
	var newReplicas int32

	switch evt.ObjectNew.(type) {
	case *appsv1alpha1.CloneSet:
		oldReplicas = *evt.ObjectOld.(*appsv1alpha1.CloneSet).Spec.Replicas
		newReplicas = *evt.ObjectNew.(*appsv1alpha1.CloneSet).Spec.Replicas
		gvk = controllerKruiseKindCS
	case *appsv1.Deployment:
		oldReplicas = *evt.ObjectOld.(*appsv1.Deployment).Spec.Replicas
		newReplicas = *evt.ObjectNew.(*appsv1.Deployment).Spec.Replicas
		gvk = controllerKindDep
	case *appsv1.ReplicaSet:
		oldReplicas = *evt.ObjectOld.(*appsv1.ReplicaSet).Spec.Replicas
		newReplicas = *evt.ObjectNew.(*appsv1.ReplicaSet).Spec.Replicas
		gvk = controllerKindRS
	case *batchv1.Job:
		oldReplicas = *evt.ObjectOld.(*batchv1.Job).Spec.Parallelism
		newReplicas = *evt.ObjectNew.(*batchv1.Job).Spec.Parallelism
		gvk = controllerKindJob
	case *appsv1.StatefulSet:
		oldReplicas = *evt.ObjectOld.(*appsv1.StatefulSet).Spec.Replicas
		newReplicas = *evt.ObjectNew.(*appsv1.StatefulSet).Spec.Replicas
		gvk = controllerKindSts
	case *appsv1beta1.StatefulSet:
		oldReplicas = *evt.ObjectOld.(*appsv1beta1.StatefulSet).Spec.Replicas
		newReplicas = *evt.ObjectNew.(*appsv1beta1.StatefulSet).Spec.Replicas
		gvk = controllerKruiseKindSts
	case *unstructured.Unstructured:
		oldReplicas = w.getReplicasFromUnstructured(evt.ObjectOld.(*unstructured.Unstructured))
		newReplicas = w.getReplicasFromUnstructured(evt.ObjectNew.(*unstructured.Unstructured))
		gvk = evt.ObjectNew.(*unstructured.Unstructured).GroupVersionKind()
	default:
		return
	}

	// workload replicas changed, and reconcile corresponding WorkloadSpread
	if oldReplicas != newReplicas {
		workloadNsn := types.NamespacedName{
			Namespace: evt.ObjectNew.GetNamespace(),
			Name:      evt.ObjectNew.GetName(),
		}
		ws, err := w.getWorkloadSpreadForWorkload(workloadNsn, gvk)
		if err != nil {
			klog.Errorf("unable to get WorkloadSpread related with %s (%s/%s), err: %v",
				gvk.Kind, workloadNsn.Namespace, workloadNsn.Name, err)
			return
		}
		if ws != nil {
			klog.V(3).Infof("%s (%s/%s) changed replicas from %d to %d managed by WorkloadSpread (%s/%s)",
				gvk.Kind, workloadNsn.Namespace, workloadNsn.Name, oldReplicas, newReplicas, ws.GetNamespace(), ws.GetName())
			nsn := types.NamespacedName{Namespace: ws.GetNamespace(), Name: ws.GetName()}
			q.Add(reconcile.Request{NamespacedName: nsn})
		}
	}
}

func (w *workloadEventHandler) getReplicasFromUnstructured(object *unstructured.Unstructured) int32 {
	if object == nil || reflect.ValueOf(object).IsNil() {
		return 0
	}
	whiteList, err := configuration.GetWSWatchCustomWorkloadWhiteList(w.Reader)
	if err != nil {
		klog.Errorf("Failed to get workloadSpread custom workload white list from kruise config map")
		return 0
	}

	gvk := object.GroupVersionKind()
	for _, workload := range whiteList.Workloads {
		if workload.GroupVersionKind.GroupKind() != gvk.GroupKind() {
			continue
		}
		var exists bool
		var replicas int64
		path := strings.Split(workload.ReplicasPath, ".")
		if len(path) > 0 {
			replicas, exists, err = unstructured.NestedInt64(object.Object, path...)
			if err != nil || !exists {
				klog.Errorf("Failed to get replicas from %v, replicas path %s", gvk, workload.ReplicasPath)
			}
		} else {
			replicas, exists, err = unstructured.NestedInt64(object.Object, "spec", "replicas")
			if err != nil || !exists {
				klog.Errorf("Failed to get replicas from %v, replicas path %s", gvk, workload.ReplicasPath)
			}
		}
		return int32(replicas)
	}
	return 0
}

func (w workloadEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	w.handleWorkload(q, evt.Object, DeleteEventAction)
}

func (w workloadEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (w *workloadEventHandler) handleWorkload(q workqueue.RateLimitingInterface,
	obj client.Object, action EventAction) {
	var gvk schema.GroupVersionKind
	switch obj.(type) {
	case *appsv1alpha1.CloneSet:
		gvk = controllerKruiseKindCS
	case *appsv1.Deployment:
		gvk = controllerKindDep
	case *appsv1.ReplicaSet:
		gvk = controllerKindRS
	case *batchv1.Job:
		gvk = controllerKindJob
	case *appsv1.StatefulSet:
		gvk = controllerKindSts
	case *appsv1beta1.StatefulSet:
		gvk = controllerKruiseKindSts
	default:
		return
	}

	workloadNsn := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	ws, err := w.getWorkloadSpreadForWorkload(workloadNsn, gvk)
	if err != nil {
		klog.Errorf("unable to get WorkloadSpread related with %s (%s/%s), err: %v",
			gvk.Kind, workloadNsn.Namespace, workloadNsn.Name, err)
		return
	}
	if ws != nil {
		klog.V(5).Infof("%s %s (%s/%s) and reconcile WorkloadSpread (%s/%s)",
			action, gvk.Kind, workloadNsn.Namespace, workloadNsn.Namespace, ws.Namespace, ws.Name)
		nsn := types.NamespacedName{Namespace: ws.GetNamespace(), Name: ws.GetName()}
		q.Add(reconcile.Request{NamespacedName: nsn})
	}
}

func (w *workloadEventHandler) getWorkloadSpreadForWorkload(
	workloadNamespaceName types.NamespacedName,
	gvk schema.GroupVersionKind) (*appsv1alpha1.WorkloadSpread, error) {
	wsList := &appsv1alpha1.WorkloadSpreadList{}
	listOptions := &client.ListOptions{Namespace: workloadNamespaceName.Namespace}
	if err := w.List(context.TODO(), wsList, listOptions); err != nil {
		klog.Errorf("List WorkloadSpread failed: %s", err.Error())
		return nil, err
	}

	for _, ws := range wsList.Items {
		if ws.DeletionTimestamp != nil {
			continue
		}

		targetRef := ws.Spec.TargetReference
		if targetRef == nil {
			continue
		}

		targetGV, err := schema.ParseGroupVersion(targetRef.APIVersion)
		if err != nil {
			klog.Errorf("failed to parse targetRef's group version: %s", targetRef.APIVersion)
			continue
		}

		if targetRef.Kind == gvk.Kind && targetGV.Group == gvk.Group && targetRef.Name == workloadNamespaceName.Name {
			return &ws, nil
		}
	}

	return nil, nil
}
