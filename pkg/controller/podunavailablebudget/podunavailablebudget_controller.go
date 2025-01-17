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

package podunavailablebudget

import (
	"context"
	"flag"
	"fmt"
	"time"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kruiseappsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseappsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	kubeClient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "podunavailablebudget-workers", concurrentReconciles, "Max concurrent workers for PodUnavailableBudget controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = policyv1alpha1.SchemeGroupVersion.WithKind("PodUnavailableBudget")
)

const (
	DeletionTimeout       = 20 * time.Second
	UpdatedDelayCheckTime = 10 * time.Second
)

var ConflictRetry = wait.Backoff{
	Steps:    4,
	Duration: 500 * time.Millisecond,
	Factor:   1.0,
	Jitter:   0.1,
}

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PodUnavailableBudget Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	if !utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetDeleteGate) &&
		!utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetUpdateGate) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePodUnavailableBudget{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		recorder:         mgr.GetEventRecorderFor("podunavailablebudget-controller"),
		controllerFinder: controllerfinder.Finder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("podunavailablebudget-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to PodUnavailableBudget
	err = c.Watch(source.Kind(mgr.GetCache(), &policyv1alpha1.PodUnavailableBudget{}, &handler.TypedEnqueueRequestForObject[*policyv1alpha1.PodUnavailableBudget]{}))
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, newEnqueueRequestForPod(mgr.GetClient()))); err != nil {
		return err
	}

	// In workload scaling scenario, there is a risk of interception by the pub webhook against the scaled pod.
	// The solution for this scenario: the pub controller listens to workload replicas changes and adjusts UnavailableAllowed in time.
	// Example for:
	// 1. cloneSet.replicas = 100, pub.MaxUnavailable = 10%, then UnavailableAllowed=10.
	// 2. at this time the cloneSet.replicas is scaled down to 50, the pub controller listens to the replicas change, triggering reconcile will adjust UnavailableAllowed to 55.
	// 3. so pub webhook will not intercept the request to delete the pods
	// deployment
	if err = c.Watch(source.Kind(mgr.GetCache(), client.Object(&apps.Deployment{}), &SetEnqueueRequestForPUB{mgr}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*apps.Deployment)
			new := e.ObjectNew.(*apps.Deployment)
			return *old.Spec.Replicas != *new.Spec.Replicas
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return true
		},
	})); err != nil {
		return err
	}

	// kruise AdvancedStatefulSet
	if err = c.Watch(source.Kind(mgr.GetCache(), client.Object(&kruiseappsv1beta1.StatefulSet{}), &SetEnqueueRequestForPUB{mgr}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*kruiseappsv1beta1.StatefulSet)
			new := e.ObjectNew.(*kruiseappsv1beta1.StatefulSet)
			return *old.Spec.Replicas != *new.Spec.Replicas
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return true
		},
	})); err != nil {
		return err
	}

	// CloneSet
	if err = c.Watch(source.Kind(mgr.GetCache(), client.Object(&kruiseappsv1alpha1.CloneSet{}), &SetEnqueueRequestForPUB{mgr}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*kruiseappsv1alpha1.CloneSet)
			new := e.ObjectNew.(*kruiseappsv1alpha1.CloneSet)
			return *old.Spec.Replicas != *new.Spec.Replicas
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return true
		},
	})); err != nil {
		return err
	}

	// StatefulSet
	if err = c.Watch(source.Kind(mgr.GetCache(), client.Object(&apps.StatefulSet{}), &SetEnqueueRequestForPUB{mgr}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*apps.StatefulSet)
			new := e.ObjectNew.(*apps.StatefulSet)
			return *old.Spec.Replicas != *new.Spec.Replicas
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return true
		},
	})); err != nil {
		return err
	}

	klog.InfoS("Added podunavailablebudget reconcile.Reconciler success")
	return nil
}

var _ reconcile.Reconciler = &ReconcilePodUnavailableBudget{}

// ReconcilePodUnavailableBudget reconciles a PodUnavailableBudget object
type ReconcilePodUnavailableBudget struct {
	client.Client
	Scheme           *runtime.Scheme
	recorder         record.EventRecorder
	controllerFinder *controllerfinder.ControllerFinder
}

// +kubebuilder:rbac:groups=policy.kruise.io,resources=podunavailablebudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy.kruise.io,resources=podunavailablebudgets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=policy.kruise.io,resources=podunavailablebudgets/finalizers,verbs=update
// +kubebuilder:rbac:groups=*,resources=*/scale,verbs=get;list;watch

