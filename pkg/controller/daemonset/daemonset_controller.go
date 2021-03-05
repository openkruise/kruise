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
	"sort"
	"sync"
	"time"

	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/daemon/util"
	daemonsetutil "k8s.io/kubernetes/pkg/controller/daemon/util"
	kubelettypes "k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/predicates"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	"k8s.io/utils/integer"
	kubeClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/client/clientset/versioned/scheme"
	kruiseutil "github.com/openkruise/kruise/pkg/util"
	kruiseExpectations "github.com/openkruise/kruise/pkg/util/expectations"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.BoolVar(&scheduleDaemonSetPods, "assign-pods-by-scheduler", true, "Use scheduler to assign pod to node.")
	flag.IntVar(&concurrentReconciles, "daemonset-workers", concurrentReconciles, "Max concurrent workers for DaemonSet controller.")
	flag.Int64Var(&extraAllowedPodNumber, "daemonset-extra-allowed-pod-number", extraAllowedPodNumber,
		"Extra allowed number of Pods that can run on one node, ensure daemonset pod to be assigned")
}

var (
	concurrentReconciles  = 3
	scheduleDaemonSetPods bool
	extraAllowedPodNumber = int64(0)

	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = appsv1alpha1.SchemeGroupVersion.WithKind("DaemonSet")

	// A TTLCache of pod creates/deletes each ds expects to see
	expectations       = kubecontroller.NewControllerExpectations()
	updateExpectations = kruiseExpectations.NewUpdateExpectations(GetPodRevision)
)

const (
	// StatusUpdateRetries limits the number of retries if sending a status update to API server fails.
	StatusUpdateRetries = 1

	IsFirstDeployedFlag = "daemonset.kruise.io/is-first-deployed"

	IsIgnoreNotReady     = "daemonset.kruise.io/ignore-not-ready"
	IsIgnoreUnscheduable = "daemonset.kruise.io/ignore-unscheduable"

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

	podInformer, err := cacher.GetInformerForKind(corev1.SchemeGroupVersion.WithKind("Pod"))
	if err != nil {
		return nil, err
	}

	nodeInformer, err := cacher.GetInformerForKind(corev1.SchemeGroupVersion.WithKind("Node"))
	if err != nil {
		return nil, err
	}

	revInformer, err := cacher.GetInformerForKind(apps.SchemeGroupVersion.WithKind("ControllerRevision"))
	if err != nil {
		return nil, err
	}
	historyLister := appslisters.NewControllerRevisionLister(revInformer.(cache.SharedIndexInformer).GetIndexer())
	podLister := corelisters.NewPodLister(podInformer.(cache.SharedIndexInformer).GetIndexer())
	nodeLister := corelisters.NewNodeLister(nodeInformer.(cache.SharedIndexInformer).GetIndexer())
	failedPodsBackoff := flowcontrol.NewBackOff(1*time.Second, 1*time.Minute)

	cli := kruiseutil.NewClientFromManager(mgr, "daemonset-controller")
	dsc := &ReconcileDaemonSet{
		client:        cli,
		eventRecorder: recorder,
		podControl:    kubecontroller.RealPodControl{KubeClient: genericClient.KubeClient, Recorder: recorder},
		crControl: kubecontroller.RealControllerRevisionControl{
			KubeClient: genericClient.KubeClient,
		},
		historyLister:       historyLister,
		podLister:           podLister,
		nodeLister:          nodeLister,
		suspendedDaemonPods: map[string]sets.String{},
		failedPodsBackoff:   failedPodsBackoff,
		inplaceControl:      inplaceupdate.New(cli, apps.ControllerRevisionHashLabelKey),
		updateExp:           updateExpectations,
	}
	dsc.podNodeIndex = podInformer.(cache.SharedIndexInformer).GetIndexer()
	return dsc, err
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("daemonset-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to DaemonSet
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.DaemonSet{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			ds := e.Object.(*appsv1alpha1.DaemonSet)
			klog.V(4).Infof("Adding daemon set %s/%s", ds.Namespace, ds.Name)
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			newDS := e.ObjectNew.(*appsv1alpha1.DaemonSet)
			klog.V(4).Infof("Updating daemon set %s/%s", newDS.Namespace, newDS.Name)
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			ds := e.Object.(*appsv1alpha1.DaemonSet)
			klog.V(4).Infof("Deleting daemon set %s/%s", ds.Namespace, ds.Name)
			dsKey, err := kubecontroller.KeyFunc(ds)
			if err != nil {
				klog.Errorf("couldn't get key for object %#v: %v", ds, err)
			} else {
				expectations.DeleteExpectations(dsKey)
				updateExpectations.DeleteExpectations(dsKey)
			}

			return true
		},
	})
	if err != nil {
		return err
	}

	// Watch for changes to Node.
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &nodeEventHandler{reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	// Watch for changes to Pod created by DaemonSet
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &podEventHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	klog.V(4).Info("finished to add daemonset-controller")
	return nil
}

