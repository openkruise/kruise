/*
Copyright 2020 The Kruise Authors.

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

package advancedcronjob

import (
	"context"
	"flag"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
)

type IndexerFunc func(manager.Manager) error

func init() {
	flag.IntVar(&concurrentReconciles, "advancedcronjob-workers", concurrentReconciles, "Max concurrent workers for AdvancedCronJob controller.")
}

var (
	concurrentReconciles = 3
	jobOwnerKey          = ".metadata.controller"
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("AdvancedCronJob")
)

// Add creates a new AdvancedCronJob Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor("advancedcronjob-controller")
	return &ReconcileAdvancedCronJob{
		Client:   utilclient.NewClientFromManager(mgr, "advancedcronjob-controller"),
		scheme:   mgr.GetScheme(),
		recorder: recorder,
		Clock:    realClock{},
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	klog.InfoS("Starting AdvancedCronJob Controller")
	c, err := controller.New("advancedcronjob-controller", mgr, controller.Options{Reconciler: r,
		MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout()})
	if err != nil {
		klog.ErrorS(err, "Failed to create AdvandedCronJob controller")
		return err
	}

	// Watch for changes to AdvancedCronJob
	src := source.Kind(mgr.GetCache(), &appsv1alpha1.AdvancedCronJob{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.AdvancedCronJob]{})
	if err = c.Watch(src); err != nil {
		klog.ErrorS(err, "Failed to watch AdvancedCronJob")
		return err
	}

	if err = watchJob(mgr, c); err != nil {
		klog.ErrorS(err, "Failed to watch Job")
		return err
	}

	if err = watchBroadcastJob(mgr, c); err != nil {
		klog.ErrorS(err, "Failed to watch BroadcastJob")
		return err
	}
	return nil
}

type realClock struct{}

func (r realClock) Now() time.Time { return time.Now() }

// clock knows how to get the current time.
// It can be used to fake out timing for testing.
type Clock interface {
	Now() time.Time
}

var (
	scheduledTimeAnnotation = "apps.kruise.io/scheduled-at"
)

var _ reconcile.Reconciler = &ReconcileAdvancedCronJob{}

// ReconcileAdvancedCronJob reconciles a AdvancedCronJob object
type ReconcileAdvancedCronJob struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	Clock
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=advancedcronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=advancedcronjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=advancedcronjobs/finalizers,verbs=update

func (r *ReconcileAdvancedCronJob) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	klog.InfoS("Running AdvancedCronJob job", "advancedCronJob", req)

	namespacedName := types.NamespacedName{
		Namespace: req.Namespace,
		Name:      req.Name,
	}

	var advancedCronJob appsv1alpha1.AdvancedCronJob

	if err := r.Get(ctx, namespacedName, &advancedCronJob); err != nil {
		klog.ErrorS(err, "Unable to fetch CronJob", "advancedCronJob", req)
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	switch FindTemplateKind(advancedCronJob.Spec) {
	case appsv1alpha1.JobTemplate:
		return r.reconcileJob(ctx, req, advancedCronJob)
	case appsv1alpha1.BroadcastJobTemplate:
		return r.reconcileBroadcastJob(ctx, req, advancedCronJob)
	default:
		klog.InfoS("No template found", "advancedCronJob", req)
	}

	return ctrl.Result{}, nil
}

func (r *ReconcileAdvancedCronJob) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.AdvancedCronJob{}).
		Complete(r)
}

func (r *ReconcileAdvancedCronJob) updateAdvancedJobStatus(request reconcile.Request, advancedCronJob *appsv1alpha1.AdvancedCronJob) error {
	klog.V(1).InfoS("Updating job status", "advancedCronJob", klog.KObj(advancedCronJob), "status", advancedCronJob.Status)
	advancedCronJobCopy := advancedCronJob.DeepCopy()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := r.Status().Update(context.TODO(), advancedCronJobCopy)
		if err == nil {
			return nil
		}

		updated := &appsv1alpha1.AdvancedCronJob{}
		err = r.Get(context.TODO(), request.NamespacedName, updated)
		if err == nil {
			advancedCronJobCopy = updated
			advancedCronJobCopy.Status = advancedCronJob.Status
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated advancedCronJob %s/%s from lister: %v", advancedCronJob.Namespace, advancedCronJob.Name, err))
		}
		return err
	})
}
