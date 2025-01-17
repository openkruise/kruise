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

package podunavailablebudget

import (
	"context"
	"time"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
)

var _ handler.TypedEventHandler[*corev1.Pod] = &enqueueRequestForPod{}

func newEnqueueRequestForPod(c client.Client) handler.TypedEventHandler[*corev1.Pod] {
	e := &enqueueRequestForPod{client: c}
	e.controllerFinder = controllerfinder.Finder
	return e
}

type enqueueRequestForPod struct {
	client           client.Client
	controllerFinder *controllerfinder.ControllerFinder
}

func (p *enqueueRequestForPod) Create(ctx context.Context, evt event.TypedCreateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	p.addPod(q, evt.Object)
}

func (p *enqueueRequestForPod) Delete(ctx context.Context, evt event.TypedDeleteEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Generic(ctx context.Context, evt event.TypedGenericEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(ctx context.Context, evt event.TypedUpdateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	p.updatePod(q, evt.ObjectOld, evt.ObjectNew)
}

func (p *enqueueRequestForPod) addPod(q workqueue.RateLimitingInterface, obj runtime.Object) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	var pub *policyv1alpha1.PodUnavailableBudget
	if pod.Annotations[pubcontrol.PodRelatedPubAnnotation] != "" {
		pub, _ = pubcontrol.PubControl.GetPubForPod(pod)
	}
	if pub == nil {
		return
	}
	klog.V(3).InfoS("Added Pod reconcile PodUnavailableBudget", "pod", klog.KObj(pod), "podUnavailableBudget", klog.KObj(pub))
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      pub.Name,
			Namespace: pub.Namespace,
		},
	})
}

func GetPubForPod(c client.Client, pod *corev1.Pod) (*policyv1alpha1.PodUnavailableBudget, error) {
	var workload *controllerfinder.ScaleAndSelector
	var err error
	if ref := metav1.GetControllerOf(pod); ref != nil {
		workload, err = controllerfinder.Finder.GetScaleAndSelectorForRef(ref.APIVersion, ref.Kind, pod.Namespace, ref.Name, "")
		if err != nil {
			klog.ErrorS(err, "Failed to find workload for pod", "pod", klog.KObj(pod))
		}
	}

	pubList := &policyv1alpha1.PodUnavailableBudgetList{}
	if err = c.List(context.TODO(), pubList, &client.ListOptions{Namespace: pod.Namespace}, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	for i := range pubList.Items {
		pub := &pubList.Items[i]
		// if targetReference isn't nil, priority to take effect
		if pub.Spec.TargetReference != nil && workload != nil {
			// belongs the same workload
			if pubcontrol.IsReferenceEqual(&policyv1alpha1.TargetReference{
				APIVersion: workload.APIVersion,
				Kind:       workload.Kind,
				Name:       workload.Name,
			}, pub.Spec.TargetReference) {
				return pub, nil
			}
		} else {
			// This error is irreversible, so continue
			labelSelector, err := util.ValidatedLabelSelectorAsSelector(pub.Spec.Selector)
			if err != nil {
				continue
			}
			// If a PUB with a nil or empty selector creeps in, it should match nothing, not everything.
			if labelSelector.Empty() || !labelSelector.Matches(labels.Set(pod.Labels)) {
				continue
			}
			return pub, nil
		}
	}
	return nil, nil
}

func (p *enqueueRequestForPod) updatePod(q workqueue.RateLimitingInterface, old, cur runtime.Object) {
	newPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}

	pub, _ := pubcontrol.PubControl.GetPubForPod(newPod)
	if pub == nil {
		return
	}
	if isReconcile, enqueueDelayTime := isPodAvailableChanged(oldPod, newPod, pub); isReconcile {
		q.AddAfter(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pub.Name,
				Namespace: pub.Namespace,
			},
		}, enqueueDelayTime)
	}

}

