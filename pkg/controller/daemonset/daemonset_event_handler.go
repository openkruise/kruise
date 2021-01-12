/*
Copyright 2020 The Kruise Authors.

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

package daemonset

import (
	"context"
	"reflect"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var _ handler.EventHandler = &podEventHandler{}

type podEventHandler struct {
	client.Reader
}

func (e *podEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	pod := evt.Object.(*v1.Pod)
	if pod.DeletionTimestamp != nil {
		// on a restart of the controller manager, it's possible a new pod shows up in a state that
		// is already pending deletion. Prevent the pod from being a creation observation.
		e.Delete(event.DeleteEvent{Meta: evt.Meta, Object: evt.Object}, q)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		req := resolveControllerRef(pod.Namespace, controllerRef)
		if req == nil {
			return
		}
		klog.V(6).Infof("Pod %s/%s created, owner: %s", pod.Namespace, pod.Name, req.Name)
		dsKey := pod.Namespace + "/" + controllerRef.Name
		expectations.CreationObserved(dsKey)
		q.Add(*req)
		return
	}

	// Otherwise, it's an orphan. Get a list of all matching DaemonSets and sync
	// them to see if anyone wants to adopt it.
	// DO NOT observe creation because no controller should be waiting for an
	// orphan.
	dsList := e.getPodDaemonSets(pod)
	if len(dsList) == 0 {
		return
	}
	klog.V(6).Infof("Orphan Pod %s/%s created, matched owner: %s", pod.Namespace, pod.Name, e.joinDaemonSetNames(dsList))
	for _, ds := range dsList {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      ds.GetName(),
			Namespace: ds.GetNamespace(),
		}})
	}
}

func (e *podEventHandler) joinDaemonSetNames(dsList []appsv1alpha1.DaemonSet) string {
	var names []string
	for _, ds := range dsList {
		names = append(names, ds.Name)
	}
	return strings.Join(names, ",")
}

func (e *podEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldPod := evt.ObjectOld.(*v1.Pod)
	curPod := evt.ObjectNew.(*v1.Pod)
	if curPod.ResourceVersion == oldPod.ResourceVersion {
		// Periodic resync will send update events for all known pods.
		// Two different versions of the same pod will always have different RVs.
		return
	}

	labelChanged := !reflect.DeepEqual(curPod.Labels, oldPod.Labels)
	if curPod.DeletionTimestamp != nil {
		// when a pod is deleted gracefully it's deletion timestamp is first modified to reflect a grace period,
		// and after such time has passed, the kubelet actually deletes it from the store. We receive an update
		// for modification of the deletion timestamp and expect an rs to create more replicas asap, not wait
		// until the kubelet actually deletes the pod. This is different from the Phase of a pod changing, because
		// an rs never initiates a phase change, and so is never asleep waiting for the same.
		e.Delete(event.DeleteEvent{Meta: evt.MetaNew, Object: evt.ObjectNew}, q)
		if labelChanged {
			// we don't need to check the oldPod.DeletionTimestamp because DeletionTimestamp cannot be unset.
			e.Delete(event.DeleteEvent{Meta: evt.MetaOld, Object: evt.ObjectOld}, q)
		}
		return
	}

	curControllerRef := metav1.GetControllerOf(curPod)
	oldControllerRef := metav1.GetControllerOf(oldPod)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		// The ControllerRef was changed. Sync the old controller, if any.
		if req := resolveControllerRef(oldPod.Namespace, oldControllerRef); req != nil {
			q.Add(*req)
		}
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		req := resolveControllerRef(curPod.Namespace, curControllerRef)
		if req == nil {
			return
		}
		klog.V(6).Infof("Pod %s/%s updated, owner: %s", curPod.Namespace, curPod.Name, req.Name)
		q.Add(*req)
		return
	}

	// Otherwise, it's an orphan. If anything changed, sync matching controllers
	// to see if anyone wants to adopt it now.
	if labelChanged || controllerRefChanged {
		dsList := e.getPodDaemonSets(curPod)
		if len(dsList) == 0 {
			return
		}
		klog.V(6).Infof("Orphan Pod %s/%s updated, matched owner: %s",
			curPod.Namespace, curPod.Name, e.joinDaemonSetNames(dsList))
		for _, ds := range dsList {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      ds.GetName(),
				Namespace: ds.GetNamespace(),
			}})
		}
	}
}

func (e *podEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	pod, ok := evt.Object.(*v1.Pod)
	if !ok {
		klog.Errorf("DeleteEvent parse pod failed, DeleteStateUnknown: %#v, obj: %#v", evt.DeleteStateUnknown, evt.Object)
		return
	}

	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		// No controller should care about orphans being deleted.
		return
	}
	req := resolveControllerRef(pod.Namespace, controllerRef)
	if req == nil {
		return
	}

	klog.V(6).Infof("Pod %s/%s deleted, owner: %s", pod.Namespace, pod.Name, req.Name)
	dsKey := pod.Namespace + "/" + controllerRef.Name
	expectations.DeletionObserved(dsKey)
	q.Add(*req)
}

func (e *podEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {

}

func resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *reconcile.Request {
	// Parse the Group out of the OwnerReference to compare it to what was parsed out of the requested OwnerType
	refGV, err := schema.ParseGroupVersion(controllerRef.APIVersion)
	if err != nil {
		klog.Errorf("Could not parse OwnerReference %v APIVersion: %v", controllerRef, err)
		return nil
	}

	// Compare the OwnerReference Group and Kind against the OwnerType Group and Kind specified by the user.
	// If the two match, create a Request for the objected referred to by
	// the OwnerReference.  Use the Name from the OwnerReference and the Namespace from the
	// object in the event.
	if controllerRef.Kind == controllerKind.Kind && refGV.Group == controllerKind.Group {
		// Match found - add a Request for the object referred to in the OwnerReference
		req := reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      controllerRef.Name,
		}}
		return &req
	}
	return nil
}

func (e *podEventHandler) getPodDaemonSets(pod *v1.Pod) []appsv1alpha1.DaemonSet {
	dsList := appsv1alpha1.DaemonSetList{}
	if err := e.List(context.TODO(), &dsList, client.InNamespace(pod.Namespace)); err != nil {
		return nil
	}

	var dsMatched []appsv1alpha1.DaemonSet
	for _, ds := range dsList.Items {
		selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
		if err != nil || selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}

		dsMatched = append(dsMatched, ds)
	}

	if len(dsMatched) > 1 {
		// ControllerRef will ensure we don't do anything crazy, but more than one
		// item in this list nevertheless constitutes user error.
		klog.Warningf("Error! More than one DaemonSet is selecting pod %s/%s : %s",
			pod.Namespace, pod.Name, e.joinDaemonSetNames(dsMatched))
	}
	return dsMatched
}

var _ handler.EventHandler = &nodeEventHandler{}

type nodeEventHandler struct {
	reader client.Reader
}

func (e *nodeEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	dsList := &appsv1alpha1.DaemonSetList{}
	err := e.reader.List(context.TODO(), dsList)
	if err != nil {
		klog.V(6).Infof("Error enqueueing daemon sets: %v", err)
		return
	}

	node := evt.Object.(*v1.Node)
	klog.V(6).Infof("add new node: %v", node.Name)
	for index, ds := range dsList.Items {
		_, shouldSchedule, _, err := NodeShouldRunDaemonPod(e.reader, node, &dsList.Items[index])
		if err != nil {
			continue
		}
		if shouldSchedule {
			klog.V(6).Infof("new node: %s triggers DaemonSet %s/%s to reconcile.", node.Name, ds.GetNamespace(), ds.GetName())
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      ds.GetName(),
				Namespace: ds.GetNamespace(),
			}})
		}
	}
}

func (e *nodeEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldNode := evt.ObjectOld.(*v1.Node)
	curNode := evt.ObjectNew.(*v1.Node)
	klog.V(6).Infof("update node: %v", curNode.Name)
	if ShouldIgnoreNodeUpdate(*oldNode, *curNode) {
		return
	}

	dsList := &appsv1alpha1.DaemonSetList{}
	err := e.reader.List(context.TODO(), dsList)
	if err != nil {
		klog.V(6).Infof("Error enqueueing daemon sets: %v", err)
		return
	}
	// TODO: it'd be nice to pass a hint with these enqueues, so that each ds would only examine the added node (unless it has other work to do, too).
	for index, ds := range dsList.Items {
		_, oldShouldSchedule, oldShouldContinueRunning, err := NodeShouldRunDaemonPod(e.reader, oldNode, &dsList.Items[index])
		if err != nil {
			continue
		}
		_, currentShouldSchedule, currentShouldContinueRunning, err := NodeShouldRunDaemonPod(e.reader, curNode, &dsList.Items[index])
		if err != nil {
			continue
		}
		if (CanNodeBeDeployed(oldNode, &ds) != CanNodeBeDeployed(curNode, &ds)) || (oldShouldSchedule != currentShouldSchedule) || (oldShouldContinueRunning != currentShouldContinueRunning) ||
			(NodeShouldUpdateBySelector(oldNode, &ds) != NodeShouldUpdateBySelector(curNode, &ds)) {
			klog.V(6).Infof("update node: %s triggers DaemonSet %s/%s to reconcile.", curNode.Name, ds.GetNamespace(), ds.GetName())
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      ds.GetName(),
				Namespace: ds.GetNamespace(),
			}})
		}
	}
}

func (e *nodeEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (e *nodeEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}
