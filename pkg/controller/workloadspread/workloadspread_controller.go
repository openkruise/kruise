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

package workloadspread

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
)

func init() {
	flag.IntVar(&concurrentReconciles, "workloadspread-workers", concurrentReconciles, "Max concurrent workers for WorkloadSpread controller.")
}

var (
	concurrentReconciles = 3
)

const (
	controllerName = "workloadspread-controller"

	// CreatPodTimeout sets maximum time from the moment a pod is added to CreatePods in WorkloadSpread.Status by webhook
	// to the time when the pod is expected to be seen by controller. If the pod has not been found by controller
	// during that time it is assumed, which means it won't be created at all and corresponding record in map can be
	// removed from WorkloadSpread.Status. It is assumed that pod/ws apiserver to controller latency is relatively small (like 1-2sec)
	// so the below value should be more enough.
	CreatPodTimeout = 30 * time.Second

	// DeletePodTimeout is similar to the CreatePodTimeout and it's the time duration for deleting Pod.
	DeletePodTimeout = 15 * time.Second
)

var (
	controllerKruiseKindCS = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
	controllerKindRS       = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	controllerKindDep      = appsv1.SchemeGroupVersion.WithKind("Deployment")
)

// this is a short cut for any sub-functions to notify the reconcile how long to wait to requeue
var durationStore = requeueduration.DurationStore{}

// Add creates a new WorkloadSpread Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for spec changes to WorkloadSpread
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.WorkloadSpread{}}, &handler.EnqueueRequestForObject{},
		predicate.GenerationChangedPredicate{})
	if err != nil {
		return err
	}

	// Watch for changes to Pods have a specific annotation
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &podEventHandler{})
	if err != nil {
		return err
	}

	// Watch for replica changes to CloneSet
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.CloneSet{}}, &workloadEventHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	// Watch for replica changes to Deployment
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &workloadEventHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	// Watch for replica changes to ReplicaSet
	err = c.Watch(&source.Kind{Type: &appsv1.ReplicaSet{}}, &workloadEventHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	return nil
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := util.NewClientFromManager(mgr, controllerName)
	return &ReconcileWorkloadSpread{
		Client:   cli,
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

var _ reconcile.Reconciler = &ReconcileWorkloadSpread{}

// ReconcileWorkloadSpread reconciles a WorkloadSpread object
type ReconcileWorkloadSpread struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=workloadspreads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=workloadspreads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete

func (r *ReconcileWorkloadSpread) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	ws := &appsv1alpha1.WorkloadSpread{}
	err := r.Get(context.TODO(), req.NamespacedName, ws)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	klog.V(3).Infof("Begin to process workloadSpread (%s/%s)", ws.Namespace, ws.Name)
	err = r.syncWorkloadSpread(ws)
	klog.V(3).Infof("Finished syncing workloadSpread (%s/%s)", ws.Namespace, ws.Name)
	return reconcile.Result{RequeueAfter: durationStore.Pop(getWorkloadSpreadKey(ws))}, err
}

// scaleAndSelector is used to return (UID, scale, selector) fields from the
// controller finder functions.
type scaleAndSelector struct {
	// controller UID, for example: rc, rs, deployment...
	types.UID
	// controller.spec.Replicas
	scale int32
	// controller.spec.Selector
	selector *metav1.LabelSelector
}

// podControllerFinder is a function type that maps a pod to a list of
// controllers and their scale.
type podControllerFinder func(ref *appsv1alpha1.TargetReference, namespace string) (*scaleAndSelector, error)

func (r *ReconcileWorkloadSpread) finders() []podControllerFinder {
	return []podControllerFinder{r.getPodKruiseCloneSet, r.getPodReplicasSet}
}

// getPodKruiseCloneSet returns the kruise cloneSet referenced by the provided controllerRef.
func (r *ReconcileWorkloadSpread) getPodKruiseCloneSet(ref *appsv1alpha1.TargetReference, namespace string) (*scaleAndSelector, error) {
	ok, _ := VerifyGroupKind(ref, controllerKruiseKindCS.Kind, []string{controllerKruiseKindCS.Group})
	if !ok {
		return nil, nil
	}

	cloneSet := &appsv1alpha1.CloneSet{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, cloneSet)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &scaleAndSelector{
		UID:      cloneSet.UID,
		scale:    *(cloneSet.Spec.Replicas),
		selector: cloneSet.Spec.Selector,
	}, nil
}

// getPodDeployment returns Pods managed by Deployment object.
func (r *ReconcileWorkloadSpread) getPodDeployment(ref *appsv1alpha1.TargetReference, namespace string) ([]*corev1.Pod, int32, error) {
	ok, _ := VerifyGroupKind(ref, controllerKindDep.Kind, []string{controllerKindDep.Group})
	if !ok {
		return nil, -1, nil
	}

	deployment := &appsv1.Deployment{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, deployment)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, 0, nil
		}
		return nil, -1, err
	}

	// List ReplicaSets owned by this Deployment
	rsList := &appsv1.ReplicaSetList{}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		klog.Errorf("Deployment (%s/%s) gets labelSelector failed: %s", namespace, deployment.Name, err.Error())
		return nil, -1, nil
	}
	listOption := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: selector,
	}
	err = r.List(context.TODO(), rsList, listOption)
	if err != nil {
		return nil, -1, err
	}

	rsSet := make(map[types.UID]struct{})
	for _, rs := range rsList.Items {
		controllerRef := metav1.GetControllerOf(&rs)
		if controllerRef.UID == deployment.UID {
			rsSet[rs.UID] = struct{}{}
		}
	}

	// List all Pods owned by this Deployment.
	podList := &corev1.PodList{}
	if err := r.List(context.TODO(), podList, listOption); err != nil {
		return nil, -1, err
	}

	matchedPods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		controllerRef := metav1.GetControllerOf(pod)
		if controllerRef == nil {
			continue
		}
		// filter UID
		if _, ok := rsSet[controllerRef.UID]; ok {
			matchedPods = append(matchedPods, pod)
		}
	}
	return matchedPods, *deployment.Spec.Replicas, nil
}

