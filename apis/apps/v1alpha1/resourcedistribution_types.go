/*
Copyright 2021 The Kruise Authors.
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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ResourceDistributionSpec defines the desired state of ResourceDistribution.
type ResourceDistributionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Resource must be the complete yaml that users want to distribute.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:EmbeddedResource
	Resource runtime.RawExtension `json:"resource"`

	// Targets defines the namespaces that users want to distribute to.
	Targets ResourceDistributionTargets `json:"targets"`
}

// ResourceDistributionTargets defines the targets of Resource.
// Four options are provided to select target namespaces.
type ResourceDistributionTargets struct {
	// Priority: ExcludedNamespaces > AllNamespaces = IncludedNamespaces = NamespaceLabelSelector.
	// Firstly, ResourceDistributionTargets will parse AllNamespaces, IncludedNamespaces, and NamespaceLabelSelector,
	// then calculate their union,
	// At last ExcludedNamespaces will act on the union to remove and exclude the designated namespaces from it.

	// If AllNamespaces is true, Resource will be distributed to the all namespaces
	// (except some forbidden namespaces, such as "kube-system" and "kube-public").
	// +optional
	AllNamespaces bool `json:"allNamespaces,omitempty"`

	// If ExcludedNamespaces is not empty, Resource will never be distributed to the listed namespaces.
	// ExcludedNamespaces has the highest priority.
	// +optional
	ExcludedNamespaces ResourceDistributionTargetNamespaces `json:"excludedNamespaces,omitempty"`

	// If IncludedNamespaces is not empty, Resource will be distributed to the listed namespaces.
	// +optional
	IncludedNamespaces ResourceDistributionTargetNamespaces `json:"includedNamespaces,omitempty"`

	// If NamespaceLabelSelector is not empty, Resource will be distributed to the matched namespaces.
	// +optional
	NamespaceLabelSelector metav1.LabelSelector `json:"namespaceLabelSelector,omitempty"`
}

type ResourceDistributionTargetNamespaces struct {
	/*
		// TODO: support regular expression in the future
		// +optional
		Pattern string `json:"pattern,omitempty"`
	*/

	// +patchMergeKey=name
	// +patchStrategy=merge
	// +optional
	List []ResourceDistributionNamespace `json:"list,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
}

// ResourceDistributionNamespace contains a namespace name
type ResourceDistributionNamespace struct {
	// Namespace name
	Name string `json:"name,omitempty"`
}

// ResourceDistributionStatus defines the observed state of ResourceDistribution.
// ResourceDistributionStatus is recorded by kruise, users' modification is invalid and meaningless.
type ResourceDistributionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Desired represents the number of total target namespaces.
	Desired int32 `json:"desired,omitempty"`

	// Succeeded represents the number of successful distributions.
	Succeeded int32 `json:"succeeded,omitempty"`

	// Failed represents the number of failed distributions.
	Failed int32 `json:"failed,omitempty"`

	// ObservedGeneration represents the .metadata.generation that the condition was set based upon.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describe the condition when Resource creating, updating and deleting.
	Conditions []ResourceDistributionCondition `json:"conditions,omitempty"`
}

// ResourceDistributionCondition allows a row to be marked with additional information.
type ResourceDistributionCondition struct {
	// Type of ResourceDistributionCondition.
	Type ResourceDistributionConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown.
	Status ResourceDistributionConditionStatus `json:"status"`

	// LastTransitionTime is the last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason describe human readable message indicating details about last transition.
	Reason string `json:"reason,omitempty"`

	// FailedNamespaces describe all failed namespaces when Status is False
	FailedNamespaces []string `json:"failedNamespace,omitempty"`
}

type ResourceDistributionConditionType string

// These are valid conditions of a ResourceDistribution.
const (
	// ResourceDistributionConflictOccurred means there are conflict with existing resources when reconciling.
	ResourceDistributionConflictOccurred ResourceDistributionConditionType = "ConflictOccurred"

	// ResourceDistributionNamespaceNotExists means some target namespaces not exist.
	ResourceDistributionNamespaceNotExists ResourceDistributionConditionType = "NamespaceNotExists"

	// ResourceDistributionGetResourceFailed means some get operations about Resource are failed.
	ResourceDistributionGetResourceFailed ResourceDistributionConditionType = "GetResourceFailed"

	// ResourceDistributionCreateResourceFailed means some create operations about Resource are failed.
	ResourceDistributionCreateResourceFailed ResourceDistributionConditionType = "CreateResourceFailed"

	// ResourceDistributionUpdateResourceFailed means some update operations about Resource are failed.
	ResourceDistributionUpdateResourceFailed ResourceDistributionConditionType = "UpdateResourceFailed"

	// ResourceDistributionDeleteResourceFailed means some delete operations about Resource are failed.
	ResourceDistributionDeleteResourceFailed ResourceDistributionConditionType = "DeleteResourceFailed"
)

type ResourceDistributionConditionStatus string

// These are valid condition statuses.
const (
	// ResourceDistributionConditionTrue means a resource is in the condition.
	ResourceDistributionConditionTrue ResourceDistributionConditionStatus = "True"

	// ResourceDistributionConditionFalse means a resource is not in the condition.
	ResourceDistributionConditionFalse ResourceDistributionConditionStatus = "False"

	// ResourceDistributionConditionUnknown means kubernetes can't decide if a resource is in the condition or not.
	ResourceDistributionConditionUnknown ResourceDistributionConditionStatus = "Unknown"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=distributor
// +kubebuilder:printcolumn:name="TOTAL",type="integer",JSONPath=".status.desired",description="The desired number of desired distribution and syncs."
// +kubebuilder:printcolumn:name="SUCCEED",type="integer",JSONPath=".status.succeeded",description="The number of successful distribution and syncs."
// +kubebuilder:printcolumn:name="FAILED",type="integer",JSONPath=".status.failed",description="The number of failed distributions and syncs."

// ResourceDistribution is the Schema for the resourcedistributions API.
type ResourceDistribution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceDistributionSpec   `json:"spec,omitempty"`
	Status ResourceDistributionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ResourceDistributionList contains a list of ResourceDistribution.
type ResourceDistributionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceDistribution `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceDistribution{}, &ResourceDistributionList{})
}
