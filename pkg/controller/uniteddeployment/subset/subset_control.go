package subset

import (
	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

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