var _ reconcile.Reconciler = &ReconcileDaemonSet{}

// ReconcileDaemonSet reconciles a DaemonSet object
type ReconcileDaemonSet struct {
	// client interface
	client        kubeClient.Client
	eventRecorder record.EventRecorder
	podControl    kubecontroller.PodControlInterface
	crControl     kubecontroller.ControllerRevisionControlInterface

	// historyLister get list/get history from the shared informers's store
	historyLister appslisters.ControllerRevisionLister
	// podLister get list/get pods from the shared informers's store
	podLister corelisters.PodLister
	// podNodeIndex indexes pods by their nodeName
	podNodeIndex cache.Indexer

	// nodeLister can list/get nodes from the shared informer's store
	nodeLister corelisters.NodeLister

	// The DaemonSet that has suspended pods on nodes; the key is node name, the value
	// is DaemonSet set that want to run pods but can't schedule in latest syncup cycle.
	suspendedDaemonPodsMutex sync.Mutex
	suspendedDaemonPods      map[string]sets.String

	failedPodsBackoff *flowcontrol.Backoff

	inplaceControl inplaceupdate.Interface

	updateExp kruiseExpectations.UpdateExpectations
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=daemonsets/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a DaemonSet object and makes changes based on the state read
// and what is in the DaemonSet.Spec
func (dsc *ReconcileDaemonSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing DaemonSet %q (%v)", request.String(), time.Since(startTime))
	}()

	return dsc.syncDaemonSet(request)
}

// getDaemonPods returns daemon pods owned by the given ds.
// This also reconciles ControllerRef by adopting/orphaning.
// Note that returned Pods are pointers to objects in the cache.
// If you want to modify one, you need to deep-copy it first.
func (dsc *ReconcileDaemonSet) getDaemonPods(ds *appsv1alpha1.DaemonSet) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
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
	dsNotDeleted := kubecontroller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh := &appsv1alpha1.DaemonSet{}
		key := types.NamespacedName{
			Namespace: ds.Namespace,
			Name:      ds.Name,
		}
		err = dsc.client.Get(context.TODO(), key, fresh)
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
	return cm.ClaimPods(pods)
}

