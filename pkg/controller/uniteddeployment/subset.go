package uniteddeployment

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

// Subset stores the details of a subset resource owned by one UnitedDeployment.
type Subset struct {
	metav1.ObjectMeta

	Spec   SubsetSpec
	Status SubsetStatus
}

// SubsetSpec stores the spec details of the Subset
type SubsetSpec struct {
	SubsetName string
	Replicas   int32
	Strategy   SubsetUpdateStrategy
	SubsetRef  ResourceRef
}

// SubsetStatus stores the observed state of the Subset.
type SubsetStatus struct {
	ObservedGeneration int64
	Replicas           int32
	ReadyReplicas      int32
	RevisionReplicas   map[string]*SubsetReplicaStatus
}

// SubsetReplicaStatus store the replicas of pods under corresponding revision
type SubsetReplicaStatus struct {
	Replicas      int32
	ReadyReplicas int32
}

// SubsetUpdateStrategy stores the strategy detail of the Subset.
type SubsetUpdateStrategy struct {
	Partition int32
}

// ResourceRef stores the Subset resource it represents.
type ResourceRef struct {
	Resources []metav1.Object
}

// ControlInterface defines the interface that UnitedDeployment uses to list, create, update, and delete Subsets.
type ControlInterface interface {
	// GetAllSubsets returns the subsets which are managed by the UnitedDeployment
	GetAllSubsets(ud *appsv1alpha1.UnitedDeployment) ([]*Subset, error)
	// // CreateSubset creates the subset depending on the inputs.
	CreateSubset(ud *appsv1alpha1.UnitedDeployment, unit string, revision string, replicas, partition int32) error
	// UpdateSubset updates the target subset with the input information.
	UpdateSubset(subSet *Subset, ud *appsv1alpha1.UnitedDeployment, revision string, replicas, partition int32) error
	// UpdateSubset is used to delete the input subset.
	DeleteSubset(*Subset) error
}
