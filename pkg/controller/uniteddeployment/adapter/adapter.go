package adapter

import (
	alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type Adapter interface {
	// NewResourceObject creates a empty subset object.
	NewResourceObject() runtime.Object
	// NewResourceListObject creates a empty subset list object.
	NewResourceListObject() runtime.Object
	// GetObjectMeta returns the ObjectMeta of the subset.
	GetObjectMeta(subset metav1.Object) *metav1.ObjectMeta
	// GetStatusObservedGeneration returns the observed generation of the subset.
	GetStatusObservedGeneration(subset metav1.Object) int64
	// GetReplicaDetails returns the replicas information of the subset status.
	GetReplicaDetails(subset metav1.Object, updatedRevision string) (specReplicas, specPartition *int32, statusReplicas, statusReadyReplicas, statusUpdatedReplicas, statusUpdatedReadyReplicas int32, err error)
	// GetSubsetFailure returns failure information of the subset.
	GetSubsetFailure() *string
	// ConvertToResourceList converts subset list object to subset array.
	ConvertToResourceList(subsetList runtime.Object) []metav1.Object
	// ApplySubsetTemplate updates the subset to the latest revision.
	ApplySubsetTemplate(ud *alpha1.UnitedDeployment, subsetName, revision string, replicas, partition int32, subset runtime.Object) error
	// IsExpected checks the subset is the expected revision or not.
	// If not, UnitedDeployment will call ApplySubsetTemplate to update it.
	IsExpected(ud *alpha1.UnitedDeployment, subset metav1.Object, revision string) bool
	// PostUpdate does some works after subset updated
	PostUpdate(ud *alpha1.UnitedDeployment, subset runtime.Object, revision string, partition int32) error
}
