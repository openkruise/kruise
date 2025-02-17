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

package statefulset

import (
	"context"
	"flag"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/history"
	sigsclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/client"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	kruiseappslisters "github.com/openkruise/kruise/pkg/client/listers/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	"github.com/openkruise/kruise/pkg/util/requeueduration"
	"github.com/openkruise/kruise/pkg/util/revisionadapter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "statefulset-workers", concurrentReconciles, "Max concurrent workers for StatefulSet controller.")
}

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind       = appsv1beta1.SchemeGroupVersion.WithKind("StatefulSet")
	concurrentReconciles = 3

	updateExpectations = expectations.NewUpdateExpectations(revisionadapter.NewDefaultImpl())
	// this is a short cut for any sub-functions to notify the reconcile how long to wait to requeue
	durationStore = requeueduration.DurationStore{}

	// predownload image field
	minimumReplicasToPreDownloadImage int32 = 3

	// global client
	sigsruntimeClient sigsclient.Client

	// determined during controller initializing
	isPreDownloadDisabled bool
)

// Add creates a new StatefulSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	// check for whether imagepulljob is disabled
	if !utildiscovery.DiscoverGVK(appsv1alpha1.SchemeGroupVersion.WithKind("ImagePullJob")) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.PreDownloadImageForInPlaceUpdate) {
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
	cacher := mgr.GetCache()
	statefulSetInformer, err := cacher.GetInformerForKind(context.TODO(), controllerKind)
	if err != nil {
		return nil, err
	}
	podInformer, err := cacher.GetInformerForKind(context.TODO(), v1.SchemeGroupVersion.WithKind("Pod"))
	if err != nil {
		return nil, err
	}
	pvcInformer, err := cacher.GetInformerForKind(context.TODO(), v1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"))
	if err != nil {
		return nil, err
	}
	scInformer, err := cacher.GetInformerForKind(context.TODO(), storagev1.SchemeGroupVersion.WithKind("StorageClass"))
	if err != nil {
		return nil, err
	}
	revInformer, err := cacher.GetInformerForKind(context.TODO(), appsv1.SchemeGroupVersion.WithKind("ControllerRevision"))
	if err != nil {
		return nil, err
	}

	statefulSetLister := kruiseappslisters.NewStatefulSetLister(statefulSetInformer.(toolscache.SharedIndexInformer).GetIndexer())
	podLister := corelisters.NewPodLister(podInformer.(toolscache.SharedIndexInformer).GetIndexer())
	pvcLister := corelisters.NewPersistentVolumeClaimLister(pvcInformer.(toolscache.SharedIndexInformer).GetIndexer())
	scLister := storagelisters.NewStorageClassLister(scInformer.(toolscache.SharedIndexInformer).GetIndexer())

	genericClient := client.GetGenericClientWithName("statefulset-controller")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: genericClient.KubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "statefulset-controller"})

	// new a client
	sigsruntimeClient = utilclient.NewClientFromManager(mgr, "statefulset-controller")

	return &ReconcileStatefulSet{
		kruiseClient: genericClient.KruiseClient,
		control: NewDefaultStatefulSetControl(
			NewStatefulPodControl(
				genericClient.KubeClient,
				podLister,
				pvcLister,
				scLister,
				recorder),
			inplaceupdate.New(utilclient.NewClientFromManager(mgr, "statefulset-controller"), revisionadapter.NewDefaultImpl()),
			lifecycle.New(utilclient.NewClientFromManager(mgr, "statefulset-controller")),
			NewRealStatefulSetStatusUpdater(genericClient.KruiseClient, statefulSetLister),
			history.NewHistory(genericClient.KubeClient, appslisters.NewControllerRevisionLister(revInformer.(toolscache.SharedIndexInformer).GetIndexer())),
			recorder,
		),
		podControl: kubecontroller.RealPodControl{KubeClient: genericClient.KubeClient, Recorder: recorder},
		podLister:  podLister,
		setLister:  statefulSetLister,
	}, nil
}

var _ reconcile.Reconciler = &ReconcileStatefulSet{}

// ReconcileStatefulSet reconciles a StatefulSet object
type ReconcileStatefulSet struct {
	// client interface
	kruiseClient kruiseclientset.Interface
	// control returns an interface capable of syncing a stateful set.
	// Abstracted out for testing.
	control StatefulSetControlInterface
	// podControl is used for patching pods.
	podControl kubecontroller.PodControlInterface
	// podLister is able to list/get pods from a shared informer's store
	podLister corelisters.PodLister
	// setLister is able to list/get stateful sets from a shared informer's store
	setLister kruiseappslisters.StatefulSetLister
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("statefulset-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to StatefulSet
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1beta1.StatefulSet{}, &handler.TypedEnqueueRequestForObject[*appsv1beta1.StatefulSet]{},
		predicate.TypedFuncs[*appsv1beta1.StatefulSet]{
			UpdateFunc: func(e event.TypedUpdateEvent[*appsv1beta1.StatefulSet]) bool {
				oldSS := e.ObjectOld
				newSS := e.ObjectNew
				if oldSS.Status.Replicas != newSS.Status.Replicas {
					klog.V(4).InfoS("Observed updated replica count for StatefulSet",
						"statefulSet", klog.KObj(newSS), "oldReplicas", oldSS.Status.Replicas, "newReplicas", newSS.Status.Replicas)
				}
				return true
			},
		}))
	if err != nil {
		return err
	}

	// Watch for changes to PVC patched by StatefulSet
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.PersistentVolumeClaim{}, &pvcEventHandler{}))
	if err != nil {
		return err
	}

	// Watch for changes to Pod created by StatefulSet
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.Pod{}, handler.TypedEnqueueRequestForOwner[*v1.Pod](
		mgr.GetScheme(), mgr.GetRESTMapper(), &appsv1beta1.StatefulSet{}, handler.OnlyControllerOwner())))
	if err != nil {
		return err
	}

	klog.V(4).InfoS("Finished to add statefulset-controller")

	return nil
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=statefulsets/finalizers,verbs=update

