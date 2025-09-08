package configmapset

import (
	"context"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// cms事件处理
type configMapSetEventHandler struct {
	client.Reader
}

var _ handler.TypedEventHandler[*appsv1alpha1.ConfigMapSet, reconcile.Request] = &configMapSetEventHandler{}

func (e *configMapSetEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*appsv1alpha1.ConfigMapSet], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	obj := evt.Object
	e.handle(obj, q)
}

func (e *configMapSetEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*appsv1alpha1.ConfigMapSet], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	obj := evt.ObjectNew
	oldObj := evt.ObjectOld
	if obj.DeletionTimestamp != nil {
		e.handle(obj, q)
	} else {
		e.handleUpdate(obj, oldObj, q)
	}
}

func (e *configMapSetEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*appsv1alpha1.ConfigMapSet], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	obj := evt.Object
	e.handle(obj, q)
}

func (e *configMapSetEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*appsv1alpha1.ConfigMapSet], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (e *configMapSetEventHandler) handle(cms *appsv1alpha1.ConfigMapSet, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	pods, err := getMatchedPods(e.Reader, cms)
	if err != nil {
		klog.Errorf("Failed to get Pods for appsv1alpha1.ConfigMapSet %s: %v", cms.Name, err)
		return
	}
	for _, pod := range pods.Items {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}})
	}
}

func (e *configMapSetEventHandler) handleUpdate(cms, oldCms *appsv1alpha1.ConfigMapSet, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if reflect.DeepEqual(cms.Spec, oldCms.Spec) {
		return
	}
	e.handle(cms, q)
}

// pod事件处理
type podEventHandler struct {
	client.Reader
}

var _ handler.TypedEventHandler[*v1.Pod, reconcile.Request] = &podEventHandler{}

func (p *podEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Pod], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	pod := evt.Object
	if pod.DeletionTimestamp != nil {
		p.Delete(ctx, event.TypedDeleteEvent[*v1.Pod]{Object: evt.Object}, q)
		return
	}
	p.handle(pod, q)
}
func (p *podEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Pod], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	obj := evt.ObjectNew
	oldObj := evt.ObjectOld
	// 不处理正在terminating的pod
	if obj.DeletionTimestamp != nil {
		klog.Infof("Pod %s/%s is terminating, skip", obj.Namespace, obj.Name)
		return
	}
	if obj.DeletionTimestamp != nil {
		p.handle(obj, q)
	} else {
		p.handleUpdate(obj, oldObj, q)
	}
}

func (p *podEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Pod], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	//obj := evt.Object.(*corev1.Pod)
	//e.handle(obj, q)
}

func (p *podEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Pod], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (p *podEventHandler) handleUpdate(pod, oldPod *v1.Pod, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if pod.Spec.NodeName == "" {
		return
	}
	p.handle(pod, q)
}

func (p *podEventHandler) handle(pod *v1.Pod, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if len(pod.Labels) == 0 {
		klog.Infof("Pod %s/%s has no labels, skip", pod.Namespace, pod.Name)
		return
	}
	cmsList, err := GetRelatedConfigMapSets(p.Reader, pod)
	if err != nil {
		klog.Errorf("Failed to get ConfigMapSet for Pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return
	}
	if len(cmsList) > 0 {
		// 每一个都要加
		for _, cms := range cmsList {
			klog.Infof("Pod %s/%s update ConfigMapSet %s", pod.Namespace, pod.Name, cms.Name)
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: cms.Namespace, Name: cms.Name}})
		}
	}
}
