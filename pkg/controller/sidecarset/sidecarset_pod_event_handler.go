package sidecarset

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/webhook/default_server/pod/mutating"
)

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	client client.Client
}

func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.addPod(q, evt.Object)
}

func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	p.deletePod(q, evt.Object)
}

func (p *enqueueRequestForPod) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updatePod(q, evt.ObjectOld, evt.ObjectNew)
}

// When a pod is added, figure out what sidecarSets it will be a member of and
// enqueue them. obj must have *v1.Pod type.
func (p *enqueueRequestForPod) addPod(q workqueue.RateLimitingInterface, obj runtime.Object) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	sidecarSets, err := p.getPodSidecarSets(pod)
	if err != nil {
		klog.Errorf("unable to get sidecarSets related with pod %s/%s, err: %v", pod.Namespace, pod.Name, err)
		return
	}

	for _, sidecarSet := range sidecarSets {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: sidecarSet.Name,
			},
		})
	}
}

func (p *enqueueRequestForPod) deletePod(q workqueue.RateLimitingInterface, obj runtime.Object) {
	if _, ok := obj.(*corev1.Pod); ok {
		p.addPod(q, obj)
		return
	}
}

func (p *enqueueRequestForPod) updatePod(q workqueue.RateLimitingInterface, old, cur runtime.Object) {
	newPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}

	podChanged := isPodChanged(oldPod, newPod)
	labelChanged := false
	if !reflect.DeepEqual(newPod.Labels, oldPod.Labels) {
		labelChanged = true
	}

	if !podChanged && !labelChanged {
		return
	}

	sidecarSets, err := p.getPodSidecarSetMemberships(newPod)
	if err != nil {
		klog.Errorf("unable to get sidecarSets of pod %s/%s, err: %v", newPod.Namespace, newPod.Name, err)
		return
	}

	if labelChanged {
		oldSidecarSets, err := p.getPodSidecarSetMemberships(oldPod)
		if err != nil {
			klog.Errorf("unable to get sidecarSets of pod %s/%s, err: %v", oldPod.Namespace, oldPod.Name, err)
			return
		}
		sidecarSets = sidecarSets.Difference(oldSidecarSets).Union(oldSidecarSets.Difference(sidecarSets))
	}

	for name := range sidecarSets {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: name,
			},
		})
	}
}

func (p *enqueueRequestForPod) getPodSidecarSetMemberships(pod *corev1.Pod) (sets.String, error) {
	set := sets.String{}
	sidecarSets, err := p.getPodSidecarSets(pod)
	if err != nil {
		return set, err
	}

	for _, sidecarSet := range sidecarSets {
		set.Insert(sidecarSet.Name)
	}
	return set, nil
}

func (p *enqueueRequestForPod) getPodSidecarSets(pod *corev1.Pod) ([]appsv1alpha1.SidecarSet, error) {
	sidecarSets := appsv1alpha1.SidecarSetList{}
	if err := p.client.List(context.TODO(), &client.ListOptions{}, &sidecarSets); err != nil {
		return nil, err
	}

	var matchedSidecarSets []appsv1alpha1.SidecarSet
	for _, sidecarSet := range sidecarSets.Items {
		matched, err := mutating.PodMatchSidecarSet(pod, sidecarSet)
		if err != nil {
			return nil, err
		}
		if matched {
			matchedSidecarSets = append(matchedSidecarSets, sidecarSet)
		}
	}

	return matchedSidecarSets, nil
}

func isPodChanged(oldPod, newPod *corev1.Pod) bool {
	// If the pod's deletion timestamp is set, remove endpoint from ready address.
	if newPod.DeletionTimestamp != oldPod.DeletionTimestamp {
		return true
	}

	// If the pod's readiness has changed, the associated endpoint address
	// will move from the unready endpoints set to the ready endpoints.
	// So for the purposes of an endpoint, a readiness change on a pod
	// means we have a changed pod.
	if podutil.IsPodReady(oldPod) != podutil.IsPodReady(newPod) {
		return true
	}

	if !isPodImageConsistent(oldPod) && isPodImageConsistent(newPod) {
		return true
	}

	return false
}
