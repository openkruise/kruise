/*
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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

package cloneset

import (
	"context"
	"flag"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclient "github.com/openkruise/kruise/pkg/client"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	revisioncontrol "github.com/openkruise/kruise/pkg/controller/cloneset/revision"
	synccontrol "github.com/openkruise/kruise/pkg/controller/cloneset/sync"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
	historyutil "github.com/openkruise/kruise/pkg/util/history"
	imagejobutilfunc "github.com/openkruise/kruise/pkg/util/imagejob/utilfunction"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	"github.com/openkruise/kruise/pkg/util/refmanager"
	"github.com/openkruise/kruise/pkg/util/volumeclaimtemplate"

	"github.com/prometheus/client_golang/prometheus"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	klog "k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func init() {
	flag.IntVar(&concurrentReconciles, "cloneset-workers", concurrentReconciles, "Max concurrent workers for CloneSet controller.")
	// register prometheus
	metrics.Registry.MustRegister(CloneSetScaleExpectationLeakageMetrics)
}

var (
	concurrentReconciles = 3

	isPreDownloadDisabled             bool
	minimumReplicasToPreDownloadImage int32 = 3
)

var (
	CloneSetScaleExpectationLeakageMetrics = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloneset_scale_expectation_leakage",
			Help: "CloneSet Scale Expectation Leakage Metrics",
			// cloneSet namespace, name
		}, []string{"namespace", "name"},
	)
)

// Add creates a new CloneSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(clonesetutils.ControllerKind) {
		return nil
	}
	if !utildiscovery.DiscoverGVK(appsv1alpha1.SchemeGroupVersion.WithKind("ImagePullJob")) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.PreDownloadImageForInPlaceUpdate) {
		isPreDownloadDisabled = true
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor("cloneset-controller")
	if cli := kruiseclient.GetGenericClientWithName("cloneset-controller"); cli != nil {
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartLogging(klog.Infof)
		eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: cli.KubeClient.CoreV1().Events("")})
		recorder = eventBroadcaster.NewRecorder(mgr.GetScheme(), v1.EventSource{Component: "cloneset-controller"})
	}
	cli := utilclient.NewClientFromManager(mgr, "cloneset-controller")
	reconciler := &ReconcileCloneSet{
		Client:            cli,
		scheme:            mgr.GetScheme(),
		recorder:          recorder,
		statusUpdater:     newStatusUpdater(cli),
		controllerHistory: historyutil.NewHistory(cli),
		revisionControl:   revisioncontrol.NewRevisionControl(),
	}
	reconciler.syncControl = synccontrol.New(cli, reconciler.recorder)
	reconciler.reconcileFunc = reconciler.doReconcile
	return reconciler
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cloneset-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to CloneSet
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.CloneSet{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.CloneSet]{},
		predicate.TypedFuncs[*appsv1alpha1.CloneSet]{
			UpdateFunc: func(e event.TypedUpdateEvent[*appsv1alpha1.CloneSet]) bool {
				oldCS := e.ObjectOld
				newCS := e.ObjectNew
				if *oldCS.Spec.Replicas != *newCS.Spec.Replicas {
					klog.V(4).InfoS("Observed updated replicas for CloneSet",
						"cloneSet", klog.KObj(newCS), "oldReplicas", *oldCS.Spec.Replicas, "newReplicas", *newCS.Spec.Replicas)
				}
				return true
			},
		}))
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.Pod{}, &podEventHandler{Reader: mgr.GetCache()}))
	if err != nil {
		return err
	}

	// Watch for changes to PVC, just ensure cache updated
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.PersistentVolumeClaim{}, &pvcEventHandler{}))
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCloneSet{}

// ReconcileCloneSet reconciles a CloneSet object
type ReconcileCloneSet struct {
	client.Client
	scheme        *runtime.Scheme
	reconcileFunc func(request reconcile.Request) (reconcile.Result, error)

	recorder          record.EventRecorder
	controllerHistory history.Interface
	statusUpdater     StatusUpdater
	revisionControl   revisioncontrol.Interface
	syncControl       synccontrol.Interface
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets/finalizers,verbs=update

// Reconcile reads that state of the cluster for a CloneSet object and makes changes based on the state read
// and what is in the CloneSet.Spec
func (r *ReconcileCloneSet) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	return r.reconcileFunc(request)
}

func (r *ReconcileCloneSet) doReconcile(request reconcile.Request) (res reconcile.Result, retErr error) {
	startTime := time.Now()
	defer func() {
		if retErr == nil {
			if res.Requeue || res.RequeueAfter > 0 {
				klog.InfoS("Finished syncing CloneSet", "cloneSet", request, "cost", time.Since(startTime), "result", res)
			} else {
				klog.InfoS("Finished syncing CloneSet", "cloneSet", request, "cost", time.Since(startTime))
			}
		} else {
			klog.ErrorS(retErr, "Failed syncing CloneSet", "cloneSet", request)
		}
		// clean the duration store
		_ = clonesetutils.DurationStore.Pop(request.String())
	}()

	// Fetch the CloneSet instance
	instance := &appsv1alpha1.CloneSet{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			klog.V(3).InfoS("CloneSet has been deleted", "cloneSet", request)
			clonesetutils.ScaleExpectations.DeleteExpectations(request.String())
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	coreControl := clonesetcore.New(instance)
	if coreControl.IsInitializing() {
		klog.V(4).InfoS("CloneSet skipped reconcile for initializing", "cloneSet", request)
		return reconcile.Result{}, nil
	}

	selector, err := util.ValidatedLabelSelectorAsSelector(instance.Spec.Selector)
	if err != nil {
		klog.ErrorS(err, "Error converting CloneSet selector", "cloneSet", request)
		// This is a non-transient error, so don't retry.
		return reconcile.Result{}, nil
	}

	// If scaling expectations have not satisfied yet, just skip this reconcile.
	if scaleSatisfied, unsatisfiedDuration, scaleDirtyPods := clonesetutils.ScaleExpectations.SatisfiedExpectations(request.String()); !scaleSatisfied {
		if unsatisfiedDuration >= expectations.ExpectationTimeout {
			// In some extreme scenarios, if the Pod is created and then quickly deleted, there may be event loss.
			// Therefore, a touting mechanism is needed to ensure that clonesets can continue to work.
			klog.InfoS("Expectation unsatisfied overtime", "cloneSet", request, "scaleDirtyPods", scaleDirtyPods, "overTime", unsatisfiedDuration)
			CloneSetScaleExpectationLeakageMetrics.WithLabelValues(request.Namespace, request.Name).Add(1)
			// TODO: check the existence of resource in apiserver using client-go directly
			/*for _, pods := range scaleDirtyPods {
				for _, name := range pods {
					_, err = kubeClient.GetGenericClient().KubeClient.CoreV1().Pods(request.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
					if err == nil {
						klog.Warningf("CloneSet(%s/%s) ScaleExpectations leakage, but Pod(%s) already exist", request.Namespace, request.Name, name)
						return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
					} else if !errors.IsNotFound(err) {
						klog.ErrorS(err, "Failed to get Pod", "cloneSet", request, "pod", name)
						return reconcile.Result{RequeueAfter: 3 * time.Second}, nil
					}
				}
			}
			klog.InfoS("CloneSet ScaleExpectation DirtyPods no longer exists, and delete ScaleExpectation", "cloneSet", request)*/
			if utilfeature.DefaultFeatureGate.Enabled(features.ForceDeleteTimeoutExpectationFeatureGate) {
				klog.InfoS("Expectation unsatisfied overtime, and force delete the timeout Expectation", "cloneSet", request, "scaleDirtyPods", scaleDirtyPods, "overTime", unsatisfiedDuration)
				clonesetutils.ScaleExpectations.DeleteExpectations(request.String())
				// In order to avoid the scale expectation timeout,
				// there is no subsequent Pod, CloneSet event causing CloneSet not to be scheduled
				return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
			}
			return reconcile.Result{}, nil
		}
		klog.V(4).InfoS("Not satisfied scale", "cloneSet", request, "scaleDirtyPods", scaleDirtyPods)
		return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
	}

	// list active and inactive Pods belongs to cs
	filteredPods, filterOutPods, err := r.getOwnedPods(instance)
	if err != nil {
		return reconcile.Result{}, err
	}
	// filteredPVCS's ownerRef is CloneSet
	filteredPVCs, err := r.getOwnedPVCs(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// If cloneSet doesn't want to reuse pvc, clean up
	// the existing pvc first, which are owned by inactive or deleted pods.
	if instance.Spec.ScaleStrategy.DisablePVCReuse {
		filteredPVCs, err = r.cleanupPVCs(instance, filteredPods, filterOutPods, filteredPVCs)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	//release Pods ownerRef
	filteredPods, err = r.claimPods(instance, filteredPods)
	if err != nil {
		return reconcile.Result{}, err
	}

	// list all revisions and sort them
	revisions, err := r.controllerHistory.ListControllerRevisions(instance, selector)
	if err != nil {
		return reconcile.Result{}, err
	}
	history.SortControllerRevisions(revisions)

	// get the current, and update revisions
	currentRevision, updateRevision, collisionCount, err := r.getActiveRevisions(instance, revisions)
	if err != nil {
		return reconcile.Result{}, err
	}

	// If resourceVersion expectations have not satisfied yet, just skip this reconcile
	clonesetutils.ResourceVersionExpectations.Observe(updateRevision)
	if isSatisfied, unsatisfiedDuration := clonesetutils.ResourceVersionExpectations.IsSatisfied(updateRevision); !isSatisfied {
		if unsatisfiedDuration < expectations.ExpectationTimeout {
			klog.V(4).InfoS("Not satisfied resourceVersion for CloneSet, wait for updateRevision updating", "cloneSet", request, "updateRevisionName", updateRevision.Name)
			return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
		}
		klog.InfoS("Expectation unsatisfied overtime for CloneSet, wait for updateRevision updating timeout", "cloneSet", request, "updateRevisionName", updateRevision.Name, "timeout", unsatisfiedDuration)
		clonesetutils.ResourceVersionExpectations.Delete(updateRevision)
	}
	for _, pod := range filteredPods {
		clonesetutils.ResourceVersionExpectations.Observe(pod)
		if isSatisfied, unsatisfiedDuration := clonesetutils.ResourceVersionExpectations.IsSatisfied(pod); !isSatisfied {
			if unsatisfiedDuration >= expectations.ExpectationTimeout {
				klog.InfoS("Expectation unsatisfied overtime for CloneSet, wait for pod updating timeout", "cloneSet", request, "pod", klog.KObj(pod), "timeout", unsatisfiedDuration)
				return reconcile.Result{}, nil
			}
			klog.V(4).InfoS("Not satisfied resourceVersion for CloneSet, wait for pod updating", "cloneSet", request, "pod", klog.KObj(pod))
			return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
		}
	}

	newStatus := appsv1alpha1.CloneSetStatus{
		ObservedGeneration: instance.Generation,
		CurrentRevision:    currentRevision.Name,
		UpdateRevision:     updateRevision.Name,
		CollisionCount:     new(int32),
		LabelSelector:      selector.String(),
	}
	*newStatus.CollisionCount = collisionCount

	if !isPreDownloadDisabled {
		if currentRevision.Name != updateRevision.Name {
			// get clone pre-download annotation
			minUpdatedReadyPodsCount := 0
			if minUpdatedReadyPods, ok := instance.Annotations[appsv1alpha1.ImagePreDownloadMinUpdatedReadyPods]; ok {
				minUpdatedReadyPodsIntStr := intstrutil.Parse(minUpdatedReadyPods)
				minUpdatedReadyPodsCount, err = intstrutil.GetScaledValueFromIntOrPercent(&minUpdatedReadyPodsIntStr, int(*instance.Spec.Replicas), true)
				if err != nil {
					klog.ErrorS(err, "Failed to GetScaledValueFromIntOrPercent of minUpdatedReadyPods for CloneSet", "cloneSet", request)
				}
			}
			updatedReadyReplicas := instance.Status.UpdatedReadyReplicas
			if updateRevision.Name != instance.Status.UpdateRevision {
				updatedReadyReplicas = 0
			}
			if int32(minUpdatedReadyPodsCount) <= updatedReadyReplicas {
				// pre-download images for new revision
				if err := r.createImagePullJobsForInPlaceUpdate(instance, currentRevision, updateRevision); err != nil {
					klog.ErrorS(err, "Failed to create ImagePullJobs for CloneSet", "cloneSet", request)
				}
			}
		} else {
			// delete ImagePullJobs if revisions have been consistent
			if err := imagejobutilfunc.DeleteJobsForWorkload(r.Client, instance); err != nil {
				klog.ErrorS(err, "Failed to delete imagepulljobs for CloneSet", "cloneSet", request)
			}
		}
	}

	// scale and update pods
	syncErr := r.syncCloneSet(instance, &newStatus, currentRevision, updateRevision, revisions, filteredPods, filteredPVCs)

	// update new status
	if err = r.statusUpdater.UpdateCloneSetStatus(instance, &newStatus, filteredPods); err != nil {
		return reconcile.Result{}, err
	}

	if err = r.truncatePodsToDelete(instance, filteredPods); err != nil {
		klog.ErrorS(err, "Failed to truncate podsToDelete for CloneSet", "cloneSet", request)
	}

	if err = r.truncateHistory(instance, filteredPods, revisions, currentRevision, updateRevision); err != nil {
		klog.ErrorS(err, "Failed to truncate history for CloneSet", "cloneSet", request)
	}

	if syncErr == nil && instance.Spec.MinReadySeconds > 0 && newStatus.AvailableReplicas != newStatus.ReadyReplicas {
		clonesetutils.DurationStore.Push(request.String(), time.Second*time.Duration(instance.Spec.MinReadySeconds))
	}
	return reconcile.Result{RequeueAfter: clonesetutils.DurationStore.Pop(request.String())}, syncErr
}

func (r *ReconcileCloneSet) syncCloneSet(
	instance *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus,
	currentRevision, updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
	filteredPods []*v1.Pod, filteredPVCs []*v1.PersistentVolumeClaim,
) error {
	if instance.DeletionTimestamp != nil {
		return nil
	}

	// get the current and update revisions of the set.
	currentSet, err := r.revisionControl.ApplyRevision(instance, currentRevision)
	if err != nil {
		return err
	}
	updateSet, err := r.revisionControl.ApplyRevision(instance, updateRevision)
	if err != nil {
		return err
	}

	var scaling bool
	var podsScaleErr error
	var podsUpdateErr error

	scaling, podsScaleErr = r.syncControl.Scale(currentSet, updateSet, currentRevision.Name, updateRevision.Name, filteredPods, filteredPVCs)
	if podsScaleErr != nil {
		newStatus.Conditions = append(newStatus.Conditions, appsv1alpha1.CloneSetCondition{
			Type:               appsv1alpha1.CloneSetConditionFailedScale,
			Status:             v1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Message:            podsScaleErr.Error(),
		})
		err = podsScaleErr
	}
	if scaling {
		return podsScaleErr
	}

	podsUpdateErr = r.syncControl.Update(updateSet, currentRevision, updateRevision, revisions, filteredPods, filteredPVCs)
	if podsUpdateErr != nil {
		newStatus.Conditions = append(newStatus.Conditions, appsv1alpha1.CloneSetCondition{
			Type:               appsv1alpha1.CloneSetConditionFailedUpdate,
			Status:             v1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Message:            podsUpdateErr.Error(),
		})
		if err == nil {
			err = podsUpdateErr
		}
	}

	return err
}

func (r *ReconcileCloneSet) getActiveRevisions(cs *appsv1alpha1.CloneSet, revisions []*apps.ControllerRevision) (
	*apps.ControllerRevision, *apps.ControllerRevision, int32, error,
) {
	var currentRevision, updateRevision *apps.ControllerRevision
	revisionCount := len(revisions)

	// Use a local copy of cs.Status.CollisionCount to avoid modifying cs.Status directly.
	// This copy is returned so the value gets carried over to cs.Status in UpdateCloneSetStatus.
	var collisionCount int32
	if cs.Status.CollisionCount != nil {
		collisionCount = *cs.Status.CollisionCount
	}

	// create a new revision from the current cs
	updateRevision, err := r.revisionControl.NewRevision(cs, clonesetutils.NextRevision(revisions), &collisionCount)
	if err != nil {
		return nil, nil, collisionCount, err
	}

	// Here, we do not consider the case where the volumeClaimTemplates Hash was not added before the upgrade feature.
	// 1. It is not possible to actually extract the previous volumeClaimTemplates Hash.
	// 2. the hash must exist in all revisions that have been updated after the feature is enabled.
	VCTHashEqual := func(revision *apps.ControllerRevision, needle *apps.ControllerRevision) bool {
		needleVolumeClaimHash, _ := volumeclaimtemplate.GetVCTemplatesHash(needle)
		tmpHash, exist := volumeclaimtemplate.GetVCTemplatesHash(revision)
		// vcTemplate hash may not exist in old revision, use hash of new revision instead
		if !exist || tmpHash != needleVolumeClaimHash {
			return false
		}
		return true
	}

	// When there is a change in the PVC only, no new revision will be generated.
	// find any equivalent revisions
	equalRevisions := history.FindEqualRevisions(revisions, updateRevision)
	equalCount := len(equalRevisions)
	if equalCount > 0 && history.EqualRevision(revisions[revisionCount-1], equalRevisions[equalCount-1]) {
		// if the equivalent revision is immediately prior the update revision has not changed
		updateRevision = revisions[revisionCount-1]
	} else if equalCount > 0 {
		lastEqualRevision := equalRevisions[equalCount-1]
		if !VCTHashEqual(lastEqualRevision, updateRevision) {
			klog.InfoS("Revision vct hash will be updated", "revisionName", lastEqualRevision.Name, "lastRevisionVCTHash", lastEqualRevision.Annotations[volumeclaimtemplate.HashAnnotation], "updateRevisionVCTHash", updateRevision.Annotations[volumeclaimtemplate.HashAnnotation])
			lastEqualRevision.Annotations[volumeclaimtemplate.HashAnnotation] = updateRevision.Annotations[volumeclaimtemplate.HashAnnotation]
		}
		// if the equivalent revision is not immediately prior we will roll back by incrementing the
		// Revision of the equivalent revision
		updateRevision, err = r.controllerHistory.UpdateControllerRevision(equalRevisions[equalCount-1], updateRevision.Revision)
		if err != nil {
			return nil, nil, collisionCount, err
		}
	} else {
		//if there is no equivalent revision we create a new one
		updateRevision, err = r.controllerHistory.CreateControllerRevision(cs, updateRevision, &collisionCount)
		if err != nil {
			return nil, nil, collisionCount, err
		}
	}

	// attempt to find the revision that corresponds to the current revision
	for i := range revisions {
		if revisions[i].Name == cs.Status.CurrentRevision {
			currentRevision = revisions[i]
			break
		}
	}

	// if the current revision is nil we initialize the history by setting it to the update revision
	if currentRevision == nil {
		currentRevision = updateRevision
	}

	return currentRevision, updateRevision, collisionCount, nil
}

func (r *ReconcileCloneSet) getOwnedPods(cs *appsv1alpha1.CloneSet) ([]*v1.Pod, []*v1.Pod, error) {
	opts := &client.ListOptions{
		Namespace:     cs.Namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(cs.UID)}),
	}
	return clonesetutils.GetActiveAndInactivePods(r.Client, opts)
}