func (r *ReconcileWorkloadSpread) getPodReplicasSet(ref *appsv1alpha1.TargetReference, namespace string) (*scaleAndSelector, error) {
	ok, _ := VerifyGroupKind(ref, controllerKindRS.Kind, []string{controllerKindRS.Group})
	if !ok {
		return nil, nil
	}

	rs := &appsv1.ReplicaSet{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, rs)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &scaleAndSelector{
		UID:      rs.UID,
		scale:    *(rs.Spec.Replicas),
		selector: rs.Spec.Selector,
	}, nil
}

func VerifyGroupKind(ref *appsv1alpha1.TargetReference, expectedKind string, expectedGroups []string) (bool, error) {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		klog.Errorf("failed to parse GroupVersion for apiVersion (%s): %s", ref.APIVersion, err.Error())
		return false, err
	}

	if ref.Kind != expectedKind {
		return false, nil
	}

	for _, group := range expectedGroups {
		if group == gv.Group {
			return true, nil
		}
	}

	return false, nil
}

// getPodsForWorkloadSpread returns Pods managed by the WorkloadSpread object.
func (r *ReconcileWorkloadSpread) getPodsForWorkloadSpread(ws *appsv1alpha1.WorkloadSpread) ([]*corev1.Pod, int32, error) {
	var scaleSelector *scaleAndSelector
	var err error

	if ws.Spec.TargetReference != nil {
		// Deployment manage Pods by ReplicaSet, so it has a different method.
		if ws.Spec.TargetReference.Kind == controllerKindDep.Kind {
			return r.getPodDeployment(ws.Spec.TargetReference, ws.Namespace)
		}

		for _, finder := range r.finders() {
			scaleSelector, err = finder(ws.Spec.TargetReference, ws.Namespace)
			if err != nil {
				klog.Errorf("WorkloadSpread (%s/%s) handle targetReference failed: %s", ws.Namespace, ws.Name, err.Error())
				return nil, -1, err
			}
			if scaleSelector != nil {
				break
			}
		}
	}

	if scaleSelector == nil || scaleSelector.selector == nil || scaleSelector.UID == "" {
		klog.Errorf("WorkloadSpread (%s/%s) TargetReference cannot be found", ws.Namespace, ws.Name)
		return nil, 0, nil
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(scaleSelector.selector)
	if err != nil {
		klog.Errorf("WorkloadSpread (%s/%s) get labelSelector failed: %s", ws.Namespace, ws.Name, err.Error())
		return nil, -1, nil
	}

	podList := &corev1.PodList{}
	listOption := &client.ListOptions{
		Namespace:     ws.Namespace,
		LabelSelector: labelSelector,
		FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(scaleSelector.UID)}),
	}
	if err := r.List(context.TODO(), podList, listOption); err != nil {
		return nil, -1, err
	}

	matchedPods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		matchedPods = append(matchedPods, &podList.Items[i])
	}
	return matchedPods, scaleSelector.scale, nil
}

