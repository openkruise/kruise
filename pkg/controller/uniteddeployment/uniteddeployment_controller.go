/*
Copyright 2019 The Kruise Authors.

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

package uniteddeployment

import (
	"context"
	"flag"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/controller/uniteddeployment/adapter"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "uniteddeployment-workers", concurrentReconciles, "Max concurrent workers for UnitedDeployment controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("UnitedDeployment")
)

const (
	controllerName = "uniteddeployment-controller"

	eventTypeRevisionProvision      = "RevisionProvision"
	eventTypeFindSubsets            = "FindSubsets"
	eventTypeDupSubsetsDelete       = "DeleteDuplicatedSubsets"
	eventTypeSubsetsUpdate          = "UpdateSubset"
	eventTypeSpecifySubbsetReplicas = "SpecifySubsetReplicas"

	slowStartInitialBatchSize = 1
)

type subSetType string

const (
	statefulSetSubSetType         subSetType = "StatefulSet"
	advancedStatefulSetSubSetType subSetType = "AdvancedStatefulSet"
	cloneSetSubSetType            subSetType = "CloneSet"
	deploymentSubSetType          subSetType = "Deployment"
)

// Add creates a new UnitedDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := utilclient.NewClientFromManager(mgr, "uniteddeployment-controller")
	return &ReconcileUnitedDeployment{
		Client: cli,
		scheme: mgr.GetScheme(),

		recorder: mgr.GetEventRecorderFor(controllerName),
		subSetControls: map[subSetType]ControlInterface{
			statefulSetSubSetType:         &SubsetControl{Client: cli, scheme: mgr.GetScheme(), adapter: &adapter.StatefulSetAdapter{Client: cli, Scheme: mgr.GetScheme()}},
			advancedStatefulSetSubSetType: &SubsetControl{Client: cli, scheme: mgr.GetScheme(), adapter: &adapter.AdvancedStatefulSetAdapter{Client: cli, Scheme: mgr.GetScheme()}},
			cloneSetSubSetType:            &SubsetControl{Client: cli, scheme: mgr.GetScheme(), adapter: &adapter.CloneSetAdapter{Client: cli, Scheme: mgr.GetScheme()}},
			deploymentSubSetType:          &SubsetControl{Client: cli, scheme: mgr.GetScheme(), adapter: &adapter.DeploymentAdapter{Client: cli, Scheme: mgr.GetScheme()}},
		},
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to UnitedDeployment
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.UnitedDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appsv1alpha1.UnitedDeployment{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1beta1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appsv1alpha1.UnitedDeployment{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1alpha1.CloneSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appsv1alpha1.UnitedDeployment{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appsv1alpha1.UnitedDeployment{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileUnitedDeployment{}

// ReconcileUnitedDeployment reconciles a UnitedDeployment object
type ReconcileUnitedDeployment struct {
	client.Client
	scheme *runtime.Scheme

	recorder       record.EventRecorder
	subSetControls map[subSetType]ControlInterface
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=uniteddeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=uniteddeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=uniteddeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets/status,verbs=get

// Reconcile reads that state of the cluster for a UnitedDeployment object and makes changes based on the state read
// and what is in the UnitedDeployment.Spec
func (r *ReconcileUnitedDeployment) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Infof("Reconcile UnitedDeployment %s/%s", request.Namespace, request.Name)
	// Fetch the UnitedDeployment instance
	instance := &appsv1alpha1.UnitedDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}
	oldStatus := instance.Status.DeepCopy()

	currentRevision, updatedRevision, _, collisionCount, err := r.constructUnitedDeploymentRevisions(instance)
	if err != nil {
		klog.Errorf("Fail to construct controller revision of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeRevisionProvision), err.Error())
		return reconcile.Result{}, err
	}

	control, subsetType := r.getSubsetControls(instance)

	klog.V(4).Infof("Get UnitedDeployment %s/%s all subsets", request.Namespace, request.Name)
	expectedRevision := currentRevision.Name
	if updatedRevision != nil {
		expectedRevision = updatedRevision.Name
	}
	nameToSubset, err := r.getNameToSubset(instance, control, expectedRevision)
	if err != nil {
		klog.Errorf("Fail to get Subsets of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed %s",
			eventTypeFindSubsets), err.Error())
		return reconcile.Result{}, err
	}

	nextReplicas, err := NewReplicaAllocator(instance).Alloc(nameToSubset)
	klog.V(4).Infof("Get UnitedDeployment %s/%s next replicas %v", instance.Namespace, instance.Name, nextReplicas)
	if err != nil {
		klog.Errorf("UnitedDeployment %s/%s Specified subset replicas is ineffective: %s",
			instance.Namespace, instance.Name, err.Error())
		r.recorder.Eventf(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed %s",
			eventTypeSpecifySubbsetReplicas), "Specified subset replicas is ineffective: %s", err.Error())
		return reconcile.Result{}, err
	}

	nextPartitions := calcNextPartitions(instance, nextReplicas)
	nextUpdate := getNextUpdate(instance, nextReplicas, nextPartitions)
	klog.V(4).Infof("Get UnitedDeployment %s/%s next update %v", instance.Namespace, instance.Name, nextUpdate)

	newStatus, err := r.manageSubsets(instance, nameToSubset, nextUpdate, currentRevision, updatedRevision, subsetType)
	if err != nil {
		klog.Errorf("Fail to update UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeSubsetsUpdate), err.Error())
		return reconcile.Result{}, err
	}

	selector, err := util.ValidatedLabelSelectorAsSelector(instance.Spec.Selector)
	if err != nil {
		klog.Errorf("Error converting UnitedDeployment %s selector: %v", request, err)
		// This is a non-transient error, so don't retry.
		return reconcile.Result{}, nil
	}
	newStatus.LabelSelector = selector.String()

	return r.updateStatus(instance, newStatus, oldStatus, nameToSubset, nextReplicas, nextPartitions, currentRevision, updatedRevision, collisionCount, control)
}

func (r *ReconcileUnitedDeployment) getNameToSubset(instance *appsv1alpha1.UnitedDeployment, control ControlInterface, expectedRevision string) (*map[string]*Subset, error) {
	subSets, err := control.GetAllSubsets(instance, expectedRevision)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeFindSubsets), err.Error())
		return nil, fmt.Errorf("fail to get all Subsets for UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	klog.V(4).Infof("Classify UnitedDeployment %s/%s by subSet name", instance.Namespace, instance.Name)
	nameToSubsets := r.classifySubsetBySubsetName(instance, subSets)

	nameToSubset, err := r.deleteDupSubset(instance, nameToSubsets, control)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeDupSubsetsDelete), err.Error())
		return nil, fmt.Errorf("fail to manage duplicate Subset of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	return nameToSubset, nil
}

func calcNextPartitions(ud *appsv1alpha1.UnitedDeployment, nextReplicas *map[string]int32) *map[string]int32 {
	partitions := map[string]int32{}
	for _, subset := range ud.Spec.Topology.Subsets {
		var subsetPartition int32
		if ud.Spec.UpdateStrategy.Type == appsv1alpha1.ManualUpdateStrategyType && ud.Spec.UpdateStrategy.ManualUpdate != nil && ud.Spec.UpdateStrategy.ManualUpdate.Partitions != nil {
			if partition, exist := ud.Spec.UpdateStrategy.ManualUpdate.Partitions[subset.Name]; exist {
				subsetPartition = partition
			}
		}

		if subsetReplicas, exist := (*nextReplicas)[subset.Name]; exist && subsetPartition > subsetReplicas {
			subsetPartition = subsetReplicas
		}

		partitions[subset.Name] = subsetPartition
	}

	return &partitions
}

func getNextUpdate(ud *appsv1alpha1.UnitedDeployment, nextReplicas *map[string]int32, nextPartitions *map[string]int32) map[string]SubsetUpdate {
	next := make(map[string]SubsetUpdate)
	for _, subset := range ud.Spec.Topology.Subsets {
		t := SubsetUpdate{}
		t.Replicas = (*nextReplicas)[subset.Name]
		t.Partition = (*nextPartitions)[subset.Name]
		t.Patch = string(subset.Patch.Raw)

		next[subset.Name] = t
	}
	return next
}

func (r *ReconcileUnitedDeployment) deleteDupSubset(ud *appsv1alpha1.UnitedDeployment, nameToSubsets map[string][]*Subset, control ControlInterface) (*map[string]*Subset, error) {
	nameToSubset := map[string]*Subset{}
	for name, subsets := range nameToSubsets {
		if len(subsets) > 1 {
			for _, subset := range subsets[1:] {
				klog.V(0).Infof("Delete duplicated Subset %s/%s for subset name %s", subset.Namespace, subset.Name, name)
				if err := control.DeleteSubset(subset); err != nil {
					if errors.IsNotFound(err) {
						continue
					}

					return &nameToSubset, err
				}
			}
		}

		if len(subsets) > 0 {
			nameToSubset[name] = subsets[0]
		}
	}

	return &nameToSubset, nil
}

func (r *ReconcileUnitedDeployment) getSubsetControls(instance *appsv1alpha1.UnitedDeployment) (ControlInterface, subSetType) {
	if instance.Spec.Template.StatefulSetTemplate != nil {
		return r.subSetControls[statefulSetSubSetType], statefulSetSubSetType
	}

	if instance.Spec.Template.AdvancedStatefulSetTemplate != nil {
		return r.subSetControls[advancedStatefulSetSubSetType], advancedStatefulSetSubSetType
	}

	if instance.Spec.Template.CloneSetTemplate != nil {
		return r.subSetControls[cloneSetSubSetType], cloneSetSubSetType
	}

	if instance.Spec.Template.DeploymentTemplate != nil {
		return r.subSetControls[deploymentSubSetType], deploymentSubSetType
	}

	// unexpected
	return nil, statefulSetSubSetType
}

func (r *ReconcileUnitedDeployment) classifySubsetBySubsetName(ud *appsv1alpha1.UnitedDeployment, subsets []*Subset) map[string][]*Subset {
	mapping := map[string][]*Subset{}

	for _, ss := range subsets {
		subSetName, err := getSubsetNameFrom(ss)
		if err != nil {
			// filter out Subset without correct Subset name
			continue
		}

		mapping[subSetName] = append(mapping[subSetName], ss)
	}
	return mapping
}

func (r *ReconcileUnitedDeployment) updateStatus(instance *appsv1alpha1.UnitedDeployment, newStatus, oldStatus *appsv1alpha1.UnitedDeploymentStatus, nameToSubset *map[string]*Subset, nextReplicas, nextPartition *map[string]int32, currentRevision, updatedRevision *appsv1.ControllerRevision, collisionCount int32, control ControlInterface) (reconcile.Result, error) {
	newStatus = r.calculateStatus(newStatus, nameToSubset, nextReplicas, nextPartition, currentRevision, updatedRevision, collisionCount, control)
	_, err := r.updateUnitedDeployment(instance, oldStatus, newStatus)
	return reconcile.Result{}, err
}

func (r *ReconcileUnitedDeployment) calculateStatus(newStatus *appsv1alpha1.UnitedDeploymentStatus, nameToSubset *map[string]*Subset, nextReplicas, nextPartition *map[string]int32, currentRevision, updatedRevision *appsv1.ControllerRevision, collisionCount int32, control ControlInterface) *appsv1alpha1.UnitedDeploymentStatus {
	expectedRevision := currentRevision.Name
	if updatedRevision != nil {
		expectedRevision = updatedRevision.Name
	}

	newStatus.Replicas = 0
	newStatus.ReadyReplicas = 0
	newStatus.UpdatedReplicas = 0
	newStatus.UpdatedReadyReplicas = 0

	// sync from status
	for _, subset := range *nameToSubset {
		subsetReplicas, subsetReadyReplicas, subsetUpdatedReplicas, subsetUpdatedReadyReplicas := replicasStatusFn(subset)
		newStatus.Replicas += subsetReplicas
		newStatus.ReadyReplicas += subsetReadyReplicas
		newStatus.UpdatedReplicas += subsetUpdatedReplicas
		newStatus.UpdatedReadyReplicas += subsetUpdatedReadyReplicas
	}

	newStatus.SubsetReplicas = *nextReplicas

	if newStatus.CurrentRevision == "" {
		// init with current revision
		newStatus.CurrentRevision = currentRevision.Name
	}

	if newStatus.UpdateStatus == nil {
		newStatus.UpdateStatus = &appsv1alpha1.UpdateStatus{}
	}

	newStatus.UpdateStatus.UpdatedRevision = expectedRevision
	newStatus.UpdateStatus.CurrentPartitions = *nextPartition

	if newStatus.UpdateStatus.UpdatedRevision != newStatus.CurrentRevision && newStatus.UpdatedReadyReplicas >= newStatus.Replicas {
		newStatus.CurrentRevision = newStatus.UpdateStatus.UpdatedRevision
	}

	var subsetFailure *string
	for _, subset := range *nameToSubset {
		failureMessage := control.GetSubsetFailure(subset)
		if failureMessage != nil {
			subsetFailure = failureMessage
			break
		}
	}

	if subsetFailure == nil {
		RemoveUnitedDeploymentCondition(newStatus, appsv1alpha1.SubsetFailure)
	} else {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(appsv1alpha1.SubsetFailure, corev1.ConditionTrue, "Error", *subsetFailure))
	}

	return newStatus
}

var replicasStatusFn = replicasStatus

func replicasStatus(subset *Subset) (replicas, readyReplicas, updatedReplicas, updatedReadyReplicas int32) {
	replicas = subset.Status.Replicas
	readyReplicas = subset.Status.ReadyReplicas
	updatedReplicas = subset.Status.UpdatedReplicas
	updatedReadyReplicas = subset.Status.UpdatedReadyReplicas
	return
}

func (r *ReconcileUnitedDeployment) updateUnitedDeployment(ud *appsv1alpha1.UnitedDeployment, oldStatus, newStatus *appsv1alpha1.UnitedDeploymentStatus) (*appsv1alpha1.UnitedDeployment, error) {
	if oldStatus.Replicas == newStatus.Replicas &&
		oldStatus.ReadyReplicas == newStatus.ReadyReplicas &&
		oldStatus.UpdatedReplicas == newStatus.UpdatedReplicas &&
		oldStatus.UpdatedReadyReplicas == newStatus.UpdatedReadyReplicas &&
		oldStatus.CurrentRevision == newStatus.CurrentRevision &&
		oldStatus.CollisionCount == newStatus.CollisionCount &&
		oldStatus.LabelSelector == newStatus.LabelSelector &&
		ud.Generation == newStatus.ObservedGeneration &&
		reflect.DeepEqual(oldStatus.SubsetReplicas, newStatus.SubsetReplicas) &&
		reflect.DeepEqual(oldStatus.UpdateStatus, newStatus.UpdateStatus) &&
		reflect.DeepEqual(oldStatus.Conditions, newStatus.Conditions) {
		return ud, nil
	}

	newStatus.ObservedGeneration = ud.Generation

	var getErr, updateErr error
	for i, obj := 0, ud; ; i++ {
		klog.V(4).Infof(fmt.Sprintf("The %d th time updating status for %v: %s/%s, ", i, obj.Kind, obj.Namespace, obj.Name) +
			fmt.Sprintf("replicas %d->%d (need %d), ", obj.Status.Replicas, newStatus.Replicas, obj.Spec.Replicas) +
			fmt.Sprintf("readyReplicas %d->%d (need %d), ", obj.Status.ReadyReplicas, newStatus.ReadyReplicas, obj.Spec.Replicas) +
			fmt.Sprintf("updatedReplicas %d->%d, ", obj.Status.UpdatedReplicas, newStatus.UpdatedReplicas) +
			fmt.Sprintf("updatedReadyReplicas %d->%d, ", obj.Status.UpdatedReadyReplicas, newStatus.UpdatedReadyReplicas) +
			fmt.Sprintf("sequence No: %v->%v", obj.Status.ObservedGeneration, newStatus.ObservedGeneration))

		obj.Status = *newStatus

		updateErr = r.Client.Status().Update(context.TODO(), obj)
		if updateErr == nil {
			return obj, nil
		}
		if i >= updateRetries {
			break
		}
		tmpObj := &appsv1alpha1.UnitedDeployment{}
		if getErr = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, tmpObj); getErr != nil {
			return nil, getErr
		}
		obj = tmpObj
	}

	klog.Errorf("fail to update UnitedDeployment %s/%s status: %s", ud.Namespace, ud.Name, updateErr)
	return nil, updateErr
}