// pkg/controller/cloneset/cloneset_controller.go Watch for changes to CloneSet
func (r *ReconcilePodUnavailableBudget) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the PodUnavailableBudget instance
	pub := &policyv1alpha1.PodUnavailableBudget{}
	err := r.Get(context.TODO(), req.NamespacedName, pub)
	if (err != nil && errors.IsNotFound(err)) || (err == nil && !pub.DeletionTimestamp.IsZero()) {
		klog.V(3).InfoS("PodUnavailableBudget is Deletion in this time", "podUnavailableBudget", req)
		if cacheErr := util.GlobalCache.Delete(&policyv1alpha1.PodUnavailableBudget{
			TypeMeta: metav1.TypeMeta{
				APIVersion: policyv1alpha1.GroupVersion.String(),
				Kind:       controllerKind.Kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
			},
		}); cacheErr != nil {
			klog.ErrorS(err, "Deleted cache failed for PodUnavailableBudget", "podUnavailableBudget", req)
		}
		// Object not found, return.  Created objects are automatically garbage collected.
		// For additional cleanup logic use finalizers.
		return reconcile.Result{}, nil
	} else if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	klog.V(3).InfoS("Began to process PodUnavailableBudget", "podUnavailableBudget", klog.KObj(pub))
	recheckTime, err := r.syncPodUnavailableBudget(pub)
	if err != nil {
		return ctrl.Result{}, err
	}
	if recheckTime != nil {
		return ctrl.Result{RequeueAfter: time.Until(*recheckTime)}, nil
	}
	return ctrl.Result{}, nil
}

func (r *ReconcilePodUnavailableBudget) syncPodUnavailableBudget(pub *policyv1alpha1.PodUnavailableBudget) (*time.Time, error) {
	currentTime := time.Now()
	pods, expectedCount, err := pubcontrol.PubControl.GetPodsForPub(pub)
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		r.recorder.Eventf(pub, corev1.EventTypeNormal, "NoPods", "No matching pods found")
	} else {
		// patch related-pub annotation in all pods of workload
		if err = r.patchRelatedPubAnnotationInPod(pub, pods); err != nil {
			klog.ErrorS(err, "PodUnavailableBudget patch pod annotation failed", "podUnavailableBudget", klog.KObj(pub))
			return nil, err
		}
	}

	klog.V(3).InfoS("PodUnavailableBudget controller pods expectedCount", "podUnavailableBudget", klog.KObj(pub), "podCount", len(pods), "expectedCount", expectedCount)
	desiredAvailable, err := r.getDesiredAvailableForPub(pub, expectedCount)
	if err != nil {
		r.recorder.Eventf(pub, corev1.EventTypeWarning, "CalculateExpectedPodCountFailed", "Failed to calculate the number of expected pods: %v", err)
		return nil, err
	}

	// for debug
	var conflictTimes int
	var costOfGet, costOfUpdate time.Duration
	var pubClone *policyv1alpha1.PodUnavailableBudget
	refresh := false
	var recheckTime *time.Time
	err = retry.RetryOnConflict(ConflictRetry, func() error {
		unlock := util.GlobalKeyedMutex.Lock(string(pub.UID))
		defer unlock()

		start := time.Now()
		if refresh {
			// fetch pub from etcd
			pubClone, err = kubeClient.GetGenericClient().KruiseClient.PolicyV1alpha1().
				PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
			if err != nil {
				klog.ErrorS(err, "Failed to get PodUnavailableBudget from etcd", "podUnavailableBudget", klog.KObj(pub))
				return err
			}
		} else {
			// compare local cache and informer cache, then get the newer one
			item, _, err := util.GlobalCache.Get(pub)
			if err != nil {
				klog.ErrorS(err, "Failed to get PodUnavailableBudget cache", "podUnavailableBudget", klog.KObj(pub))
			}
			if localCached, ok := item.(*policyv1alpha1.PodUnavailableBudget); ok {
				pubClone = localCached.DeepCopy()
			} else {
				pubClone = pub.DeepCopy()
			}
			informerCached := &policyv1alpha1.PodUnavailableBudget{}
			if err := r.Get(context.TODO(), types.NamespacedName{Namespace: pub.Namespace,
				Name: pub.Name}, informerCached); err == nil {
				var localRV, informerRV int64
				_ = runtime.Convert_string_To_int64(&pubClone.ResourceVersion, &localRV, nil)
				_ = runtime.Convert_string_To_int64(&informerCached.ResourceVersion, &informerRV, nil)
				if informerRV > localRV {
					pubClone = informerCached
				}
			}
		}
		costOfGet += time.Since(start)

		// disruptedPods contains information about pods whose eviction or deletion was processed by the API handler but has not yet been observed by the PodUnavailableBudget.
		// unavailablePods contains information about pods whose specification changed(in-place update), in case of informer cache latency, after 5 seconds to remove it.
		var disruptedPods, unavailablePods map[string]metav1.Time
		disruptedPods, unavailablePods, recheckTime = r.buildDisruptedAndUnavailablePods(pods, pubClone, currentTime)
		currentAvailable := countAvailablePods(pods, disruptedPods, unavailablePods)

		start = time.Now()
		updateErr := r.updatePubStatus(pubClone, currentAvailable, desiredAvailable, expectedCount, disruptedPods, unavailablePods)
		costOfUpdate += time.Since(start)
		if updateErr == nil {
			return nil
		}
		// update failed, and retry
		refresh = true
		conflictTimes++
		return updateErr
	})
	klog.V(3).InfoS("Controller cost of PodUnavailableBudget", "podUnavailableBudget", klog.KObj(pub), "conflictTimes", conflictTimes,
		"costOfGet", costOfGet, "costOfUpdate", costOfUpdate)
	if err != nil {
		klog.ErrorS(err, "Failed to update PodUnavailableBudget status", "podUnavailableBudget", klog.KObj(pub))
	}
	return recheckTime, err
}

