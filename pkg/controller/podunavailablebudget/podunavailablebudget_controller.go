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
	"strings"
	"time"

	kruiseappsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseappsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	kubeClient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

	// patch related-pub annotation Reconcile prefix
	PatchPubAnnotationReconcilePrefix = "patch#"
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
		controllerFinder: controllerfinder.NewControllerFinder(mgr.GetClient()),
		pubControl:       pubcontrol.NewPubControl(mgr.GetClient()),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("podunavailablebudget-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to PodUnavailableBudget
	err = c.Watch(&source.Kind{Type: &policyv1alpha1.PodUnavailableBudget{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	if err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, newEnqueueRequestForPod(mgr.GetClient())); err != nil {
		return err
	}

	//In workload scaling scenario, there is a risk of interception by the pub webhook against the scaled pod.
	//The solution for this scenario: the pub controller listens to workload replicas changes and adjusts UnavailableAllowed in time.
	// Example for:
	// 1. cloneSet.replicas = 100, pub.MaxUnavailable = 10%, then UnavailableAllowed=10.
	// 2. at this time the cloneSet.replicas is scaled down to 50, the pub controller listens to the replicas change, triggering reconcile will adjust UnavailableAllowed to 55.
	// 3. so pub webhook will not intercept the request to delete the pods
	// deployment
	if err = c.Watch(&source.Kind{Type: &apps.Deployment{}}, &SetEnqueueRequestForPUB{mgr}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*apps.Deployment)
			new := e.ObjectNew.(*apps.Deployment)
			if *old.Spec.Replicas != *new.Spec.Replicas {
				return true
			}
			return false
		},
	}); err != nil {
		return err
	}

	//kruise AdvancedStatefulSet
	if err = c.Watch(&source.Kind{Type: &kruiseappsv1beta1.StatefulSet{}}, &SetEnqueueRequestForPUB{mgr}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*kruiseappsv1beta1.StatefulSet)
			new := e.ObjectNew.(*kruiseappsv1beta1.StatefulSet)
			if *old.Spec.Replicas != *new.Spec.Replicas {
				return true
			}
			return false
		},
	}); err != nil {
		return err
	}

	//CloneSet
	if err = c.Watch(&source.Kind{Type: &kruiseappsv1alpha1.CloneSet{}}, &SetEnqueueRequestForPUB{mgr}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*kruiseappsv1alpha1.CloneSet)
			new := e.ObjectNew.(*kruiseappsv1alpha1.CloneSet)
			if *old.Spec.Replicas != *new.Spec.Replicas {
				return true
			}
			return false
		},
	}); err != nil {
		return err
	}

	//StatefulSet
	if err = c.Watch(&source.Kind{Type: &apps.StatefulSet{}}, &SetEnqueueRequestForPUB{mgr}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*apps.StatefulSet)
			new := e.ObjectNew.(*apps.StatefulSet)
			if *old.Spec.Replicas != *new.Spec.Replicas {
				return true
			}
			return false
		},
	}); err != nil {
		return err
	}

	klog.Infof("add podunavailablebudget reconcile.Reconciler success")
	return nil
}

var _ reconcile.Reconciler = &ReconcilePodUnavailableBudget{}

// ReconcilePodUnavailableBudget reconciles a PodUnavailableBudget object
type ReconcilePodUnavailableBudget struct {
	client.Client
	Scheme           *runtime.Scheme
	recorder         record.EventRecorder
	controllerFinder *controllerfinder.ControllerFinder
	pubControl       pubcontrol.PubControl
}

// +kubebuilder:rbac:groups=policy.kruise.io,resources=podunavailablebudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy.kruise.io,resources=podunavailablebudgets/status,verbs=get;update;patch

