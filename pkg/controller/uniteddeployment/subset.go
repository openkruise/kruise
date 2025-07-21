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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// Subset stores the details of a subset resource owned by one UnitedDeployment.
type Subset struct {
	metav1.ObjectMeta

	Spec   SubsetSpec
	Status SubsetStatus
}

// SubsetSpec stores the spec details of the Subset
type SubsetSpec struct {
	SubsetName     string
	Replicas       int32
	UpdateStrategy SubsetUpdateStrategy
	SubsetRef      ResourceRef
	SubsetPods     []*corev1.Pod
}

// SubsetStatus stores the observed state of the Subset.
type SubsetStatus struct {
	ObservedGeneration   int64
	Replicas             int32
	ReadyReplicas        int32
	UpdatedReplicas      int32
	UpdatedReadyReplicas int32
	UpdatedRevision      string
	UnschedulableStatus  SubsetUnschedulableStatus
}

// SubsetUnschedulableStatus stores the unschedulable status of the Subset, which is used by adaptive strategy.
// There are two sub-strategies of adaptive strategy:
//   - default strategy: Pending pods will be deleted with the corresponding subset marked as unschedulable.
//   - reservation strategy: Pending pods will be reserved and temporary Pods will be created in another subset to
//     take over their work until they have been scheduled and started.
type SubsetUnschedulableStatus struct {
	// In the Adaptive strategy, a subset is considered Unschedulable when Pods within it are in a Pending state due to
	// scheduling failures (default mode), or when there are reserved Pods in it (reservation mode).
	Unschedulable bool

	// In reservation adaptive strategy, MarkedAsUnschedulable is used to ensure the number of ready replicas in a
	// recovered unschedulable subset meets expectations, thereby avoiding premature deletion of temp Pods that could
	// lead to insufficient working replicas.
	//
	// Specifically, when a subset is marked as unschedulable, even if there are no reserved Pods in it (for example,
	// all reserved Pods have been successfully scheduled and started), all newly created Pods will be directly marked
	// as reserved. In this way, the corresponding number of temporary Pods will only be scaled down after the new Pods
	// are confirmed to have successfully started and exited the reserved state.
	MarkedAsUnschedulable bool
	// In reservation adaptive strategy, it is the number of reserved pods in the subset.
	// Please refer to the function CheckPodReallyInReservedStatus.
	ReservedPods int32
	// The number of Pending Pods, used by normal adaptive strategy only. It is always 0 in reservation strategy.
	PendingPods int32
	// Healthy running pods with old revision and marked as reserved (timeouted)
	UpdateTimeoutPods int32
}

func (s *Subset) Allocatable() bool {
	return !s.Status.UnschedulableStatus.Unschedulable && !s.Status.UnschedulableStatus.MarkedAsUnschedulable
}

// SubsetUpdateStrategy stores the strategy detail of the Subset.
type SubsetUpdateStrategy struct {
	Partition int32
}

// SubsetUpdate stores the subset field that may need to be updated
type SubsetUpdate struct {
	Replicas  int32
	Partition int32
	Patch     string
}

// ResourceRef stores the Subset resource it represents.
type ResourceRef struct {
	Resources []metav1.Object
}

// ControlInterface defines the interface that UnitedDeployment uses to list, create, update, and delete Subsets.
type ControlInterface interface {
	// GetAllSubsets returns the subsets which are managed by the UnitedDeployment.
	GetAllSubsets(ud *appsv1alpha1.UnitedDeployment, updatedRevision string) ([]*Subset, error)
	// CreateSubset creates the subset depending on the inputs.
	CreateSubset(ud *appsv1alpha1.UnitedDeployment, unit string, revision string, replicas, partition int32) error
	// UpdateSubset updates the target subset with the input information.
	UpdateSubset(subSet *Subset, ud *appsv1alpha1.UnitedDeployment, revision string, replicas, partition int32) error
	// DeleteSubset is used to delete the input subset.
	DeleteSubset(*Subset) error
	// GetSubsetFailure extracts the subset failure message to expose on UnitedDeployment status.
	GetSubsetFailure(*Subset) *string
}
