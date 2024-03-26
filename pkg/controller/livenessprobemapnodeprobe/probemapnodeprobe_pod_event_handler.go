package livenessprobemapnodeprobe

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
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
	obj, ok := evt.Object.(*v1.Pod)
	if !ok {
		return
	}
	p.queue(q, obj)
	return
}

func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(*v1.Pod)
	if !ok {
		return
	}
	p.queue(q, obj)
	return
}

func (p *enqueueRequestForPod) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	obj, ok := evt.Object.(*v1.Pod)
	if !ok {
		return
	}
	p.queue(q, obj)
	return
}

func (p *enqueueRequestForPod) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	new, ok := evt.ObjectNew.(*v1.Pod)
	if !ok {
		return
	}
	p.queue(q, new)
	return
}

func (p *enqueueRequestForPod) queue(q workqueue.RateLimitingInterface, pod *v1.Pod) {
	if usingEnhancedLivenessProbe(pod) && getRawEnhancedLivenessProbeConfig(pod) != "" {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		})
	}
}
