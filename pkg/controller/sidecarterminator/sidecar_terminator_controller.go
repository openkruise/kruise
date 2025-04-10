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
	"encoding/json"
	"flag"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "sidecarterminator-workers", concurrentReconciles, "Max concurrent workers for SidecarTerminator controller.")
}

var (
	concurrentReconciles                         = 3
	SidecarTerminated    corev1.PodConditionType = "SidecarTerminated"
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
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}), &enqueueRequestForPod{}); err != nil {
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
	klog.V(3).InfoS("SidecarTerminator -- begin to process pod for reconcile", "pod", request)
	defer func() {
		klog.V(3).InfoS("SidecarTerminator -- process pod done", "pod", request, "cost", time.Since(start))
	}()

	return r.doReconcile(pod)
}

func (r *ReconcileSidecarTerminator) doReconcile(pod *corev1.Pod) (reconcile.Result, error) {
	if !isInterestingPod(pod) {
		return reconcile.Result{}, nil
	}
	vk, err := IsPodRunningOnVirtualKubelet(pod, r.Client)
	if err != nil {
		klog.ErrorS(err, "SidecarTerminator -- Error occurred when try to check if pod is running on virtual-kubelet", "pod", klog.KObj(pod))
		return reconcile.Result{}, err
	}
	normalSidecarNames, ignoreExitCodeSidecarNames, inplaceUpdateSidecarNames := getSidecarContainerNames(pod, vk)
	if err = r.executeInPlaceUpdateAction(pod, inplaceUpdateSidecarNames); err != nil {
		return reconcile.Result{}, err
	}
	if err = r.executeKillContainerAction(pod, normalSidecarNames); err != nil {
		return reconcile.Result{}, err
	}
	if ignoreExitCodeSidecarNames.Len() == 0 || !containersCompleted(pod, normalSidecarNames) || !containersCompleted(pod, inplaceUpdateSidecarNames) {
		return reconcile.Result{}, nil
	}
	if err := r.markJobPodTerminated(pod); err != nil {
		return reconcile.Result{}, err
	}
	// JobSidecarTerminator relies on the exit code of the sidecar to be zero. If it is non-zero, it will result in a Pod Status of Failed.
	// To solve this problem, we can set the Pod Status to Completed in advance.
	// However, the above will also have some problems. For example, there are some CSI and ENI controllers that recycle resources based on Pod completion.
	// If you set it in advance, the scheduler will think that the resources have been recovered, but the bottom reason is that the resources are still being used by the previous Pod,
	// which will result in resource conflict, and then the new Pod startup will fail.
	// Because of this, we open a new ENV KRUISE_TERMINATE_SIDECAR_IGNORE_EXIT_CODE to open this ability, the user also need to understand the risk behind, before deciding whether to use.
	if err := r.executeKillContainerAction(pod, ignoreExitCodeSidecarNames); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// markJobPodTerminated terminate the job pod and skip the state of the sidecar containers
// This method should only be called before the executeKillContainerAction
func (r *ReconcileSidecarTerminator) markJobPodTerminated(pod *corev1.Pod) error {
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		return nil
	}

	// after the pod is terminated by the sidecar terminator, kubelet will kill the containers that are not in the terminal phase
	// 1. sidecar container terminate with non-zero exit code
	// 2. sidecar container is not in a terminal phase (still running or waiting)
	klog.V(3).InfoS("All of the main containers are completed, will terminate the job pod", "pod", klog.KObj(pod))
	// terminate the pod, ignore the status of the sidecar containers.
	// in kubelet,pods are not allowed to transition out of terminal phases.

	// patch pod condition
	status := corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:               SidecarTerminated,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Message:            "Terminated by Sidecar Terminator",
			},
		},
	}

	mainContainers, _ := groupMainSidecarContainers(pod)
	// patch pod phase
	if containersSucceeded(pod, mainContainers) {
		status.Phase = corev1.PodSucceeded
	} else {
		status.Phase = corev1.PodFailed
	}
	klog.V(3).InfoS("Terminated the job pod", "pod", klog.KObj(pod), "phase", status.Phase)

	by, _ := json.Marshal(status)
	patchCondition := fmt.Sprintf(`{"status":%s}`, string(by))
	rcvObject := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name}}

	if err := r.Status().Patch(context.TODO(), rcvObject, client.RawPatch(types.StrategicMergePatchType, []byte(patchCondition))); err != nil {
		return fmt.Errorf("failed to patch pod status: %v", err)
	}

	return nil
}

func containersCompleted(pod *corev1.Pod, containers sets.Set[string]) bool {
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

func containersSucceeded(pod *corev1.Pod, containers sets.Set[string]) bool {
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