func (r *ReconcileCloneSet) getOwnedPVCs(cs *appsv1alpha1.CloneSet) ([]*v1.PersistentVolumeClaim, error) {
	opts := &client.ListOptions{
		Namespace:     cs.Namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(cs.UID)}),
	}

	pvcList := v1.PersistentVolumeClaimList{}
	if err := r.List(context.TODO(), &pvcList, opts, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	var filteredPVCs []*v1.PersistentVolumeClaim
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if pvc.DeletionTimestamp == nil {
			filteredPVCs = append(filteredPVCs, pvc)
		}
	}

	return filteredPVCs, nil
}

// truncatePodsToDelete truncates any non-live pod names in spec.scaleStrategy.podsToDelete.
func (r *ReconcileCloneSet) truncatePodsToDelete(cs *appsv1alpha1.CloneSet, pods []*v1.Pod) error {
	if len(cs.Spec.ScaleStrategy.PodsToDelete) == 0 {
		return nil
	}

	existingPods := sets.NewString()
	for _, p := range pods {
		existingPods.Insert(p.Name)
	}

	var newPodsToDelete []string
	for _, podName := range cs.Spec.ScaleStrategy.PodsToDelete {
		if existingPods.Has(podName) {
			newPodsToDelete = append(newPodsToDelete, podName)
		}
	}

	if len(newPodsToDelete) == len(cs.Spec.ScaleStrategy.PodsToDelete) {
		return nil
	}

	newCS := cs.DeepCopy()
	newCS.Spec.ScaleStrategy.PodsToDelete = newPodsToDelete
	return r.Update(context.TODO(), newCS)
}

