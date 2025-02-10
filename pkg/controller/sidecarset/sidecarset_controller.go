/*
Copyright 2019 The Kruise Authors.

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

package sidecarset

import (
	"context"
	"flag"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "sidecarset-workers", concurrentReconciles, "Max concurrent workers for SidecarSet controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("SidecarSet")
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new SidecarSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor("sidecarset-controller")
	cli := utilclient.NewClientFromManager(mgr, "sidecarset-controller")
	return &ReconcileSidecarSet{
		Client:    cli,
		scheme:    mgr.GetScheme(),
		processor: NewSidecarSetProcessor(cli, recorder),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("sidecarset-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to SidecarSet
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.SidecarSet{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.SidecarSet]{}, predicate.TypedFuncs[*appsv1alpha1.SidecarSet]{
		UpdateFunc: func(e event.TypedUpdateEvent[*appsv1alpha1.SidecarSet]) bool {
			oldScS := e.ObjectOld
			newScS := e.ObjectNew
			if oldScS.GetGeneration() != newScS.GetGeneration() {
				klog.V(3).InfoS("Observed updated Spec for SidecarSet", "sidecarSet", klog.KObj(newScS))
				return true
			}
			return false
		},
	}))
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &enqueueRequestForPod{reader: mgr.GetCache()})); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSidecarSet{}

// ReconcileSidecarSet reconciles a SidecarSet object
type ReconcileSidecarSet struct {
	client.Client
	scheme    *runtime.Scheme
	processor *Processor
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=sidecarsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=sidecarsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=sidecarsets/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a SidecarSet object and makes changes based on the state read
// and what is in the SidecarSet.Spec
func (r *ReconcileSidecarSet) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the SidecarSet instance
	sidecarSet := &appsv1alpha1.SidecarSet{}
	err := r.Get(context.TODO(), request.NamespacedName, sidecarSet)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	klog.V(3).InfoS("Began to process sidecarset for reconcile", "sidecarSet", klog.KObj(sidecarSet))
	return r.processor.UpdateSidecarSet(sidecarSet)
}
