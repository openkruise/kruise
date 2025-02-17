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

var _ handler.TypedEventHandler[*v1.Pod] = &podEventHandler{}

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

func (e *podEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	logger := klog.FromContext(ctx)
	pod := evt.Object
	if pod.DeletionTimestamp != nil {
		// on a restart of the controller manager, it's possible a new pod shows up in a state that
		// is already pending deletion. Prevent the pod from being a creation observation.
		e.Delete(ctx, event.TypedDeleteEvent[*v1.Pod]{Object: evt.Object}, q)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		ds := e.resolveControllerRef(pod.Namespace, controllerRef)
		if ds == nil {
			return
		}
		klog.V(4).InfoS("Pod added", "pod", klog.KObj(pod))
		e.expectations.CreationObserved(logger, keyFunc(ds))
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
	klog.V(4).InfoS("Orphan Pod created", "pod", klog.KObj(pod), "owner", joinDaemonSetNames(dsList))
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

func (e *podEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	oldPod := evt.ObjectOld
	curPod := evt.ObjectNew
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
		e.deletePod(ctx, curPod, q, false)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		ds := e.resolveControllerRef(curPod.Namespace, curControllerRef)
		if ds == nil {
			return
		}
		klog.V(4).InfoS("Pod updated", "pod", klog.KObj(curPod), "owner", klog.KObj(ds))
		enqueueDaemonSet(q, ds)
		return
	}

	// Otherwise, it's an orphan. If anything changed, sync matching controllers
	// to see if anyone wants to adopt it now.
	dsList := e.getPodDaemonSets(curPod)
	if len(dsList) == 0 {
		return
	}
	klog.V(4).InfoS("Orphan Pod updated", "pod", klog.KObj(curPod), "owner", joinDaemonSetNames(dsList))
	labelChanged := !reflect.DeepEqual(curPod.Labels, oldPod.Labels)
	if labelChanged || controllerRefChanged {
		for _, ds := range dsList {
			enqueueDaemonSet(q, ds)
		}
	}
}

func (e *podEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	pod := evt.Object

	e.deletePod(ctx, pod, q, true)
}

func (e *podEventHandler) deletePod(ctx context.Context, pod *v1.Pod, q workqueue.RateLimitingInterface, isDeleted bool) {
	logger := klog.FromContext(ctx)
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
		e.expectations.DeletionObserved(logger, keyFunc(ds))
	}
	if isDeleted {
		e.deletionUIDCache.Delete(pod.UID)
		klog.V(4).InfoS("Pod deleted", "pod", klog.KObj(pod), "owner", klog.KObj(ds))
	} else {
		klog.V(4).InfoS("Pod terminating", "pod", klog.KObj(pod), "owner", klog.KObj(ds))
	}
	enqueueDaemonSet(q, ds)
}

func (e *podEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Pod], q workqueue.RateLimitingInterface) {

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
		klog.InfoS("Error! More than one DaemonSet is selecting pod", "pod", klog.KObj(pod), "daemonSets", joinDaemonSetNames(dsMatched))
	}
	return dsMatched
}

var _ handler.TypedEventHandler[*v1.Node] = &nodeEventHandler{}

type nodeEventHandler struct {
	reader client.Reader
}

func (e *nodeEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Node], q workqueue.RateLimitingInterface) {
	dsList := &appsv1alpha1.DaemonSetList{}
	err := e.reader.List(context.TODO(), dsList)
	if err != nil {
		klog.V(4).ErrorS(err, "Error enqueueing DaemonSets")
		return
	}

	node := evt.Object
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

func (e *nodeEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Node], q workqueue.RateLimitingInterface) {
	oldNode := evt.ObjectOld
	curNode := evt.ObjectNew
	if shouldIgnoreNodeUpdate(*oldNode, *curNode) {
		return
	}

	dsList := &appsv1alpha1.DaemonSetList{}
	err := e.reader.List(context.TODO(), dsList)
	if err != nil {
		klog.V(4).ErrorS(err, "Error listing DaemonSets")
		return
	}
	// TODO: it'd be nice to pass a hint with these enqueues, so that each ds would only examine the added node (unless it has other work to do, too).
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		oldShouldRun, oldShouldContinueRunning := nodeShouldRunDaemonPod(oldNode, ds)
		currentShouldRun, currentShouldContinueRunning := nodeShouldRunDaemonPod(curNode, ds)
		if (oldShouldRun != currentShouldRun) || (oldShouldContinueRunning != currentShouldContinueRunning) ||
			(NodeShouldUpdateBySelector(oldNode, ds) != NodeShouldUpdateBySelector(curNode, ds)) {
			klog.V(6).InfoS("Update node triggers DaemonSet to reconcile", "nodeName", curNode.Name, "daemonSet", klog.KObj(ds))
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      ds.GetName(),
				Namespace: ds.GetNamespace(),
			}})
		}
	}
}

func (e *nodeEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Node], q workqueue.RateLimitingInterface) {
}

func (e *nodeEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Node], q workqueue.RateLimitingInterface) {
}
