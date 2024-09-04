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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
)

const (
	// CloneSetInstanceID is a unique id for Pods and PVCs.
	// Each pod and the pvcs it owns have the same instance-id.
	CloneSetInstanceID = "apps.kruise.io/cloneset-instance-id"

	// DefaultCloneSetMaxUnavailable is the default value of maxUnavailable for CloneSet update strategy.
	DefaultCloneSetMaxUnavailable = "20%"

	// CloneSetScalingExcludePreparingDeleteKey is the label key that enables scalingExcludePreparingDelete
	// only for this CloneSet, which means it will calculate scale number excluding Pods in PreparingDelete state.
	CloneSetScalingExcludePreparingDeleteKey = "apps.kruise.io/cloneset-scaling-exclude-preparing-delete"
)

// CloneSetSpec defines the desired state of CloneSet
type CloneSetSpec struct {
	// Replicas is the desired number of replicas of the given Template.
	// These are replicas in the sense that they are instantiations of the
	// same Template.
	// If unspecified, defaults to 1.
	Replicas *int32 `json:"replicas,omitempty"`

	// Selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector *metav1.LabelSelector `json:"selector"`

	// Template describes the pods that will be created.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Template v1.PodTemplateSpec `json:"template"`

	// VolumeClaimTemplates is a list of claims that pods are allowed to reference.
	// Note that PVC will be deleted when its pod has been deleted.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	VolumeClaimTemplates []v1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`

	// ScaleStrategy indicates the ScaleStrategy that will be employed to
	// create and delete Pods in the CloneSet.
	ScaleStrategy CloneSetScaleStrategy `json:"scaleStrategy,omitempty"`

	// UpdateStrategy indicates the UpdateStrategy that will be employed to
	// update Pods in the CloneSet when a revision is made to Template.
	UpdateStrategy CloneSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// RevisionHistoryLimit is the maximum number of revisions that will
	// be maintained in the CloneSet's revision history. The revision history
	// consists of all revisions not represented by a currently applied
	// CloneSetSpec version. The default value is 10.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// Minimum number of seconds for which a newly created pod should be ready
	// without any of its container crashing, for it to be considered available.
	// Defaults to 0 (pod will be considered available as soon as it is ready)
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// Lifecycle defines the lifecycle hooks for Pods pre-available(pre-normal), pre-delete, in-place update.
	Lifecycle *appspub.Lifecycle `json:"lifecycle,omitempty"`
}

// CloneSetScaleStrategy defines strategies for pods scale.
type CloneSetScaleStrategy struct {
	// PodsToDelete is the names of Pod should be deleted.
	// Note that this list will be truncated for non-existing pod names.
	PodsToDelete []string `json:"podsToDelete,omitempty"`
	// The maximum number of pods that can be unavailable for scaled pods.
	// This field can control the changes rate of replicas for CloneSet so as to minimize the impact for users' service.
	// The scale will fail if the number of unavailable pods were greater than this MaxUnavailable at scaling up.
	// MaxUnavailable works only when scaling up.
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// Indicate if cloneSet will reuse already existed pvc to
	// rebuild a new pod
	DisablePVCReuse bool `json:"disablePVCReuse,omitempty"`
}

// CloneSetUpdateStrategy defines strategies for pods update.
type CloneSetUpdateStrategy struct {
	// Type indicates the type of the CloneSetUpdateStrategy.
	// Default is ReCreate.
	Type CloneSetUpdateStrategyType `json:"type,omitempty"`
	// Partition is the desired number of pods in old revisions.
	// Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
	// Absolute number is calculated from percentage by rounding up by default.
	// It means when partition is set during pods updating, (replicas - partition value) number of pods will be updated.
	// Default value is 0.
	Partition *intstr.IntOrString `json:"partition,omitempty"`
	// The maximum number of pods that can be unavailable during update or scale.
	// Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
	// Absolute number is calculated from percentage by rounding up by default.
	// When maxSurge > 0, absolute number is calculated from percentage by rounding down.
	// Defaults to 20%.
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	// The maximum number of pods that can be scheduled above the desired replicas during update or specified delete.
	// Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
	// Absolute number is calculated from percentage by rounding up.
	// Defaults to 0.
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
	// Paused indicates that the CloneSet is paused.
	// Default value is false
	Paused bool `json:"paused,omitempty"`
	// Priorities are the rules for calculating the priority of updating pods.
	// Each pod to be updated, will pass through these terms and get a sum of weights.
	PriorityStrategy *appspub.UpdatePriorityStrategy `json:"priorityStrategy,omitempty"`
	// ScatterStrategy defines the scatter rules to make pods been scattered when update.
	// This will avoid pods with the same key-value to be updated in one batch.
	// - Note that pods will be scattered after priority sort. So, although priority strategy and scatter strategy can be applied together, we suggest to use either one of them.
	// - If scatterStrategy is used, we suggest to just use one term. Otherwise, the update order can be hard to understand.
	ScatterStrategy UpdateScatterStrategy `json:"scatterStrategy,omitempty"`
	// InPlaceUpdateStrategy contains strategies for in-place update.
	InPlaceUpdateStrategy *appspub.InPlaceUpdateStrategy `json:"inPlaceUpdateStrategy,omitempty"`
}

// CloneSetUpdateStrategyType defines strategies for pods in-place update.
type CloneSetUpdateStrategyType string

