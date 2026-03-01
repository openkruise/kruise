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

package containerrecreaterequest

import (
	"context"
	"flag"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/util/slice"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/podadapter"
	utilpodreadiness "github.com/openkruise/kruise/pkg/util/podreadiness"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
)

const (
	responseTimeout = time.Minute
)

func init() {
	flag.IntVar(&concurrentReconciles, "crr-workers", concurrentReconciles, "Max concurrent workers for ContainerRecreateRequest controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("ContainerRecreateRequest")
)

// Add creates a new ContainerRecreateRequest Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) || !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileContainerRecreateRequest {
	cli := utilclient.NewClientFromManager(mgr, "containerrecreaterequest-controller")
	return &ReconcileContainerRecreateRequest{
		Client:              cli,
		clock:               clock.RealClock{},
		podReadinessControl: utilpodreadiness.NewForAdapter(&podadapter.AdapterRuntimeClient{Client: cli}),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileContainerRecreateRequest) error {
	// Create a new controller
	c, err := controller.New("containerrecreaterequest-controller", mgr, controller.Options{Reconciler: r,
		MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout()})
	if err != nil {
		return err
	}

	// Watch for changes to ContainerRecreateRequest
	k := source.Kind(mgr.GetCache(), &appsv1alpha1.ContainerRecreateRequest{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.ContainerRecreateRequest]{})
	err = c.Watch(k)
	if err != nil {
		return err
	}

	// Watch for pod for jobs that have pod selector
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.Pod{}, &podEventHandler{Reader: mgr.GetCache()}))
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileContainerRecreateRequest{}

