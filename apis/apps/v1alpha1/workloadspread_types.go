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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// WorkloadSpreadSpec defines the desired state of WorkloadSpread.
type WorkloadSpreadSpec struct {
	// TargetReference is the target workload that WorkloadSpread want to control.
	TargetReference *TargetReference `json:"targetRef"`

	// Subsets describes the pods distribution details between each of subsets.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Subsets []WorkloadSpreadSubset `json:"subsets" patchStrategy:"merge" patchMergeKey:"name"`

	// ScheduleStrategy indicates the strategy the WorkloadSpread used to preform the schedule between each of subsets.
	// +optional
	ScheduleStrategy WorkloadSpreadScheduleStrategy `json:"scheduleStrategy,omitempty"`
}

// TargetReference contains enough information to let you identify an workload
type TargetReference struct {
	// API version of the referent.
	APIVersion string `json:"apiVersion"`
	// Kind of the referent.
	Kind string `json:"kind"`
	// Name of the referent.
	Name string `json:"name"`
}

// WorkloadSpreadScheduleStrategyType is a string enumeration type that enumerates
// all possible schedule strategies for the WorkloadSpread controller.
// +kubebuilder:validation:Enum=Adaptive;Fixed;""
type WorkloadSpreadScheduleStrategyType string

const (
	// AdaptiveWorkloadSpreadScheduleStrategyType represents that user can permit that controller reschedule Pod
	// or simulate a schedule process to check whether Pod can run in some subset.
	AdaptiveWorkloadSpreadScheduleStrategyType WorkloadSpreadScheduleStrategyType = "Adaptive"
	// FixedWorkloadSpreadScheduleStrategyType represents to give up reschedule and simulation schedule feature.
	FixedWorkloadSpreadScheduleStrategyType WorkloadSpreadScheduleStrategyType = "Fixed"
)

// WorkloadSpreadScheduleStrategy defines the schedule performance of WorkloadSpread
type WorkloadSpreadScheduleStrategy struct {
	// Type indicates the type of the WorkloadSpreadScheduleStrategy.
	// Default is Fixed
	// +optional
	Type WorkloadSpreadScheduleStrategyType `json:"type,omitempty"`

	// Adaptive is used to communicate parameters when Type is AdaptiveWorkloadSpreadScheduleStrategyType.
	// +optional
	Adaptive *AdaptiveWorkloadSpreadStrategy `json:"adaptive,omitempty"`
}

// AdaptiveWorkloadSpreadStrategy is used to communicate parameters when Type is AdaptiveWorkloadSpreadScheduleStrategyType.
type AdaptiveWorkloadSpreadStrategy struct {
	// DisableSimulationSchedule indicates whether to disable the feature of simulation schedule.
	// Default is false.
	// Webhook can take a simple general predicates to check whether Pod can be scheduled into this subset,
	// but it just considers the Node resource and cannot replace scheduler to do richer predicates practically.
	// +optional
	DisableSimulationSchedule bool `json:"disableSimulationSchedule,omitempty"`

	// RescheduleCriticalSeconds indicates how long controller will reschedule a schedule failed Pod to the subset that has
	// redundant capacity after the subset where the Pod lives. If a Pod was scheduled failed and still in a unschedulabe status
	// over RescheduleCriticalSeconds duration, the controller will reschedule it to a suitable subset.
	// +optional
	RescheduleCriticalSeconds *int32 `json:"rescheduleCriticalSeconds,omitempty"`
}

// WorkloadSpreadSubset defines the details of a subset.
type WorkloadSpreadSubset struct {
	// Name should be unique between all of the subsets under one WorkloadSpread.
	Name string `json:"name"`

	// Indicates the node required selector to form the subset.
	// +optional
	RequiredNodeSelectorTerm *corev1.NodeSelectorTerm `json:"requiredNodeSelectorTerm,omitempty"`

	// Indicates the node preferred selector to form the subset.
	// +optional
	PreferredNodeSelectorTerms []corev1.PreferredSchedulingTerm `json:"preferredNodeSelectorTerms,omitempty"`

	// Indicates the tolerations the pods under this subset have.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// MaxReplicas indicates the desired max replicas of this subset.
	// +optional
	MaxReplicas *intstr.IntOrString `json:"maxReplicas,omitempty"`

	// Patch indicates patching podTemplate to the Pod.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Patch runtime.RawExtension `json:"patch,omitempty"`
}