func (r *ReconcilePodUnavailableBudget) patchRelatedPubAnnotationInPod(pub *policyv1alpha1.PodUnavailableBudget, pods []*corev1.Pod) error {
	var updatedPods []*corev1.Pod
	for i := range pods {
		if pods[i].Annotations[pubcontrol.PodRelatedPubAnnotation] == "" {
			updatedPods = append(updatedPods, pods[i].DeepCopy())
		}
	}
	if len(updatedPods) == 0 {
		return nil
	}
	// update related-pub annotation in pods
	for _, pod := range updatedPods {
		body := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, pubcontrol.PodRelatedPubAnnotation, pub.Name)
		if err := r.Patch(context.TODO(), pod, client.RawPatch(types.StrategicMergePatchType, []byte(body))); err != nil {
			return err
		}
	}
	klog.V(3).InfoS("Patched PodUnavailableBudget old pods related-pub annotation success", "podUnavailableBudget", klog.KObj(pub), "podCount", len(updatedPods))
	return nil
}

func countAvailablePods(pods []*corev1.Pod, disruptedPods, unavailablePods map[string]metav1.Time) (currentAvailable int32) {
	recordPods := sets.String{}
	for pName := range disruptedPods {
		recordPods.Insert(pName)
	}
	for pName := range unavailablePods {
		recordPods.Insert(pName)
	}

	for _, pod := range pods {
		if !kubecontroller.IsPodActive(pod) {
			continue
		}
		// ignore disrupted or unavailable pods, where the Pod is considered unavailable
		if recordPods.Has(pod.Name) {
			continue
		}
		// pod consistent and ready
		if pubcontrol.PubControl.IsPodStateConsistent(pod) && pubcontrol.PubControl.IsPodReady(pod) {
			currentAvailable++
		}
	}

	return
}

func (r *ReconcilePodUnavailableBudget) getDesiredAvailableForPub(pub *policyv1alpha1.PodUnavailableBudget, expectedCount int32) (desiredAvailable int32, err error) {
	if pub.Spec.MaxUnavailable != nil {
		var maxUnavailable int
		maxUnavailable, err = intstr.GetScaledValueFromIntOrPercent(pub.Spec.MaxUnavailable, int(expectedCount), true)
		if err != nil {
			return
		}

		desiredAvailable = expectedCount - int32(maxUnavailable)
		if desiredAvailable < 0 {
			desiredAvailable = 0
		}
	} else if pub.Spec.MinAvailable != nil {
		if pub.Spec.MinAvailable.Type == intstr.Int {
			desiredAvailable = pub.Spec.MinAvailable.IntVal
		} else if pub.Spec.MinAvailable.Type == intstr.String {
			var minAvailable int
			minAvailable, err = intstr.GetScaledValueFromIntOrPercent(pub.Spec.MinAvailable, int(expectedCount), true)
			if err != nil {
				return
			}
			desiredAvailable = int32(minAvailable)
		}
	}
	return
}

