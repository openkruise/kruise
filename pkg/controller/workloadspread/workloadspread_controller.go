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
	"math"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	ctrlUtil "github.com/openkruise/kruise/pkg/controller/util"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/configuration"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
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

	// FakeSubsetName is a fake subset name for such pods that do not match any subsets
	FakeSubsetName = "kruise.io/workloadspread-fake-subset-name"

	// IgnorePatchExistingPodsAnnotation ignore ws.Spec.Subsets[x].Patch for existing pods
	IgnorePatchExistingPodsAnnotation = "workloadspread.kruise.io/ignore-patch-existing-pods-metadata"
)

var (
	controllerKruiseKindWS  = appsv1alpha1.SchemeGroupVersion.WithKind("WorkloadSpread")
	controllerKruiseKindCS  = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
	controllerKruiseKindSts = appsv1alpha1.SchemeGroupVersion.WithKind("StatefulSet")
	controllerKindSts       = appsv1.SchemeGroupVersion.WithKind("StatefulSet")
	controllerKindRS        = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	controllerKindDep       = appsv1.SchemeGroupVersion.WithKind("Deployment")
	controllerKindJob       = batchv1.SchemeGroupVersion.WithKind("Job")
)

// this is a short cut for any sub-functions to notify the reconcile how long to wait to requeue
var durationStore = requeueduration.DurationStore{}

// Add creates a new WorkloadSpread Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKruiseKindWS) {
		return nil
	}
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

	// Watch WorkloadSpread
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.WorkloadSpread{}}, &handler.EnqueueRequestForObject{})
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

	// Watch for parallelism changes to Job
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &workloadEventHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	// Watch for replicas changes to other CRD
	whiteList, err := configuration.GetWSWatchCustomWorkloadWhiteList(mgr.GetClient())
	if err != nil {
		return err
	}
	if len(whiteList.Workloads) > 0 {
		workloadHandler := &workloadEventHandler{Reader: mgr.GetClient()}
		for _, workload := range whiteList.Workloads {
			if _, err := ctrlUtil.AddWatcherDynamically(c, workloadHandler, workload.GroupVersionKind, "WorkloadSpread"); err != nil {
				return err
			}
		}
	}
	return nil
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := utilclient.NewClientFromManager(mgr, controllerName)
	return &ReconcileWorkloadSpread{
		Client:           cli,
		scheme:           mgr.GetScheme(),
		recorder:         mgr.GetEventRecorderFor(controllerName),
		controllerFinder: controllerfinder.Finder,
	}
}

var _ reconcile.Reconciler = &ReconcileWorkloadSpread{}

// ReconcileWorkloadSpread reconciles a WorkloadSpread object
type ReconcileWorkloadSpread struct {
	client.Client
	scheme           *runtime.Scheme
	recorder         record.EventRecorder
	controllerFinder *controllerfinder.ControllerFinder
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=workloadspreads,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=workloadspreads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=workloadspreads/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch;delete

func (r *ReconcileWorkloadSpread) Reconcile(_ context.Context, req reconcile.Request) (reconcile.Result, error) {
	ws := &appsv1alpha1.WorkloadSpread{}
	err := r.Get(context.TODO(), req.NamespacedName, ws)

	if (err != nil && errors.IsNotFound(err)) || (err == nil && !ws.DeletionTimestamp.IsZero()) {
		// delete cache if this workloadSpread has been deleted
		if cacheErr := util.GlobalCache.Delete(&appsv1alpha1.WorkloadSpread{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps.kruise.io/v1alpha1",
				Kind:       "WorkloadSpread",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: req.Namespace,
				Name:      req.Name,
			},
		}); cacheErr != nil {
			klog.Warningf("Failed to delete workloadSpread(%s/%s) cache after deletion, err: %v", req.Namespace, req.Name, cacheErr)
		}
		return reconcile.Result{}, nil
	} else if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	startTime := time.Now()
	klog.V(3).Infof("Begin to process WorkloadSpread (%s/%s)", ws.Namespace, ws.Name)
	err = r.syncWorkloadSpread(ws)
	klog.V(3).Infof("Finished syncing WorkloadSpread (%s/%s), cost: %v", ws.Namespace, ws.Name, time.Since(startTime))
	return reconcile.Result{RequeueAfter: durationStore.Pop(getWorkloadSpreadKey(ws))}, err
}