func (dsc *ReconcileDaemonSet) syncDaemonSet(request reconcile.Request) (reconcile.Result, error) {
	dsKey := request.NamespacedName.String()
	ds := &appsv1alpha1.DaemonSet{}
	err := dsc.client.Get(context.TODO(), request.NamespacedName, ds)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("DaemonSet has been deleted %s/%s", request.NamespacedName.Namespace, request.NamespacedName.Name)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("unable to retrieve DaemonSet %s/%s from store: %v", request.NamespacedName.Namespace, request.NamespacedName.Name, err)
	}

	// Don't process a daemon set until all its creations and deletions have been processed.
	// For example if daemon set foo asked for 3 new daemon pods in the previous call to manage,
	// then we do not want to call manage on foo until the daemon pods have been created.
	if ds.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// Construct histories of the DaemonSet, and get the hash of current history
	cur, old, err := dsc.constructHistory(ds)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to construct revisions of DaemonSet: %v", err)
	}
	hash := cur.Labels[apps.DefaultDaemonSetUniqueLabelKey]
	nodeList, err := dsc.nodeLister.List(labels.Everything())
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("couldn't get nodeList %v", err)
	}

	if !expectations.SatisfiedExpectations(dsKey) {
		return dsc.updateDaemonSetStatus(ds, nodeList, hash, true)
	}

	if ds.Spec.UpdateStrategy.RollingUpdate != nil &&
		ds.Spec.UpdateStrategy.RollingUpdate.Paused != nil &&
		*ds.Spec.UpdateStrategy.RollingUpdate.Paused {
		klog.V(4).Infof("DaemonSet %s deployment and update is paused.", ds.Name)
		return dsc.updateDaemonSetStatus(ds, nodeList, hash, true)
	}

	delay, err := dsc.manage(ds, hash)
	if err != nil {
		return reconcile.Result{}, err
	}
	if delay != 0 {
		return reconcile.Result{RequeueAfter: delay}, nil
	}

	// Process rolling updates if we're ready.
	if expectations.SatisfiedExpectations(dsKey) {
		switch ds.Spec.UpdateStrategy.Type {
		case appsv1alpha1.OnDeleteDaemonSetStrategyType:
		case appsv1alpha1.RollingUpdateDaemonSetStrategyType:
			delay, err = dsc.rollingUpdate(ds, hash)
			if err != nil {
				klog.Errorf("rollingUpdate failed: %v", err)
				return reconcile.Result{}, err
			}
			if delay != 0 {
				klog.V(6).Infof("delay to requeue.: %v", delay)
				return reconcile.Result{RequeueAfter: delay}, err
			}
		}
	}

	err = dsc.cleanupHistory(ds, old)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to clean up revisions of DaemonSet: %v", err)
	}

	return dsc.updateDaemonSetStatus(ds, nodeList, hash, true)
}

func (dsc *ReconcileDaemonSet) getDaemonSetsForPod(pod *corev1.Pod) []*appsv1alpha1.DaemonSet {
	sets, err := dsc.GetPodDaemonSets(pod)
	if err != nil {
		return nil
	}
	if len(sets) > 1 {
		// ControllerRef will ensure we don't do anything crazy, but more than one
		// item in this list nevertheless constitutes user error.
		utilruntime.HandleError(fmt.Errorf("user error! more than one daemon is selecting pods with labels: %+v", pod.Labels))
	}
	return sets
}

// Predicates checks if a DaemonSet's pod can be scheduled on a node using GeneralPredicates
// and PodToleratesNodeTaints predicate
func Predicates(pod *corev1.Pod, nodeInfo *schedulernodeinfo.NodeInfo) (bool, []predicates.PredicateFailureReason, error) {
	var predicateFails []predicates.PredicateFailureReason

	// If ScheduleDaemonSetPods is enabled, only check nodeSelector, nodeAffinity and toleration/taint match.
	// https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/
	if scheduleDaemonSetPods {
		fit, reasons, err := checkNodeFitness(pod, nil, nodeInfo)
		if err != nil {
			return false, predicateFails, err
		}
		if !fit {
			predicateFails = append(predicateFails, reasons...)
		}

		return len(predicateFails) == 0, predicateFails, nil
	}

	critical := kubelettypes.IsCriticalPod(pod)

	fit, reasons, err := predicates.PodToleratesNodeTaints(pod, nil, nodeInfo)
	if err != nil {
		return false, predicateFails, err
	}
	if !fit {
		predicateFails = append(predicateFails, reasons...)
	}

	if critical {
		// If the pod is marked as critical and support for critical pod annotations is enabled,
		// check predicates for critical pods only.
		fit, reasons, err = predicates.EssentialPredicates(pod, nil, nodeInfo)
	} else {
		fit, reasons, err = predicates.GeneralPredicates(pod, nil, nodeInfo)
	}
	if err != nil {
		return false, predicateFails, err
	}
	if !fit {
		predicateFails = append(predicateFails, reasons...)
	}

	return len(predicateFails) == 0, predicateFails, nil
}

