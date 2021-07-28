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

	// Targets defines the namespaces that users want to distribute to
	Targets ResourceDistributionTargets `json:"targets,omitempty"`
}

// ResourceDistributionTargets defines the targets of Resource.
// Four options are provided to select target namespaces.
// If more than options were selected, their **intersection** will be selected.
type ResourceDistributionTargets struct {
	// Resource will be distributed to all namespaces, except listed namespaces
	// Default target
	// +optional
	ExcludedNamespaces []ResourceDistributionNamespace `json:"excludedNamespaces,omitempty"`

	// If it is not empty, Resource will be distributed to the listed namespaces
	// +optional
	IncludedNamespaces []ResourceDistributionNamespace `json:"includedNamespaces,omitempty"`

	// If NamespaceLabelSelector is not empty, Resource will be distributed to the matched namespaces
	// +optional
	NamespaceLabelSelector metav1.LabelSelector `json:"namespaceLabelSelector,omitempty"`
}

type ResourceDistributionNamespace struct {
	// Namespace name
	Name string `json:"name,omitempty"`
}

// ResourceDistributionStatus defines the observed state of ResourceDistribution.
// ResourceDistributionStatus is recorded by kruise, users' modification is invalid and meaningless.
type ResourceDistributionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Describe ResourceDistribution Status
	Description string `json:"description,omitempty"`

	// DistributedResources lists the resources that has been distributed by this ResourceDistribution
	// Example: "ns-1": "12334234", "ns-1" denotes a namespace name, "12334234" is the Resource version
	DistributedResources map[string]string `json:"distributedResources,omitempty"`
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
