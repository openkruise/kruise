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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/uniteddeployment/adapter"
	"github.com/openkruise/kruise/pkg/util/refmanager"
)

// SubsetControl provides subset operations of MutableSet.
type SubsetControl struct {
	client.Client

	scheme  *runtime.Scheme
	adapter adapter.Adapter
}

// GetAllSubsets returns all subsets owned by the UnitedDeployment.
func (m *SubsetControl) GetAllSubsets(ud *alpha1.UnitedDeployment, updatedRevision string) (subSets []*Subset, err error) {
	selector, err := metav1.LabelSelectorAsSelector(ud.Spec.Selector)
	if err != nil {
		return nil, err
	}

	setList := m.adapter.NewResourceListObject()
	err = m.Client.List(context.TODO(), setList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	manager, err := refmanager.New(m.Client, ud.Spec.Selector, ud, m.scheme)
	if err != nil {
		return nil, err
	}

	v := reflect.ValueOf(setList).Elem().FieldByName("Items")
	selected := make([]metav1.Object, v.Len())
	for i := 0; i < v.Len(); i++ {
		selected[i] = v.Index(i).Addr().Interface().(metav1.Object)
	}
	claimedSets, err := manager.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	for _, claimedSet := range claimedSets {
		subSet, err := m.convertToSubset(claimedSet, updatedRevision)
		if err != nil {
			return nil, err
		}
		subSets = append(subSets, subSet)
	}
	return subSets, nil
}

// CreateSubset creates the Subset depending on the inputs.
func (m *SubsetControl) CreateSubset(ud *alpha1.UnitedDeployment, subsetName string, revision string, replicas, partition int32) error {
	set := m.adapter.NewResourceObject()
	if err := m.adapter.ApplySubsetTemplate(ud, subsetName, revision, replicas, partition, set); err != nil {
		return err
	}

	klog.V(4).InfoS("Replicas when creating Subset for UnitedDeployment", "replicas", replicas, "unitedDeployment", klog.KObj(ud))
	return m.Create(context.TODO(), set)
}

// UpdateSubset is used to update the subset. The target Subset workload can be found with the input subset.
func (m *SubsetControl) UpdateSubset(subset *Subset, ud *alpha1.UnitedDeployment, revision string, replicas, partition int32) error {
	set := m.adapter.NewResourceObject()
	var updateError error
	for i := 0; i < updateRetries; i++ {
		getError := m.Client.Get(context.TODO(), m.objectKey(&subset.ObjectMeta), set)
		if getError != nil {
			return getError
		}

		if err := m.adapter.ApplySubsetTemplate(ud, subset.Spec.SubsetName, revision, replicas, partition, set); err != nil {
			return err
		}

		updateError = m.Client.Update(context.TODO(), set)
		if updateError == nil {
			break
		}
	}

	if updateError != nil {
		return updateError
	}

	return m.adapter.PostUpdate(ud, set, revision, partition)
}

// DeleteSubset is called to delete the subset. The target Subset workload can be found with the input subset.
func (m *SubsetControl) DeleteSubset(subSet *Subset) error {
	set := subSet.Spec.SubsetRef.Resources[0].(client.Object)
	return m.Delete(context.TODO(), set, client.PropagationPolicy(metav1.DeletePropagationBackground))
}

// GetSubsetFailure return the error message extracted form Subset workload status conditions.
func (m *SubsetControl) GetSubsetFailure(*Subset) *string {
	return m.adapter.GetSubsetFailure()
}

func (m *SubsetControl) convertToSubset(set metav1.Object, updatedRevision string) (*Subset, error) {
	subset := &Subset{}
	subset.ObjectMeta = metav1.ObjectMeta{
		Name:                       set.GetName(),
		GenerateName:               set.GetGenerateName(),
		Namespace:                  set.GetNamespace(),
		SelfLink:                   set.GetSelfLink(),
		UID:                        set.GetUID(),
		ResourceVersion:            set.GetResourceVersion(),
		Generation:                 set.GetGeneration(),
		CreationTimestamp:          set.GetCreationTimestamp(),
		DeletionTimestamp:          set.GetDeletionTimestamp(),
		DeletionGracePeriodSeconds: set.GetDeletionGracePeriodSeconds(),
		Labels:                     set.GetLabels(),
		Annotations:                set.GetAnnotations(),
		OwnerReferences:            set.GetOwnerReferences(),
		Finalizers:                 set.GetFinalizers(),
	}

	pods, err := m.adapter.GetSubsetPods(set)
	if err != nil {
		return nil, err
	}
	subset.Spec.SubsetPods = pods

	subSetName, err := getSubsetNameFrom(set)
	if err != nil {
		return nil, err
	}
	subset.Spec.SubsetName = subSetName

	if specReplicas := m.adapter.GetSpecReplicas(set); specReplicas != nil {
		subset.Spec.Replicas = *specReplicas
	}

	if specPartition := m.adapter.GetSpecPartition(set, pods); specPartition != nil {
		subset.Spec.UpdateStrategy.Partition = *specPartition
	}
	subset.Spec.SubsetRef.Resources = append(subset.Spec.SubsetRef.Resources, set)

	subset.Status.ObservedGeneration = m.adapter.GetStatusObservedGeneration(set)
	subset.Status.Replicas = m.adapter.GetStatusReplicas(set)
	subset.Status.ReadyReplicas = m.adapter.GetStatusReadyReplicas(set)
	subset.Status.UpdatedReplicas, subset.Status.UpdatedReadyReplicas = adapter.CalculateUpdatedReplicas(pods, updatedRevision)

	return subset, nil
}

func (m *SubsetControl) objectKey(objMeta *metav1.ObjectMeta) client.ObjectKey {
	return types.NamespacedName{
		Namespace: objMeta.Namespace,
		Name:      objMeta.Name,
	}
}