// checkNodeFitness runs a set of predicates that select candidate nodes for the DaemonSet;
// the predicates include:
//   - PodFitsHost: checks pod's NodeName against node
//   - PodMatchNodeSelector: checks pod's ImagePullJobNodeSelector and NodeAffinity against node
//   - PodToleratesNodeTaints: exclude tainted node unless pod has specific toleration
func checkNodeFitness(pod *corev1.Pod, meta predicates.PredicateMetadata, nodeInfo *schedulernodeinfo.NodeInfo) (bool, []predicates.PredicateFailureReason, error) {
	var predicateFails []predicates.PredicateFailureReason
	fit, reasons, err := predicates.PodFitsHost(pod, meta, nodeInfo)
	if err != nil {
		return false, predicateFails, err
	}
	if !fit {
		predicateFails = append(predicateFails, reasons...)
	}

	fit, reasons, err = predicates.PodMatchNodeSelector(pod, meta, nodeInfo)
	if err != nil {
		return false, predicateFails, err
	}
	if !fit {
		predicateFails = append(predicateFails, reasons...)
	}

	fit, reasons, err = predicates.PodToleratesNodeTaints(pod, nil, nodeInfo)
	if err != nil {
		return false, predicateFails, err
	}
	if !fit {
		predicateFails = append(predicateFails, reasons...)
	}
	return len(predicateFails) == 0, predicateFails, nil
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
	newPod := &corev1.Pod{Spec: ds.Spec.Template.Spec, ObjectMeta: ds.Spec.Template.ObjectMeta}
	newPod.Namespace = ds.Namespace
	newPod.Spec.NodeName = nodeName

	// Added default tolerations for DaemonSet pods.
	daemonsetutil.AddOrUpdateDaemonPodTolerations(&newPod.Spec)

	return newPod
}

