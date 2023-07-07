/*
Copyright 2023 The Kruise Authors.

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

package sidecarterminator

import (
	"context"
	"flag"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func init() {
	flag.IntVar(&concurrentReconciles, "sidecarterminator-workers", concurrentReconciles, "Max concurrent workers for SidecarTerminator controller.")
}

var (
	concurrentReconciles = 3
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new SidecarTerminator Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utilfeature.DefaultFeatureGate.Enabled(features.SidecarTerminator) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := utilclient.NewClientFromManager(mgr, "sidecarterminator-controller")
	recorder := mgr.GetEventRecorderFor("sidecarterminator-controller")
	return &ReconcileSidecarTerminator{
		Client:   cli,
		recorder: recorder,
		scheme:   mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("sidecarterminator-controller", mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: concurrentReconciles,
		CacheSyncTimeout:        util.GetControllerCacheSyncTimeout(),
		RateLimiter:             ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	if err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &enqueueRequestForPod{}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSidecarTerminator{}

// ReconcileSidecarTerminator reconciles a SidecarTerminator object
type ReconcileSidecarTerminator struct {
	client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
}

// Reconcile get the pod whose sidecar containers should be stopped, and stop them.
func (r *ReconcileSidecarTerminator) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	pod := &corev1.Pod{}
	err := r.Get(context.TODO(), request.NamespacedName, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	start := time.Now()
	klog.V(3).Infof("SidecarTerminator -- begin to process pod %v for reconcile", request.NamespacedName)
	defer func() {
		klog.V(3).Infof("SidecarTerminator -- process pod %v done, cost: %v", request.NamespacedName, time.Since(start))
	}()

	return r.doReconcile(pod)
}

func (r *ReconcileSidecarTerminator) doReconcile(pod *corev1.Pod) (reconcile.Result, error) {
	if !isInterestingPod(pod) {
		return reconcile.Result{}, nil
	}

	if containersCompleted(pod, getSidecar(pod)) {
		klog.V(3).Infof("SidecarTerminator -- all sidecars of pod(%v/%v) have been completed, no need to process", pod.Namespace, pod.Name)
		return reconcile.Result{}, nil
	}

	if pod.Spec.RestartPolicy == corev1.RestartPolicyOnFailure && !containersSucceeded(pod, getMain(pod)) {
		klog.V(3).Infof("SidecarTerminator -- pod(%v/%v) is trying to restart, no need to process", pod.Namespace, pod.Name)
		return reconcile.Result{}, nil
	}

	sidecarNeedToExecuteKillContainer, sidecarNeedToExecuteInPlaceUpdate, err := r.groupSidecars(pod)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.executeInPlaceUpdateAction(pod, sidecarNeedToExecuteInPlaceUpdate); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.executeKillContainerAction(pod, sidecarNeedToExecuteKillContainer); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileSidecarTerminator) groupSidecars(pod *corev1.Pod) (sets.String, sets.String, error) {
	runningOnVK, err := IsPodRunningOnVirtualKubelet(pod, r.Client)
	if err != nil {
		return nil, nil, client.IgnoreNotFound(err)
	}

	inPlaceUpdate := sets.NewString()
	killContainer := sets.NewString()
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		for j := range container.Env {
			if !runningOnVK && container.Env[j].Name == appsv1alpha1.KruiseTerminateSidecarEnv &&
				strings.EqualFold(container.Env[j].Value, "true") {
				killContainer.Insert(container.Name)
				break
			}
			if container.Env[j].Name == appsv1alpha1.KruiseTerminateSidecarWithImageEnv &&
				container.Env[j].Value != "" {
				inPlaceUpdate.Insert(container.Name)
			}
		}
	}
	return killContainer, inPlaceUpdate, nil
}

func containersCompleted(pod *corev1.Pod, containers sets.String) bool {
	if len(pod.Spec.Containers) != len(pod.Status.ContainerStatuses) {
		return false
	}

	for i := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[i]
		if containers.Has(status.Name) && status.State.Terminated == nil {
			return false
		}
	}
	return true
}

func containersSucceeded(pod *corev1.Pod, containers sets.String) bool {
	if len(pod.Spec.Containers) != len(pod.Status.ContainerStatuses) {
		return false
	}

	for i := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[i]
		if containers.Has(status.Name) &&
			(status.State.Terminated == nil || status.State.Terminated.ExitCode != int32(0)) {
			return false
		}
	}
	return true
}
