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

package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// UpdateStrategyType is a string enumeration type that enumerates
// all possible update strategies for the UnitedDeployment controller.
type UpdateStrategyType string

const (
	// ManualUpdateStrategyType indicate the partition of each subset.
	ManualUpdateStrategyType UpdateStrategyType = "Manual"
)

// UnitedDeploymentSpec defines the desired state of UnitedDeployment
type UnitedDeploymentSpec struct {
	// Replicas is the totally desired number of replicas of all the owning workloads.
	// If unspecified, defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	Selector *metav1.LabelSelector `json:"selector"`

	// Template is the object that describes the subset that will be created.
	// +optional
	Template SubsetTemplate `json:"template,omitempty"`

	// Contains the information of subset topology.
	// +optional
	Topology Topology `json:"topology,omitempty"`

	// Strategy indicates the action of updating
	// +optional
	UpdateStrategy UnitedDeploymentUpdateStrategy `json:"updateStrategy,omitempty"`

	// Indicates the number of histories to be conserved.
	// If unspecified, defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// SubsetTemplate defines the subset under the UnitedDeployment.
type SubsetTemplate struct {
	// StatefulSet template
	// +optional
	StatefulSetTemplate *StatefulSetTemplateSpec `json:"statefulSetTemplate,omitempty"`
}

// StatefulSetTemplateSpec defines the subset template of StatefulSet.
type StatefulSetTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              appsv1.StatefulSetSpec `json:"spec"`
}

// UnitedDeploymentUpdateStrategy defines the update strategy of UnitedDeployment.
type UnitedDeploymentUpdateStrategy struct {
	// Type of UnitedDeployment update.
	// Default is Manual.
	// +optional
	Type UpdateStrategyType `json:"type,omitempty"`
	// Indicate the partition of each subset.
	// +optional
	ManualUpdate *ManualUpdate `json:"manualUpdate,omitempty"`
}

// ManualUpdate is a update strategy which allow users to provide the partition of each subset.
type ManualUpdate struct {
	// Indicates number of subset partition.
	// +optional
	Partitions map[string]int32 `json:"partitions,omitempty"`
}

// Topology defines the spread detail of each subset under UnitedDeployment.
type Topology struct {
	// Contains the details of each subset.
	// +optional
	Subsets []Subset `json:"subsets,omitempty"`
}

// Subset defines the detail of a subset.
type Subset struct {
	// Indicates the name of this subset, which will be used to generate
	// subset workload name prefix in the format '<deployment-name>-<subset-name>-'.
	Name string `json:"name"`

	// Indicates the node select strategy to form the subset.
	// +optional
	NodeSelector corev1.NodeSelector `json:"nodeSelector,omitempty"`

	// Indicates the number of the subset replicas or percentage of it on the UnitedDeployment replicas.
	// If nil, the number of replicas in this subset is determined by controller.
	// +optional
	Replicas *intstr.IntOrString `json:"replicas,omitempty"`
}

// UnitedDeploymentStatus defines the observed state of UnitedDeployment.
type UnitedDeploymentStatus struct {
	// ObservedGeneration is the most recent generation observed for this UnitedDeployment. It corresponds to the
	// UnitedDeployment's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// The number of ready replicas.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Replicas is the most recently observed number of replicas.
	Replicas int32 `json:"replicas"`

	// The number of pods in current version.
	UpdatedReplicas int32 `json:"updatedReplicas"`

	// The number of ready current revision replicas for this UnitedDeployment.
	// +optional
	UpdatedReadyReplicas int32 `json:"updatedReadyReplicas,omitempty"`

	// Count of hash collisions for the UnitedDeployment. The UnitedDeployment controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// CurrentRevision, if not empty, indicates the current version of the UnitedDeployment.
	CurrentRevision string `json:"currentRevision"`

	// Records the topology detail information of the replicas of each subset.
	// +optional
	SubsetReplicas map[string]int32 `json:"subsetReplicas,omitempty"`

	// Represents the latest available observations of a UnitedDeployment's current state.
	// +optional
	Conditions []UnitedDeploymentCondition `json:"conditions,omitempty"`

	// Records the information of update progress.
	// +optional
	UpdateStatus *UpdateStatus `json:"updateStatus,omitempty"`
}

// UnitedDeploymentConditionType indicates valid conditions type of a UnitedDeployment.
type UnitedDeploymentConditionType string

// UnitedDeploymentCondition describes current state of a UnitedDeployment.
type UnitedDeploymentCondition struct {
	// Type of in place set condition.
	Type UnitedDeploymentConditionType `json:"type,omitempty"`

	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status,omitempty"`

	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// UpdateStatus defines the observed update state of UnitedDeployment.
type UpdateStatus struct {
	// Records the latest revision.
	// +optional
	UpdatedRevision string `json:"updatedRevision,omitempty"`

	// Records the current partition.
	// +optional
	CurrentPartitions map[string]int32 `json:"currentPartitions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UnitedDeployment is the Schema for the uniteddeployments API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:resource:shortName=ud
// +kubebuilder:printcolumn:name="DESIRED",type="integer",JSONPath=".spec.replicas",description="The desired number of pods."
// +kubebuilder:printcolumn:name="CURRENT",type="integer",JSONPath=".status.replicas",description="The number of currently all pods."
// +kubebuilder:printcolumn:name="UPDATED",type="integer",JSONPath=".status.updatedReplicas",description="The number of pods updated."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyReplicas",description="The number of pods ready."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
type UnitedDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UnitedDeploymentSpec   `json:"spec,omitempty"`
	Status UnitedDeploymentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UnitedDeploymentList contains a list of UnitedDeployment.
type UnitedDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UnitedDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UnitedDeployment{}, &UnitedDeploymentList{})
}