func (r *ReconcileWorkloadSpread) getPodJob(ref *appsv1alpha1.TargetReference, namespace string) ([]*corev1.Pod, int32, error) {
	ok, err := wsutil.VerifyGroupKind(ref, controllerKindJob.Kind, []string{controllerKindJob.Group})
	if err != nil || !ok {
		return nil, -1, err
	}

	job := &batchv1.Job{}
	err = r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, job)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			klog.V(3).Infof("cannot find Job (%s/%s)", namespace, ref.Name)
			return nil, 0, nil
		}
		return nil, -1, err
	}

	labelSelector, err := util.ValidatedLabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		klog.Errorf("gets labelSelector failed: %s", err.Error())
		return nil, -1, nil
	}

	podList := &corev1.PodList{}
	listOption := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
		FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(job.UID)}),
	}
	err = r.List(context.TODO(), podList, listOption)
	if err != nil {
		return nil, -1, err
	}

	matchedPods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		matchedPods = append(matchedPods, &podList.Items[i])
	}
	return matchedPods, *(job.Spec.Parallelism), nil
}

// getPodsForWorkloadSpread returns Pods managed by the WorkloadSpread object.
// return two parameters
// 1. podList for workloadSpread
// 2. workloadReplicas
func (r *ReconcileWorkloadSpread) getPodsForWorkloadSpread(ws *appsv1alpha1.WorkloadSpread) ([]*corev1.Pod, int32, error) {
	if ws.Spec.TargetReference == nil {
		return nil, -1, nil
	}

	var pods []*corev1.Pod
	var workloadReplicas int32
	var err error
	targetRef := ws.Spec.TargetReference

	switch targetRef.Kind {
	case controllerKindJob.Kind:
		pods, workloadReplicas, err = r.getPodJob(targetRef, ws.Namespace)
	default:
		pods, workloadReplicas, err = r.controllerFinder.GetPodsForRef(targetRef.APIVersion, targetRef.Kind, ws.Namespace, targetRef.Name, false)
	}

	if err != nil {
		klog.Errorf("WorkloadSpread (%s/%s) handles targetReference failed: %s", ws.Namespace, ws.Name, err.Error())
		return nil, -1, err
	}

	return pods, workloadReplicas, err
}