// syncWorkloadSpread is the main logic of the WorkloadSpread controller. Firstly, we get Pods from workload managed by
// WorkloadSpread and then classify these Pods to each corresponding subset. Secondly, we set Pod deletion-cost annotation
// value by compare the number of subset's Pods with the subset's maxReplicas, and then we consider rescheduling failed Pods.
// Lastly, we update the WorkloadSpread's Status and clean up schedule failed Pods. controller should collaborate with webhook
// to maintain WorkloadSpread status together. The controller is responsible for calculating the real status, and the webhook
// mainly counts missingReplicas and records the creation or deletion entry of Pod into map.
func (r *ReconcileWorkloadSpread) syncWorkloadSpread(ws *appsv1alpha1.WorkloadSpread) error {
	pods, workloadReplicas, err := r.getPodsForWorkloadSpread(ws)
	if err != nil || workloadReplicas == -1 {
		klog.Errorf("WorkloadSpread (%s/%s) get matched pods failed: %s", ws.Namespace, ws.Name, err.Error())
		return err
	}
	if len(pods) == 0 {
		klog.Warningf("WorkloadSpread (%s/%s) has no matched pods", ws.Namespace, ws.Name)
	}

	// group Pods by subset
	podMap := getPodMap(ws, pods)

	// update Pod's deletion-cost annotation in each subset
	for _, subset := range ws.Spec.Subsets {
		if err := r.syncSubsetPodDeletionCost(ws, &subset, podMap[subset.Name], workloadReplicas); err != nil {
			return err
		}
	}

	// calculate status and reschedule
	status, scheduleFailedPodMap := r.calculateWorkloadSpreadStatus(ws, podMap, workloadReplicas)
	if status == nil {
		return nil
	}

	// update status
	err = r.UpdateWorkloadSpreadStatus(ws, status)
	if err != nil {
		return err
	}

	// clean up the unscheduable Pods
	return r.cleanupUnscheduledPods(ws, scheduleFailedPodMap)
}

func getInjectWorkloadSpreadFromPod(pod *corev1.Pod) *wsutil.InjectWorkloadSpread {
	injectStr, exist := pod.GetAnnotations()[wsutil.MatchedWorkloadSpreadSubsetAnnotations]
	if !exist {
		return nil
	}

	injectWS := &wsutil.InjectWorkloadSpread{}
	err := json.Unmarshal([]byte(injectStr), injectWS)
	if err != nil {
		klog.Errorf("failed to unmarshal %s from Pod (%s/%s)", injectStr, pod.Namespace, pod.Name)
		return nil
	}
	return injectWS
}

// getPodMap returns a map, the key is the name of subset and the value represents the Pods of the corresponding subset.
func getPodMap(ws *appsv1alpha1.WorkloadSpread, pods []*corev1.Pod) map[string][]*corev1.Pod {
	podMap := make(map[string][]*corev1.Pod, len(ws.Spec.Subsets))
	for _, subset := range ws.Spec.Subsets {
		podMap[subset.Name] = []*corev1.Pod{}
	}

	for i := range pods {
		injectWS := getInjectWorkloadSpreadFromPod(pods[i])
		if injectWS == nil {
			continue
		}
		if injectWS.Name != ws.Name {
			continue
		}
		if _, exist := podMap[injectWS.Subset]; exist {
			podMap[injectWS.Subset] = append(podMap[injectWS.Subset], pods[i])
		}
	}

	return podMap
}