// WorkloadSpreadStatus defines the observed state of WorkloadSpread.
type WorkloadSpreadStatus struct {
	// ObservedGeneration is the most recent generation observed for this WorkloadSpread. It corresponds to the
	// WorkloadSpread's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ObservedGeneration is the most recent replicas of target workload observed for this WorkloadSpread.
	//ObservedWorkloadReplicas int32 `json:"observedWorkloadReplicas"`

	// Contains the status of each subset. Each element in this array represents one subset
	// +optional
	SubsetStatuses []WorkloadSpreadSubsetStatus `json:"subsetStatuses,omitempty"`

	// VersionedSubsetStatuses is to solve rolling-update problems, where the creation of new-version pod
	// may be earlier than deletion of old-version pod. We have to calculate the pod subset distribution for
	// each version.
	VersionedSubsetStatuses map[string][]WorkloadSpreadSubsetStatus `json:"versionedSubsetStatuses,omitempty"`
}

type WorkloadSpreadSubsetConditionType string

const (
	// SubsetSchedulable means the nodes in this subset have sufficient resources to schedule a fixed number of Pods of a workload.
	// When one or more one pods in a subset have condition[PodScheduled] = false, the subset is considered temporarily unschedulable.
	// The condition[PodScheduled] = false, which means the cluster does not have enough resources to schedule at this moment,
	// even if only single Pod scheduled fails.
	// After a period of time(e.g. 5m), the controller will recover the subset to be schedulable.
	SubsetSchedulable WorkloadSpreadSubsetConditionType = "Schedulable"
)

type WorkloadSpreadSubsetCondition struct {
	// Type of in place set condition.
	Type WorkloadSpreadSubsetConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`

	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// WorkloadSpreadSubsetStatus defines the observed state of subset
type WorkloadSpreadSubsetStatus struct {
	// Name should be unique between all of the subsets under one WorkloadSpread.
	Name string `json:"name"`

	// Replicas is the most recently observed number of active replicas for subset.
	Replicas int32 `json:"replicas"`

	// Conditions is an array of current observed subset conditions.
	// +optional
	Conditions []WorkloadSpreadSubsetCondition `json:"conditions,omitempty"`

	// MissingReplicas is the number of active replicas belong to this subset not be found.
	// MissingReplicas > 0 indicates the subset is still missing MissingReplicas pods to create
	// MissingReplicas = 0 indicates the subset already has enough pods, there is no need to create
	// MissingReplicas = -1 indicates the subset's MaxReplicas not set, then there is no limit for pods number
	MissingReplicas int32 `json:"missingReplicas"`

	// CreatingPods contains information about pods whose creation was processed by
	// the webhook handler but not yet been observed by the WorkloadSpread controller.
	// A pod will be in this map from the time when the webhook handler processed the
	// creation request to the time when the pod is seen by controller.
	// The key in the map is the name of the pod and the value is the time when the webhook
	// handler process the creation request. If the real creation didn't happen and a pod is
	// still in this map, it will be removed from the list automatically by WorkloadSpread controller
	// after some time.
	// If everything goes smooth this map should be empty for the most of the time.
	// Large number of entries in the map may indicate problems with pod creations.
	// +optional
	CreatingPods map[string]metav1.Time `json:"creatingPods,omitempty"`

	// DeletingPods is similar with CreatingPods and it contains information about pod deletion.
	// +optional
	DeletingPods map[string]metav1.Time `json:"deletingPods,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ws
// +kubebuilder:printcolumn:name=WorkloadName,type=string,JSONPath=".spec.targetRef.name"
// +kubebuilder:printcolumn:name=WorkloadKind,type=string,JSONPath=".spec.targetRef.kind"
// +kubebuilder:printcolumn:name=Adaptive,type=boolean,JSONPath=`.spec.scheduleStrategy.type[?(@ == "Adaptive")]`,description="Whether use the adaptive reschedule strategy"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkloadSpread is the Schema for the WorkloadSpread API
type WorkloadSpread struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadSpreadSpec   `json:"spec,omitempty"`
	Status WorkloadSpreadStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadSpreadList contains a list of WorkloadSpread
type WorkloadSpreadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadSpread `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadSpread{}, &WorkloadSpreadList{})
}