func (r *ReconcilePodUnavailableBudget) buildDisruptedAndUnavailablePods(pods []*corev1.Pod, pub *policyv1alpha1.PodUnavailableBudget, currentTime time.Time) (
	// disruptedPods, unavailablePods, recheckTime
	map[string]metav1.Time, map[string]metav1.Time, *time.Time) {

	disruptedPods := pub.Status.DisruptedPods
	unavailablePods := pub.Status.UnavailablePods

	resultDisruptedPods := make(map[string]metav1.Time)
	resultUnavailablePods := make(map[string]metav1.Time)
	var recheckTime *time.Time

	if disruptedPods == nil && unavailablePods == nil {
		return resultDisruptedPods, resultUnavailablePods, recheckTime
	}
	for _, pod := range pods {
		if !kubecontroller.IsPodActive(pod) {
			continue
		}

		// handle disruption pods which will be eviction or deletion
		disruptionTime, found := disruptedPods[pod.Name]
		if found {
			expectedDeletion := disruptionTime.Time.Add(DeletionTimeout)
			if expectedDeletion.Before(currentTime) {
				r.recorder.Eventf(pod, corev1.EventTypeWarning, "NotDeleted", "Pod was expected by PUB %s/%s to be deleted but it wasn't",
					pub.Namespace, pub.Name)
			} else {
				resultDisruptedPods[pod.Name] = disruptionTime
				if recheckTime == nil || expectedDeletion.Before(*recheckTime) {
					recheckTime = &expectedDeletion
				}
			}
		}

		// handle unavailable pods which have been in-updated specification
		unavailableTime, found := unavailablePods[pod.Name]
		if found {
			// in case of informer cache latency, after 10 seconds to remove it
			expectedUpdate := unavailableTime.Time.Add(UpdatedDelayCheckTime)
			if expectedUpdate.Before(currentTime) {
				continue
			} else {
				resultUnavailablePods[pod.Name] = unavailableTime
				if recheckTime == nil || expectedUpdate.Before(*recheckTime) {
					recheckTime = &expectedUpdate
				}
			}

		}
	}
	return resultDisruptedPods, resultUnavailablePods, recheckTime
}

func (r *ReconcilePodUnavailableBudget) updatePubStatus(pub *policyv1alpha1.PodUnavailableBudget, currentAvailable, desiredAvailable, expectedCount int32,
	disruptedPods, unavailablePods map[string]metav1.Time) error {

	unavailableAllowed := currentAvailable - desiredAvailable
	if unavailableAllowed <= 0 {
		unavailableAllowed = 0
	}

	if pub.Status.CurrentAvailable == currentAvailable &&
		pub.Status.DesiredAvailable == desiredAvailable &&
		pub.Status.TotalReplicas == expectedCount &&
		pub.Status.UnavailableAllowed == unavailableAllowed &&
		pub.Status.ObservedGeneration == pub.Generation &&
		apiequality.Semantic.DeepEqual(pub.Status.DisruptedPods, disruptedPods) &&
		apiequality.Semantic.DeepEqual(pub.Status.UnavailablePods, unavailablePods) {
		return nil
	}

	pub.Status = policyv1alpha1.PodUnavailableBudgetStatus{
		CurrentAvailable:   currentAvailable,
		DesiredAvailable:   desiredAvailable,
		TotalReplicas:      expectedCount,
		UnavailableAllowed: unavailableAllowed,
		DisruptedPods:      disruptedPods,
		UnavailablePods:    unavailablePods,
		ObservedGeneration: pub.Generation,
	}
	err := r.Client.Status().Update(context.TODO(), pub)
	if err != nil {
		return err
	}
	if err = util.GlobalCache.Add(pub); err != nil {
		klog.ErrorS(err, "Added cache failed for PodUnavailableBudget", "podUnavailableBudget", klog.KObj(pub))
	}
	klog.V(3).InfoS("PodUnavailableBudget update status", "podUnavailableBudget", klog.KObj(pub), "disruptedPods", len(disruptedPods), "unavailablePods", len(unavailablePods),
		"expectedCount", expectedCount, "desiredAvailable", desiredAvailable, "currentAvailable", currentAvailable, "unavailableAllowed", unavailableAllowed)
	return nil
}
