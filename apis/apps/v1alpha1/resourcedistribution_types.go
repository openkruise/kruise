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

	// Resource must be the complete yaml that users want to distribute
	Resource runtime.RawExtension `json:"resource,omitempty"`

	// WritePolicy define the behavior when other resources with the same name already exist in target namespaces
	WritePolicy ResourceDistributionWritePolicyType `json:"writePolicy,omitempty"`

	// Targets defines the namespaces that users want to distribute to
	Targets ResourceDistributionTargets `json:"targets,omitempty"`
}

// ResourceDistributionWritePolicyType defines the behaviors,
// when Resource has the same kind and name with existing resources.
type ResourceDistributionWritePolicyType string

const (
	// RESOURCEDISTRIBUTION_STRICT_WRITEPOLICY means that ResourceDistribution is successful if and only if there is no Name Conflict
	// In case of conflict, Resource will not be distributed anywhere
	// Default WritePolicy
	RESOURCEDISTRIBUTION_STRICT_WRITEPOLICY ResourceDistributionWritePolicyType = "strict"

	// RESOURCEDISTRIBUTION_IGNORE_WRITEPOLICY means that ResourceDistribution will ignore conflicting namespaces.
	// In case of conflict, Resource just will be distributed to the non-conflicting namespaces, AND
	// will NOT be distributed to the conflicting namespaces
	RESOURCEDISTRIBUTION_IGNORE_WRITEPOLICY ResourceDistributionWritePolicyType = "ignore"

	// RESOURCEDISTRIBUTION_OVERWRITE_WRITEPOLICY means that conflicting resources will be over-written
	RESOURCEDISTRIBUTION_OVERWRITE_WRITEPOLICY ResourceDistributionWritePolicyType = "overwrite"
)

// ResourceDistributionTargets defines the targets of Resource.
// Four options are provided to select target namespaces.
// If more than options were selected, their **intersection** will be selected.
type ResourceDistributionTargets struct {
	// Resource will be distributed to all namespaces, except listed namespaces
	// Default target
	// +optional
	All []ResourceDistributionTargetException `json:"all,omitempty"`

	// If it is not empty, Resource will be distributed to the listed namespaces
	// +optional
	Namespaces []ResourceDistributionNamespace `json:"namespaces,omitempty"`

	// If NamespaceLabelSelector is not empty, Resource will be distributed to the matched namespaces
	// +optional
	NamespaceLabelSelector metav1.LabelSelector `json:"namespaceLabelSelector,omitempty"`

	// If WorkloadLabelSelector is not empty, Resource will be distributed to the namespaces that contain any matched workload
	// +optional
	WorkloadLabelSelector ResourceDistributionWorkloadLabelSelector `json:"workloadLabelSelector,omitempty"`
}

type ResourceDistributionTargetException struct {
	// The name of excepted namespace
	Exception string `json:"exception,omitempty"`
}

type ResourceDistributionNamespace struct {
	// Namespace name
	Name string `json:"name,omitempty"`
}

type ResourceDistributionWorkloadLabelSelector struct {
	// Workload APIVersion
	APIVersion string `json:"apiVersion,omitempty"`
	// Workload Kind
	Kind string `json:"kind,omitempty"`
	// Workload labels that users want to select
	metav1.LabelSelector `json:",inline"`
}

// ResourceDistributionStatus defines the observed state of ResourceDistribution.
// ResourceDistributionStatus is recorded by kruise, users' modification is invalid and meaningless.
type ResourceDistributionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Describe ResourceDistribution Status
	Description string `json:"description,omitempty"`

	// List conflicting namespaces
	ConflictingNamespaces []ResourceDistributionNamespace `json:"conflictingNamespaces,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

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
