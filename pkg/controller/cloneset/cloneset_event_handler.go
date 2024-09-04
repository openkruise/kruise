/*
Copyright 2019 The Kruise Authors.

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

package cloneset

import (
	"context"
	"reflect"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

var (
	// initialingRateLimiter calculates the delay duration for existing Pods
	// triggered Create event when the Informer cache has just synced.
	initialingRateLimiter = workqueue.NewItemExponentialFailureRateLimiter(3*time.Second, 30*time.Second)
)

type podEventHandler struct {
	client.Reader
}

var _ handler.EventHandler = &podEventHandler{}

func (e *podEventHandler) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	pod := evt.Object.(*v1.Pod)
	if pod.DeletionTimestamp != nil {
		// on a restart of the controller manager, it's possible a new pod shows up in a state that
		// is already pending deletion. Prevent the pod from being a creation observation.
		e.Delete(ctx, event.DeleteEvent{Object: evt.Object}, q)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		req := resolveControllerRef(pod.Namespace, controllerRef)
		if req == nil {
			return
		}
		klog.V(4).InfoS("Pod created", "pod", klog.KObj(pod), "owner", req)

		isSatisfied, _, _ := clonesetutils.ScaleExpectations.SatisfiedExpectations(req.String())
		clonesetutils.ScaleExpectations.ObserveScale(req.String(), expectations.Create, pod.Name)
		if isSatisfied {
			// If the scale expectation is satisfied, it should be an existing Pod and the Informer
			// cache should have just synced.
			q.AddAfter(*req, initialingRateLimiter.When(req))
		} else {
			// Otherwise, add it immediately and reset the rate limiter
			initialingRateLimiter.Forget(req)
			q.Add(*req)
		}
		return
	}

	// Otherwise, it's an orphan. Get a list of all matching CloneSets and sync
	// them to see if anyone wants to adopt it.
	// DO NOT observe creation because no controller should be waiting for an
	// orphan.
	csList := e.getPodCloneSets(pod)
	if len(csList) == 0 {
		return
	}
	klog.V(4).InfoS("Orphan Pod created", "pod", klog.KObj(pod), "owner", e.joinCloneSetNames(csList))
	for _, cs := range csList {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      cs.GetName(),
			Namespace: cs.GetNamespace(),
		}})
	}
}

func (e *podEventHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
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
		e.Delete(ctx, event.DeleteEvent{Object: evt.ObjectNew}, q)
		if labelChanged {
			// we don't need to check the oldPod.DeletionTimestamp because DeletionTimestamp cannot be unset.
			e.Delete(ctx, event.DeleteEvent{Object: evt.ObjectOld}, q)
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
		// TODO(Abner-1): delete it when fixes only resize resource
		//old, _ := json.Marshal(oldPod)
		//cur, _ := json.Marshal(curPod)
		//patches, _ := jsonpatch.CreatePatch(old, cur)
		//pjson, _ := json.Marshal(patches)
		//klog.V(4).InfoS("Pod updated json", "pod", klog.KObj(curPod), "patch", pjson)

		req := resolveControllerRef(curPod.Namespace, curControllerRef)
		if req == nil {
			return
		}

		if utilfeature.DefaultFeatureGate.Enabled(features.CloneSetEventHandlerOptimization) {
			if !controllerRefChanged && !labelChanged && e.shouldIgnoreUpdate(req, oldPod, curPod) {
				return
			}
		}

		klog.V(4).InfoS("Pod updated", "pod", klog.KObj(curPod), "owner", req)
		q.Add(*req)
		return
	}

	// Otherwise, it's an orphan. If anything changed, sync matching controllers
	// to see if anyone wants to adopt it now.
	if labelChanged || controllerRefChanged {
		csList := e.getPodCloneSets(curPod)
		if len(csList) == 0 {
			return
		}
		klog.V(4).InfoS("Orphan Pod updated", "pod", klog.KObj(curPod), "owner", e.joinCloneSetNames(csList))
		for _, cs := range csList {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      cs.GetName(),
				Namespace: cs.GetNamespace(),
			}})
		}
	}
}

func (e *podEventHandler) shouldIgnoreUpdate(req *reconcile.Request, oldPod, curPod *v1.Pod) bool {
	cs := &appsv1alpha1.CloneSet{}
	if err := e.Get(context.TODO(), req.NamespacedName, cs); err != nil {
		return false
	}

	return clonesetcore.New(cs).IgnorePodUpdateEvent(oldPod, curPod)
}

func (e *podEventHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	pod, ok := evt.Object.(*v1.Pod)
	if !ok {
		klog.ErrorS(nil, "Skipped pod deletion event", "deleteStateUnknown", evt.DeleteStateUnknown, "obj", evt.Object)
		return
	}
	clonesetutils.ResourceVersionExpectations.Delete(pod)

	controllerRef := metav1.GetControllerOf(pod)
	if controllerRef == nil {
		// No controller should care about orphans being deleted.
		return
	}
	req := resolveControllerRef(pod.Namespace, controllerRef)
	if req == nil {
		return
	}

	klog.V(4).InfoS("Pod deleted", "pod", klog.KObj(pod), "owner", req)
	clonesetutils.ScaleExpectations.ObserveScale(req.String(), expectations.Delete, pod.Name)
	q.Add(*req)
}

func (e *podEventHandler) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {

}

func resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *reconcile.Request {
	// Parse the Group out of the OwnerReference to compare it to what was parsed out of the requested OwnerType
	refGV, err := schema.ParseGroupVersion(controllerRef.APIVersion)
	if err != nil {
		klog.ErrorS(err, "Could not parse APIVersion in OwnerReference", "ownerRef", controllerRef)
		return nil
	}

	// Compare the OwnerReference Group and Kind against the OwnerType Group and Kind specified by the user.
	// If the two match, create a Request for the objected referred to by
	// the OwnerReference.  Use the Name from the OwnerReference and the Namespace from the
	// object in the event.
	if controllerRef.Kind == clonesetutils.ControllerKind.Kind && refGV.Group == clonesetutils.ControllerKind.Group {
		// Match found - add a Request for the object referred to in the OwnerReference
		req := reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      controllerRef.Name,
		}}
		return &req
	}
	return nil
}

func (e *podEventHandler) getPodCloneSets(pod *v1.Pod) []appsv1alpha1.CloneSet {
	csList := appsv1alpha1.CloneSetList{}
	if err := e.List(context.TODO(), &csList, client.InNamespace(pod.Namespace)); err != nil {
		return nil
	}

	var csMatched []appsv1alpha1.CloneSet
	for _, cs := range csList.Items {
		selector, err := metav1.LabelSelectorAsSelector(cs.Spec.Selector)
		if err != nil || selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}

		csMatched = append(csMatched, cs)
	}

	if len(csMatched) > 1 {
		// ControllerRef will ensure we don't do anything crazy, but more than one
		// item in this list nevertheless constitutes user error.
		klog.InfoS("Error! More than one CloneSet is selecting pod", "pod", klog.KObj(pod), "cloneSets", e.joinCloneSetNames(csMatched))
	}
	return csMatched
}

func (e *podEventHandler) joinCloneSetNames(csList []appsv1alpha1.CloneSet) string {
	var names []string
	for _, cs := range csList {
		names = append(names, cs.Name)
	}
	return strings.Join(names, ",")
}

type pvcEventHandler struct {
}

var _ handler.EventHandler = &pvcEventHandler{}

func (e *pvcEventHandler) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	pvc := evt.Object.(*v1.PersistentVolumeClaim)
	if pvc.DeletionTimestamp != nil {
		e.Delete(ctx, event.DeleteEvent{Object: evt.Object}, q)
		return
	}

	if controllerRef := metav1.GetControllerOf(pvc); controllerRef != nil {
		if req := resolveControllerRef(pvc.Namespace, controllerRef); req != nil {
			isSatisfied, _, _ := clonesetutils.ScaleExpectations.SatisfiedExpectations(req.String())
			clonesetutils.ScaleExpectations.ObserveScale(req.String(), expectations.Create, pvc.Name)
			if isSatisfied {
				// If the scale expectation is satisfied, it should be an existing Pod and the Informer
				// cache should have just synced.
				q.AddAfter(*req, initialingRateLimiter.When(req))
			} else {
				// Otherwise, add it immediately and reset the rate limiter
				initialingRateLimiter.Forget(req)
				q.Add(*req)
			}
		}
	}
}

func (e *pvcEventHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	pvc := evt.ObjectNew.(*v1.PersistentVolumeClaim)
	if pvc.DeletionTimestamp != nil {
		e.Delete(ctx, event.DeleteEvent{Object: evt.ObjectNew}, q)
	}
}

func (e *pvcEventHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	pvc, ok := evt.Object.(*v1.PersistentVolumeClaim)
	if !ok {
		klog.ErrorS(nil, "Skipped pvc deletion event", "deleteStateUnknown", evt.DeleteStateUnknown, "obj", evt.Object)
		return
	}

	if controllerRef := metav1.GetControllerOf(pvc); controllerRef != nil {
		if req := resolveControllerRef(pvc.Namespace, controllerRef); req != nil {
			clonesetutils.ScaleExpectations.ObserveScale(req.String(), expectations.Delete, pvc.Name)
			q.Add(*req)
		}
	}
}

func (e *pvcEventHandler) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {

}
