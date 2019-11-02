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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
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
	statefulSetSubSetType subSetType = "StatefulSet"
)

// Add creates a new UnitedDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileUnitedDeployment{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),

		recorder: mgr.GetRecorder(controllerName),
		subSetControls: map[subSetType]ControlInterface{
			statefulSetSubSetType: &StatefulSetControl{Client: mgr.GetClient(), scheme: mgr.GetScheme()},
		},
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
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

// Reconcile reads that state of the cluster for a UnitedDeployment object and makes changes based on the state read
// and what is in the UnitedDeployment.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps.kruise.io,resources=uniteddeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=uniteddeployments/status,verbs=get;update;patch
func (r *ReconcileUnitedDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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

	currentRevision, updatedRevision, _, _, err := r.constructUnitedDeploymentRevisions(instance)
	if err != nil {
		klog.Errorf("Fail to construct controller revision of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeRevisionProvision), err.Error())
		return reconcile.Result{}, err
	}

	control, subsetType := r.getSubsetControls(instance)

	klog.V(4).Infof("Get UnitedDeployment %s/%s all subsets", request.Namespace, request.Name)
	nameToSubset, err := r.getNameToSubset(instance, control)
	if err != nil {
		klog.Errorf("Fail to get Subsets of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeFindSubsets), err.Error())
		return reconcile.Result{}, nil
	}

	nextReplicas, effectiveSpecifiedReplicas := GetAllocatedReplicas(nameToSubset, instance)
	klog.V(4).Infof("Get UnitedDeployment %s/%s next replicas %v", instance.Namespace, instance.Name, nextReplicas)
	if !effectiveSpecifiedReplicas {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeSpecifySubbsetReplicas), "Specified subset replicas is ineffective")
	}

	nextPartitions := calcNextPartitions(instance, nextReplicas)
	klog.V(4).Infof("Get UnitedDeployment %s/%s next partition %v", instance.Namespace, instance.Name, nextPartitions)

	if err := r.manageSubsets(instance, nameToSubset, nextReplicas, nextPartitions, currentRevision, updatedRevision, subsetType); err != nil {
		klog.Errorf("Fail to update UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeSubsetsUpdate), err.Error())
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileUnitedDeployment) getNameToSubset(instance *appsv1alpha1.UnitedDeployment, control ControlInterface) (nameToSubset map[string]*Subset, err error) {
	subSets, err := control.GetAllSubsets(instance)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeFindSubsets), err.Error())
		return nil, fmt.Errorf("fail to get all Subsets for UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	klog.V(4).Infof("Classify UnitedDeployment %s/%s by subSet name", instance.Namespace, instance.Name)
	nameToSubsets := r.classifySubsetBySubsetName(instance, subSets)

	nameToSubset, err = r.deleteDupSubset(instance, nameToSubsets, control)
	if err != nil {
		r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeDupSubsetsDelete), err.Error())
		return nil, fmt.Errorf("fail to manage duplicate Subset of UnitedDeployment %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	return
}

func calcNextPartitions(ud *appsv1alpha1.UnitedDeployment, nextReplicas map[string]int32) map[string]int32 {
	partitions := map[string]int32{}
	for _, subset := range ud.Spec.Topology.Subsets {
		var subsetPartition int32
		if isManualUpdateStrategy(ud) && ud.Spec.Strategy.ManualUpdate != nil && ud.Spec.Strategy.ManualUpdate.Partitions != nil {
			if partition, exist := ud.Spec.Strategy.ManualUpdate.Partitions[subset.Name]; exist {
				subsetPartition = partition
			}
		}

		if subsetReplicas, exist := nextReplicas[subset.Name]; exist && subsetPartition > subsetReplicas {
			subsetPartition = subsetReplicas
		}

		partitions[subset.Name] = subsetPartition
	}

	return partitions
}

func isManualUpdateStrategy(ud *appsv1alpha1.UnitedDeployment) bool {
	return len(ud.Spec.Strategy.Type) == 0 || ud.Spec.Strategy.Type == appsv1alpha1.ManualUpdateStrategyType
}

var subsetReplicasFn = subSetReplicas

func subSetReplicas(subset *Subset) int32 {
	return subset.Status.Replicas
}

func (r *ReconcileUnitedDeployment) deleteDupSubset(ud *appsv1alpha1.UnitedDeployment, nameToSubsets map[string][]*Subset, control ControlInterface) (map[string]*Subset, error) {
	nameToSubset := map[string]*Subset{}
	for name, subsets := range nameToSubsets {
		if len(subsets) > 1 {
			for _, subset := range subsets[1:] {
				klog.V(0).Infof("Delete duplicated Subset %s/%s for subset name %s", subset.Namespace, subset.Name, name)
				if err := control.DeleteSubset(subset); err != nil {
					if errors.IsNotFound(err) {
						continue
					}

					return nameToSubset, err
				}
			}
		}

		if len(subsets) > 0 {
			nameToSubset[name] = subsets[0]
		}
	}

	return nameToSubset, nil
}

func (r *ReconcileUnitedDeployment) getSubsetControls(instance *appsv1alpha1.UnitedDeployment) (ControlInterface, subSetType) {
	if instance.Spec.Template.StatefulSetTemplate != nil {
		return r.subSetControls[statefulSetSubSetType], statefulSetSubSetType
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

		_, exist := mapping[subSetName]
		if !exist {
			var subsetWithName []*Subset
			subsetWithName = append(subsetWithName, ss)
			mapping[subSetName] = subsetWithName
		} else {
			mapping[subSetName] = append(mapping[subSetName], ss)
		}
	}
	return mapping
}
