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

package podreadiness

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
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
	utilpodreadiness "github.com/openkruise/kruise/pkg/util/podreadiness"
)

var (
	concurrentReconciles = 3
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcilePodReadiness {
	return &ReconcilePodReadiness{
		Client: utilclient.NewClientFromManager(mgr, "pod-readiness-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcilePodReadiness) error {
	// Create a new controller
	c, err := controller.New("pod-readiness-controller", mgr, controller.Options{Reconciler: r,
		MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout()})
	if err != nil {
		return err
	}

	err = c.Watch(source.Kind(mgr.GetCache(), &v1.Pod{}, &handler.TypedEnqueueRequestForObject[*v1.Pod]{}, predicate.TypedFuncs[*v1.Pod]{
		CreateFunc: func(e event.TypedCreateEvent[*v1.Pod]) bool {
			pod := e.Object
			return utilpodreadiness.ContainsReadinessGate(pod) && utilpodreadiness.GetReadinessCondition(pod) == nil
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*v1.Pod]) bool {
			pod := e.ObjectNew
			return utilpodreadiness.ContainsReadinessGate(pod) && utilpodreadiness.GetReadinessCondition(pod) == nil
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*v1.Pod]) bool {
			return false
		},
		GenericFunc: func(e event.TypedGenericEvent[*v1.Pod]) bool {
			return false
		},
	}))
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePodReadiness{}

// ReconcilePodReadiness reconciles a Pod object
type ReconcilePodReadiness struct {
	client.Client
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch

func (r *ReconcilePodReadiness) Reconcile(_ context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	klog.V(3).InfoS("Starting to process Pod", "pod", request)
	defer func() {
		if err != nil {
			klog.ErrorS(err, "Failed to process Pod", "pod", request, "elapsedTime", time.Since(start))
		} else {
			klog.InfoS("Finished to process Pod", "pod", request, "elapsedTime", time.Since(start))
		}
	}()

	pod := &v1.Pod{}
	err = r.Get(context.TODO(), request.NamespacedName, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if pod.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}
	if !utilpodreadiness.ContainsReadinessGate(pod) {
		return reconcile.Result{}, nil
	}
	if utilpodreadiness.GetReadinessCondition(pod) != nil {
		return reconcile.Result{}, nil
	}

	// patch pod condition
	status := v1.PodStatus{
		Conditions: []v1.PodCondition{
			{
				Type:               appspub.KruisePodReadyConditionType,
				Status:             v1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
			},
		},
	}
	by, _ := json.Marshal(status)
	patchBody := fmt.Sprintf(`{"status":%s}`, string(by))
	rcvObject := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name}}

	if err := r.Status().Patch(context.TODO(), rcvObject, client.RawPatch(types.StrategicMergePatchType, []byte(patchBody))); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to patch pod status: %v", err)
	}
	return reconcile.Result{}, nil
}