// Reconcile reads that state of the cluster for a StatefulSet object and makes changes based on the state read
// and what is in the StatefulSet.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Pods
func (ssc *ReconcileStatefulSet) Reconcile(ctx context.Context, request reconcile.Request) (res reconcile.Result, retErr error) {
	key := request.NamespacedName.String()
	namespace := request.Namespace
	name := request.Name

	startTime := time.Now()
	defer func() {
		if retErr == nil {
			if res.Requeue || res.RequeueAfter > 0 {
				klog.InfoS("Finished syncing StatefulSet", "statefulSet", request, "elapsedTime", time.Since(startTime), "result", res)
			} else {
				klog.InfoS("Finished syncing StatefulSet", "statefulSet", request, "elapsedTime", time.Since(startTime))
			}
		} else {
			klog.ErrorS(retErr, "Finished syncing StatefulSet error", "statefulSet", request, "elapsedTime", time.Since(startTime))
		}
	}()

	set, err := ssc.setLister.StatefulSets(namespace).Get(name)
	if errors.IsNotFound(err) {
		klog.InfoS("StatefulSet deleted", "statefulSet", key)
		updateExpectations.DeleteExpectations(key)
		return reconcile.Result{}, nil
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to retrieve StatefulSet %v from store: %v", key, err))
		return reconcile.Result{}, err
	}

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("error converting StatefulSet %v selector: %v", key, err))
		// This is a non-transient error, so don't retry.
		return reconcile.Result{}, nil
	}

	if err := ssc.adoptOrphanRevisions(set); err != nil {
		return reconcile.Result{}, err
	}

	pods, err := ssc.getPodsForStatefulSet(ctx, set, selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = ssc.syncStatefulSet(ctx, set, pods)
	return reconcile.Result{RequeueAfter: durationStore.Pop(getStatefulSetKey(set))}, err
}

// adoptOrphanRevisions adopts any orphaned ControllerRevisions matched by set's Selector.
func (ssc *ReconcileStatefulSet) adoptOrphanRevisions(set *appsv1beta1.StatefulSet) error {
	revisions, err := ssc.control.ListRevisions(set)
	if err != nil {
		return err
	}
	orphanRevisions := make([]*appsv1.ControllerRevision, 0)
	for i := range revisions {
		if metav1.GetControllerOf(revisions[i]) == nil {
			orphanRevisions = append(orphanRevisions, revisions[i])
		}
	}
	if len(orphanRevisions) > 0 {
		fresh, err := ssc.kruiseClient.AppsV1beta1().StatefulSets(set.Namespace).Get(context.TODO(), set.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if fresh.UID != set.UID {
			return fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return ssc.control.AdoptOrphanRevisions(set, orphanRevisions)
	}
	return nil
}

// getPodsForStatefulSet returns the Pods that a given StatefulSet should manage.
// It also reconciles ControllerRef by adopting/orphaning.
//
// NOTE: Returned Pods are pointers to objects from the cache.
//
//	If you need to modify one, you need to copy it first.
func (ssc *ReconcileStatefulSet) getPodsForStatefulSet(ctx context.Context, set *appsv1beta1.StatefulSet, selector labels.Selector) ([]*v1.Pod, error) {
	// List all pods to include the pods that don't match the selector anymore but
	// has a ControllerRef pointing to this StatefulSet.
	pods, err := ssc.podLister.Pods(set.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	filter := func(pod *v1.Pod) bool {
		// Only claim if it matches our StatefulSet name. Otherwise release/ignore.
		return isMemberOf(set, pod)
	}

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := kubecontroller.RecheckDeletionTimestamp(func(ctx context.Context) (metav1.Object, error) {
		fresh, err := ssc.kruiseClient.AppsV1beta1().StatefulSets(set.Namespace).Get(ctx, set.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != set.UID {
			return nil, fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return fresh, nil
	})

	cm := kubecontroller.NewPodControllerRefManager(ssc.podControl, set, selector, controllerKind, canAdoptFunc)
	return cm.ClaimPods(ctx, pods, filter)
}

// syncStatefulSet syncs a tuple of (statefulset, []*v1.Pod).
func (ssc *ReconcileStatefulSet) syncStatefulSet(ctx context.Context, set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
	klog.V(4).InfoS("Syncing StatefulSet with pods", "statefulSet", klog.KObj(set), "podCount", len(pods))
	// TODO: investigate where we mutate the set during the update as it is not obvious.
	if err := ssc.control.UpdateStatefulSet(ctx, set.DeepCopy(), pods); err != nil {
		return err
	}
	klog.V(4).InfoS("Successfully synced StatefulSet", "statefulSet", klog.KObj(set))
	return nil
}
