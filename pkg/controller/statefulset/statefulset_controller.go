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
	"fmt"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	kruiseappslisters "github.com/openkruise/kruise/pkg/client/listers/apps/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = appsv1alpha1.SchemeGroupVersion.WithKind("StatefulSet")

// Add creates a new StatefulSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	cacher := mgr.GetCache()
	statefulSetInformer, err := cacher.GetInformerForKind(controllerKind)
	if err != nil {
		return nil, err
	}
	podInformer, err := cacher.GetInformerForKind(v1.SchemeGroupVersion.WithKind("Pod"))
	if err != nil {
		return nil, err
	}
	pvcInformer, err := cacher.GetInformerForKind(v1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"))
	if err != nil {
		return nil, err
	}
	revInformer, err := cacher.GetInformerForKind(appsv1.SchemeGroupVersion.WithKind("ControllerRevision"))
	if err != nil {
		return nil, err
	}

	statefulSetLister := kruiseappslisters.NewStatefulSetLister(statefulSetInformer.GetIndexer())
	podLister := corelisters.NewPodLister(podInformer.GetIndexer())

	genericClient := client.GetGenericClient()
	//recorder := mgr.GetRecorder("statefulset-controller")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: genericClient.KubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "statefulset-controller"})

	return &ReconcileStatefulSet{
		kruiseClient: genericClient.KruiseClient,
		control: NewDefaultStatefulSetControl(
			NewRealStatefulPodControl(
				genericClient.KubeClient,
				statefulSetLister,
				podLister,
				corelisters.NewPersistentVolumeClaimLister(pvcInformer.GetIndexer()),
				recorder),
			NewRealStatefulSetStatusUpdater(genericClient.KruiseClient, statefulSetLister),
			history.NewHistory(genericClient.KubeClient, appslisters.NewControllerRevisionLister(revInformer.GetIndexer())),
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
	control ControlInterface
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
	c, err := controller.New("statefulset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to StatefulSet
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.StatefulSet{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSS := e.ObjectOld.(*appsv1alpha1.StatefulSet)
			newSS := e.ObjectNew.(*appsv1alpha1.StatefulSet)
			if oldSS.Status.Replicas != newSS.Status.Replicas {
				klog.V(4).Infof("Observed updated replica count for StatefulSet: %v, %d->%d", newSS.Name, oldSS.Status.Replicas, newSS.Status.Replicas)
			}
			return true
		},
	})
	if err != nil {
		return err
	}

	// Watch for changes to Pod created by StatefulSet
	err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appsv1alpha1.StatefulSet{},
	})
	if err != nil {
		return err
	}

	klog.V(4).Infof("finished to add statefulset-controller")

	return nil
}

// Reconcile reads that state of the cluster for a StatefulSet object and makes changes based on the state read
// and what is in the StatefulSet.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Pods
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=statefulsets/status,verbs=get;update;patch
func (ssc *ReconcileStatefulSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	key := request.NamespacedName.String()
	namespace := request.Namespace
	name := request.Name

	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing statefulset %q (%v)", key, time.Since(startTime))
	}()

	set, err := ssc.setLister.StatefulSets(namespace).Get(name)
	if errors.IsNotFound(err) {
		klog.Infof("StatefulSet has been deleted %v", key)
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

	pods, err := ssc.getPodsForStatefulSet(set, selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, ssc.syncStatefulSet(set, pods)
}

// adoptOrphanRevisions adopts any orphaned ControllerRevisions matched by set's Selector.
func (ssc *ReconcileStatefulSet) adoptOrphanRevisions(set *appsv1alpha1.StatefulSet) error {
	revisions, err := ssc.control.ListRevisions(set)
	if err != nil {
		return err
	}
	hasOrphans := false
	for i := range revisions {
		if metav1.GetControllerOf(revisions[i]) == nil {
			hasOrphans = true
			break
		}
	}
	if hasOrphans {
		fresh, err := ssc.kruiseClient.AppsV1alpha1().StatefulSets(set.Namespace).Get(set.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if fresh.UID != set.UID {
			return fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return ssc.control.AdoptOrphanRevisions(set, revisions)
	}
	return nil
}

// getPodsForStatefulSet returns the Pods that a given StatefulSet should manage.
// It also reconciles ControllerRef by adopting/orphaning.
//
// NOTE: Returned Pods are pointers to objects from the cache.
//       If you need to modify one, you need to copy it first.
func (ssc *ReconcileStatefulSet) getPodsForStatefulSet(set *appsv1alpha1.StatefulSet, selector labels.Selector) ([]*v1.Pod, error) {
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
	canAdoptFunc := kubecontroller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := ssc.kruiseClient.AppsV1alpha1().StatefulSets(set.Namespace).Get(set.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != set.UID {
			return nil, fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return fresh, nil
	})

	cm := kubecontroller.NewPodControllerRefManager(ssc.podControl, set, selector, controllerKind, canAdoptFunc)
	return cm.ClaimPods(pods, filter)
}

// syncStatefulSet syncs a tuple of (statefulset, []*v1.Pod).
func (ssc *ReconcileStatefulSet) syncStatefulSet(set *appsv1alpha1.StatefulSet, pods []*v1.Pod) error {
	klog.V(4).Infof("Syncing StatefulSet %v/%v with %d pods", set.Namespace, set.Name, len(pods))
	// TODO: investigate where we mutate the set during the update as it is not obvious.
	if err := ssc.control.UpdateStatefulSet(set.DeepCopy(), pods); err != nil {
		return err
	}
	klog.V(4).Infof("Successfully synced StatefulSet %s/%s successful", set.Namespace, set.Name)
	return nil
}
