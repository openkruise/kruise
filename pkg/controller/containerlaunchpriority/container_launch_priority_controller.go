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
	"strconv"
	"time"

	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utilcontainerlaunchpriority "github.com/openkruise/kruise/pkg/util/containerlaunchpriority"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
			_, containersReady := podutil.GetPodCondition(&pod.Status, v1.ContainersReady)
			// If in vk scenario, there will be not containerReady condition
			return utilcontainerlaunchpriority.ExistsPriorities(pod) && (containersReady == nil || containersReady.Status != v1.ConditionTrue)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			pod := e.ObjectNew.(*v1.Pod)
			_, containersReady := podutil.GetPodCondition(&pod.Status, v1.ContainersReady)
			// If in vk scenario, there will be not containerReady condition
			return utilcontainerlaunchpriority.ExistsPriorities(pod) && (containersReady == nil || containersReady.Status != v1.ConditionTrue)
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

	// set next starting containers
	_, containersReady := podutil.GetPodCondition(&pod.Status, v1.ContainersReady)
	if containersReady != nil && containersReady.Status != v1.ConditionTrue {
		patchKey := r.findNextPatchKey(pod)
		if patchKey == nil {
			return reconcile.Result{}, nil
		}
		key := "p_" + strconv.Itoa(*patchKey)
		if err = r.patchOnKeyNotExist(barrier, key); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileContainerLaunchPriority) findNextPatchKey(pod *v1.Pod) *int {
	var priority *int
	var containerPendingSet = make(map[string]bool)
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			continue
		}
		containerPendingSet[status.Name] = true
	}
	for _, c := range pod.Spec.Containers {
		if _, ok := containerPendingSet[c.Name]; ok {
			p := utilcontainerlaunchpriority.GetContainerPriority(&c)
			if p == nil {
				continue
			}
			if priority == nil || *p > *priority {
				priority = p
			}
		}
	}
	return priority
}

func (r *ReconcileContainerLaunchPriority) patchOnKeyNotExist(barrier *v1.ConfigMap, key string) error {
	if _, ok := barrier.Data[key]; !ok {
		body := fmt.Sprintf(
			`{"data":{"%s":"true"}}`,
			key,
		)
		return r.Client.Patch(context.TODO(), barrier, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
	}
	return nil
}