// return two parameters
// 1. current WorkloadSpreadStatus
// 2. a map, the key is the subsetName, the value is the schedule failed Pods belongs to the subset.
func (r *ReconcileWorkloadSpread) calculateWorkloadSpreadStatus(ws *appsv1alpha1.WorkloadSpread,
	podMap map[string][]*corev1.Pod, workloadReplicas int32) (*appsv1alpha1.WorkloadSpreadStatus, map[string][]*corev1.Pod) {
	// set the generation in the returned status
	status := appsv1alpha1.WorkloadSpreadStatus{}
	status.ObservedGeneration = ws.Generation
	status.SubsetStatuses = make([]appsv1alpha1.WorkloadSpreadSubsetStatus, len(ws.Spec.Subsets))
	scheduleFailedPodMap := make(map[string][]*corev1.Pod)

	// Using a map to restore name and old status of subset, because user could adjust the spec's subset sequence
	// to change priority of subset. We guarantee that operation and use subset name to distinguish which subset
	// from old status.
	oldSubsetStatuses := ws.Status.SubsetStatuses
	oldSubsetStatusMap := make(map[string]*appsv1alpha1.WorkloadSpreadSubsetStatus, len(oldSubsetStatuses))
	for i := range oldSubsetStatuses {
		oldSubsetStatusMap[oldSubsetStatuses[i].Name] = &oldSubsetStatuses[i]
	}

	var rescheduleCriticalSeconds int32 = 0
	if ws.Spec.ScheduleStrategy.Type == appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType &&
		ws.Spec.ScheduleStrategy.Adaptive != nil &&
		ws.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds != nil {
		rescheduleCriticalSeconds = *ws.Spec.ScheduleStrategy.Adaptive.RescheduleCriticalSeconds
	}

	for i := 0; i < len(ws.Spec.Subsets); i++ {
		subset := &ws.Spec.Subsets[i]

		// calculate subset status
		subsetStatus := r.calculateWorkloadSpreadSubsetStatus(ws, podMap[subset.Name], subset,
			oldSubsetStatusMap[subset.Name], workloadReplicas)
		if subsetStatus == nil {
			return nil, nil
		}

		// don't reschedule the last subset.
		if rescheduleCriticalSeconds > 0 && i != len(ws.Spec.Subsets)-1 {
			subsetUnscheduledStatus, pods := rescheduleSubset(ws, podMap[subset.Name], oldSubsetStatusMap[subset.Name])
			subsetStatus.SubsetUnscheduledStatus = *subsetUnscheduledStatus
			scheduleFailedPodMap[subset.Name] = pods
		} else {
			subsetStatus.SubsetUnscheduledStatus.Unschedulable = false
			subsetStatus.SubsetUnscheduledStatus.UnscheduledTime = metav1.Now()
			subsetStatus.SubsetUnscheduledStatus.FailedCount = 0
		}

		status.SubsetStatuses[i] = *subsetStatus
	}

	return &status, scheduleFailedPodMap
}