// pkg/controller/cloneset/cloneset_controller.go Watch for changes to CloneSet
func (r *ReconcilePodUnavailableBudget) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	// patch related-pub annotation in  pods
	if strings.Contains(req.Name, PatchPubAnnotationReconcilePrefix) {
		if err := r.patchRelatedPubAnnotationInPod(req); err != nil {
			klog.Errorf("PodUnavailableBudget patch related-pub annotation in workload(%s/%s)'s pods failed: %s", req.Namespace, req.Name, err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Fetch the PodUnavailableBudget instance
	pub := &policyv1alpha1.PodUnavailableBudget{}
	err := r.Get(context.TODO(), req.NamespacedName, pub)
	if (err != nil && errors.IsNotFound(err)) || (err == nil && !pub.DeletionTimestamp.IsZero()) {
		klog.V(3).Infof("pub(%s/%s) is Deletion in this time", req.Namespace, req.Name)
		if cacheErr := util.GlobalCache.Delete(&policyv1alpha1.PodUnavailableBudget{
			TypeMeta: metav1.TypeMeta{
				APIVersion: policyv1alpha1.GroupVersion.String(),
				Kind:       "PodUnavailableBudget",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
			},
		}); cacheErr != nil {
			klog.Errorf("Delete cache failed for PodUnavailableBudget(%s/%s): %s", req.Namespace, req.Name, err.Error())
		}
		// Object not found, return.  Created objects are automatically garbage collected.
		// For additional cleanup logic use finalizers.
		return reconcile.Result{}, nil
	} else if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	klog.V(3).Infof("begin to process podUnavailableBudget(%s/%s)", pub.Namespace, pub.Name)
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
	pods, expectedCount, err := r.pubControl.GetPodsForPub(pub)
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		r.recorder.Eventf(pub, corev1.EventTypeNormal, "NoPods", "No matching pods found")
	}

	klog.V(3).Infof("pub(%s/%s) controller pods(%d) expectedCount(%d)", pub.Namespace, pub.Name, len(pods), expectedCount)
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
				klog.Errorf("Get PodUnavailableBudget(%s/%s) failed from etcd: %s", pub.Namespace, pub.Name, err.Error())
				return err
			}
		} else {
			// compare local cache and informer cache, then get the newer one
			item, _, err := util.GlobalCache.Get(pub)
			if err != nil {
				klog.Errorf("Get PodUnavailableBudget(%s/%s) cache failed: %s", pub.Namespace, pub.Name, err.Error())
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
		currentAvailable := countAvailablePods(pods, disruptedPods, unavailablePods, r.pubControl)

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
	klog.V(3).Infof("Controller cost of pub(%s/%s): conflict times %v, cost of Get %v, cost of Update %v",
		pub.Namespace, pub.Name, conflictTimes, costOfGet, costOfUpdate)
	if err != nil {
		klog.Errorf("update pub(%s/%s) status failed: %s", pub.Namespace, pub.Name, err.Error())
	}
	return recheckTime, err
}

// patch related-pub annotation in all pods of workload
func (r *ReconcilePodUnavailableBudget) patchRelatedPubAnnotationInPod(req ctrl.Request) error {
	// req.Name Format = patch#{apiVersion}#{workload.Kind}#{workload.Name}
	// example for patch#apps/v1#Deployment#echoserver
	arr := strings.Split(req.Name, "#")
	if len(arr) != 4 {
		klog.Warningf("Reconcile PodUnavailableBudget workload(%s) is invalid", req.Name)
		return nil
	}
	// fetch workload
	apiVersion, kind, ns, name := arr[1], arr[2], req.Namespace, arr[3]
	workload, err := r.controllerFinder.GetScaleAndSelectorForRef(apiVersion, kind, ns, name, "")
	if err != nil {
		return err
	} else if workload == nil {
		klog.Warningf("Reconcile PodUnavailableBudget workload(%s) is Not Found", req.Name)
		return nil
	}

	// fetch pub for workload
	pub, err := r.getPubForWorkload(workload)
	if err != nil || pub == nil {
		return err
	}
	pods, _, err := r.controllerFinder.GetPodsForRef(workload.APIVersion, workload.Kind, workload.Name, workload.Metadata.Namespace, true)
	if err != nil {
		return err
	}
	updatedPods := make([]*corev1.Pod, 0, len(pods))
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
		if err = r.Patch(context.TODO(), pod, client.RawPatch(types.StrategicMergePatchType, []byte(body))); err != nil {
			return err
		}
	}
	klog.V(3).Infof("patch pub(%s/%s) old pods(%d) related-pub annotation success", pub.Namespace, pub.Name, len(updatedPods))
	return nil
}

func countAvailablePods(pods []*corev1.Pod, disruptedPods, unavailablePods map[string]metav1.Time, control pubcontrol.PubControl) (currentAvailable int32) {
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
		if control.IsPodStateConsistent(pod) && control.IsPodReady(pod) {
			currentAvailable++
		}
	}

	return
}

// This function returns pods using the PodUnavailableBudget object.
func (r *ReconcilePodUnavailableBudget) getPodsForPub(pub *policyv1alpha1.PodUnavailableBudget) ([]*corev1.Pod, error) {
	// if targetReference isn't nil, priority to take effect
	var listOptions *client.ListOptions
	if pub.Spec.TargetReference != nil {
		ref := pub.Spec.TargetReference
		matchedPods, _, err := r.controllerFinder.GetPodsForRef(ref.APIVersion, ref.Kind, ref.Name, pub.Namespace, true)
		return matchedPods, err
	} else if pub.Spec.Selector == nil {
		r.recorder.Eventf(pub, corev1.EventTypeWarning, "NoSelector", "Selector cannot be empty")
		return nil, nil
	}
	// get pods for selector
	labelSelector, err := util.GetFastLabelSelector(pub.Spec.Selector)
	if err != nil {
		r.recorder.Eventf(pub, corev1.EventTypeWarning, "Selector", fmt.Sprintf("Label selector failed: %s", err.Error()))
		return nil, nil
	}
	listOptions = &client.ListOptions{Namespace: pub.Namespace, LabelSelector: labelSelector}
	podList := &corev1.PodList{}
	if err := r.List(context.TODO(), podList, listOptions); err != nil {
		return nil, err
	}

	matchedPods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		if kubecontroller.IsPodActive(pod) {
			matchedPods = append(matchedPods, pod)
		}
	}
	return matchedPods, nil
}

