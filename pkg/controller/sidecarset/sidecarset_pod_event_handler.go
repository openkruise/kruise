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

var _ handler.TypedEventHandler[*corev1.Pod] = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	reader client.Reader
}

func (p *enqueueRequestForPod) Create(ctx context.Context, evt event.TypedCreateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	p.addPod(q, evt.Object)
}

func (p *enqueueRequestForPod) Delete(ctx context.Context, evt event.TypedDeleteEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	p.deletePod(evt.Object)
}

func (p *enqueueRequestForPod) Generic(ctx context.Context, evt event.TypedGenericEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(ctx context.Context, evt event.TypedUpdateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	p.updatePod(q, evt.ObjectOld, evt.ObjectNew)
}

func (p *enqueueRequestForPod) deletePod(obj runtime.Object) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	sidecarSets, err := p.getPodMatchedSidecarSets(pod)
	if err != nil {
		klog.ErrorS(err, "Unable to get sidecarSets related with pod", "pod", klog.KObj(pod))
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
		klog.ErrorS(err, "Unable to get sidecarSets related with pod", "pod", klog.KObj(pod))
		return
	}

	for _, sidecarSet := range sidecarSets {
		klog.V(3).InfoS("Created pod and reconcile sidecarSet", "pod", klog.KObj(pod), "sidecarSet", klog.KObj(sidecarSet))
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
		klog.ErrorS(err, "Unable to get sidecarSets of pod", "pod", klog.KObj(newPod))
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
					klog.V(6).InfoS("Could not find SidecarSet for Pod", "pod", klog.KObj(pod), "sidecarSetName", sidecarSetName)
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

	return matchedSidecarSets, nil
}

func isPodStatusChanged(oldPod, newPod *corev1.Pod) bool {
	// If the pod's deletion timestamp is set, remove endpoint from ready address.
	if oldPod.DeletionTimestamp.IsZero() && !newPod.DeletionTimestamp.IsZero() {
		klog.V(3).InfoS("Pod DeletionTimestamp changed, and reconcile sidecarSet", "pod", klog.KObj(newPod))
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
		klog.V(3).InfoS("Pod Ready changed, and reconcile SidecarSet", "pod", klog.KObj(newPod), "oldReady", oldReady, "newReady", newReady)
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
		klog.V(3).InfoS("Pod's sidecar containers consistent changed, and reconcile SidecarSet",
			"pod", klog.KObj(newPod), "oldConsistent", oldConsistent, "newConsistent", newConsistent, "sidecarSet", klog.KObj(sidecarSet))
		enqueueDelayTime = 5 * time.Second
		return true, enqueueDelayTime
	}

	// If the pod's labels changed, and sidecarSet enable selector updateStrategy, should reconcile.
	if !reflect.DeepEqual(oldPod.Labels, newPod.Labels) && sidecarSet.Spec.UpdateStrategy.Selector != nil {
		klog.V(3).InfoS("Pod's Labels changed and SidecarSet enable selector upgrade strategy, and reconcile SidecarSet",
			"pod", klog.KObj(newPod), "sidecarSet", klog.KObj(sidecarSet))
		return true, 0
	}

	return false, enqueueDelayTime
}