// syncWorkloadSpread is the main logic of the WorkloadSpread controller. Firstly, we get Pods from workload managed by
// WorkloadSpread and then classify these Pods to each corresponding subset. Secondly, we set Pod deletion-cost annotation
// value by compare the number of subset's Pods with the subset's maxReplicas, and then we consider rescheduling failed Pods.
// Lastly, we update the WorkloadSpread's Status and clean up scheduled failed Pods. controller should collaborate with webhook
// to maintain WorkloadSpread status together. The controller is responsible for calculating the real status, and the webhook
// mainly counts missingReplicas and records the creation or deletion entry of Pod into map.
func (r *ReconcileWorkloadSpread) syncWorkloadSpread(ws *appsv1alpha1.WorkloadSpread) error {
	pods, workloadReplicas, err := r.getPodsForWorkloadSpread(ws)
	if err != nil || workloadReplicas == -1 {
		if err != nil {
			klog.Errorf("WorkloadSpread (%s/%s) gets matched pods failed: %v", ws.Namespace, ws.Name, err)
		}
		return err
	}
	if len(pods) == 0 {
		klog.Warningf("WorkloadSpread (%s/%s) has no matched pods, target workload's replicas[%d]", ws.Namespace, ws.Name, workloadReplicas)
	}

	// group Pods by subset
	podMap, err := r.groupPod(ws, pods, workloadReplicas)
	if err != nil {
		return err
	}

	// update deletion-cost for each subset
	err = r.updateDeletionCost(ws, podMap, workloadReplicas)
	if err != nil {
		return err
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

	// clean up unschedulable Pods
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

// groupPod returns a map, the key is the name of subset and the value represents the Pods of the corresponding subset.
func (r *ReconcileWorkloadSpread) groupPod(ws *appsv1alpha1.WorkloadSpread, pods []*corev1.Pod, replicas int32) (map[string][]*corev1.Pod, error) {
	podMap := make(map[string][]*corev1.Pod, len(ws.Spec.Subsets)+1)
	podMap[FakeSubsetName] = []*corev1.Pod{}
	subsetMissingReplicas := make(map[string]int)
	for _, subset := range ws.Spec.Subsets {
		podMap[subset.Name] = []*corev1.Pod{}
		subsetMissingReplicas[subset.Name], _ = intstr.GetScaledValueFromIntOrPercent(
			intstr.ValueOrDefault(subset.MaxReplicas, intstr.FromInt(math.MaxInt32)), int(replicas), true)
	}

	// count managed pods for each subset
	for i := range pods {
		injectWS := getInjectWorkloadSpreadFromPod(pods[i])
		if isNotMatchedWS(injectWS, ws) {
			continue
		}
		if _, exist := podMap[injectWS.Subset]; !exist {
			continue
		}
		subsetMissingReplicas[injectWS.Subset]--
	}

	for i := range pods {
		subsetName, err := r.getSuitableSubsetNameForPod(ws, pods[i], subsetMissingReplicas)
		if err != nil {
			return nil, err
		}

		if _, exist := podMap[subsetName]; exist {
			podMap[subsetName] = append(podMap[subsetName], pods[i])
		} else {
			// for the scene where the original subset of the pod was deleted.
			podMap[FakeSubsetName] = append(podMap[FakeSubsetName], pods[i])
		}
	}

	return podMap, nil
}

// getSuitableSubsetNameForPod will return (FakeSubsetName, nil) if not found suitable subset for pod
func (r *ReconcileWorkloadSpread) getSuitableSubsetNameForPod(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod, subsetMissingReplicas map[string]int) (string, error) {
	injectWS := getInjectWorkloadSpreadFromPod(pod)
	if isNotMatchedWS(injectWS, ws) {
		// process the pods that were created before workloadSpread
		matchedSubset, err := r.getAndUpdateSuitableSubsetName(ws, pod, subsetMissingReplicas)
		if err != nil {
			return "", err
		} else if matchedSubset == nil {
			return FakeSubsetName, nil
		}
		return matchedSubset.Name, nil
	}
	return injectWS.Subset, nil
}

// getSuitableSubsetForOldPod returns a suitable subset for the pod which was created before workloadSpread.
// getSuitableSubsetForOldPod will return (nil, nil) if there is no suitable subset for the pod.
func (r *ReconcileWorkloadSpread) getAndUpdateSuitableSubsetName(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod, subsetMissingReplicas map[string]int) (*appsv1alpha1.WorkloadSpreadSubset, error) {
	if len(pod.Spec.NodeName) == 0 {
		return nil, nil
	}

	node := &corev1.Node{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, node); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	var maxPreferredScore int64 = -1
	var favoriteSubset *appsv1alpha1.WorkloadSpreadSubset
	for i := range ws.Spec.Subsets {
		subset := &ws.Spec.Subsets[i]
		// in case of that this pod was scheduled to the node which matches a subset of workloadSpread
		matched, preferredScore, err := matchesSubset(pod, node, subset, subsetMissingReplicas[subset.Name])
		if err != nil {
			// requiredSelectorTerm field was validated at webhook stage, so this error should not occur
			// this error should not be returned, because it is a non-transient error
			klog.Errorf("unexpected error occurred when matching pod (%s/%s) with subset, please check requiredSelectorTerm field of subset (%s) in WorkloadSpread (%s/%s), err: %s",
				pod.Namespace, pod.Name, subset.Name, ws.Namespace, ws.Name, err.Error())
		}
		// select the most favorite subsets for the pod by subset.PreferredNodeSelectorTerms
		if matched && preferredScore > maxPreferredScore {
			favoriteSubset = subset
			maxPreferredScore = preferredScore
		}
	}

	if favoriteSubset != nil {
		if err := r.patchFavoriteSubsetMetadataToPod(pod, ws, favoriteSubset); err != nil {
			return nil, err
		}
		subsetMissingReplicas[favoriteSubset.Name]--
		return favoriteSubset, nil
	}

	return nil, nil
}

// patchFavoriteSubsetMetadataToPod patch MatchedWorkloadSpreadSubsetAnnotations to the pod,
// and select labels/annotations form favoriteSubset.patch, then patch them to the pod;
func (r *ReconcileWorkloadSpread) patchFavoriteSubsetMetadataToPod(pod *corev1.Pod, ws *appsv1alpha1.WorkloadSpread, favoriteSubset *appsv1alpha1.WorkloadSpreadSubset) error {
	patchMetadata := make(map[string]interface{})
	// decode favoriteSubset.patch.raw and add their labels and annotations to the patch
	if favoriteSubset.Patch.Raw != nil && !strings.EqualFold(ws.Annotations[IgnorePatchExistingPodsAnnotation], "true") {
		patchField := map[string]interface{}{}
		if err := json.Unmarshal(favoriteSubset.Patch.Raw, &patchField); err == nil {
			if metadata, ok := patchField["metadata"].(map[string]interface{}); ok && metadata != nil {
				patchMetadata = metadata
			}
		}
	}

	injectWS, _ := json.Marshal(&wsutil.InjectWorkloadSpread{
		Name:   ws.Name,
		Subset: favoriteSubset.Name,
	})

	if annotations, ok := patchMetadata["annotations"].(map[string]interface{}); ok && annotations != nil {
		annotations[wsutil.MatchedWorkloadSpreadSubsetAnnotations] = string(injectWS)
	} else {
		patchMetadata["annotations"] = map[string]interface{}{
			wsutil.MatchedWorkloadSpreadSubsetAnnotations: string(injectWS),
		}
	}

	patch, _ := json.Marshal(map[string]interface{}{
		"metadata": patchMetadata,
	})

	if err := r.Patch(context.TODO(), pod, client.RawPatch(types.StrategicMergePatchType, patch)); err != nil {
		klog.Errorf(`Failed to patch "matched-workloadspread:{Name: %s, Subset: %s} annotation" for pod (%s/%s), err: %s`,
			ws.Name, favoriteSubset.Name, pod.Namespace, pod.Name, err.Error())
		return err
	}

	return nil
}

// return two parameters
// 1. current WorkloadSpreadStatus
// 2. a map, the key is the subsetName, the value is the schedule failed Pods belongs to the subset.
func (r *ReconcileWorkloadSpread) calculateWorkloadSpreadStatus(ws *appsv1alpha1.WorkloadSpread,
	podMap map[string][]*corev1.Pod, workloadReplicas int32) (*appsv1alpha1.WorkloadSpreadStatus, map[string][]*corev1.Pod) {
	// set the generation in the returned status
	status := appsv1alpha1.WorkloadSpreadStatus{}
	status.ObservedGeneration = ws.Generation
	// status.ObservedWorkloadReplicas = workloadReplicas
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

	var rescheduleCriticalSeconds int32
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
		if rescheduleCriticalSeconds > 0 {
			if i != len(ws.Spec.Subsets)-1 {
				pods := r.rescheduleSubset(ws, podMap[subset.Name], subsetStatus, oldSubsetStatusMap[subset.Name])
				scheduleFailedPodMap[subset.Name] = pods
			} else {
				oldCondition := GetWorkloadSpreadSubsetCondition(oldSubsetStatusMap[subset.Name], appsv1alpha1.SubsetSchedulable)
				if oldCondition != nil {
					setWorkloadSpreadSubsetCondition(subsetStatus, oldCondition.DeepCopy())
				}
				setWorkloadSpreadSubsetCondition(subsetStatus, NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionTrue, "", ""))
			}
		} else {
			removeWorkloadSpreadSubsetCondition(subsetStatus, appsv1alpha1.SubsetSchedulable)
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
	var active int32

	for _, pod := range pods {
		// remove this Pod from creatingPods map because this Pod has been created.
		injectWS := getInjectWorkloadSpreadFromPod(pod)
		if injectWS != nil && injectWS.UID != "" {
			// Deployment or other native k8s workload has not generated the full pod.Name when webhook is mutating Pod.
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

		active++
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
					"DeletePodFailed", "Pod %s/%s was expected to be deleted but it wasn't", ws.Namespace, pod.Name)
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

	// record active replicas number
	subsetStatus.Replicas = active

	// oldCreatingPods has remaining Pods that not be found by controller.
	for podID, createTime := range oldCreatingPods {
		expectedCreation := createTime.Time.Add(CreatPodTimeout)
		// created this Pod timeout
		if expectedCreation.Before(currentTime) {
			r.recorder.Eventf(ws, corev1.EventTypeWarning,
				"CreatePodFailed", "Pod %s/%s was expected to be created but it wasn't", ws.Namespace, podID)
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
		// status.ObservedWorkloadReplicas == ws.Status.ObservedWorkloadReplicas &&
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

		if oldStatus.Replicas != newStatus.Replicas {
			log += fmt.Sprintf(" <Replicas: %d -> %d>", oldStatus.Replicas, newStatus.Replicas)
		} else {
			log += fmt.Sprintf(" <Replicas: %d>", newStatus.Replicas)
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
	unlock := util.GlobalKeyedMutex.Lock(string(ws.GetUID()))
	defer unlock()
	// If this update fails, don't retry it. Allow the failure to get handled &
	// retried in `processNextWorkItem()`.
	err := r.Status().Update(context.TODO(), ws)
	if err == nil {
		if cacheErr := util.GlobalCache.Add(ws); cacheErr != nil {
			klog.Warningf("Failed to update workloadSpread(%s/%s) cache after update status, err: %v", ws.Namespace, ws.Name, cacheErr)
		}
	}
	return err
}

func getWorkloadSpreadKey(o metav1.Object) string {
	return o.GetNamespace() + "/" + o.GetName()
}

func isNotMatchedWS(injectWS *wsutil.InjectWorkloadSpread, ws *appsv1alpha1.WorkloadSpread) bool {
	if injectWS == nil || injectWS.Name != ws.Name || injectWS.Subset == "" {
		return true
	}
	return false
}
