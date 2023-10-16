package sidecarset

import (
	"context"
	"reflect"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	reader client.Reader
}

func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.addPod(q, evt.Object)
}

func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	p.deletePod(evt.Object)
}

func (p *enqueueRequestForPod) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updatePod(q, evt.ObjectOld, evt.ObjectNew)
}

func (p *enqueueRequestForPod) deletePod(obj runtime.Object) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	sidecarSets, err := p.getPodMatchedSidecarSets(pod)
	if err != nil {
		klog.Errorf("unable to get sidecarSets related with pod %s/%s, err: %v", pod.Namespace, pod.Name, err)
		return
	}
	for _, sidecarSet := range sidecarSets {
		sidecarcontrol.UpdateExpectations.DeleteObject(sidecarSet.Name, pod)
	}
}

// When a pod is added, figure out what sidecarSets it will be a member of and
// enqueue them. obj must have *v1.Pod type.
func (p *enqueueRequestForPod) addPod(q workqueue.RateLimitingInterface, obj runtime.Object) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	sidecarSets, err := p.getPodMatchedSidecarSets(pod)
	if err != nil {
		klog.Errorf("unable to get sidecarSets related with pod %s/%s, err: %v", pod.Namespace, pod.Name, err)
		return
	}

	for _, sidecarSet := range sidecarSets {
		klog.V(3).Infof("Create pod(%s/%s) and reconcile sidecarSet(%s)", pod.Namespace, pod.Name, sidecarSet.Name)
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: sidecarSet.Name,
			},
		})
	}
}

func (p *enqueueRequestForPod) updatePod(q workqueue.RateLimitingInterface, old, cur runtime.Object) {
	newPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}

	matchedSidecarSets, err := p.getPodMatchedSidecarSets(newPod)
	if err != nil {
		klog.Errorf("unable to get sidecarSets of pod %s/%s, err: %v", newPod.Namespace, newPod.Name, err)
		return
	}
	for _, sidecarSet := range matchedSidecarSets {
		if sidecarSet.Spec.UpdateStrategy.Type == appsv1alpha1.NotUpdateSidecarSetStrategyType {
			continue
		}
		var isChanged bool
		var enqueueDelayTime time.Duration
		//check whether pod status is changed
		if isChanged = isPodStatusChanged(oldPod, newPod); !isChanged {
			//check whether pod consistent is changed
			isChanged, enqueueDelayTime = isPodConsistentChanged(oldPod, newPod, sidecarSet)
		}
		if isChanged {
			q.AddAfter(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: sidecarSet.Name,
				},
			}, enqueueDelayTime)
		}
	}

}

func (p *enqueueRequestForPod) getPodMatchedSidecarSets(pod *corev1.Pod) ([]*appsv1alpha1.SidecarSet, error) {
	sidecarSetNames, ok := pod.Annotations[sidecarcontrol.SidecarSetListAnnotation]

	var matchedSidecarSets []*appsv1alpha1.SidecarSet
	if ok && len(sidecarSetNames) > 0 {
		for _, sidecarSetName := range strings.Split(sidecarSetNames, ",") {
			sidecarSet := new(appsv1alpha1.SidecarSet)
			if err := p.reader.Get(context.TODO(), types.NamespacedName{
				Name: sidecarSetName,
			}, sidecarSet); err != nil {
				if errors.IsNotFound(err) {
					klog.V(6).Infof("pod(%s/%s) sidecarSet(%s) Not Found", pod.Namespace, pod.Name, sidecarSetName)
					continue
				}
				return nil, err
			}
			if sidecarcontrol.IsPodConsistentWithSidecarSet(pod, sidecarSet) {
				matchedSidecarSets = append(matchedSidecarSets, sidecarSet)
			}
		}
		return matchedSidecarSets, nil
	}

	// This code will trigger an invalid reconcile, where the Pod matches the sidecarSet selector but does not inject the sidecar container.
	// Comment out this code to reduce some invalid reconcile.
	/*sidecarSets := appsv1alpha1.SidecarSetList{}
	if err := p.reader.List(context.TODO(), &sidecarSets); err != nil {
		return nil, err
	}

	for _, sidecarSet := range sidecarSets.Items {
		matched, err := sidecarcontrol.PodMatchedSidecarSet(pod, sidecarSet)
		if err != nil {
			return nil, err
		}
		if matched {
			matchedSidecarSets = append(matchedSidecarSets, &sidecarSet)
		}
	}*/
	return matchedSidecarSets, nil
}

func isPodStatusChanged(oldPod, newPod *corev1.Pod) bool {
	// If the pod's deletion timestamp is set, remove endpoint from ready address.
	if oldPod.DeletionTimestamp.IsZero() && !newPod.DeletionTimestamp.IsZero() {
		klog.V(3).Infof("pod(%s/%s) DeletionTimestamp changed, and reconcile sidecarSet", newPod.Namespace, newPod.Name)
		return true
		// oldPod Deletion is set, then no reconcile
	} else if !oldPod.DeletionTimestamp.IsZero() {
		return false
	}

	// If the pod's readiness has changed, the associated endpoint address
	// will move from the unready endpoints set to the ready endpoints.
	// So for the purposes of an endpoint, a readiness change on a pod
	// means we have a changed pod.
	oldReady := podutil.IsPodReady(oldPod)
	newReady := podutil.IsPodReady(newPod)
	if oldReady != newReady {
		klog.V(3).Infof("pod(%s/%s) Ready changed(from %v to %v), and reconcile sidecarSet",
			newPod.Namespace, newPod.Name, oldReady, newReady)
		return true
	}

	return false
}

func isPodConsistentChanged(oldPod, newPod *corev1.Pod, sidecarSet *appsv1alpha1.SidecarSet) (bool, time.Duration) {
	control := sidecarcontrol.New(sidecarSet)
	var enqueueDelayTime time.Duration
	// contain sidecar empty container
	oldConsistent := control.IsPodStateConsistent(oldPod, nil)
	newConsistent := control.IsPodStateConsistent(newPod, nil)
	if oldConsistent != newConsistent {
		klog.V(3).Infof("pod(%s/%s) sidecar containers consistent changed(from %v to %v), and reconcile sidecarSet(%s)",
			newPod.Namespace, newPod.Name, oldConsistent, newConsistent, sidecarSet.Name)
		enqueueDelayTime = 5 * time.Second
		return true, enqueueDelayTime
	}

	// If the pod's labels changed, and sidecarSet enable selector updateStrategy, should reconcile.
	if !reflect.DeepEqual(oldPod.Labels, newPod.Labels) && sidecarSet.Spec.UpdateStrategy.Selector != nil {
		klog.V(3).Infof("pod(%s/%s) Labels changed and sidecarSet (%s) enable selector upgrade strategy, "+
			"and reconcile sidecarSet", newPod.Namespace, newPod.Name, sidecarSet.Name)
		return true, 0
	}

	return false, enqueueDelayTime
}