const (
	// RecreateCloneSetUpdateStrategyType indicates that we always delete Pod and create new Pod
	// during Pod update, which is the default behavior.
	RecreateCloneSetUpdateStrategyType CloneSetUpdateStrategyType = "ReCreate"
	// InPlaceIfPossibleCloneSetUpdateStrategyType indicates that we try to in-place update Pod instead of
	// recreating Pod when possible. Currently, only image update of pod spec is allowed. Any other changes to the pod
	// spec will fall back to ReCreate CloneSetUpdateStrategyType where pod will be recreated.
	InPlaceIfPossibleCloneSetUpdateStrategyType CloneSetUpdateStrategyType = "InPlaceIfPossible"
	// InPlaceOnlyCloneSetUpdateStrategyType indicates that we will in-place update Pod instead of
	// recreating pod. Currently we only allow image update for pod spec. Any other changes to the pod spec will be
	// rejected by kube-apiserver
	InPlaceOnlyCloneSetUpdateStrategyType CloneSetUpdateStrategyType = "InPlaceOnly"
)

// CloneSetStatus defines the observed state of CloneSet
type CloneSetStatus struct {
	// ObservedGeneration is the most recent generation observed for this CloneSet. It corresponds to the
	// CloneSet's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Replicas is the number of Pods created by the CloneSet controller.
	Replicas int32 `json:"replicas"`

	// ReadyReplicas is the number of Pods created by the CloneSet controller that have a Ready Condition.
	ReadyReplicas int32 `json:"readyReplicas"`

	// AvailableReplicas is the number of Pods created by the CloneSet controller that have a Ready Condition for at least minReadySeconds.
	AvailableReplicas int32 `json:"availableReplicas"`

	// UpdatedReplicas is the number of Pods created by the CloneSet controller from the CloneSet version
	// indicated by updateRevision.
	UpdatedReplicas int32 `json:"updatedReplicas"`

	// UpdatedReadyReplicas is the number of Pods created by the CloneSet controller from the CloneSet version
	// indicated by updateRevision and have a Ready Condition.
	UpdatedReadyReplicas int32 `json:"updatedReadyReplicas"`

	// UpdatedAvailableReplicas is the number of Pods created by the CloneSet controller from the CloneSet version
	// indicated by updateRevision and have a Ready Condition for at least minReadySeconds.
	// Notice: when enable InPlaceWorkloadVerticalScaling, only resource resize updating pod will also be unavailable.
	// This means these pod will be counted in maxUnavailable.
	UpdatedAvailableReplicas int32 `json:"updatedAvailableReplicas,omitempty"`

	// ExpectedUpdatedReplicas is the number of Pods that should be updated by CloneSet controller.
	// This field is calculated via Replicas - Partition.
	ExpectedUpdatedReplicas int32 `json:"expectedUpdatedReplicas,omitempty"`

	// UpdateRevision, if not empty, indicates the latest revision of the CloneSet.
	UpdateRevision string `json:"updateRevision,omitempty"`

	// currentRevision, if not empty, indicates the current revision version of the CloneSet.
	CurrentRevision string `json:"currentRevision,omitempty"`

	// CollisionCount is the count of hash collisions for the CloneSet. The CloneSet controller
	// uses this field as a collision avoidance mechanism when it needs to create the name for the
	// newest ControllerRevision.
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// Conditions represents the latest available observations of a CloneSet's current state.
	Conditions []CloneSetCondition `json:"conditions,omitempty"`

	// LabelSelector is label selectors for query over pods that should match the replica count used by HPA.
	LabelSelector string `json:"labelSelector,omitempty"`
}

// CloneSetConditionType is type for CloneSet conditions.
type CloneSetConditionType string

const (
	// CloneSetConditionFailedScale indicates cloneset controller failed to create or delete pods/pvc.
	CloneSetConditionFailedScale CloneSetConditionType = "FailedScale"
	// CloneSetConditionFailedUpdate indicates cloneset controller failed to update pods.
	CloneSetConditionFailedUpdate CloneSetConditionType = "FailedUpdate"
)

// CloneSetCondition describes the state of a CloneSet at a certain point.
type CloneSetCondition struct {
	// Type of CloneSet condition.
	Type CloneSetConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// +genclient
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.labelSelector
// +kubebuilder:resource:shortName=clone
// +kubebuilder:printcolumn:name="DESIRED",type="integer",JSONPath=".spec.replicas",description="The desired number of pods."
// +kubebuilder:printcolumn:name="UPDATED",type="integer",JSONPath=".status.updatedReplicas",description="The number of pods updated."
// +kubebuilder:printcolumn:name="UPDATED_READY",type="integer",JSONPath=".status.updatedReadyReplicas",description="The number of pods updated and ready."
// +kubebuilder:printcolumn:name="UPDATED_AVAILABLE",type="integer",JSONPath=".status.updatedAvailableReplicas",description="The number of pods updated and available."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyReplicas",description="The number of pods ready."
// +kubebuilder:printcolumn:name="TOTAL",type="integer",JSONPath=".status.replicas",description="The number of currently all pods."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// +kubebuilder:printcolumn:name="CONTAINERS",type="string",priority=1,JSONPath=".spec.template.spec.containers[*].name",description="The containers of currently cloneset."
// +kubebuilder:printcolumn:name="IMAGES",type="string",priority=1,JSONPath=".spec.template.spec.containers[*].image",description="The images of currently cloneset."
// +kubebuilder:printcolumn:name="SELECTOR",type="string",priority=1,JSONPath=".status.labelSelector",description="The selector of currently cloneset."

// CloneSet is the Schema for the clonesets API
type CloneSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloneSetSpec   `json:"spec,omitempty"`
	Status CloneSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloneSetList contains a list of CloneSet
type CloneSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloneSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloneSet{}, &CloneSetList{})
}