func (dsc *ReconcileDaemonSet) updateDaemonSetStatus(ds *appsv1alpha1.DaemonSet, nodeList []*corev1.Node, hash string, updateObservedGen bool) (reconcile.Result, error) {
	nodeToDaemonPods, err := dsc.getNodesToDaemonPods(ds)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}

	var desiredNumberScheduled, currentNumberScheduled, numberMisscheduled, numberReady, updatedNumberScheduled, numberAvailable = 0, 0, 0, 0, 0, 0
	for _, node := range nodeList {
		if !CanNodeBeDeployed(node, ds) {
			continue
		}
		wantToRun, _, _, err := NodeShouldRunDaemonPod(dsc.client, node, ds)
		if err != nil {
			return reconcile.Result{}, err
		}

		scheduled := len(nodeToDaemonPods[node.Name]) > 0

		if wantToRun {
			if CanNodeBeDeployed(node, ds) {
				desiredNumberScheduled++
			}
			if scheduled {
				currentNumberScheduled++
				// Sort the daemon pods by creation time, so that the oldest is first.
				daemonPods := nodeToDaemonPods[node.Name]
				sort.Sort(podByCreationTimestampAndPhase(daemonPods))
				pod := daemonPods[0]
				if podutil.IsPodReady(pod) {
					numberReady++
					if podutil.IsPodAvailable(pod, ds.Spec.MinReadySeconds, metav1.Now()) {
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

	err = storeDaemonSetStatus(dsc.client, ds, desiredNumberScheduled, currentNumberScheduled, numberMisscheduled, numberReady, updatedNumberScheduled, numberAvailable, numberUnavailable, updateObservedGen, hash)
	if err != nil {
		return reconcile.Result{
			Requeue: true,
		}, fmt.Errorf("error storing status for daemon set %#v: %v", ds, err)
	}

	// Resync the DaemonSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	if ds.Spec.MinReadySeconds >= 0 && numberReady != numberAvailable {
		return reconcile.Result{RequeueAfter: time.Duration(ds.Spec.MinReadySeconds) * time.Second}, nil
	}
	return reconcile.Result{}, nil
}

// manage manages the scheduling and running of Pods of ds on nodes.
// After figuring out which nodes should run a Pod of ds but not yet running one and
// which nodes should not run a Pod of ds but currently running one, it calls function
// syncNodes with a list of pods to remove and a list of nodes to run a Pod of ds.
func (dsc *ReconcileDaemonSet) manage(ds *appsv1alpha1.DaemonSet, hash string) (delay time.Duration, err error) {
	// Find out the pods which are created for the nodes by DaemonSets.
	nodeToDaemonPods, err := dsc.getNodesToDaemonPods(ds)
	if err != nil {
		return delay, fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %s: %v", ds.Name, err)
	}

	nodeList, err := dsc.nodeLister.List(labels.Everything())
	if err != nil {
		return delay, fmt.Errorf("couldn't get nodeList %v", err)
	}

	var nodesNeedingDaemonPods, podsToDelete []string

	for _, node := range nodeList {
		if !CanNodeBeDeployed(node, ds) {
			continue
		}
		delay, nodesNeedingDaemonPodsOnNode, podsToDeleteOnNode, err := dsc.podsShouldBeOnNode(node, nodeToDaemonPods, ds, hash)
		if err != nil {
			continue
		}
		if delay != 0 {
			return delay, nil
		}

		nodesNeedingDaemonPods = append(nodesNeedingDaemonPods, nodesNeedingDaemonPodsOnNode...)
		podsToDelete = append(podsToDelete, podsToDeleteOnNode...)
	}

	// This is the first deploy process.
	if ds.Spec.UpdateStrategy.Type == appsv1alpha1.RollingUpdateDaemonSetStrategyType &&
		ds.Spec.UpdateStrategy.RollingUpdate != nil {
		if ds.Spec.UpdateStrategy.RollingUpdate.Partition != nil && *ds.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
			partition := *ds.Spec.UpdateStrategy.RollingUpdate.Partition
			if ds.Status.DesiredNumberScheduled == 0 && ds.Status.CurrentNumberScheduled == 0 {
				_, err := dsc.updateDaemonSetStatus(ds, nodeList, hash, true)
				if err != nil {
					klog.Errorf("updateDaemonSetStatus failed in first deploy process")
				}
			}
			unDeployedNum := ds.Status.DesiredNumberScheduled - ds.Status.CurrentNumberScheduled
			// node count changes or daemonSet in first deploy.
			isFirstDeployed, ok := ds.Labels[IsFirstDeployedFlag]
			if unDeployedNum > 0 && (!ok || ok && isFirstDeployed != "false") {
				if int32(len(nodesNeedingDaemonPods)) >= partition {
					sort.Strings(nodesNeedingDaemonPods)
					nodesNeedingDaemonPods = append(nodesNeedingDaemonPods[:0], nodesNeedingDaemonPods[partition:]...)
				} else {
					nodesNeedingDaemonPods = []string{}
				}
				_, oldPods := dsc.getAllDaemonSetPods(ds, nodeToDaemonPods, hash)
				for _, pod := range oldPods {
					podsToDelete = append(podsToDelete, pod.Name)
				}
			} else {
				if ds.Labels == nil {
					ds.Labels = make(map[string]string)
				}
				if !ok || ds.Labels[IsFirstDeployedFlag] != "false" {
					ds.Labels[IsFirstDeployedFlag] = "false"
					if err := dsc.client.Update(context.TODO(), ds); err != nil {
						return delay, fmt.Errorf("failed to update %s: %v", ds.Name, err)
					}
				}
			}
		}
	}
	// Label new pods using the hash label value of the current history when creating them
	return delay, dsc.syncNodes(ds, podsToDelete, nodesNeedingDaemonPods, hash)
}

// syncNodes deletes given pods and creates new daemon set pods on the given nodes
// returns slice with erros if any
func (dsc *ReconcileDaemonSet) syncNodes(ds *appsv1alpha1.DaemonSet, podsToDelete, nodesNeedingDaemonPods []string, hash string) error {
	dsKey, err := kubecontroller.KeyFunc(ds)
	if err != nil {
		return fmt.Errorf("couldn't get key for object %#v: %v", ds, err)
	}
	createDiff := len(nodesNeedingDaemonPods)
	deleteDiff := len(podsToDelete)

	burstReplicas := getBurstReplicas(ds)
	if createDiff > burstReplicas {
		createDiff = burstReplicas
	}
	if deleteDiff > burstReplicas {
		deleteDiff = burstReplicas
	}

	if err := expectations.SetExpectations(dsKey, createDiff, deleteDiff); err != nil {
		utilruntime.HandleError(err)
	}
	// error channel to communicate back failures.  make the buffer big enough to avoid any blocking
	errCh := make(chan error, createDiff+deleteDiff)

	klog.V(4).Infof("Nodes needing daemon pods for daemon set %s: %+v, creating %d", ds.Name, nodesNeedingDaemonPods, createDiff)
	createWait := sync.WaitGroup{}
	// If the returned error is not nil we have a parse error.
	// The controller handles this via the hash.
	generation, err := GetTemplateGeneration(ds)
	if err != nil {
		generation = nil
	}
	template := util.CreatePodTemplate(ds.Spec.Template, generation, hash)

	//if ds.Spec.UpdateStrategy.Type == appsv1alpha1.RollingUpdateDaemonSetStrategyType &&
	//	ds.Spec.UpdateStrategy.RollingUpdate != nil &&
	//	ds.Spec.UpdateStrategy.RollingUpdate.Type == appsv1alpha1.InplaceRollingUpdateType {
	//	readinessGate := corev1.PodReadinessGate{
	//		ConditionType: appsv1alpha1.InPlaceUpdateReady,
	//	}
	//	template.Spec.ReadinessGates = append(template.Spec.ReadinessGates, readinessGate)
	//}

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

					err = dsc.podControl.CreatePodsWithControllerRef(ds.Namespace, podTemplate,
						ds, metav1.NewControllerRef(ds, controllerKind))
				} else {
					// If pod is scheduled by DaemonSetController, set its '.spec.scheduleName'.
					podTemplate.Spec.SchedulerName = "kubernetes.io/daemonset-controller"

					err = dsc.podControl.CreatePodsOnNode(nodesNeedingDaemonPods[ix], ds.Namespace, podTemplate,
						ds, metav1.NewControllerRef(ds, controllerKind))
				}

				if err != nil && errors.IsTimeout(err) {
					// Pod is created but its initialization has timed out.
					// If the initialization is successful eventually, the
					// controller will observe the creation via the informer.
					// If the initialization fails, or if the pod keeps
					// uninitialized for a long time, the informer will not
					// receive any update, and the controller will create a new
					// pod when the expectation expires.
					return
				}
				if err != nil {
					klog.V(2).Infof("Failed creation, decrementing expectations for set %q/%q", ds.Namespace, ds.Name)
					expectations.CreationObserved(dsKey)
					errCh <- err
					utilruntime.HandleError(err)
				}
			}(i)
		}
		createWait.Wait()
		// any skipped pods that we never attempted to start shouldn't be expected.
		skippedPods := createDiff - batchSize
		if errorCount < len(errCh) && skippedPods > 0 {
			klog.V(2).Infof("Slow-start failure. Skipping creation of %d pods, decrementing expectations for set %q/%q", skippedPods, ds.Namespace, ds.Name)
			for i := 0; i < skippedPods; i++ {
				expectations.CreationObserved(dsKey)
			}
			// The skipped pods will be retried later. The next controller resync will
			// retry the slow start process.
			break
		}
	}

	klog.V(4).Infof("Pods to delete for daemon set %s: %+v, deleting %d", ds.Name, podsToDelete, deleteDiff)
	deleteWait := sync.WaitGroup{}
	deleteWait.Add(deleteDiff)
	for i := 0; i < deleteDiff; i++ {
		go func(ix int) {
			defer deleteWait.Done()
			if err := dsc.podControl.DeletePod(ds.Namespace, podsToDelete[ix], ds); err != nil {
				klog.V(2).Infof("Failed deletion, decrementing expectations for set %q/%q", ds.Namespace, ds.Name)
				expectations.DeletionObserved(dsKey)
				errCh <- err
				utilruntime.HandleError(err)
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

// podsShouldBeOnNode figures out the DaemonSet pods to be created and deleted on the given node:
//   - nodesNeedingDaemonPods: the pods need to start on the node
//   - podsToDelete: the Pods need to be deleted on the node
//   - err: unexpected error
func (dsc *ReconcileDaemonSet) podsShouldBeOnNode(
	node *corev1.Node,
	nodeToDaemonPods map[string][]*corev1.Pod,
	ds *appsv1alpha1.DaemonSet,
	hash string,
) (delay time.Duration, nodesNeedingDaemonPods, podsToDelete []string, err error) {

	wantToRun, shouldSchedule, shouldContinueRunning, err := NodeShouldRunDaemonPod(dsc.client, node, ds)
	if err != nil {
		return
	}

	daemonPods, exists := nodeToDaemonPods[node.Name]
	dsKey, _ := cache.MetaNamespaceKeyFunc(ds)

	dsc.removeSuspendedDaemonPods(node.Name, dsKey)

	// Ignore the pods that belong to previous generations
	// Only applies when the rollingUpdate type is SurgingRollingUpdateType
	daemonPods = dsc.pruneSurgingDaemonPods(ds, daemonPods, hash)

	switch {
	case wantToRun && !shouldSchedule:
		// If daemon pod is supposed to run, but can not be scheduled, add to suspended list.
		dsc.addSuspendedDaemonPods(node.Name, dsKey)
	case shouldSchedule && !exists:
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
					delay = dsc.failedPodsBackoff.Get(backoffKey)
					klog.V(4).Infof("Deleting failed pod %s/%s on node %s has been limited by backoff - %v remaining",
						pod.Namespace, pod.Name, node.Name, delay)
					continue
				}

				dsc.failedPodsBackoff.Next(backoffKey, now)

				msg := fmt.Sprintf("Found failed daemon pod %s/%s on node %s, will try to kill it", pod.Namespace, pod.Name, node.Name)
				klog.V(2).Infof(msg)
				// Emit an event so that it's discoverable to users.
				dsc.eventRecorder.Eventf(ds, corev1.EventTypeWarning, FailedDaemonPodReason, msg)
				podsToDelete = append(podsToDelete, pod.Name)
			} else {
				daemonPodsRunning = append(daemonPodsRunning, pod)
			}
		}
		// If daemon pod is supposed to be running on node, but more than 1 daemon pod is running, delete the excess daemon pods.
		// Sort the daemon pods by creation time, so the oldest is preserved.
		if len(daemonPodsRunning) > 1 {
			sort.Sort(podByCreationTimestampAndPhase(daemonPodsRunning))
			for i := 1; i < len(daemonPodsRunning); i++ {
				podsToDelete = append(podsToDelete, daemonPodsRunning[i].Name)
			}
		}
	case !shouldContinueRunning && exists:
		// If daemon pod isn't supposed to run on node, but it is, delete all daemon pods on node.
		for _, pod := range daemonPods {
			if pod.DeletionTimestamp != nil {
				continue
			}
			klog.V(5).Infof("If daemon pod isn't supposed to run on node, but it is, delete all daemon pods on node.")
			podsToDelete = append(podsToDelete, pod.Name)
		}
	}
	return
}

