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

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	revisioncontrol "github.com/openkruise/kruise/pkg/controller/cloneset/revision"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util/expectations"
	historyutil "github.com/openkruise/kruise/pkg/util/history"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/history"
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
	flag.IntVar(&concurrentReconciles, "cloneset-workers", concurrentReconciles, "Max concurrent workers for CloneSet controller.")
}

var (
	concurrentReconciles = 10

	scaleExpectations  = kubecontroller.NewUIDTrackingControllerExpectations(kubecontroller.NewControllerExpectations())
	updateExpectations = expectations.NewUpdateExpectations(clonesetutils.GetPodRevision)
)

// Add creates a new CloneSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	reconciler := &ReconcileCloneSet{
		Client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		statusUpdater:     newStatusUpdater(mgr.GetClient()),
		controllerHistory: historyutil.NewHistory(mgr.GetClient()),
		revisionControl:   revisioncontrol.NewRevisionControl(),
	}
	reconciler.reconcileFunc = reconciler.doReconcile
	return reconciler
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cloneset-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to CloneSet
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.CloneSet{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCS := e.ObjectOld.(*appsv1alpha1.CloneSet)
			newCS := e.ObjectNew.(*appsv1alpha1.CloneSet)
			if *oldCS.Spec.Replicas != *newCS.Spec.Replicas {
				klog.V(4).Infof("Observed updated replicas for CloneSet: %s/%s, %d->%d",
					newCS.Namespace, newCS.Name, *oldCS.Spec.Replicas, *newCS.Spec.Replicas)
			}
			return true
		},
	})
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &podEventHandler{Reader: mgr.GetClient()})
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

	controllerHistory history.Interface
	statusUpdater     StatusUpdater
	revisionControl   revisioncontrol.Interface
}

// Reconcile reads that state of the cluster for a CloneSet object and makes changes based on the state read
// and what is in the CloneSet.Spec
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets/status,verbs=get;update;patch
func (r *ReconcileCloneSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	return r.reconcileFunc(request)
}

func (r *ReconcileCloneSet) doReconcile(request reconcile.Request) (reconcile.Result, error) {
	startTime := time.Now()
	defer func() {
		klog.V(3).Infof("Finished syncing CloneSet %s, cost %v", request, time.Since(startTime))
	}()

	// Fetch the CloneSet instance
	instance := &appsv1alpha1.CloneSet{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			klog.V(3).Infof("CloneSet %s has been deleted.", request)
			scaleExpectations.DeleteExpectations(request.String())
			updateExpectations.DeleteExpectations(request.String())
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	selector, err := metav1.LabelSelectorAsSelector(instance.Spec.Selector)
	if err != nil {
		klog.Errorf("Error converting CloneSet %s selector: %v", request, err)
		// This is a non-transient error, so don't retry.
		return reconcile.Result{}, nil
	}

	// list all active Pods and PVCs belongs to cs
	filteredPods, filteredPVCs, err := r.getOwnedResource(instance)
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
	currentRevision, updateRevision, collisionCount, err := r.getActiveRevisions(instance, revisions, clonesetutils.GetPodsRevisions(filteredPods))
	if err != nil {
		return reconcile.Result{}, err
	}

	// get the current and update revisions of the set.
	currentSet, err := r.revisionControl.ApplyRevision(instance, currentRevision)
	if err != nil {
		return reconcile.Result{}, err
	}
	updateSet, err := r.revisionControl.ApplyRevision(instance, updateRevision)
	if err != nil {
		return reconcile.Result{}, err
	}

	newStatus := appsv1alpha1.CloneSetStatus{
		ObservedGeneration: instance.Generation,
		UpdateRevision:     updateRevision.Name,
		CollisionCount:     new(int32),
	}
	*newStatus.CollisionCount = collisionCount

	scaleSatisfied := scaleExpectations.SatisfiedExpectations(request.String())
	updateSatisfied, updateDirtyPods := updateExpectations.SatisfiedExpectations(request.String(), updateRevision.Name)
	if scaleSatisfied && updateSatisfied {
		// TODO(FillZpp): manage replicas
		// TODO(FillZpp): manage update
	} else {
		klog.V(3).Infof("Skip scale and update for CloneSet %s, because of scaleSatisfied=%v, updateSatisfied=%v, updateDirtyPods=%v",
			request, scaleSatisfied, updateSatisfied, updateDirtyPods)
	}

	if err = r.statusUpdater.UpdateCloneSetStatus(instance, newStatus, filteredPods); err != nil {
		return reconcile.Result{}, err
	}

	if err = r.truncateHistory(instance, filteredPods, revisions, currentRevision, updateRevision); err != nil {
		klog.Errorf("Failed to truncate history for %s: %v", request, err)
	}

	// TODO(FillZpp): remove this debug log
	klog.V(5).Infof("CloneSet %s got %v %v %v %v", request, filteredPVCs, currentSet, updateSet, collisionCount)
	return reconcile.Result{}, nil
}

func (r *ReconcileCloneSet) getActiveRevisions(cs *appsv1alpha1.CloneSet, revisions []*apps.ControllerRevision, podsRevisions sets.String) (
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

	// find any equivalent revisions
	equalRevisions := history.FindEqualRevisions(revisions, updateRevision)
	equalCount := len(equalRevisions)

	if equalCount > 0 && history.EqualRevision(revisions[revisionCount-1], equalRevisions[equalCount-1]) {
		// if the equivalent revision is immediately prior the update revision has not changed
		updateRevision = revisions[revisionCount-1]
	} else if equalCount > 0 {
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
		if podsRevisions.Has(revisions[i].Name) {
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

func (r *ReconcileCloneSet) getOwnedResource(cs *appsv1alpha1.CloneSet) ([]*v1.Pod, []*v1.PersistentVolumeClaim, error) {
	activePods, err := clonesetutils.GetActivePods(r.Client, cs.Namespace)
	if err != nil {
		return nil, nil, err
	}
	var filteredPods []*v1.Pod
	for _, p := range activePods {
		if ref := metav1.GetControllerOf(p); ref != nil && ref.UID == cs.UID {
			filteredPods = append(filteredPods, p)
		}
	}

	pvcList := v1.PersistentVolumeClaimList{}
	if err := r.List(context.TODO(), client.InNamespace(cs.Namespace), &pvcList); err != nil {
		return nil, nil, err
	}
	var filteredPVCs []*v1.PersistentVolumeClaim
	for i, pvc := range pvcList.Items {
		if ref := metav1.GetControllerOf(&pvc); ref != nil && ref.UID == cs.UID {
			filteredPVCs = append(filteredPVCs, &pvcList.Items[i])
		}
	}

	return filteredPods, filteredPVCs, nil
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
	live := clonesetutils.GetPodsRevisions(pods)
	live.Insert(current.Name, update.Name)

	// collect live revisions and historic revisions
	for i := range revisions {
		if !live.Has(revisions[i].Name) {
			noLiveRevisions = append(noLiveRevisions, revisions[i])
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