func (r *ReconcilePodUnavailableBudget) getDesiredAvailableForPub(pub *policyv1alpha1.PodUnavailableBudget, expectedCount int32) (desiredAvailable int32, err error) {
	if pub.Spec.MaxUnavailable != nil {
		var maxUnavailable int
		maxUnavailable, err = intstr.GetScaledValueFromIntOrPercent(pub.Spec.MaxUnavailable, int(expectedCount), false)
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

func (r *ReconcilePodUnavailableBudget) getExpectedScale(pub *policyv1alpha1.PodUnavailableBudget, pods []*corev1.Pod) (int32, error) {
	// if spec.targetRef!=nil, expectedCount=targetRef.spec.replicas
	if pub.Spec.TargetReference != nil {
		ref := controllerfinder.ControllerReference{
			APIVersion: pub.Spec.TargetReference.APIVersion,
			Kind:       pub.Spec.TargetReference.Kind,
			Name:       pub.Spec.TargetReference.Name,
		}
		for _, finder := range r.controllerFinder.Finders() {
			scaleNSelector, err := finder(ref, pub.Namespace)
			if err != nil {
				klog.Errorf("podUnavailableBudget(%s/%s) handle TargetReference failed: %s", pub.Namespace, pub.Name, err.Error())
				return 0, err
			}
			if scaleNSelector != nil && scaleNSelector.Metadata.DeletionTimestamp.IsZero() {
				return scaleNSelector.Scale, nil
			}
		}

		// if target reference workload not found, or reference selector is nil
		return 0, nil
	}

	// 1. Find the controller for each pod.  If any pod has 0 controllers,
	// that's an error. With ControllerRef, a pod can only have 1 controller.
	// A mapping from controllers to their scale.
	controllerScale := map[types.UID]int32{}
	for _, pod := range pods {
		ref := metav1.GetControllerOf(pod)
		if ref == nil {
			continue
		}
		// If we already know the scale of the controller there is no need to do anything.
		if _, found := controllerScale[ref.UID]; found {
			continue
		}
		// Check all the supported controllers to find the desired scale.
		workload, err := r.controllerFinder.GetScaleAndSelectorForRef(ref.APIVersion, ref.Kind, pod.Namespace, ref.Name, ref.UID)
		if err != nil && !errors.IsNotFound(err) {
			return 0, err
		}
		if workload != nil && workload.Metadata.DeletionTimestamp.IsZero() {
			controllerScale[workload.UID] = workload.Scale
		}
	}

	// 2. Add up all the controllers.
	var expectedCount int32
	for _, count := range controllerScale {
		expectedCount += count
	}

	return expectedCount, nil
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

		//handle disruption pods which will be eviction or deletion
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
		klog.Errorf("Add cache failed for PodUnavailableBudget(%s/%s): %s", pub.Namespace, pub.Name, err.Error())
	}
	klog.V(3).Infof("pub(%s/%s) update status(disruptedPods:%d, unavailablePods:%d, expectedCount:%d, desiredAvailable:%d, currentAvailable:%d, unavailableAllowed:%d)",
		pub.Namespace, pub.Name, len(disruptedPods), len(unavailablePods), expectedCount, desiredAvailable, currentAvailable, unavailableAllowed)
	return nil
}

func (r *ReconcilePodUnavailableBudget) getPubForWorkload(workload *controllerfinder.ScaleAndSelector) (*policyv1alpha1.PodUnavailableBudget, error) {
	pubList := &policyv1alpha1.PodUnavailableBudgetList{}
	if err := r.List(context.TODO(), pubList, &client.ListOptions{Namespace: workload.Metadata.Namespace}, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	for i := range pubList.Items {
		pub := &pubList.Items[i]
		// if targetReference isn't nil, priority to take effect
		if pub.Spec.TargetReference != nil {
			// belongs the same workload
			if isReferenceEqual(&policyv1alpha1.TargetReference{
				APIVersion: workload.APIVersion,
				Kind:       workload.Kind,
				Name:       workload.Name,
			}, pub.Spec.TargetReference) {
				return pub, nil
			}
		} else {
			// This error is irreversible, so continue
			labelSelector, err := util.GetFastLabelSelector(pub.Spec.Selector)
			if err != nil {
				continue
			}
			// If a PUB with a nil or empty selector creeps in, it should match nothing, not everything.
			if labelSelector.Empty() || !labelSelector.Matches(labels.Set(workload.TempLabels)) {
				continue
			}
			return pub, nil
		}
	}
	klog.V(6).Infof("could not find PodUnavailableBudget for workload %s in namespace %s with labels: %v", workload.Name, workload.Metadata.Namespace, workload.TempLabels)
	return nil, nil
}