func isPodAvailableChanged(oldPod, newPod *corev1.Pod, pub *policyv1alpha1.PodUnavailableBudget) (bool, time.Duration) {
	var enqueueDelayTime time.Duration
	// If the pod's deletion timestamp is set, remove endpoint from ready address.
	if oldPod.DeletionTimestamp.IsZero() && !newPod.DeletionTimestamp.IsZero() {
		enqueueDelayTime = time.Second * 5
		klog.V(3).InfoS("Pod DeletionTimestamp changed, and reconcile PodUnavailableBudget after 5s", "pod", klog.KObj(newPod), "podUnavailableBudget", klog.KObj(pub))
		return true, enqueueDelayTime
		// oldPod Deletion is set, then no reconcile
	} else if !oldPod.DeletionTimestamp.IsZero() {
		return false, enqueueDelayTime
	}

	control := pubcontrol.PubControl
	// If the pod's readiness has changed, the associated endpoint address
	// will move from the unready endpoints set to the ready endpoints.
	// So for the purposes of an endpoint, a readiness change on a pod
	// means we have a changed pod.
	oldReady := control.IsPodReady(oldPod) && control.IsPodStateConsistent(oldPod)
	newReady := control.IsPodReady(newPod) && control.IsPodStateConsistent(newPod)
	if oldReady != newReady {
		klog.V(3).InfoS("Pod ConsistentAndReady changed, and reconcile PodUnavailableBudget", "pod", klog.KObj(newPod), "oldReady", oldReady,
			"newReady", newReady, "podUnavailableBudget", klog.KObj(pub))
		return true, enqueueDelayTime
	}

	return false, enqueueDelayTime
}

var _ handler.EventHandler = &SetEnqueueRequestForPUB{}

type SetEnqueueRequestForPUB struct {
	mgr manager.Manager
}

// Create implements EventHandler
func (e *SetEnqueueRequestForPUB) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.addSetRequest(evt.Object, q)
}

// Update implements EventHandler
func (e *SetEnqueueRequestForPUB) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.addSetRequest(evt.ObjectNew, q)
}

// Delete implements EventHandler
func (e *SetEnqueueRequestForPUB) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.addSetRequest(evt.Object, q)
}

// Generic implements EventHandler
func (e *SetEnqueueRequestForPUB) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *SetEnqueueRequestForPUB) addSetRequest(object client.Object, q workqueue.RateLimitingInterface) {
	gvk, _ := apiutil.GVKForObject(object, e.mgr.GetScheme())
	targetRef := &policyv1alpha1.TargetReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
	}
	var namespace string
	var temLabels map[string]string
	switch gvk.Kind {
	// cloneSet
	case controllerfinder.ControllerKruiseKindCS.Kind:
		obj := object.(*appsv1alpha1.CloneSet)
		targetRef.Name, namespace = obj.Name, obj.Namespace
		temLabels = obj.Spec.Template.Labels
	// deployment
	case controllerfinder.ControllerKindDep.Kind:
		obj := object.(*apps.Deployment)
		targetRef.Name, namespace = obj.Name, obj.Namespace
		temLabels = obj.Spec.Template.Labels

	// statefulSet
	case controllerfinder.ControllerKindSS.Kind:
		// kruise advanced statefulSet
		if gvk.Group == controllerfinder.ControllerKruiseKindSS.Group {
			obj := object.(*appsv1beta1.StatefulSet)
			targetRef.Name, namespace = obj.Name, obj.Namespace
			temLabels = obj.Spec.Template.Labels
		} else {
			obj := object.(*apps.StatefulSet)
			targetRef.Name, namespace = obj.Name, obj.Namespace
			temLabels = obj.Spec.Template.Labels
		}
	}
	// fetch matched pub
	pubList := &policyv1alpha1.PodUnavailableBudgetList{}
	if err := e.mgr.GetClient().List(context.TODO(), pubList, &client.ListOptions{Namespace: namespace}); err != nil {
		klog.ErrorS(err, "SetEnqueueRequestForPUB list pub failed")
		return
	}
	var matched policyv1alpha1.PodUnavailableBudget
	for _, pub := range pubList.Items {
		// if targetReference isn't nil, priority to take effect
		if pub.Spec.TargetReference != nil {
			// belongs the same workload
			if pubcontrol.IsReferenceEqual(targetRef, pub.Spec.TargetReference) {
				matched = pub
				break
			}
		} else {
			// This error is irreversible, so continue
			labelSelector, err := util.ValidatedLabelSelectorAsSelector(pub.Spec.Selector)
			if err != nil {
				continue
			}
			// If a PUB with a nil or empty selector creeps in, it should match nothing, not everything.
			if labelSelector.Empty() || !labelSelector.Matches(labels.Set(temLabels)) {
				continue
			}
			matched = pub
			break
		}
	}

	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      matched.Name,
			Namespace: matched.Namespace,
		},
	})
	klog.V(3).InfoS("Workload changed, and reconcile PodUnavailableBudget",
		"wordload", klog.KRef(namespace, targetRef.Name), "podUnavailableBudget", klog.KRef(matched.Namespace, matched.Name))
}