// calculateWorkloadSpreadSubsetStatus returns the current subsetStatus for subset.
func (r *ReconcileWorkloadSpread) calculateWorkloadSpreadSubsetStatus(ws *appsv1alpha1.WorkloadSpread,
	pods []*corev1.Pod,
	subset *appsv1alpha1.WorkloadSpreadSubset,
	oldSubsetStatus *appsv1alpha1.WorkloadSpreadSubsetStatus,
	workloadReplicas int32) *appsv1alpha1.WorkloadSpreadSubsetStatus {
	// current subsetStatus in this reconcile
	subsetStatus := &appsv1alpha1.WorkloadSpreadSubsetStatus{}
	subsetStatus.Name = subset.Name
	subsetStatus.CreatingPods = make(map[string]metav1.Time)
	subsetStatus.DeletingPods = make(map[string]metav1.Time)

	var err error
	var subsetMaxReplicas int
	if subset.MaxReplicas == nil {
		// MaxReplicas is nil, which means there is no limit for subset replicas, using -1 to represent it.
		subsetMaxReplicas = -1
	} else {
		subsetMaxReplicas, err = intstr.GetValueFromIntOrPercent(subset.MaxReplicas, int(workloadReplicas), true)
		if err != nil || subsetMaxReplicas < 0 {
			klog.Errorf("failed to get maxReplicas value from subset (%s) of WorkloadSpread (%s/%s)",
				subset.Name, ws.Namespace, ws.Name)
			return nil
		}
	}
	// initialize missingReplicas to subsetMaxReplicas
	subsetStatus.MissingReplicas = int32(subsetMaxReplicas)

	currentTime := time.Now()
	var oldCreatingPods map[string]metav1.Time
	var oldDeletingPods map[string]metav1.Time
	if oldSubsetStatus != nil {
		// make a deep copy because we may need to remove some element later and compare old status with current status.
		oldCreatingPods = make(map[string]metav1.Time, len(oldSubsetStatus.CreatingPods))
		for k, v := range oldSubsetStatus.CreatingPods {
			oldCreatingPods[k] = v
		}
		oldDeletingPods = oldSubsetStatus.DeletingPods
	}

	for _, pod := range pods {
		// remove this Pod from creatingPods map because this Pod has been created.
		injectWS := getInjectWorkloadSpreadFromPod(pod)
		if injectWS.UID != "" {
			// Deployment or other workload has not generated the full pod.Name when webhook is mutating Pod.
			// So webhook generates a UID to identify Pod and restore it into the creatingPods map. The generated
			// UID and pod.Name have the same function.
			delete(oldCreatingPods, injectWS.UID)
		} else {
			delete(oldCreatingPods, pod.Name)
		}

		// not active
		if !kubecontroller.IsPodActive(pod) {
			continue
		}

		// count missingReplicas
		if subsetStatus.MissingReplicas > 0 {
			subsetStatus.MissingReplicas--
		}

		// some Pods in oldDeletingPods map, which records Pods we want to delete by webhook.
		if deleteTime, exist := oldDeletingPods[pod.Name]; exist {
			expectedDeletion := deleteTime.Time.Add(DeletePodTimeout)
			// deleted this Pod timeout, so we consider removing it from oldDeletingPods map, which means deleted failed.
			if expectedDeletion.Before(currentTime) {
				r.recorder.Eventf(ws, corev1.EventTypeWarning,
					"DeletePodFailed", "Pod %s/%s was expected to be deleted but it wasn't，managed by WorkloadSpread %s/%s",
					ws.Namespace, pod.Name, ws.Namespace, ws.Name)
			} else {
				// no timeout, there may be some latency, to restore it into deletingPods map.
				subsetStatus.DeletingPods[pod.Name] = deleteTime

				// missingReplicas + 1, suppose it has been deleted
				if subsetStatus.MissingReplicas < int32(subsetMaxReplicas) {
					subsetStatus.MissingReplicas++
				}

				// requeue key in order to clean it from map when expectedDeletion is equal to currentTime.
				durationStore.Push(getWorkloadSpreadKey(ws), expectedDeletion.Sub(currentTime))
			}
		}
	}

	// oldCreatingPods has remaining Pods that not be found by controller.
	for podID, createTime := range oldCreatingPods {
		expectedCreation := createTime.Time.Add(CreatPodTimeout)
		// created this Pod timeout
		if expectedCreation.Before(currentTime) {
			r.recorder.Eventf(ws, corev1.EventTypeWarning,
				"CreatePodFailed", "Pod %s/%s was expected to be created but it wasn't，managed by WorkloadSpread %s/%s",
				ws.Namespace, podID, ws.Namespace, ws.Name)
		} else {
			// no timeout, to restore it into creatingPods map.
			subsetStatus.CreatingPods[podID] = createTime

			// missingReplicas - 1, suppose it has been created
			if subsetStatus.MissingReplicas > 0 {
				subsetStatus.MissingReplicas--
			}

			// requeue key when expectedCreation is equal to currentTime.
			durationStore.Push(getWorkloadSpreadKey(ws), expectedCreation.Sub(currentTime))
		}
	}

	return subsetStatus
}

