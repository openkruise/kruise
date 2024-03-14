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

package containerlauchpriority

import (
	"context"
	"fmt"
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/kube-openapi/pkg/util/sets"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utilcontainerlaunchpriority "github.com/openkruise/kruise/pkg/util/containerlaunchpriority"
)

const (
	concurrentReconciles = 4
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileContainerLaunchPriority {
	return &ReconcileContainerLaunchPriority{
		Client:   utilclient.NewClientFromManager(mgr, "container-launch-priority-controller"),
		recorder: mgr.GetEventRecorderFor("container-launch-priority-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileContainerLaunchPriority) error {
	// Create a new controller
	c, err := controller.New("container-launch-priority-controller", mgr, controller.Options{Reconciler: r,
		MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout()})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			pod := e.Object.(*v1.Pod)
			return shouldEnqueue(pod, mgr.GetCache())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			pod := e.ObjectNew.(*v1.Pod)
			return shouldEnqueue(pod, mgr.GetCache())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func shouldEnqueue(pod *v1.Pod, r client.Reader) bool {
	if pod.Annotations[appspub.ContainerLaunchPriorityCompletedKey] == "true" {
		return false
	}
	if _, containersReady := podutil.GetPodCondition(&pod.Status, v1.ContainersReady); containersReady != nil && containersReady.Status == v1.ConditionTrue {
		return false
	}

	nextPriorities := findNextPriorities(pod)
	if len(nextPriorities) == 0 {
		return false
	}

	var barrier = &v1.ConfigMap{}
	var barrierNamespacedName = types.NamespacedName{
		Namespace: pod.GetNamespace(),
		Name:      pod.Name + "-barrier",
	}
	if err := r.Get(context.TODO(), barrierNamespacedName, barrier); err != nil {
		return true
	}
	return !isExistsInBarrier(nextPriorities[len(nextPriorities)-1], barrier)
}

var _ reconcile.Reconciler = &ReconcileContainerLaunchPriority{}

// ReconcileContainerLaunchPriority reconciles a Pod object
type ReconcileContainerLaunchPriority struct {
	client.Client
	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *ReconcileContainerLaunchPriority) Reconcile(_ context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	klog.V(3).Infof("Starting to process Pod %v", request.NamespacedName)
	defer func() {
		if err != nil {
			klog.Warningf("Failed to process Pod %v, elapsedTime %v, error: %v", request.NamespacedName, time.Since(start), err)
		} else {
			klog.Infof("Finish to process Pod %v, elapsedTime %v", request.NamespacedName, time.Since(start))
		}
	}()

	// get pod and barrier
	var pod = &v1.Pod{}
	err = r.Get(context.TODO(), request.NamespacedName, pod)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}
	var barrier = &v1.ConfigMap{}
	var barrierNamespacedName = types.NamespacedName{
		Namespace: pod.GetNamespace(),
		Name:      pod.Name + "-barrier",
	}
	err = r.Get(context.TODO(), barrierNamespacedName, barrier)
	if errors.IsNotFound(err) {
		barrier.Namespace = pod.GetNamespace()
		barrier.Name = pod.Name + "-barrier"
		barrier.OwnerReferences = append(barrier.OwnerReferences, metav1.OwnerReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       pod.Name,
			UID:        pod.UID,
		})
		klog.V(4).Infof("Creating ConfigMap %s for Pod %s/%s", barrier.Name, pod.Namespace, pod.Name)
		temErr := r.Client.Create(context.TODO(), barrier)
		if temErr != nil && !errors.IsAlreadyExists(temErr) {
			return reconcile.Result{}, temErr
		}
		return reconcile.Result{Requeue: true}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	// handle the pod and barrier
	if err = r.handle(pod, barrier); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileContainerLaunchPriority) handle(pod *v1.Pod, barrier *v1.ConfigMap) error {
	nextPriorities := findNextPriorities(pod)

	// If there is no more priorities, or the lowest priority exists in barrier, mask as completed.
	if len(nextPriorities) == 0 || isExistsInBarrier(nextPriorities[0], barrier) {
		return r.patchCompleted(pod)
	}

	// Try to add the current priority if not exists.
	if !isExistsInBarrier(nextPriorities[len(nextPriorities)-1], barrier) {
		if err := r.addPriorityIntoBarrier(barrier, nextPriorities[len(nextPriorities)-1]); err != nil {
			return err
		}
	}

	// After adding the current priority, if the lowest priority is same to the current one, mark as completed.
	if nextPriorities[len(nextPriorities)-1] == nextPriorities[0] {
		return r.patchCompleted(pod)
	}
	return nil
}

func (r *ReconcileContainerLaunchPriority) addPriorityIntoBarrier(barrier *v1.ConfigMap, priority int) error {
	klog.V(3).Infof("Adding priority %d into barrier %s/%s", priority, barrier.Namespace, barrier.Name)
	body := fmt.Sprintf(`{"data":{"%s":"true"}}`, utilcontainerlaunchpriority.GetKey(priority))
	return r.Client.Patch(context.TODO(), barrier, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
}

func (r *ReconcileContainerLaunchPriority) patchCompleted(pod *v1.Pod) error {
	klog.V(3).Infof("Marking pod %s/%s as launch priority completed", pod.Namespace, pod.Name)
	body := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"true"}}}`, appspub.ContainerLaunchPriorityCompletedKey)
	return r.Client.Patch(context.TODO(), pod, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
}

func findNextPriorities(pod *v1.Pod) (priorities []int) {
	containerReadySet := sets.NewString()
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			containerReadySet.Insert(status.Name)
		}
	}
	for _, c := range pod.Spec.Containers {
		if containerReadySet.Has(c.Name) {
			continue
		}
		priority := utilcontainerlaunchpriority.GetContainerPriority(&c)
		if priority == nil {
			continue
		}

		priorities = append(priorities, *priority)
	}
	if len(priorities) > 0 {
		sort.Ints(priorities)
	}
	return
}

func isExistsInBarrier(priority int, barrier *v1.ConfigMap) bool {
	_, exists := barrier.Data[utilcontainerlaunchpriority.GetKey(priority)]
	return exists
}
