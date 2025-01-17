package ephemeraljob

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/expectations"
)

type podHandler struct {
	client.Reader
}

func (e *podHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	pod := evt.Object

	e.handle(pod, q)
}

func (e *podHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
}

func (e *podHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	oldPod := evt.ObjectOld
	curPod := evt.ObjectNew
	if curPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}

	if reflect.DeepEqual(curPod.Status.EphemeralContainerStatuses, oldPod.Status.EphemeralContainerStatuses) {
		return
	}

	e.handle(curPod, q)
}

func (e *podHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*corev1.Pod], q workqueue.RateLimitingInterface) {
	pod := evt.Object
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

func (e *ejobHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*appsv1alpha1.EphemeralJob], q workqueue.RateLimitingInterface) {
	ejob := evt.Object
	if ejob.DeletionTimestamp != nil {
		return
	}

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ejob.Namespace, Name: ejob.Name}})
}

func (e *ejobHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*appsv1alpha1.EphemeralJob], q workqueue.RateLimitingInterface) {
}

func (e *ejobHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*appsv1alpha1.EphemeralJob], q workqueue.RateLimitingInterface) {
	oldEJob := evt.ObjectOld
	curEJob := evt.ObjectNew
	if oldEJob.ResourceVersion == curEJob.ResourceVersion {
		return
	}

	if curEJob.DeletionTimestamp != nil {
		klog.V(3).InfoS("Observed deleting EphemeralJob", "ephemeralJob", klog.KObj(curEJob))
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: curEJob.Namespace, Name: curEJob.Name}})
		return
	}

	// only update these fields
	if oldEJob.Spec.TTLSecondsAfterFinished != curEJob.Spec.TTLSecondsAfterFinished ||
		oldEJob.Spec.Paused != curEJob.Spec.Paused || oldEJob.Spec.Parallelism != curEJob.Spec.Parallelism ||
		oldEJob.Spec.Replicas != curEJob.Spec.Replicas {
		klog.V(3).InfoS("Observed updated Spec for EphemeralJob", "ephemeralJob", klog.KObj(curEJob))
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: curEJob.Namespace, Name: curEJob.Name}})
	}
}

func (e *ejobHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*appsv1alpha1.EphemeralJob], q workqueue.RateLimitingInterface) {
	ejob := evt.Object
	key := types.NamespacedName{Namespace: ejob.Namespace, Name: ejob.Name}
	scaleExpectations.DeleteExpectations(key.String())
}