// removeSuspendedDaemonPods removes DaemonSet which has pods that 'want to run,
// but should not schedule' for the node from suspended queue.
func (dsc *ReconcileDaemonSet) removeSuspendedDaemonPods(node, ds string) {
	dsc.suspendedDaemonPodsMutex.Lock()
	defer dsc.suspendedDaemonPodsMutex.Unlock()

	if _, found := dsc.suspendedDaemonPods[node]; !found {
		return
	}
	dsc.suspendedDaemonPods[node].Delete(ds)

	if len(dsc.suspendedDaemonPods[node]) == 0 {
		delete(dsc.suspendedDaemonPods, node)
	}
}

// addSuspendedDaemonPods adds DaemonSet which has pods that 'want to run,
// but should not schedule' for the node to the suspended queue.
func (dsc *ReconcileDaemonSet) addSuspendedDaemonPods(node, ds string) {
	dsc.suspendedDaemonPodsMutex.Lock()
	defer dsc.suspendedDaemonPodsMutex.Unlock()

	if _, found := dsc.suspendedDaemonPods[node]; !found {
		dsc.suspendedDaemonPods[node] = sets.NewString()
	}
	dsc.suspendedDaemonPods[node].Insert(ds)
}

// getNodesToDaemonPods returns a map from nodes to daemon pods (corresponding to ds) created for the nodes.
// This also reconciles ControllerRef by adopting/orphaning.
// Note that returned Pods are pointers to objects in the cache.
// If you want to modify one, you need to deep-copy it first.
func (dsc *ReconcileDaemonSet) getNodesToDaemonPods(ds *appsv1alpha1.DaemonSet) (map[string][]*corev1.Pod, error) {
	claimedPods, err := dsc.getDaemonPods(ds)
	if err != nil {
		return nil, err
	}
	// Group Pods by Node name.
	nodeToDaemonPods := make(map[string][]*corev1.Pod)
	for _, pod := range claimedPods {
		nodeName, err := util.GetTargetNodeName(pod)
		if err != nil {
			klog.Warningf("Failed to get target node name of Pod %v/%v in DaemonSet %v/%v",
				pod.Namespace, pod.Name, ds.Namespace, ds.Name)
			continue
		}
		node := &corev1.Node{}
		err = dsc.client.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node)
		if err != nil {
			klog.V(4).Infof("get node %s failed: %v", nodeName, err)
			if errors.IsNotFound(err) {
				// Unable to find the target node in the cluster, which means it has been drained from cluster,
				// we still add this node to nodeToDaemonPods in order that the daemon pods that failed to be
				// scheduled on it will be removed in later reconcile loop.
				nodeToDaemonPods[nodeName] = append(nodeToDaemonPods[nodeName], pod)
			}
			continue
		}
		if CanNodeBeDeployed(node, ds) {
			nodeToDaemonPods[nodeName] = append(nodeToDaemonPods[nodeName], pod)
		}
	}

	return nodeToDaemonPods, nil
}

