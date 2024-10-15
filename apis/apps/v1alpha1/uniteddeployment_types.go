/*
Copyright 2020 The Kruise Authors.

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
	"time"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// UpdateStrategyType is a string enumeration type that enumerates
// all possible update strategies for the UnitedDeployment controller.
type UpdateStrategyType string

const (
	// ManualUpdateStrategyType indicates the partition of each subset.
	// The update progress is able to be controlled by updating the partitions
	// of each subset.
	ManualUpdateStrategyType UpdateStrategyType = "Manual"
)

// UnitedDeploymentConditionType indicates valid conditions type of a UnitedDeployment.
type UnitedDeploymentConditionType string

const (
	// SubsetProvisioned means all the expected subsets are provisioned and unexpected subsets are deleted.
	SubsetProvisioned UnitedDeploymentConditionType = "SubsetProvisioned"
	// SubsetUpdated means all the subsets are updated.
	SubsetUpdated UnitedDeploymentConditionType = "SubsetUpdated"
	// SubsetFailure is added to a UnitedDeployment when one of its subsets has failure during its own reconciling.
	SubsetFailure UnitedDeploymentConditionType = "SubsetFailure"
)

// UnitedDeploymentSpec defines the desired state of UnitedDeployment.
type UnitedDeploymentSpec struct {
	// Replicas is the total desired replicas of all the subsets.
	// If unspecified, defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	Selector *metav1.LabelSelector `json:"selector"`

	// Template describes the subset that will be created.
	// +optional
	Template SubsetTemplate `json:"template,omitempty"`

	// Topology describes the pods distribution detail between each of subsets.
	// +optional
	Topology Topology `json:"topology,omitempty"`

	// UpdateStrategy indicates the strategy the UnitedDeployment use to preform the update,
	// when template is changed.
	// +optional
	UpdateStrategy UnitedDeploymentUpdateStrategy `json:"updateStrategy,omitempty"`

	// Indicates the number of histories to be conserved.
	// If unspecified, defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// SubsetTemplate defines the subset template under the UnitedDeployment.
// UnitedDeployment will provision every subset based on one workload templates in SubsetTemplate.
type SubsetTemplate struct {
	// StatefulSet template
	// +optional
	StatefulSetTemplate *StatefulSetTemplateSpec `json:"statefulSetTemplate,omitempty"`

	// AdvancedStatefulSet template
	// +optional
	AdvancedStatefulSetTemplate *AdvancedStatefulSetTemplateSpec `json:"advancedStatefulSetTemplate,omitempty"`

	// CloneSet template
	// +optional
	CloneSetTemplate *CloneSetTemplateSpec `json:"cloneSetTemplate,omitempty"`

	// Deployment template
	// +optional
	DeploymentTemplate *DeploymentTemplateSpec `json:"deploymentTemplate,omitempty"`
}

// StatefulSetTemplateSpec defines the subset template of StatefulSet.
type StatefulSetTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Spec appsv1.StatefulSetSpec `json:"spec"`
}

// AdvancedStatefulSetTemplateSpec defines the subset template of AdvancedStatefulSet.
type AdvancedStatefulSetTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              v1beta1.StatefulSetSpec `json:"spec"`
}

// CloneSetTemplateSpec defines the subset template of CloneSet.
type CloneSetTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CloneSetSpec `json:"spec"`
}

// DeploymentTemplateSpec defines the subset template of Deployment.
type DeploymentTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Spec appsv1.DeploymentSpec `json:"spec"`
}

// UnitedDeploymentUpdateStrategy defines the update performance
// when template of UnitedDeployment is changed.
type UnitedDeploymentUpdateStrategy struct {
	// Type of UnitedDeployment update strategy.
	// Default is Manual.
	// +optional
	Type UpdateStrategyType `json:"type,omitempty"`
	// Includes all of the parameters a Manual update strategy needs.
	// +optional
	ManualUpdate *ManualUpdate `json:"manualUpdate,omitempty"`
}

// ManualUpdate is a update strategy which allows users to control the update progress
// by providing the partition of each subset.
type ManualUpdate struct {
	// Indicates number of subset partition.
	// +optional
	Partitions map[string]int32 `json:"partitions,omitempty"`
}

// Topology defines the spread detail of each subset under UnitedDeployment.
// A UnitedDeployment manages multiple homogeneous workloads which are called subset.
// Each of subsets under the UnitedDeployment is described in Topology.
type Topology struct {
	// Contains the details of each subset. Each element in this array represents one subset
	// which will be provisioned and managed by UnitedDeployment.
	// +patchMergeKey=name
	// +patchStrategy=merge
	// +optional
	Subsets []Subset `json:"subsets,omitempty" patchStrategy:"merge" patchMergeKey:"name"`

	// ScheduleStrategy indicates the strategy the UnitedDeployment used to preform the schedule between each of subsets.
	// +optional
	ScheduleStrategy UnitedDeploymentScheduleStrategy `json:"scheduleStrategy,omitempty"`
}

// Subset defines the detail of a subset.
type Subset struct {
	// Indicates subset name as a DNS_LABEL, which will be used to generate
	// subset workload name prefix in the format '<deployment-name>-<subset-name>-'.
	// Name should be unique between all of the subsets under one UnitedDeployment.
	Name string `json:"name"`

	// Indicates the node selector to form the subset. Depending on the node selector,
	// pods provisioned could be distributed across multiple groups of nodes.
	// A subset's nodeSelectorTerm is not allowed to be updated.
	// +optional
	NodeSelectorTerm corev1.NodeSelectorTerm `json:"nodeSelectorTerm,omitempty"`

	// Indicates the tolerations the pods under this subset have.
	// A subset's tolerations is not allowed to be updated.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Indicates the number of the pod to be created under this subset. Replicas could also be
	// percentage like '10%', which means 10% of UnitedDeployment replicas of pods will be distributed
	// under this subset. If nil, the number of replicas in this subset is determined by controller.
	// Controller will try to keep all the subsets with nil replicas have average pods.
	// Replicas and MinReplicas/MaxReplicas are mutually exclusive in a UnitedDeployment.
	// +optional
	Replicas *intstr.IntOrString `json:"replicas,omitempty"`

	// Indicates the lower bounded replicas of the subset.
	// MinReplicas must be more than or equal to 0 if it is set.
	// Controller will prioritize satisfy minReplicas for each subset
	// according to the order of Topology.Subsets.
	// Defaults to 0.
	// +optional
	MinReplicas *intstr.IntOrString `json:"minReplicas,omitempty"`

	// Indicates the upper bounded replicas of the subset.
	// MaxReplicas must be more than or equal to MinReplicas.
	// MaxReplicas == nil means no limitation.
	// Please ensure that at least one subset has empty MaxReplicas(no limitation) to avoid stuck scaling.
	// Defaults to nil.
	// +optional
	MaxReplicas *intstr.IntOrString `json:"maxReplicas,omitempty"`

	// Patch indicates patching to the templateSpec.
	// Patch takes precedence over other fields
	// If the Patch also modifies the Replicas, NodeSelectorTerm or Tolerations, use value in the Patch
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Patch runtime.RawExtension `json:"patch,omitempty"`
}

// UnitedDeploymentScheduleStrategyType is a string enumeration type that enumerates
// all possible schedule strategies for the UnitedDeployment controller.
// +kubebuilder:validation:Enum=Adaptive;Fixed;""
type UnitedDeploymentScheduleStrategyType string

const (
	// AdaptiveUnitedDeploymentScheduleStrategyType represents that when a pod is stuck in the pending status and cannot
	// be scheduled, allow it to be rescheduled to another subset.
	AdaptiveUnitedDeploymentScheduleStrategyType UnitedDeploymentScheduleStrategyType = "Adaptive"
	// FixedUnitedDeploymentScheduleStrategyType represents that pods are strictly scheduled to the selected subset
	// even if scheduling fail.
	FixedUnitedDeploymentScheduleStrategyType UnitedDeploymentScheduleStrategyType = "Fixed"
)

const (
	DefaultRescheduleCriticalDuration      = 30 * time.Second
	DefaultUnschedulableStatusLastDuration = 300 * time.Second
)

// AdaptiveUnitedDeploymentStrategy is used to communicate parameters when Type is AdaptiveUnitedDeploymentScheduleStrategyType.
type AdaptiveUnitedDeploymentStrategy struct {
	// RescheduleCriticalSeconds indicates how long controller will reschedule a schedule failed Pod to the subset that has
	// redundant capacity after the subset where the Pod lives. If a Pod was scheduled failed and still in an unschedulabe status
	// over RescheduleCriticalSeconds duration, the controller will reschedule it to a suitable subset. Default is 30 seconds.
	// +optional
	RescheduleCriticalSeconds *int32 `json:"rescheduleCriticalSeconds,omitempty"`

	// UnschedulableLastSeconds is used to set the number of seconds for a Subset to recover from an unschedulable state,
	// with a default value of 300 seconds.
	// +optional
	UnschedulableLastSeconds *int32 `json:"unschedulableLastSeconds,omitempty"`
}

// UnitedDeploymentScheduleStrategy defines the schedule performance of UnitedDeployment.
type UnitedDeploymentScheduleStrategy struct {
	// Type indicates the type of the UnitedDeploymentScheduleStrategy.
	// Default is Fixed
	// +optional
	Type UnitedDeploymentScheduleStrategyType `json:"type,omitempty"`

	// Adaptive is used to communicate parameters when Type is AdaptiveUnitedDeploymentScheduleStrategyType.
	// +optional
	Adaptive *AdaptiveUnitedDeploymentStrategy `json:"adaptive,omitempty"`
}

func (s *UnitedDeploymentScheduleStrategy) IsAdaptive() bool {
	return s.Type == AdaptiveUnitedDeploymentScheduleStrategyType
}

func (s *UnitedDeploymentScheduleStrategy) GetRescheduleCriticalDuration() time.Duration {
	if s.Adaptive == nil || s.Adaptive.RescheduleCriticalSeconds == nil {
		return DefaultRescheduleCriticalDuration
	}
	return time.Duration(*s.Adaptive.RescheduleCriticalSeconds) * time.Second
}

func (s *UnitedDeploymentScheduleStrategy) GetUnschedulableLastDuration() time.Duration {
	if s.Adaptive == nil || s.Adaptive.UnschedulableLastSeconds == nil {
		return DefaultUnschedulableStatusLastDuration
	}
	return time.Duration(*s.Adaptive.UnschedulableLastSeconds) * time.Second
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

	// Record the conditions of each subset.
	SubsetStatuses []UnitedDeploymentSubsetStatus `json:"subsetStatuses,omitempty"`
	// Represents the latest available observations of a UnitedDeployment's current state.
	// +optional
	Conditions []UnitedDeploymentCondition `json:"conditions,omitempty"`

	// Records the information of update progress.
	// +optional
	UpdateStatus *UpdateStatus `json:"updateStatus,omitempty"`

	// LabelSelector is label selectors for query over pods that should match the replica count used by HPA.
	LabelSelector string `json:"labelSelector,omitempty"`
}

func (s *UnitedDeploymentStatus) GetSubsetStatus(subset string) *UnitedDeploymentSubsetStatus {
	for i, subsetStatus := range s.SubsetStatuses {
		if subsetStatus.Name == subset {
			return &s.SubsetStatuses[i]
		}
	}
	return nil
}

func (u *UnitedDeployment) InitSubsetStatuses() {
	for _, subset := range u.Spec.Topology.Subsets {
		if u.Status.GetSubsetStatus(subset.Name) == nil {
			u.Status.SubsetStatuses = append(u.Status.SubsetStatuses, UnitedDeploymentSubsetStatus{Name: subset.Name})
		}
	}
}

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

	// A human-readable message indicating details about the transition.
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

type UnitedDeploymentSubsetStatus struct {
	// Subset name specified in Topology.Subsets
	Name string `json:"name,omitempty"`
	// Recores the current replicas. Currently unused.
	Replicas int32 `json:"replicas,omitempty"`
	// Records the current partition. Currently unused.
	Partition int32 `json:"partition,omitempty"`
	// Conditions is an array of current observed subset conditions.
	Conditions []UnitedDeploymentSubsetCondition `json:"conditions,omitempty"`
}

func (s *UnitedDeploymentSubsetStatus) GetCondition(condType UnitedDeploymentSubsetConditionType) *UnitedDeploymentSubsetCondition {
	for _, condition := range s.Conditions {
		if condition.Type == condType {
			return &condition
		}
	}
	return nil
}

func (s *UnitedDeploymentSubsetStatus) SetCondition(condType UnitedDeploymentSubsetConditionType, status corev1.ConditionStatus, reason, message string) {
	var currentCond *UnitedDeploymentSubsetCondition
	for i, c := range s.Conditions {
		if c.Type == condType {
			currentCond = &s.Conditions[i]
			break
		}
	}
	if currentCond != nil && currentCond.Status == status && currentCond.Reason == reason {
		return
	}
	if currentCond == nil {
		s.Conditions = append(s.Conditions, UnitedDeploymentSubsetCondition{Type: condType})
		currentCond = &s.Conditions[len(s.Conditions)-1]
	}
	currentCond.LastTransitionTime = metav1.Now()
	currentCond.Status = status
	currentCond.Reason = reason
	currentCond.Message = message
}

type UnitedDeploymentSubsetConditionType string

const (
	// UnitedDeploymentSubsetSchedulable means new pods allocated into the subset will keep pending.
	UnitedDeploymentSubsetSchedulable UnitedDeploymentSubsetConditionType = "Schedulable"
)

type UnitedDeploymentSubsetCondition struct {
	Type               UnitedDeploymentSubsetConditionType `json:"type"`
	Status             corev1.ConditionStatus              `json:"status"`
	LastTransitionTime metav1.Time                         `json:"lastTransitionTime,omitempty"`
	Reason             string                              `json:"reason,omitempty"`
	Message            string                              `json:"message,omitempty"`
}

// +genclient
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.labelSelector
// +kubebuilder:resource:shortName=ud
// +kubebuilder:printcolumn:name="DESIRED",type="integer",JSONPath=".spec.replicas",description="The desired number of pods."
// +kubebuilder:printcolumn:name="CURRENT",type="integer",JSONPath=".status.replicas",description="The number of currently all pods."
// +kubebuilder:printcolumn:name="UPDATED",type="integer",JSONPath=".status.updatedReplicas",description="The number of pods updated."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyReplicas",description="The number of pods ready."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// UnitedDeployment is the Schema for the uniteddeployments API
type UnitedDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UnitedDeploymentSpec   `json:"spec,omitempty"`
	Status UnitedDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UnitedDeploymentList contains a list of UnitedDeployment
type UnitedDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UnitedDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UnitedDeployment{}, &UnitedDeploymentList{})
}