// truncateHistory truncates any non-live ControllerRevisions in revisions from cs's history. The UpdateRevision and
// CurrentRevision in cs's Status are considered to be live. Any revisions associated with the Pods in pods are also
// considered to be live. Non-live revisions are deleted, starting with the revision with the lowest Revision, until
// only RevisionHistoryLimit revisions remain. If the returned error is nil the operation was successful. This method
// expects that revisions is sorted when supplied.
func (r *ReconcileCloneSet) truncateHistory(
	cs *appsv1alpha1.CloneSet,
	pods []*v1.Pod,
	revisions []*apps.ControllerRevision,
	current *apps.ControllerRevision,
	update *apps.ControllerRevision,
) error {
	noLiveRevisions := make([]*apps.ControllerRevision, 0, len(revisions))

	// collect live revisions and historic revisions
	for i := range revisions {
		if revisions[i].Name != current.Name && revisions[i].Name != update.Name {
			var found bool
			for _, pod := range pods {
				if clonesetutils.EqualToRevisionHash("", pod, revisions[i].Name) {
					found = true
					break
				}
			}
			if !found {
				noLiveRevisions = append(noLiveRevisions, revisions[i])
			}
		}
	}
	historyLen := len(noLiveRevisions)
	historyLimit := 10
	if cs.Spec.RevisionHistoryLimit != nil {
		historyLimit = int(*cs.Spec.RevisionHistoryLimit)
	}
	if historyLen <= historyLimit {
		return nil
	}
	// delete any non-live history to maintain the revision limit.
	noLiveRevisions = noLiveRevisions[:(historyLen - historyLimit)]
	for i := 0; i < len(noLiveRevisions); i++ {
		if err := r.controllerHistory.DeleteControllerRevision(noLiveRevisions[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileCloneSet) claimPods(instance *appsv1alpha1.CloneSet, pods []*v1.Pod) ([]*v1.Pod, error) {
	mgr, err := refmanager.New(r, instance.Spec.Selector, instance, r.scheme)
	if err != nil {
		return nil, err
	}

	selected := make([]metav1.Object, len(pods))
	for i, pod := range pods {
		selected[i] = pod
	}

	claimed, err := mgr.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	claimedPods := make([]*v1.Pod, len(claimed))
	for i, pod := range claimed {
		claimedPods[i] = pod.(*v1.Pod)
	}

	return claimedPods, nil
}

// cleanUp unUsed pvcs, and return used pvcs.
// If pvc owner pod does not exist, the pvc can be deleted directly,
// else update pvc's ownerReference to pod.
func (r *ReconcileCloneSet) cleanupPVCs(
	cs *appsv1alpha1.CloneSet,
	activePods, inactivePods []*v1.Pod,
	pvcs []*v1.PersistentVolumeClaim,
) ([]*v1.PersistentVolumeClaim, error) {
	activeIds := sets.NewString()
	for _, pod := range activePods {
		if id := clonesetutils.GetInstanceID(pod); id != "" {
			activeIds.Insert(id)
		}
	}
	inactiveIds := map[string]*v1.Pod{}
	for i := range inactivePods {
		pod := inactivePods[i]
		if id := clonesetutils.GetInstanceID(pod); id != "" {
			inactiveIds[id] = pod
		}
	}
	var inactivePVCs, toDeletePVCs, activePVCs []*v1.PersistentVolumeClaim
	for i := range pvcs {
		pvc := pvcs[i]
		if activeIds.Has(clonesetutils.GetInstanceID(pvc)) {
			activePVCs = append(activePVCs, pvc)
			continue
		}
		_, ok := inactiveIds[clonesetutils.GetInstanceID(pvc)]
		if ok {
			inactivePVCs = append(inactivePVCs, pvc.DeepCopy())
		} else {
			toDeletePVCs = append(toDeletePVCs, pvc.DeepCopy())
		}
	}
	if len(inactivePVCs) == 0 && len(toDeletePVCs) == 0 {
		return activePVCs, nil
	}

	// update useless pvc owner to pod
	for i := range inactivePVCs {
		pvc := inactivePVCs[i]
		pod := inactiveIds[clonesetutils.GetInstanceID(pvc)]
		// There is no need to judge whether the ownerRef has met expectations
		// because the pvc(listed in the previous list)'s ownerRef must be CloneSet
		_ = updateClaimOwnerRefToPod(pvc, cs, pod)
		if err := r.updateOnePVC(cs, pvc); err != nil && !errors.IsNotFound(err) {
			return nil, err
		}
		klog.V(3).InfoS("Updated CloneSet pvc's ownerRef to Pod", "cloneSet", klog.KObj(cs), "pvc", klog.KObj(pvc))
	}
	// delete pvc directly
	for i := range toDeletePVCs {
		pvc := toDeletePVCs[i]
		if err := r.deleteOnePVC(cs, pvc); err != nil && !errors.IsNotFound(err) {
			return nil, err
		}
		klog.V(3).InfoS("Deleted CloneSet pvc directly", "cloneSet", klog.KObj(cs), "pvc", klog.KObj(pvc))
	}
	return activePVCs, nil
}

func (r *ReconcileCloneSet) updateOnePVC(cs *appsv1alpha1.CloneSet, pvc *v1.PersistentVolumeClaim) error {
	if err := r.Client.Update(context.TODO(), pvc); err != nil {
		r.recorder.Eventf(cs, v1.EventTypeWarning, "FailedUpdate", "failed to update PVC %s: %v", pvc.Name, err)
		return err
	}
	return nil
}

func (r *ReconcileCloneSet) deleteOnePVC(cs *appsv1alpha1.CloneSet, pvc *v1.PersistentVolumeClaim) error {
	clonesetutils.ScaleExpectations.ExpectScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
	if err := r.Delete(context.TODO(), pvc); err != nil {
		clonesetutils.ScaleExpectations.ObserveScale(clonesetutils.GetControllerKey(cs), expectations.Delete, pvc.Name)
		r.recorder.Eventf(cs, v1.EventTypeWarning, "FailedDelete", "failed to clean up PVC %s: %v", pvc.Name, err)
		return err
	}
	return nil
}

func updateClaimOwnerRefToPod(pvc *v1.PersistentVolumeClaim, cs *appsv1alpha1.CloneSet, pod *v1.Pod) bool {
	util.RemoveOwnerRef(pvc, cs)
	return util.SetOwnerRef(pvc, pod, schema.GroupVersionKind{Version: "v1", Kind: "Pod"})
}