// CanNodeBeDeployed checks if the node is ready for new daemon Pod.
func CanNodeBeDeployed(node *corev1.Node, ds *appsv1alpha1.DaemonSet) bool {
	isNodeScheduable := true
	isNodeReady := true

	if !ignoreNotUnscheduable(ds) {
		isNodeScheduable = !node.Spec.Unschedulable
	}

	if !ignoreNotReady(ds) {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
				isNodeReady = false
				break
			}
		}
	}
	return isNodeScheduable && isNodeReady
}

func ignoreNotReady(ds *appsv1alpha1.DaemonSet) bool {
	if ds.Annotations != nil {
		if val, ok := ds.Annotations[IsIgnoreNotReady]; ok && val == "true" {
			return true
		}
	}
	return false
}

func ignoreNotUnscheduable(ds *appsv1alpha1.DaemonSet) bool {
	if ds.Annotations != nil {
		if val, ok := ds.Annotations[IsIgnoreUnscheduable]; ok && val == "true" {
			return true
		}
	}
	return false
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

func (dsc *ReconcileDaemonSet) cleanupHistory(ds *appsv1alpha1.DaemonSet, old []*apps.ControllerRevision) error {
	nodesToDaemonPods, err := dsc.getNodesToDaemonPods(ds)
	if err != nil {
		return fmt.Errorf("couldn't get node to daemon pod mapping for daemon set %q: %v", ds.Name, err)
	}

	if ds.Spec.RevisionHistoryLimit == nil {
		ds.Spec.RevisionHistoryLimit = new(int32)
		*ds.Spec.RevisionHistoryLimit = 10
	}
	toKeep := int(*ds.Spec.RevisionHistoryLimit)
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

	// Find all live history with the above hashes
	liveHistory := make(map[string]bool)
	for _, history := range old {
		if hash := history.Labels[apps.DefaultDaemonSetUniqueLabelKey]; liveHashes[hash] {
			liveHistory[history.Name] = true
		}
	}

	// Clean up old history from smallest to highest revision (from oldest to newest)
	sort.Sort(historiesByRevision(old))
	for _, history := range old {
		if toKill <= 0 {
			break
		}
		if liveHistory[history.Name] {
			continue
		}
		// Clean up
		err := dsc.client.Delete(context.TODO(), history)
		if err != nil {
			return err
		}
		toKill--
	}
	return nil
}
