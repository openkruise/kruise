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
	"sync"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

var _ handler.EventHandler = &podEventHandler{}

type podEventHandler struct {
	client.Reader
	expectations     kubecontroller.ControllerExpectationsInterface
	deletionUIDCache sync.Map
}

func enqueueDaemonSet(q workqueue.RateLimitingInterface, ds *appsv1alpha1.DaemonSet) {
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      ds.GetName(),
		Namespace: ds.GetNamespace(),
	}})
}

func (e *podEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	pod := evt.Object.(*v1.Pod)
	if pod.DeletionTimestamp != nil {
		// on a restart of the controller manager, it's possible a new pod shows up in a state that
		// is already pending deletion. Prevent the pod from being a creation observation.
		e.Delete(event.DeleteEvent{Object: evt.Object}, q)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		ds := e.resolveControllerRef(pod.Namespace, controllerRef)
		if ds == nil {
			return
		}
		klog.V(4).Infof("Pod %s/%s added.", pod.Namespace, pod.Name)
		e.expectations.CreationObserved(keyFunc(ds))
		enqueueDaemonSet(q, ds)
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
	klog.V(4).Infof("Orphan Pod %s/%s created, matched owner: %s", pod.Namespace, pod.Name, joinDaemonSetNames(dsList))
	for _, ds := range dsList {
		enqueueDaemonSet(q, ds)
	}
}

func joinDaemonSetNames(dsList []*appsv1alpha1.DaemonSet) string {
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

	curControllerRef := metav1.GetControllerOf(curPod)
	oldControllerRef := metav1.GetControllerOf(oldPod)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		// The ControllerRef was changed. Sync the old controller, if any.
		if ds := e.resolveControllerRef(oldPod.Namespace, oldControllerRef); ds != nil {
			enqueueDaemonSet(q, ds)
		}
	}

	if curPod.DeletionTimestamp != nil {
		// when a pod is deleted gracefully its deletion timestamp is first modified to reflect a grace period,
		// and after such time has passed, the kubelet actually deletes it from the store. We receive an update
		// for modification of the deletion timestamp and expect an ds to create more replicas asap, not wait
		// until the kubelet actually deletes the pod.
		e.deletePod(curPod, q, false)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		ds := e.resolveControllerRef(curPod.Namespace, curControllerRef)
		if ds == nil {
			return
		}
		klog.V(4).Infof("Pod %s/%s updated, owner: %s", curPod.Namespace, curPod.Name, ds.Name)
		enqueueDaemonSet(q, ds)
		return
	}

	// Otherwise, it's an orphan. If anything changed, sync matching controllers
	// to see if anyone wants to adopt it now.
	dsList := e.getPodDaemonSets(curPod)
	if len(dsList) == 0 {
		return
	}
	klog.V(4).Infof("Orphan Pod %s/%s updated, matched owner: %s", curPod.Namespace, curPod.Name, joinDaemonSetNames(dsList))
	labelChanged := !reflect.DeepEqual(curPod.Labels, oldPod.Labels)
	if labelChanged || controllerRefChanged {
		for _, ds := range dsList {
			enqueueDaemonSet(q, ds)
		}
	}
}

func (e *podEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	pod, ok := evt.Object.(*v1.Pod)
	if !ok {
		klog.Errorf("DeleteEvent parse pod failed, DeleteStateUnknown: %#v, obj: %#v", evt.DeleteStateUnknown, evt.Object)
		return
	}
	e.deletePod(pod, q, true)
}

func (e *podEventHandler) deletePod(pod *v1.Pod, q workqueue.RateLimitingInterface, isDeleted bool) {
	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		// No controller should care about orphans being deleted.
		return
	}
	ds := e.resolveControllerRef(pod.Namespace, controllerRef)
	if ds == nil {
		return
	}

	if _, loaded := e.deletionUIDCache.LoadOrStore(pod.UID, struct{}{}); !loaded {
		e.expectations.DeletionObserved(keyFunc(ds))
	}
	if isDeleted {
		e.deletionUIDCache.Delete(pod.UID)
		klog.V(4).Infof("Pod %s/%s deleted, owner: %s", pod.Namespace, pod.Name, ds.Name)
	} else {
		klog.V(4).Infof("Pod %s/%s terminating, owner: %s", pod.Namespace, pod.Name, ds.Name)
	}
	enqueueDaemonSet(q, ds)
}

func (e *podEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {

}

func (e *podEventHandler) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *appsv1alpha1.DaemonSet {
	if controllerRef.Kind != controllerKind.Kind || controllerRef.APIVersion != controllerKind.GroupVersion().String() {
		return nil
	}

	ds := &appsv1alpha1.DaemonSet{}
	if err := e.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: controllerRef.Name}, ds); err != nil {
		return nil
	}
	if ds.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return ds
}

func (e *podEventHandler) getPodDaemonSets(pod *v1.Pod) []*appsv1alpha1.DaemonSet {
	dsList := appsv1alpha1.DaemonSetList{}
	if err := e.List(context.TODO(), &dsList, client.InNamespace(pod.Namespace)); err != nil {
		return nil
	}

	var dsMatched []*appsv1alpha1.DaemonSet
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		selector, err := util.ValidatedLabelSelectorAsSelector(ds.Spec.Selector)
		if err != nil || selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}

		dsMatched = append(dsMatched, ds)
	}

	if len(dsMatched) > 1 {
		// ControllerRef will ensure we don't do anything crazy, but more than one
		// item in this list nevertheless constitutes user error.
		klog.Warningf("Error! More than one DaemonSet is selecting pod %s/%s : %s", pod.Namespace, pod.Name, joinDaemonSetNames(dsMatched))
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
		klog.V(4).Infof("Error enqueueing daemon sets: %v", err)
		return
	}

	node := evt.Object.(*v1.Node)
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		if shouldSchedule, _ := nodeShouldRunDaemonPod(node, ds); shouldSchedule {
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
	if shouldIgnoreNodeUpdate(*oldNode, *curNode) {
		return
	}

	dsList := &appsv1alpha1.DaemonSetList{}
	err := e.reader.List(context.TODO(), dsList)
	if err != nil {
		klog.V(4).Infof("Error listing daemon sets: %v", err)
		return
	}
	// TODO: it'd be nice to pass a hint with these enqueues, so that each ds would only examine the added node (unless it has other work to do, too).
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		oldShouldRun, oldShouldContinueRunning := nodeShouldRunDaemonPod(oldNode, ds)
		currentShouldRun, currentShouldContinueRunning := nodeShouldRunDaemonPod(curNode, ds)
		if (oldShouldRun != currentShouldRun) || (oldShouldContinueRunning != currentShouldContinueRunning) ||
			(NodeShouldUpdateBySelector(oldNode, ds) != NodeShouldUpdateBySelector(curNode, ds)) {
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