// ReconcileContainerRecreateRequest reconciles a ContainerRecreateRequest object
type ReconcileContainerRecreateRequest struct {
	client.Client
	clock               clock.Clock
	podReadinessControl utilpodreadiness.Interface
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=containerrecreaterequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=containerrecreaterequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=containerrecreaterequests/finalizers,verbs=update

// Reconcile reads that state of the cluster for a ContainerRecreateRequest object and makes changes based on the state read
// and what is in the ContainerRecreateRequest.Spec
func (r *ReconcileContainerRecreateRequest) Reconcile(_ context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	klog.V(3).InfoS("Starting to process CRR", "containerRecreateRequest", request)
	defer func() {
		if err != nil {
			klog.ErrorS(err, "Failed to process CRR", "containerRecreateRequest", request, "elapsedTime", time.Since(start))
		} else if res.RequeueAfter > 0 {
			klog.InfoS("Finished processing CRR with scheduled retry", "containerRecreateRequest", request, "elapsedTime", time.Since(start), "retryAfter", res.RequeueAfter)
		} else {
			klog.InfoS("Finished processing CRR", "containerRecreateRequest", request, "elapsedTime", time.Since(start))
		}
	}()

	crr := &appsv1alpha1.ContainerRecreateRequest{}
	err = r.Get(context.TODO(), request.NamespacedName, crr)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	pod := &v1.Pod{}
	podErr := r.Get(context.TODO(), types.NamespacedName{Namespace: crr.Namespace, Name: crr.Spec.PodName}, pod)
	if podErr != nil && !errors.IsNotFound(podErr) {
		return reconcile.Result{}, fmt.Errorf("failed to get Pod for CRR: %v", podErr)
	}

	if crr.DeletionTimestamp != nil || crr.Status.CompletionTime != nil {
		if slice.ContainsString(crr.Finalizers, appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey, nil) {
			if err := r.releasePodNotReady(crr, pod); err != nil {
				return reconcile.Result{}, err
			}
		}

		if crr.DeletionTimestamp != nil {
			return reconcile.Result{}, nil
		}

		if _, ok := crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]; ok {
			body := fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`, appsv1alpha1.ContainerRecreateRequestActiveKey)
			return reconcile.Result{}, r.Patch(context.TODO(), crr, client.RawPatch(types.MergePatchType, []byte(body)))
		}

		var leftTime time.Duration
		if crr.Spec.TTLSecondsAfterFinished != nil {
			leftTime = time.Duration(*crr.Spec.TTLSecondsAfterFinished)*time.Second - time.Since(crr.Status.CompletionTime.Time)
			if leftTime <= 0 {
				klog.InfoS("Deleting CRR for ttlSecondsAfterFinished", "containerRecreateRequest", klog.KObj(crr))
				if err = r.Delete(context.TODO(), crr); err != nil {
					return reconcile.Result{}, fmt.Errorf("delete CRR error: %v", err)
				}
				return reconcile.Result{}, nil
			}
		}
		return reconcile.Result{RequeueAfter: leftTime}, nil
	}

	if errors.IsNotFound(podErr) || pod.DeletionTimestamp != nil || string(pod.UID) != crr.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey] {
		klog.InfoS("Completed CRR as failure for Pod has gone",
			"containerRecreateRequest", klog.KObj(crr), "podName", crr.Spec.PodName, "podUID", crr.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey])
		return reconcile.Result{}, r.completeCRR(crr, "pod has gone")
	}

	duration := requeueduration.Duration{}

	// daemon has not responded over a 1min
	if crr.Status.Phase == "" {
		leftTime := responseTimeout - time.Since(crr.CreationTimestamp.Time)
		if leftTime <= 0 {
			klog.InfoS("Completed CRR as failure for daemon has not responded for a long time", "containerRecreateRequest", klog.KObj(crr))
			return reconcile.Result{}, r.completeCRR(crr, "daemon has not responded for a long time")
		}
		duration.Update(leftTime)
	}

	// crr has running over deadline time
	if crr.Spec.ActiveDeadlineSeconds != nil {
		leftTime := time.Duration(*crr.Spec.ActiveDeadlineSeconds)*time.Second - time.Since(crr.CreationTimestamp.Time)
		if leftTime <= 0 {
			klog.InfoS("Completed CRR as failure for recreating has exceeded the activeDeadlineSeconds", "containerRecreateRequest", klog.KObj(crr))
			return reconcile.Result{}, r.completeCRR(crr, "recreating has exceeded the activeDeadlineSeconds")
		}
		duration.Update(leftTime)
	}

	if crr.Status.Phase != appsv1alpha1.ContainerRecreateRequestRecreating {
		return reconcile.Result{RequeueAfter: duration.Get()}, nil
	}

	// sync containerStatuses from Pod to CRR
	if err := r.syncContainerStatuses(crr, pod); err != nil {
		return reconcile.Result{}, fmt.Errorf("sync containerStatuses error: %v", err)
	}

	// make Pod not ready if unreadyGracePeriodSeconds has set
	if crr.Spec.Strategy.UnreadyGracePeriodSeconds != nil && crr.Annotations[appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey] == "" {
		if err = r.acquirePodNotReady(crr, pod); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{RequeueAfter: duration.Get()}, nil
}

func (r *ReconcileContainerRecreateRequest) syncContainerStatuses(crr *appsv1alpha1.ContainerRecreateRequest, pod *v1.Pod) error {
	syncContainerStatuses := make([]appsv1alpha1.ContainerRecreateRequestSyncContainerStatus, 0, len(crr.Spec.Containers))
	for i := range crr.Spec.Containers {
		c := &crr.Spec.Containers[i]
		containerStatus := util.GetContainerStatus(c.Name, pod)
		if containerStatus == nil {
			klog.InfoS("Could not find container in Pod Status for CRR", "containerName", c.Name, "containerRecreateRequest", klog.KObj(crr))
			continue
		} else if containerStatus.State.Running == nil || containerStatus.State.Running.StartedAt.Before(&crr.CreationTimestamp) {
			// ignore non-running and history status
			continue
		}
		syncContainerStatuses = append(syncContainerStatuses, appsv1alpha1.ContainerRecreateRequestSyncContainerStatus{
			Name:         containerStatus.Name,
			Ready:        containerStatus.Ready,
			RestartCount: containerStatus.RestartCount,
			ContainerID:  containerStatus.ContainerID,
		})
	}
	syncContainerStatusesStr := util.DumpJSON(syncContainerStatuses)
	if crr.Annotations[appsv1alpha1.ContainerRecreateRequestSyncContainerStatusesKey] != syncContainerStatusesStr {
		body := util.DumpJSON(syncPatchBody{Metadata: syncPatchMetadata{Annotations: map[string]string{appsv1alpha1.ContainerRecreateRequestSyncContainerStatusesKey: syncContainerStatusesStr}}})
		return r.Patch(context.TODO(), crr, client.RawPatch(types.MergePatchType, []byte(body)))
	}
	return nil
}

type syncPatchBody struct {
	Metadata syncPatchMetadata `json:"metadata"`
}

type syncPatchMetadata struct {
	Annotations map[string]string `json:"annotations"`
}

func (r *ReconcileContainerRecreateRequest) acquirePodNotReady(crr *appsv1alpha1.ContainerRecreateRequest, pod *v1.Pod) error {
	// Note that we should add the finalizer first, then update pod condition, finally patch the label

	if r.podReadinessControl.ContainsReadinessGate(pod) {
		if !slice.ContainsString(crr.Finalizers, appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey, nil) {
			err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				newCRR := &appsv1alpha1.ContainerRecreateRequest{}
				if err := r.Get(context.TODO(), types.NamespacedName{Namespace: crr.Namespace, Name: crr.Name}, newCRR); err != nil {
					return err
				}
				newCRR.Finalizers = append(newCRR.Finalizers, appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey)
				return r.Update(context.TODO(), newCRR)
			})
			if err != nil {
				return fmt.Errorf("add finalizer error: %v", err)
			}
		}

		err := r.podReadinessControl.AddNotReadyKey(pod, getReadinessMessage(crr))
		if err != nil {
			return fmt.Errorf("add Pod not ready error: %v", err)
		}
	} else {
		klog.InfoS("CRR could not set Pod to not ready, because Pod has no readinessGate",
			"containerRecreateRequest", klog.KObj(crr), "pod", klog.KObj(pod), "readinessGate", appspub.KruisePodReadyConditionType)
	}

	body := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey, r.clock.Now().Format(time.RFC3339))
	return r.Patch(context.TODO(), crr, client.RawPatch(types.MergePatchType, []byte(body)))
}

func (r *ReconcileContainerRecreateRequest) releasePodNotReady(crr *appsv1alpha1.ContainerRecreateRequest, pod *v1.Pod) error {
	if pod != nil && pod.DeletionTimestamp == nil && r.podReadinessControl.ContainsReadinessGate(pod) {
		err := r.podReadinessControl.RemoveNotReadyKey(pod, getReadinessMessage(crr))
		if err != nil {
			return fmt.Errorf("remove Pod not ready error: %v", err)
		}
	}

	if slice.ContainsString(crr.Finalizers, appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey, nil) {
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			newCRR := &appsv1alpha1.ContainerRecreateRequest{}
			if err := r.Get(context.TODO(), types.NamespacedName{Namespace: crr.Namespace, Name: crr.Name}, newCRR); err != nil {
				return err
			}
			newCRR.Finalizers = slice.RemoveString(newCRR.Finalizers, appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey, nil)
			return r.Update(context.TODO(), newCRR)
		})
		if err != nil {
			return fmt.Errorf("remove finalizer error: %v", err)
		}
	}
	return nil
}

func (r *ReconcileContainerRecreateRequest) completeCRR(crr *appsv1alpha1.ContainerRecreateRequest, msg string) error {
	now := metav1.NewTime(r.clock.Now())
	crr.Status.Phase = appsv1alpha1.ContainerRecreateRequestCompleted
	crr.Status.CompletionTime = &now
	crr.Status.Message = msg
	return r.Status().Update(context.TODO(), crr)
}

func getReadinessMessage(crr *appsv1alpha1.ContainerRecreateRequest) utilpodreadiness.Message {
	return utilpodreadiness.Message{UserAgent: "ContainerRecreateRequest", Key: fmt.Sprintf("%s/%s", crr.Namespace, crr.Name)}
}