func (r *ReconcileWorkloadSpread) UpdateWorkloadSpreadStatus(ws *appsv1alpha1.WorkloadSpread,
	status *appsv1alpha1.WorkloadSpreadStatus) error {
	if status.ObservedGeneration == ws.Status.ObservedGeneration &&
		apiequality.Semantic.DeepEqual(status.SubsetStatuses, ws.Status.SubsetStatuses) {
		return nil
	}

	clone := ws.DeepCopy()
	clone.Status = *status

	err := r.writeWorkloadSpreadStatus(clone)
	if err == nil {
		klog.V(3).Info(makeStatusChangedLog(ws, status))
	}
	return err
}

func makeStatusChangedLog(ws *appsv1alpha1.WorkloadSpread, status *appsv1alpha1.WorkloadSpreadStatus) string {
	oldSubsetStatuses := ws.Status.SubsetStatuses
	oldSubsetStatusMap := make(map[string]*appsv1alpha1.WorkloadSpreadSubsetStatus, len(oldSubsetStatuses))
	for i := range oldSubsetStatuses {
		oldSubsetStatusMap[oldSubsetStatuses[i].Name] = &oldSubsetStatuses[i]
	}

	log := fmt.Sprintf("WorkloadSpread (%s/%s) changes Status:", ws.Namespace, ws.Name)

	for i, subset := range ws.Spec.Subsets {
		oldStatus, ok := oldSubsetStatusMap[subset.Name]
		if !ok {
			continue
		}
		newStatus := status.SubsetStatuses[i]

		log += fmt.Sprintf(" (<subset name: %s>", subset.Name)

		if oldStatus.SubsetUnscheduledStatus.Unschedulable != newStatus.SubsetUnscheduledStatus.Unschedulable {
			log += fmt.Sprintf(" <unschedulable: %v -> %v",
				oldStatus.SubsetUnscheduledStatus.Unschedulable, newStatus.SubsetUnscheduledStatus.Unschedulable)
		}
		if newStatus.SubsetUnscheduledStatus.Unschedulable {
			log += fmt.Sprintf(" <unscheduledTime: %v>", newStatus.SubsetUnscheduledStatus.UnscheduledTime)
		}

		if oldStatus.MissingReplicas != newStatus.MissingReplicas {
			log += fmt.Sprintf(" <missingReplicas: %d -> %d>", oldStatus.MissingReplicas, newStatus.MissingReplicas)
		} else {
			log += fmt.Sprintf(" <missingReplicas: %d>", newStatus.MissingReplicas)
		}

		if len(oldStatus.CreatingPods) != len(newStatus.CreatingPods) {
			log += fmt.Sprintf(" <creatingPods length: %d -> %d>", len(oldStatus.CreatingPods), len(newStatus.CreatingPods))
		} else {
			log += fmt.Sprintf(" <creatingPods length: %d>", len(newStatus.CreatingPods))
		}

		if len(oldStatus.DeletingPods) != len(newStatus.DeletingPods) {
			log += fmt.Sprintf(" <deletingPods length: %d -> %d>", len(oldStatus.DeletingPods), len(newStatus.DeletingPods))
		} else {
			log += fmt.Sprintf(" <deletingPods length: %d>", len(newStatus.DeletingPods))
		}

		log += ")"
	}

	return log
}

func (r *ReconcileWorkloadSpread) writeWorkloadSpreadStatus(ws *appsv1alpha1.WorkloadSpread) error {
	// If this update fails, don't retry it. Allow the failure to get handled &
	// retried in `processNextWorkItem()`.
	return r.Status().Update(context.TODO(), ws)
}

func getWorkloadSpreadKey(o metav1.Object) string {
	return o.GetNamespace() + "/" + o.GetName()
}
