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

package orderedcontainer

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
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
	priorityBarrier      = "KRUISE_CONTAINER_BARRIER"
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileOrderedContainer {
	return &ReconcileOrderedContainer{
		Client:   util.NewClientFromManager(mgr, "ordered-container-controller"),
		recorder: mgr.GetEventRecorderFor("ordered-container-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileOrderedContainer) error {
	// Create a new controller
	c, err := controller.New("ordered-container-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			pod := e.Object.(*v1.Pod)
			return r.validate(pod)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			pod := e.ObjectNew.(*v1.Pod)
			return r.validate(pod) && !r.getConditionStatus(pod, v1.ContainersReady)
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

var _ reconcile.Reconciler = &ReconcileOrderedContainer{}

// ReconcileOrderedContainer reconciles a Pod object
type ReconcileOrderedContainer struct {
	client.Client
	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *ReconcileOrderedContainer) Reconcile(request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	if klog.V(3) {
		klog.Infof("Starting to process Pod %v", request.NamespacedName)
	}
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
	if err != nil {
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
			APIVersion: pod.APIVersion,
			Kind:       pod.Kind,
			Name:       pod.Name,
			UID:        pod.UID,
		})
		temErr := r.Client.Create(context.TODO(), barrier)
		if temErr != nil {
			return reconcile.Result{}, temErr
		}
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	// set next starting pod
	if !r.getConditionStatus(pod, v1.ContainersReady) {
		var count = r.getNumberOfReadyContainers(pod.Status.ContainerStatuses)
		key := "p_" + strconv.Itoa(count)
		if err = r.patchOnKeyNotExist(barrier, key); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileOrderedContainer) validate(pod *v1.Pod) bool {
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	for _, v := range pod.Spec.Containers[0].Env {
		if v.Name == priorityBarrier {
			return true
		}
	}
	return false
}

func (r *ReconcileOrderedContainer) getConditionStatus(pod *v1.Pod, conditionType v1.PodConditionType) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status == v1.ConditionTrue
		}
	}
	return false
}

func (r *ReconcileOrderedContainer) getNumberOfReadyContainers(containerStatus []v1.ContainerStatus) int {
	var count int
	for _, status := range containerStatus {
		if status.Ready {
			count++
		}
	}
	return count
}

func (r *ReconcileOrderedContainer) patchOnKeyNotExist(barrier *v1.ConfigMap, key string) error {
	if _, ok := barrier.Data[key]; !ok {
		body := fmt.Sprintf(
			`{"data":{"%s":"true"}}`,
			key,
		)
		return r.Client.Patch(context.TODO(), barrier, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
	}
	return nil
}
