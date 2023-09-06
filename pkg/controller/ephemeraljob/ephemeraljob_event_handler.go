package ephemeraljob

import (
	"context"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/expectations"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type podHandler struct {
	client.Reader
}

func (e *podHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	pod, ok := evt.Object.(*corev1.Pod)
	if !ok {
		return
	}

	e.handle(pod, q)
}

func (e *podHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *podHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldPod := evt.ObjectOld.(*corev1.Pod)
	curPod := evt.ObjectNew.(*corev1.Pod)
	if curPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}

	if reflect.DeepEqual(curPod.Status.EphemeralContainerStatuses, oldPod.Status.EphemeralContainerStatuses) {
		return
	}

	e.handle(curPod, q)
}

func (e *podHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	pod, ok := evt.Object.(*corev1.Pod)
	if !ok {
		return
	}
	e.handle(pod, q)
}

func (e *podHandler) handle(pod *corev1.Pod, q workqueue.RateLimitingInterface) {
	ephemeralJobs := appsv1alpha1.EphemeralJobList{}
	if err := e.List(context.TODO(), &ephemeralJobs, client.InNamespace(pod.Namespace)); err != nil {
		return
	}

	for _, ejob := range ephemeralJobs.Items {
		matched, _ := podMatchedEphemeralJob(pod, &ejob)
		if matched {
			key := types.NamespacedName{Namespace: ejob.Namespace, Name: ejob.Name}.String()
			for _, podEphemeralContainerName := range getPodEphemeralContainers(pod, &ejob) {
				scaleExpectations.ObserveScale(key, expectations.Create, podEphemeralContainerName)
			}
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ejob.Namespace, Name: ejob.Name}})
		}
	}
}

type ejobHandler struct {
	client.Reader
}

func (e *ejobHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	ejob, ok := evt.Object.(*appsv1alpha1.EphemeralJob)
	if !ok {
		return
	}
	if ejob.DeletionTimestamp != nil {
		return
	}

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ejob.Namespace, Name: ejob.Name}})
}

func (e *ejobHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *ejobHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldEJob := evt.ObjectOld.(*appsv1alpha1.EphemeralJob)
	curEJob := evt.ObjectNew.(*appsv1alpha1.EphemeralJob)
	if oldEJob.ResourceVersion == curEJob.ResourceVersion {
		return
	}

	if curEJob.DeletionTimestamp != nil {
		klog.V(3).Infof("Observed deleting ephemeral job: %s/%s", curEJob.Namespace, curEJob.Name)
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: curEJob.Namespace, Name: curEJob.Name}})
		return
	}

	// only update these fields
	if oldEJob.Spec.TTLSecondsAfterFinished != curEJob.Spec.TTLSecondsAfterFinished ||
		oldEJob.Spec.Paused != curEJob.Spec.Paused || oldEJob.Spec.Parallelism != curEJob.Spec.Parallelism ||
		oldEJob.Spec.Replicas != curEJob.Spec.Replicas {
		klog.V(3).Infof("Observed updated Spec for ephemeral job: %s/%s", curEJob.Namespace, curEJob.Name)
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: curEJob.Namespace, Name: curEJob.Name}})
	}
}

func (e *ejobHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	ejob, ok := evt.Object.(*appsv1alpha1.EphemeralJob)
	if !ok {
		return
	}
	key := types.NamespacedName{Namespace: ejob.Namespace, Name: ejob.Name}
	scaleExpectations.DeleteExpectations(key.String())
}
