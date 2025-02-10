/*
Copyright 2020 The Kruise Authors.
Copyright 2015 The Kubernetes Authors.

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

package daemonset

import (
	"context"
	"flag"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/retry"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/daemon/util"
	"k8s.io/utils/integer"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/client/clientset/versioned/scheme"
	kruiseappslisters "github.com/openkruise/kruise/pkg/client/listers/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	kruiseutil "github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	kruiseExpectations "github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	imagejobutilfunc "github.com/openkruise/kruise/pkg/util/imagejob/utilfunction"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	"github.com/openkruise/kruise/pkg/util/revisionadapter"
)

func init() {
	flag.BoolVar(&scheduleDaemonSetPods, "assign-pods-by-scheduler", true, "Use scheduler to assign pod to node.")
	flag.IntVar(&concurrentReconciles, "daemonset-workers", concurrentReconciles, "Max concurrent workers for DaemonSet controller.")
}

var (
	concurrentReconciles  = 3
	scheduleDaemonSetPods bool

	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = appsv1alpha1.SchemeGroupVersion.WithKind("DaemonSet")

	onceBackoffGC sync.Once
	// this is a short cut for any sub-functions to notify the reconcile how long to wait to requeue
	durationStore = requeueduration.DurationStore{}

	isPreDownloadDisabled bool
)

const (
	// BurstReplicas is a rate limiter for booting pods on a lot of pods.
	// The value of 250 is chosen b/c values that are too high can cause registry DoS issues.
	BurstReplicas = 250

	// ProgressiveCreatePod indicates daemon pods created in manage phase will be controlled by partition.
	// This annotation will be added to DaemonSet when it is created, and removed if partition is set to 0.
	ProgressiveCreatePod = "daemonset.kruise.io/progressive-create-pod"

	// BackoffGCInterval is the time that has to pass before next iteration of backoff GC is run
	BackoffGCInterval = 1 * time.Minute
)

// Reasons for DaemonSet events
const (
	// SelectingAllReason is added to an event when a DaemonSet selects all Pods.
	SelectingAllReason = "SelectingAll"
	// FailedPlacementReason is added to an event when a DaemonSet can't schedule a Pod to a specified node.
	FailedPlacementReason = "FailedPlacement"
	// FailedDaemonPodReason is added to an event when the status of a Pod of a DaemonSet is 'Failed'.
	FailedDaemonPodReason = "FailedDaemonPod"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new DaemonSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	if !utildiscovery.DiscoverGVK(appsv1alpha1.SchemeGroupVersion.WithKind("ImagePullJob")) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.PreDownloadImageForDaemonSetUpdate) {
		isPreDownloadDisabled = true
	}
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	genericClient := client.GetGenericClientWithName("daemonset-controller")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: genericClient.KubeClient.CoreV1().Events("")})

	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "daemonset-controller"})
	cacher := mgr.GetCache()

	dsInformer, err := cacher.GetInformerForKind(context.TODO(), controllerKind)
	if err != nil {
		return nil, err
	}
	podInformer, err := cacher.GetInformerForKind(context.TODO(), corev1.SchemeGroupVersion.WithKind("Pod"))
	if err != nil {
		return nil, err
	}
	nodeInformer, err := cacher.GetInformerForKind(context.TODO(), corev1.SchemeGroupVersion.WithKind("Node"))
	if err != nil {
		return nil, err
	}
	revInformer, err := cacher.GetInformerForKind(context.TODO(), apps.SchemeGroupVersion.WithKind("ControllerRevision"))
	if err != nil {
		return nil, err
	}

	dsLister := kruiseappslisters.NewDaemonSetLister(dsInformer.(cache.SharedIndexInformer).GetIndexer())
	historyLister := appslisters.NewControllerRevisionLister(revInformer.(cache.SharedIndexInformer).GetIndexer())
	podLister := corelisters.NewPodLister(podInformer.(cache.SharedIndexInformer).GetIndexer())
	nodeLister := corelisters.NewNodeLister(nodeInformer.(cache.SharedIndexInformer).GetIndexer())
	failedPodsBackoff := flowcontrol.NewBackOff(1*time.Second, 15*time.Minute)
	revisionAdapter := revisionadapter.NewDefaultImpl()

	cli := utilclient.NewClientFromManager(mgr, "daemonset-controller")
	dsc := &ReconcileDaemonSet{
		Client:        cli,
		kubeClient:    genericClient.KubeClient,
		kruiseClient:  genericClient.KruiseClient,
		eventRecorder: recorder,
		podControl:    kubecontroller.RealPodControl{KubeClient: genericClient.KubeClient, Recorder: recorder},
		crControl: kubecontroller.RealControllerRevisionControl{
			KubeClient: genericClient.KubeClient,
		},
		lifecycleControl:            lifecycle.New(cli),
		expectations:                kubecontroller.NewControllerExpectations(),
		resourceVersionExpectations: kruiseExpectations.NewResourceVersionExpectation(),
		dsLister:                    dsLister,
		historyLister:               historyLister,
		podLister:                   podLister,
		nodeLister:                  nodeLister,
		failedPodsBackoff:           failedPodsBackoff,
		inplaceControl:              inplaceupdate.New(cli, revisionAdapter),
		revisionAdapter:             revisionAdapter,
	}
	return dsc, err
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("daemonset-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: kruiseutil.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	dsc := r.(*ReconcileDaemonSet)

	logger := klog.FromContext(context.TODO())
	// Watch for changes to DaemonSet
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.DaemonSet{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.DaemonSet]{}, predicate.TypedFuncs[*appsv1alpha1.DaemonSet]{
		CreateFunc: func(e event.TypedCreateEvent[*appsv1alpha1.DaemonSet]) bool {
			ds := e.Object
			klog.V(4).InfoS("Adding DaemonSet", "daemonSet", klog.KObj(ds))
			return true
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*appsv1alpha1.DaemonSet]) bool {
			oldDS := e.ObjectOld
			newDS := e.ObjectNew
			if oldDS.UID != newDS.UID {
				dsc.expectations.DeleteExpectations(logger, keyFunc(oldDS))
			}
			klog.V(4).InfoS("Updating DaemonSet", "daemonSet", klog.KObj(newDS))
			return true
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*appsv1alpha1.DaemonSet]) bool {
			ds := e.Object
			klog.V(4).InfoS("Deleting DaemonSet", "daemonSet", klog.KObj(ds))
			dsc.expectations.DeleteExpectations(logger, keyFunc(ds))
			newPodForDSCache.Delete(ds.UID)
			return true
		},
	}))
	if err != nil {
		return err
	}

	// Watch for changes to Node.
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Node{}, &nodeEventHandler{reader: mgr.GetCache()}))
	if err != nil {
		return err
	}

	// Watch for changes to Pod created by DaemonSet
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &podEventHandler{Reader: mgr.GetCache(), expectations: dsc.expectations}))
	if err != nil {
		return err
	}

	// TODO: Do we need to watch ControllerRevision?

	klog.V(4).InfoS("Finished to add daemonset-controller")
	return nil
}

var _ reconcile.Reconciler = &ReconcileDaemonSet{}

// ReconcileDaemonSet reconciles a DaemonSet object
type ReconcileDaemonSet struct {
	runtimeclient.Client
	kubeClient       clientset.Interface
	kruiseClient     kruiseclientset.Interface
	eventRecorder    record.EventRecorder
	podControl       kubecontroller.PodControlInterface
	crControl        kubecontroller.ControllerRevisionControlInterface
	lifecycleControl lifecycle.Interface

	// A TTLCache of pod creates/deletes each ds expects to see
	expectations kubecontroller.ControllerExpectationsInterface
	// A cache of pod resourceVersion expecatations
	resourceVersionExpectations kruiseExpectations.ResourceVersionExpectation

	// dsLister can list/get daemonsets from the shared informer's store
	dsLister kruiseappslisters.DaemonSetLister
	// historyLister get list/get history from the shared informers's store
	historyLister appslisters.ControllerRevisionLister
	// podLister get list/get pods from the shared informers's store
	podLister corelisters.PodLister
	// nodeLister can list/get nodes from the shared informer's store
	nodeLister corelisters.NodeLister

	failedPodsBackoff *flowcontrol.Backoff

	inplaceControl  inplaceupdate.Interface
	revisionAdapter revisionadapter.Interface
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=daemonsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=daemonsets/finalizers,verbs=update

// Reconcile reads that state of the cluster for a DaemonSet object and makes changes based on the state read
// and what is in the DaemonSet.Spec
func (dsc *ReconcileDaemonSet) Reconcile(ctx context.Context, request reconcile.Request) (res reconcile.Result, retErr error) {
	onceBackoffGC.Do(func() {
		go wait.Until(dsc.failedPodsBackoff.GC, BackoffGCInterval, ctx.Done())
	})
	startTime := time.Now()
	defer func() {
		if retErr == nil {
			if res.Requeue || res.RequeueAfter > 0 {
				klog.InfoS("Finished syncing DaemonSet", "daemonSet", request, "cost", time.Since(startTime), "result", res)
			} else {
				klog.InfoS("Finished syncing DaemonSet", "daemonSet", request, "cost", time.Since(startTime))
			}
		} else {
			klog.ErrorS(retErr, "Failed syncing DaemonSet", "daemonSet", request)
		}
		// clean the duration store
		_ = durationStore.Pop(request.String())
	}()

	err := dsc.syncDaemonSet(ctx, request)
	return reconcile.Result{RequeueAfter: durationStore.Pop(request.String())}, err
}

// getDaemonPods returns daemon pods owned by the given ds.
// This also reconciles ControllerRef by adopting/orphaning.
// Note that returned Pods are pointers to objects in the cache.
// If you want to modify one, you need to deep-copy it first.
func (dsc *ReconcileDaemonSet) getDaemonPods(ctx context.Context, ds *appsv1alpha1.DaemonSet) ([]*corev1.Pod, error) {
	selector, err := kruiseutil.ValidatedLabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return nil, err
	}

	// List all pods to include those that don't match the selector anymore but
	// have a ControllerRef pointing to this controller.
	pods, err := dsc.podLister.Pods(ds.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Pods (see #42639).
	dsNotDeleted := kubecontroller.RecheckDeletionTimestamp(func(ctx context.Context) (metav1.Object, error) {
		fresh, err := dsc.kruiseClient.AppsV1alpha1().DaemonSets(ds.Namespace).Get(ctx, ds.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != ds.UID {
			return nil, fmt.Errorf("original DaemonSet %v/%v is gone: got uid %v, wanted %v", ds.Namespace, ds.Name, fresh.UID, ds.UID)
		}
		return fresh, nil
	})

	// Use ControllerRefManager to adopt/orphan as needed.
	cm := kubecontroller.NewPodControllerRefManager(dsc.podControl, ds, selector, controllerKind, dsNotDeleted)
	return cm.ClaimPods(ctx, pods)
}

func (dsc *ReconcileDaemonSet) syncDaemonSet(ctx context.Context, request reconcile.Request) error {
	logger := klog.FromContext(ctx)
	dsKey := request.NamespacedName.String()
	ds, err := dsc.dsLister.DaemonSets(request.Namespace).Get(request.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).InfoS("DaemonSet has been deleted", "daemonSet", request)
			dsc.expectations.DeleteExpectations(logger, dsKey)
			return nil
		}
		return fmt.Errorf("unable to retrieve DaemonSet %s from store: %v", dsKey, err)
	}

	// Don't process a daemon set until all its creations and deletions have been processed.
	// For example if daemon set foo asked for 3 new daemon pods in the previous call to manage,
	// then we do not want to call manage on foo until the daemon pods have been created.
	if ds.DeletionTimestamp != nil {
		return nil
	}

	everything := metav1.LabelSelector{}
	if reflect.DeepEqual(ds.Spec.Selector, &everything) {
		dsc.eventRecorder.Eventf(ds, corev1.EventTypeWarning, SelectingAllReason, "This DaemonSet is selecting all pods. A non-empty selector is required.")
		return nil
	}

	nodeList, err := dsc.nodeLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("couldn't get list of nodes when syncing DaemonSet %#v: %v", ds, err)
	}

	// Construct histories of the DaemonSet, and get the hash of current history
	cur, old, err := dsc.constructHistory(ctx, ds)
	if err != nil {
		return fmt.Errorf("failed to construct revisions of DaemonSet: %v", err)
	}
	hash := cur.Labels[apps.DefaultDaemonSetUniqueLabelKey]

	if !dsc.expectations.SatisfiedExpectations(logger, dsKey) || !dsc.hasPodExpectationsSatisfied(ctx, ds) {
		return dsc.updateDaemonSetStatus(ctx, ds, nodeList, hash, false)
	}

	if !isPreDownloadDisabled && dsc.Client != nil {
		if ds.Status.UpdatedNumberScheduled != ds.Status.DesiredNumberScheduled ||
			hash != ds.Status.DaemonSetHash {
			// get ads pre-download annotation
			minUpdatedReadyPodsCount := 0
			if minUpdatedReadyPods, ok := ds.Annotations[appsv1alpha1.ImagePreDownloadMinUpdatedReadyPods]; ok {
				minUpdatedReadyPodsIntStr := intstrutil.Parse(minUpdatedReadyPods)
				minUpdatedReadyPodsCount, err = intstrutil.GetScaledValueFromIntOrPercent(&minUpdatedReadyPodsIntStr, int(ds.Status.DesiredNumberScheduled), true)
				if err != nil {
					klog.ErrorS(err, "Failed to GetScaledValueFromIntOrPercent of minUpdatedReadyPods for DaemonSet", "daemonSet", request)
				}
			}
			// todo: check whether the updatedReadyPodsCount greater than minUpdatedReadyPodsCount
			_ = minUpdatedReadyPodsCount
			// pre-download images for new revision
			if err := dsc.createImagePullJobsForInPlaceUpdate(ds, old, cur); err != nil {
				klog.ErrorS(err, "Failed to create ImagePullJobs for DaemonSet", "daemonSet", request)
			}
		} else {
			// delete ImagePullJobs if revisions have been consistent
			if err := imagejobutilfunc.DeleteJobsForWorkload(dsc.Client, ds); err != nil {
				klog.ErrorS(err, "Failed to delete ImagePullJobs for DaemonSet", "daemonSet", request)
			}
		}
	}

	err = dsc.manage(ctx, ds, nodeList, hash)
	if err != nil {
		return err
	}

	// return and wait next reconcile if expectation changed to unsatisfied
	if !dsc.expectations.SatisfiedExpectations(logger, dsKey) || !dsc.hasPodExpectationsSatisfied(ctx, ds) {
		return dsc.updateDaemonSetStatus(ctx, ds, nodeList, hash, false)
	}

	if err := dsc.refreshUpdateStates(ctx, ds); err != nil {
		return err
	}

	// Process rolling updates if we're ready. For all kinds of update should not be executed if the update
	// expectation is not satisfied.
	if !isDaemonSetPaused(ds) {
		switch ds.Spec.UpdateStrategy.Type {
		case appsv1alpha1.OnDeleteDaemonSetStrategyType:
		case appsv1alpha1.RollingUpdateDaemonSetStrategyType:
			err = dsc.rollingUpdate(ctx, ds, nodeList, cur, old)
			if err != nil {
				return err
			}
		}
	}

	err = dsc.cleanupHistory(ctx, ds, old)
	if err != nil {
		return fmt.Errorf("failed to clean up revisions of DaemonSet: %v", err)
	}

	return dsc.updateDaemonSetStatus(ctx, ds, nodeList, hash, true)
}

// Predicates checks if a DaemonSet's pod can run on a node.
func Predicates(pod *corev1.Pod, node *corev1.Node, taints []corev1.Taint) (fitsNodeName, fitsNodeAffinity, fitsTaints bool) {
	fitsNodeName = len(pod.Spec.NodeName) == 0 || pod.Spec.NodeName == node.Name
	// Ignore parsing errors for backwards compatibility.
	fitsNodeAffinity, _ = nodeaffinity.GetRequiredNodeAffinity(pod).Match(node)
	_, hasUntoleratedTaint := v1helper.FindMatchingUntoleratedTaint(taints, pod.Spec.Tolerations, func(t *corev1.Taint) bool {
		return t.Effect == corev1.TaintEffectNoExecute || t.Effect == corev1.TaintEffectNoSchedule
	})
	fitsTaints = !hasUntoleratedTaint
	return
}

func isControlledByDaemonSet(p *corev1.Pod, uuid types.UID) bool {
	for _, ref := range p.OwnerReferences {
		if ref.Controller != nil && *ref.Controller && ref.UID == uuid {
			return true
		}
	}
	return false
}

// NewPod creates a new pod
func NewPod(ds *appsv1alpha1.DaemonSet, nodeName string) *corev1.Pod {
	// firstly load the cache before lock
	if pod := loadNewPodForDS(ds); pod != nil {
		return pod
	}

	newPodForDSLock.Lock()
	defer newPodForDSLock.Unlock()

	// load the cache again after locked
	if pod := loadNewPodForDS(ds); pod != nil {
		return pod
	}

	newPod := &corev1.Pod{Spec: ds.Spec.Template.Spec, ObjectMeta: ds.Spec.Template.ObjectMeta}
	newPod.Namespace = ds.Namespace
	// no need to set nodeName
	// newPod.Spec.NodeName = nodeName

	// Added default tolerations for DaemonSet pods.
	util.AddOrUpdateDaemonPodTolerations(&newPod.Spec)

	newPodForDSCache.Store(ds.UID, &newPodForDS{generation: ds.Generation, pod: newPod})
	return newPod
}

func (dsc *ReconcileDaemonSet) updateDaemonSetStatus(ctx context.Context, ds *appsv1alpha1.DaemonSet, nodeList []*corev1.Node, hash string, updateObservedGen bool) error {
	nodeToDaemonPods, err := dsc.getNodesToDaemonPods(ctx, ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for DaemonSet %q: %v", ds.Name, err)
	}

	var desiredNumberScheduled, currentNumberScheduled, numberMisscheduled, numberReady, updatedNumberScheduled, numberAvailable int
	now := dsc.failedPodsBackoff.Clock.Now()
	for _, node := range nodeList {
		shouldRun, _ := nodeShouldRunDaemonPod(node, ds)
		scheduled := len(nodeToDaemonPods[node.Name]) > 0

		if shouldRun {
			desiredNumberScheduled++
			if scheduled {
				currentNumberScheduled++
				// Sort the daemon pods by creation time, so that the oldest is first.
				daemonPods := nodeToDaemonPods[node.Name]
				sort.Sort(podByCreationTimestampAndPhase(daemonPods))
				pod := daemonPods[0]
				if podutil.IsPodReady(pod) {
					numberReady++
					if isDaemonPodAvailable(pod, ds.Spec.MinReadySeconds, metav1.Time{Time: now}) {
						numberAvailable++
					}
				}
				// If the returned error is not nil we have a parse error.
				// The controller handles this via the hash.
				generation, err := GetTemplateGeneration(ds)
				if err != nil {
					generation = nil
				}
				if util.IsPodUpdated(pod, hash, generation) {
					updatedNumberScheduled++
				}
			}
		} else {
			if scheduled {
				numberMisscheduled++
			}
		}
	}
	numberUnavailable := desiredNumberScheduled - numberAvailable

	err = dsc.storeDaemonSetStatus(ctx, ds, desiredNumberScheduled, currentNumberScheduled, numberMisscheduled, numberReady, updatedNumberScheduled, numberAvailable, numberUnavailable, updateObservedGen, hash)
	if err != nil {
		return fmt.Errorf("error storing status for DaemonSet %v: %v", ds.Name, err)
	}

	// Resync the DaemonSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	if ds.Spec.MinReadySeconds >= 0 && numberReady != numberAvailable {
		durationStore.Push(keyFunc(ds), time.Duration(ds.Spec.MinReadySeconds)*time.Second)
	}
	return nil
}

func (dsc *ReconcileDaemonSet) storeDaemonSetStatus(
	ctx context.Context,
	ds *appsv1alpha1.DaemonSet,
	desiredNumberScheduled,
	currentNumberScheduled,
	numberMisscheduled,
	numberReady,
	updatedNumberScheduled,
	numberAvailable,
	numberUnavailable int,
	updateObservedGen bool,
	hash string) error {
	if int(ds.Status.DesiredNumberScheduled) == desiredNumberScheduled &&
		int(ds.Status.CurrentNumberScheduled) == currentNumberScheduled &&
		int(ds.Status.NumberMisscheduled) == numberMisscheduled &&
		int(ds.Status.NumberReady) == numberReady &&
		int(ds.Status.UpdatedNumberScheduled) == updatedNumberScheduled &&
		int(ds.Status.NumberAvailable) == numberAvailable &&
		int(ds.Status.NumberUnavailable) == numberUnavailable &&
		ds.Status.ObservedGeneration >= ds.Generation &&
		ds.Status.DaemonSetHash == hash {
		return nil
	}

	dsClient := dsc.kruiseClient.AppsV1alpha1().DaemonSets(ds.Namespace)
	toUpdate := ds.DeepCopy()
	var updateErr, getErr error
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if updateObservedGen {
			toUpdate.Status.ObservedGeneration = ds.Generation
		}
		toUpdate.Status.DesiredNumberScheduled = int32(desiredNumberScheduled)
		toUpdate.Status.CurrentNumberScheduled = int32(currentNumberScheduled)
		toUpdate.Status.NumberMisscheduled = int32(numberMisscheduled)
		toUpdate.Status.NumberReady = int32(numberReady)
		toUpdate.Status.UpdatedNumberScheduled = int32(updatedNumberScheduled)
		toUpdate.Status.NumberAvailable = int32(numberAvailable)
		toUpdate.Status.NumberUnavailable = int32(numberUnavailable)
		toUpdate.Status.DaemonSetHash = hash

		if _, updateErr = dsClient.UpdateStatus(ctx, toUpdate, metav1.UpdateOptions{}); updateErr == nil {
			klog.InfoS("Updated DaemonSet status", "daemonSet", klog.KObj(ds), "status", kruiseutil.DumpJSON(toUpdate.Status))
			return nil
		}

		klog.ErrorS(updateErr, "Failed to Update DaemonSet status", "daemonSet", klog.KObj(ds), "status", ds.Status)
		// Update the set with the latest resource version for the next poll
		if toUpdate, getErr = dsClient.Get(ctx, ds.Name, metav1.GetOptions{}); getErr != nil {
			// If the GET fails we can't trust status.Replicas anymore. This error
			// is bound to be more interesting than the update failure.
			klog.ErrorS(getErr, "Failed to get DaemonSet for status update", "daemonSet", klog.KObj(ds))
			return getErr
		}
		return updateErr
	})
}

// manage manages the scheduling and running of Pods of ds on nodes.
// After figuring out which nodes should run a Pod of ds but not yet running one and
// which nodes should not run a Pod of ds but currently running one, it calls function
// syncNodes with a list of pods to remove and a list of nodes to run a Pod of ds.
func (dsc *ReconcileDaemonSet) manage(ctx context.Context, ds *appsv1alpha1.DaemonSet, nodeList []*corev1.Node, hash string) error {
	// Find out the pods which are created for the nodes by DaemonSets.
	nodeToDaemonPods, err := dsc.getNodesToDaemonPods(ctx, ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for DaemonSet %s: %v", ds.Name, err)
	}

	// For each node, if the node is running the daemon pod but isn't supposed to, kill the daemon
	// pod. If the node is supposed to run the daemon pod, but isn't, create the daemon pod on the node.
	var nodesNeedingDaemonPods, podsToDelete []string
	var nodesDesireScheduled, newPodCount int
	for _, node := range nodeList {
		nodesNeedingDaemonPodsOnNode, podsToDeleteOnNode := dsc.podsShouldBeOnNode(node, nodeToDaemonPods, ds, hash)

		nodesNeedingDaemonPods = append(nodesNeedingDaemonPods, nodesNeedingDaemonPodsOnNode...)
		podsToDelete = append(podsToDelete, podsToDeleteOnNode...)

		if shouldRun, _ := nodeShouldRunDaemonPod(node, ds); shouldRun {
			nodesDesireScheduled++
		}
		if newPod, _, ok := findUpdatedPodsOnNode(ds, nodeToDaemonPods[node.Name], hash); ok && newPod != nil {
			newPodCount++
		}
	}

	// Remove unscheduled pods assigned to not existing nodes when daemonset pods are scheduled by scheduler.
	// If node doesn't exist then pods are never scheduled and can't be deleted by PodGCController.
	podsToDelete = append(podsToDelete, getUnscheduledPodsWithoutNode(nodeList, nodeToDaemonPods)...)

	// This is the first deploy process.
	if ds.Spec.UpdateStrategy.Type == appsv1alpha1.RollingUpdateDaemonSetStrategyType && ds.Spec.UpdateStrategy.RollingUpdate != nil {
		if ds.Spec.UpdateStrategy.RollingUpdate.Partition != nil && *ds.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
			partition := *ds.Spec.UpdateStrategy.RollingUpdate.Partition

			// Creates pods on nodes that needing daemon pod. If progressive annotation is true, the creation will controlled
			// by partition and only some of daemon pods will be created. Otherwise daemon pods will be created on every
			// node that need to start a daemon pod.
			nodesNeedingDaemonPods = GetNodesNeedingPods(newPodCount, nodesDesireScheduled, int(partition), isDaemonSetCreationProgressively(ds), nodesNeedingDaemonPods)
		}
	}

	// Label new pods using the hash label value of the current history when creating them
	return dsc.syncNodes(ctx, ds, podsToDelete, nodesNeedingDaemonPods, hash)
}

// syncNodes deletes given pods and creates new daemon set pods on the given nodes
// returns slice with errors if any
func (dsc *ReconcileDaemonSet) syncNodes(ctx context.Context, ds *appsv1alpha1.DaemonSet, podsToDelete, nodesNeedingDaemonPods []string, hash string) error {
	if ds.Spec.Lifecycle != nil && ds.Spec.Lifecycle.PreDelete != nil {
		var err error
		podsToDelete, err = dsc.syncWithPreparingDelete(ds, podsToDelete)
		if err != nil {
			return err
		}
	}

	logger := klog.FromContext(ctx)
	dsKey := keyFunc(ds)
	createDiff := len(nodesNeedingDaemonPods)
	deleteDiff := len(podsToDelete)

	burstReplicas := getBurstReplicas(ds)
	if createDiff > burstReplicas {
		createDiff = burstReplicas
	}
	if deleteDiff > burstReplicas {
		deleteDiff = burstReplicas
	}

	if err := dsc.expectations.SetExpectations(logger, dsKey, createDiff, deleteDiff); err != nil {
		utilruntime.HandleError(err)
	}
	// error channel to communicate back failures.  make the buffer big enough to avoid any blocking
	errCh := make(chan error, createDiff+deleteDiff)

	klog.V(4).InfoS("Nodes needing daemon pods for DaemonSet", "daemonSet", klog.KObj(ds), "nodes", nodesNeedingDaemonPods, "count", createDiff)
	createWait := sync.WaitGroup{}
	// If the returned error is not nil we have a parse error.
	// The controller handles this via the hash.
	generation, err := GetTemplateGeneration(ds)
	if err != nil {
		generation = nil
	}
	template := util.CreatePodTemplate(ds.Spec.Template, generation, hash)

	if ds.Spec.UpdateStrategy.Type == appsv1alpha1.RollingUpdateDaemonSetStrategyType &&
		ds.Spec.UpdateStrategy.RollingUpdate != nil &&
		ds.Spec.UpdateStrategy.RollingUpdate.Type == appsv1alpha1.InplaceRollingUpdateType {
		readinessGate := corev1.PodReadinessGate{
			ConditionType: appspub.InPlaceUpdateReady,
		}
		template.Spec.ReadinessGates = append(template.Spec.ReadinessGates, readinessGate)
	}

	// Batch the pod creates. Batch sizes start at SlowStartInitialBatchSize
	// and double with each successful iteration in a kind of "slow start".
	// This handles attempts to start large numbers of pods that would
	// likely all fail with the same error. For example a project with a
	// low quota that attempts to create a large number of pods will be
	// prevented from spamming the API service with the pod create requests
	// after one of its pods fails.  Conveniently, this also prevents the
	// event spam that those failures would generate.
	batchSize := integer.IntMin(createDiff, kubecontroller.SlowStartInitialBatchSize)
	for pos := 0; createDiff > pos; batchSize, pos = integer.IntMin(2*batchSize, createDiff-(pos+batchSize)), pos+batchSize {
		errorCount := len(errCh)
		createWait.Add(batchSize)
		for i := pos; i < pos+batchSize; i++ {
			go func(ix int) {
				defer createWait.Done()
				var err error

				podTemplate := template.DeepCopy()
				if scheduleDaemonSetPods {
					// The pod's NodeAffinity will be updated to make sure the Pod is bound
					// to the target node by default scheduler. It is safe to do so because there
					// should be no conflicting node affinity with the target node.
					podTemplate.Spec.Affinity = util.ReplaceDaemonSetPodNodeNameNodeAffinity(
						podTemplate.Spec.Affinity, nodesNeedingDaemonPods[ix])
				} else {
					// If pod is scheduled by DaemonSetController, set its '.spec.scheduleName'.
					podTemplate.Spec.SchedulerName = "kubernetes.io/daemonset-controller"
					podTemplate.Spec.NodeName = nodesNeedingDaemonPods[ix]
				}

				err = dsc.podControl.CreatePods(ctx, ds.Namespace, podTemplate, ds, metav1.NewControllerRef(ds, controllerKind))

				if err != nil {
					if errors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
						// If the namespace is being torn down, we can safely ignore
						// this error since all subsequent creations will fail.
						return
					}
				}
				if err != nil {
					klog.V(2).InfoS("Failed creation, decrementing expectations for DaemonSet", "daemonSet", klog.KObj(ds))
					dsc.expectations.CreationObserved(logger, dsKey)
					errCh <- err
					utilruntime.HandleError(err)
				}
			}(i)
		}
		createWait.Wait()
		// any skipped pods that we never attempted to start shouldn't be expected.
		skippedPods := createDiff - (batchSize + pos)
		if errorCount < len(errCh) && skippedPods > 0 {
			klog.V(2).InfoS("Slow-start failure. Skipping creation of pods, decrementing expectations for DaemonSet", "skippedPodCount", skippedPods, "daemonSet", klog.KObj(ds))
			dsc.expectations.LowerExpectations(logger, dsKey, skippedPods, 0)
			// The skipped pods will be retried later. The next controller resync will
			// retry the slow start process.
			break
		}
	}

	klog.V(4).InfoS("Pods to delete for DaemonSet", "daemonSet", klog.KObj(ds), "podsToDelete", podsToDelete, "count", deleteDiff)
	deleteWait := sync.WaitGroup{}
	deleteWait.Add(deleteDiff)
	for i := 0; i < deleteDiff; i++ {
		go func(ix int) {
			defer deleteWait.Done()
			if err := dsc.podControl.DeletePod(ctx, ds.Namespace, podsToDelete[ix], ds); err != nil {
				dsc.expectations.DeletionObserved(logger, dsKey)
				if !errors.IsNotFound(err) {
					klog.V(2).InfoS("Failed deletion, decremented expectations for DaemonSet", "daemonSet", klog.KObj(ds))
					errCh <- err
					utilruntime.HandleError(err)
				}
			}
		}(i)
	}
	deleteWait.Wait()

	// collect errors if any for proper reporting/retry logic in the controller
	var errors []error
	close(errCh)
	for err := range errCh {
		errors = append(errors, err)
	}
	return utilerrors.NewAggregate(errors)
}

func (dsc *ReconcileDaemonSet) syncWithPreparingDelete(ds *appsv1alpha1.DaemonSet, podsToDelete []string) (podsCanDelete []string, err error) {
	for _, podName := range podsToDelete {
		pod, err := dsc.podLister.Pods(ds.Namespace).Get(podName)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		if !lifecycle.IsPodHooked(ds.Spec.Lifecycle.PreDelete, pod) {
			podsCanDelete = append(podsCanDelete, podName)
			continue
		}
		markPodNotReady := ds.Spec.Lifecycle.PreDelete.MarkPodNotReady
		if updated, gotPod, err := dsc.lifecycleControl.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingDelete, markPodNotReady); err != nil {
			return nil, err
		} else if updated {
			klog.V(3).InfoS("DaemonSet has marked Pod as PreparingDelete", "daemonSet", klog.KObj(ds), "podName", podName)
			dsc.resourceVersionExpectations.Expect(gotPod)
		}
	}
	return
}

// podsShouldBeOnNode figures out the DaemonSet pods to be created and deleted on the given node:
//   - nodesNeedingDaemonPods: the pods need to start on the node
//   - podsToDelete: the Pods need to be deleted on the node
//   - err: unexpected error
func (dsc *ReconcileDaemonSet) podsShouldBeOnNode(
	node *corev1.Node,
	nodeToDaemonPods map[string][]*corev1.Pod,
	ds *appsv1alpha1.DaemonSet,
	hash string,
) (nodesNeedingDaemonPods, podsToDelete []string) {

	shouldRun, shouldContinueRunning := nodeShouldRunDaemonPod(node, ds)
	daemonPods, exists := nodeToDaemonPods[node.Name]

	switch {
	case shouldRun && !exists:
		// If daemon pod is supposed to be running on node, but isn't, create daemon pod.
		nodesNeedingDaemonPods = append(nodesNeedingDaemonPods, node.Name)
	case shouldContinueRunning:
		// If a daemon pod failed, delete it
		// If there's non-daemon pods left on this node, we will create it in the next sync loop
		var daemonPodsRunning []*corev1.Pod
		for _, pod := range daemonPods {
			if pod.DeletionTimestamp != nil {
				continue
			}
			if pod.Status.Phase == corev1.PodFailed {
				// This is a critical place where DS is often fighting with kubelet that rejects pods.
				// We need to avoid hot looping and backoff.
				backoffKey := failedPodsBackoffKey(ds, node.Name)

				now := dsc.failedPodsBackoff.Clock.Now()
				inBackoff := dsc.failedPodsBackoff.IsInBackOffSinceUpdate(backoffKey, now)
				if inBackoff {
					delay := dsc.failedPodsBackoff.Get(backoffKey)
					klog.V(4).InfoS("Deleting failed pod on node has been limited by backoff",
						"pod", klog.KObj(pod), "nodeName", node.Name, "backoffDelay", delay)
					durationStore.Push(keyFunc(ds), delay)
					continue
				}

				dsc.failedPodsBackoff.Next(backoffKey, now)

				klog.V(2).InfoS("Found failed daemon pod on node, will try to kill it", "pod", klog.KObj(pod), "nodeName", node.Name)
				// Emit an event so that it's discoverable to users.
				dsc.eventRecorder.Eventf(ds, corev1.EventTypeWarning, FailedDaemonPodReason,
					fmt.Sprintf("Found failed daemon pod %s/%s on node %s, will try to kill it", pod.Namespace, pod.Name, node.Name))
				podsToDelete = append(podsToDelete, pod.Name)
			} else if isPodPreDeleting(pod) {
				klog.V(3).InfoS("Found daemon pod on node is in PreparingDelete state, will try to kill it", "pod", klog.KObj(pod), "nodeName", node.Name)
				podsToDelete = append(podsToDelete, pod.Name)
			} else {
				daemonPodsRunning = append(daemonPodsRunning, pod)
			}
		}

		// When surge is not enabled, if there is more than 1 running pod on a node delete all but the oldest
		if !allowSurge(ds) {
			if len(daemonPodsRunning) <= 1 {
				// There are no excess pods to be pruned, and no pods to create
				break
			}

			sort.Sort(podByCreationTimestampAndPhase(daemonPodsRunning))
			for i := 1; i < len(daemonPodsRunning); i++ {
				podsToDelete = append(podsToDelete, daemonPodsRunning[i].Name)
			}
			break
		}

		if len(daemonPodsRunning) <= 1 {
			// // There are no excess pods to be pruned
			if len(daemonPodsRunning) == 0 && shouldRun {
				// We are surging so we need to have at least one non-deleted pod on the node
				nodesNeedingDaemonPods = append(nodesNeedingDaemonPods, node.Name)
			}
			break
		}

		// When surge is enabled, we allow 2 pods if and only if the oldest pod matching the current hash state
		// is not ready AND the oldest pod that doesn't match the current hash state is ready. All other pods are
		// deleted. If neither pod is ready, only the one matching the current hash revision is kept.
		var oldestNewPod, oldestOldPod *corev1.Pod
		sort.Sort(podByCreationTimestampAndPhase(daemonPodsRunning))
		for _, pod := range daemonPodsRunning {
			if pod.Labels[apps.ControllerRevisionHashLabelKey] == hash {
				if oldestNewPod == nil {
					oldestNewPod = pod
					continue
				}
			} else {
				if oldestOldPod == nil {
					oldestOldPod = pod
					continue
				}
			}
			podsToDelete = append(podsToDelete, pod.Name)
		}
		if oldestNewPod != nil && oldestOldPod != nil {
			switch {
			case !podutil.IsPodReady(oldestOldPod):
				klog.V(5).InfoS("Pod from DaemonSet is no longer ready and will be replaced with newer pod",
					"daemonSet", klog.KObj(ds), "oldestOldPod", klog.KObj(oldestOldPod), "oldestNewPod", klog.KObj(oldestNewPod))
				podsToDelete = append(podsToDelete, oldestOldPod.Name)
			case podutil.IsPodAvailable(oldestNewPod, ds.Spec.MinReadySeconds, metav1.Time{Time: dsc.failedPodsBackoff.Clock.Now()}):
				klog.V(5).InfoS("Pod from DaemonSet is now ready and will replace older pod",
					"daemonSet", klog.KObj(ds), "oldestOldPod", klog.KObj(oldestOldPod), "oldestNewPod", klog.KObj(oldestNewPod))
				podsToDelete = append(podsToDelete, oldestOldPod.Name)
			case podutil.IsPodReady(oldestNewPod) && ds.Spec.MinReadySeconds > 0:
				durationStore.Push(keyFunc(ds), podAvailableWaitingTime(oldestNewPod, ds.Spec.MinReadySeconds, dsc.failedPodsBackoff.Clock.Now()))
			}
		}

	case !shouldContinueRunning && exists:
		// If daemon pod isn't supposed to run on node, but it is, delete all daemon pods on node.
		for _, pod := range daemonPods {
			if pod.DeletionTimestamp != nil {
				continue
			}
			klog.V(5).InfoS("If daemon pod isn't supposed to run on node, but it is, delete daemon pod on node.", "nodeName", node.Name, "pod", klog.KObj(pod))
			podsToDelete = append(podsToDelete, pod.Name)
		}
	}

	return nodesNeedingDaemonPods, podsToDelete
}

// getNodesToDaemonPods returns a map from nodes to daemon pods (corresponding to ds) created for the nodes.
// This also reconciles ControllerRef by adopting/orphaning.
// Note that returned Pods are pointers to objects in the cache.
// If you want to modify one, you need to deep-copy it first.
func (dsc *ReconcileDaemonSet) getNodesToDaemonPods(ctx context.Context, ds *appsv1alpha1.DaemonSet) (map[string][]*corev1.Pod, error) {
	claimedPods, err := dsc.getDaemonPods(ctx, ds)
	if err != nil {
		return nil, err
	}
	// Group Pods by Node name.
	nodeToDaemonPods := make(map[string][]*corev1.Pod)
	for _, pod := range claimedPods {
		nodeName, err := util.GetTargetNodeName(pod)
		if err != nil {
			klog.InfoS("Failed to get target node name of Pod in DaemonSet", "pod", klog.KObj(pod), "daemonSet", klog.KObj(ds))
			continue
		}
		nodeToDaemonPods[nodeName] = append(nodeToDaemonPods[nodeName], pod)
	}

	return nodeToDaemonPods, nil
}

func failedPodsBackoffKey(ds *appsv1alpha1.DaemonSet, nodeName string) string {
	return fmt.Sprintf("%s/%d/%s", ds.UID, ds.Status.ObservedGeneration, nodeName)
}

type podByCreationTimestampAndPhase []*corev1.Pod

func (o podByCreationTimestampAndPhase) Len() int      { return len(o) }
func (o podByCreationTimestampAndPhase) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o podByCreationTimestampAndPhase) Less(i, j int) bool {
	// Scheduled Pod first
	if len(o[i].Spec.NodeName) != 0 && len(o[j].Spec.NodeName) == 0 {
		return true
	}

	if len(o[i].Spec.NodeName) == 0 && len(o[j].Spec.NodeName) != 0 {
		return false
	}

	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}

func (dsc *ReconcileDaemonSet) cleanupHistory(ctx context.Context, ds *appsv1alpha1.DaemonSet, old []*apps.ControllerRevision) error {
	nodesToDaemonPods, err := dsc.getNodesToDaemonPods(ctx, ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for DaemonSet %q: %v", ds.Name, err)
	}

	toKeep := 10
	if ds.Spec.RevisionHistoryLimit != nil {
		toKeep = int(*ds.Spec.RevisionHistoryLimit)
	}
	toKill := len(old) - toKeep
	if toKill <= 0 {
		return nil
	}

	// Find all hashes of live pods
	liveHashes := make(map[string]bool)
	for _, pods := range nodesToDaemonPods {
		for _, pod := range pods {
			if hash := pod.Labels[apps.DefaultDaemonSetUniqueLabelKey]; len(hash) > 0 {
				liveHashes[hash] = true
			}
		}
	}

	// Clean up old history from smallest to highest revision (from oldest to newest)
	sort.Sort(historiesByRevision(old))
	for _, history := range old {
		if toKill <= 0 {
			break
		}
		if hash := history.Labels[apps.DefaultDaemonSetUniqueLabelKey]; liveHashes[hash] {
			continue
		}
		// Clean up
		err := dsc.kubeClient.AppsV1().ControllerRevisions(ds.Namespace).Delete(ctx, history.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		toKill--
	}
	return nil
}

func (dsc *ReconcileDaemonSet) refreshUpdateStates(ctx context.Context, ds *appsv1alpha1.DaemonSet) error {
	dsKey := keyFunc(ds)
	pods, err := dsc.getDaemonPods(ctx, ds)
	if err != nil {
		return err
	}

	opts := &inplaceupdate.UpdateOptions{}
	opts = inplaceupdate.SetOptionsDefaults(opts)
	for _, pod := range pods {
		if dsc.inplaceControl == nil {
			continue
		}
		res := dsc.inplaceControl.Refresh(pod, opts)
		if res.RefreshErr != nil {
			klog.ErrorS(res.RefreshErr, "DaemonSet failed to update pod condition for inplace", "daemonSet", klog.KObj(ds), "pod", klog.KObj(pod))
			return res.RefreshErr
		}
		if res.DelayDuration != 0 {
			durationStore.Push(dsKey, res.DelayDuration)
		}
	}

	return nil
}

func (dsc *ReconcileDaemonSet) hasPodExpectationsSatisfied(ctx context.Context, ds *appsv1alpha1.DaemonSet) bool {
	dsKey := keyFunc(ds)
	pods, err := dsc.getDaemonPods(ctx, ds)
	if err != nil {
		klog.ErrorS(err, "Failed to get pods for DaemonSet")
		return false
	}

	for _, pod := range pods {
		dsc.resourceVersionExpectations.Observe(pod)
		if isSatisfied, unsatisfiedDuration := dsc.resourceVersionExpectations.IsSatisfied(pod); !isSatisfied {
			if unsatisfiedDuration >= kruiseExpectations.ExpectationTimeout {
				klog.ErrorS(nil, "Expectation unsatisfied resourceVersion overtime for DaemonSet, wait for pod updating",
					"daemonSet", klog.KObj(ds), "pod", klog.KObj(pod), "timeout", unsatisfiedDuration)
			} else {
				klog.V(5).InfoS("Not satisfied resourceVersion for DaemonSet, wait for pod updating", "daemonSet", klog.KObj(ds), "pod", klog.KObj(pod))
				durationStore.Push(dsKey, kruiseExpectations.ExpectationTimeout-unsatisfiedDuration)
			}
			return false
		}
	}
	return true
}
