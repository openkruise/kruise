package subset

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Subset stores the details of a subset resource owned by one UnitedDeployment.
type Subset struct {
	metav1.ObjectMeta

	Spec   Spec
	Status Status
}

// Spec stores the spec details of the Subset
type Spec struct {
	SubsetName string
	Replicas   int32
	Strategy   UpdateStrategy
	SubsetRef  ResourceRef
}

// Status stores the observed state of the Subset.
type Status struct {
	ObservedGeneration int64
	Replicas           int32
	ReadyReplicas      int32
	RevisionReplicas   map[string]*ReplicaStatus
}

// ReplicaStatus store the replicas of pods under corresponding revision
type ReplicaStatus struct {
	Replicas      int32
	ReadyReplicas int32
}

// UpdateStrategy stores the strategy detail of the Subset.
type UpdateStrategy struct {
	Partition int32
}

// ResourceRef stores the Subset resource it represents.
type ResourceRef struct {
	Resources []metav1.Object
}
