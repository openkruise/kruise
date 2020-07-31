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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DaemonSetUpdateStrategy is a struct used to control the update strategy for a DaemonSet.
type DaemonSetUpdateStrategy struct {
	// Type of daemon set update. Can be "RollingUpdate" or "OnDelete". Default is RollingUpdate.
	// +optional
	Type DaemonSetUpdateStrategyType `json:"type,omitempty" protobuf:"bytes,1,opt,name=type"`

	// Rolling update config params. Present only if type = "RollingUpdate".
	// +optional
	RollingUpdate *RollingUpdateDaemonSet `json:"rollingUpdate,omitempty" protobuf:"bytes,2,opt,name=rollingUpdate"`
}

type DaemonSetUpdateStrategyType string
type RollingUpdateType string

const (
	// Replace the old daemons by new ones using rolling update i.e replace them on each node one after the other.
	RollingUpdateDaemonSetStrategyType DaemonSetUpdateStrategyType = "RollingUpdate"

	// Replace the old daemons only when it's killed
	OnDeleteDaemonSetStrategyType DaemonSetUpdateStrategyType = "OnDelete"

	// StandardRollingUpdateType replace the old daemons by new ones using rolling update i.e replace them on each node one after the other.
	// this is the default type for RollingUpdate.
	StandardRollingUpdateType RollingUpdateType = "Standard"

	// Replace container image without killing the pod.
	InplaceRollingUpdateType RollingUpdateType = "Inplace"

	// SurgingRollingUpdateType replaces the old daemons by new ones using rolling update i.e replace them on each node one
	// after the other, creating the new pod and then killing the old one.
	SurgingRollingUpdateType RollingUpdateType = "Surging"
)

// Spec to control the desired behavior of daemon set rolling update.
type RollingUpdateDaemonSet struct {
	// Type is to specify which kind of rollingUpdate.
	Type RollingUpdateType `json:"rollingUpdateType,omitempty" protobuf:"bytes,1,opt,name=rollingUpdateType"`

	// The maximum number of DaemonSet pods that can be unavailable during the
	// update. Value can be an absolute number (ex: 5) or a percentage of total
	// number of DaemonSet pods at the start of the update (ex: 10%). Absolute
	// number is calculated from percentage by rounding up.
	// This cannot be 0.
	// Default value is 1.
	// Example: when this is set to 30%, at most 30% of the total number of nodes
	// that should be running the daemon pod (i.e. status.desiredNumberScheduled)
	// can have their pods stopped for an update at any given
	// time. The update starts by stopping at most 30% of those DaemonSet pods
	// and then brings up new DaemonSet pods in their place. Once the new pods
	// are available, it then proceeds onto other DaemonSet pods, thus ensuring
	// that at least 70% of original number of DaemonSet pods are available at
	// all times during the update.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty" protobuf:"bytes,2,opt,name=maxUnavailable"`

	// A label query over nodes that are managed by the daemon set RollingUpdate.
	// Must match in order to be controlled.
	// It must match the node's labels.
	Selector *metav1.LabelSelector `json:"selector,omitempty" protobuf:"bytes,3,opt,name=selector"`

	// The number of DaemonSet pods remained to be old version.
	// Default value is 0.
	// Maximum value is status.DesiredNumberScheduled, which means no pod will be updated.
	// +optional
	Partition *int32 `json:"partition,omitempty" protobuf:"varint,4,opt,name=partition"`

	// Indicates that the daemon set is paused and will not be processed by the
	// daemon set controller.
	// +optional
	Paused *bool `json:"paused,omitempty" protobuf:"varint,5,opt,name=paused"`

	// Only when type=SurgingRollingUpdateType, it works.
	// The maximum number of DaemonSet pods that can be scheduled above the desired number of pods
	// during the update. Value can be an absolute number (ex: 5) or a percentage of the total number
	// of DaemonSet pods at the start of the update (ex: 10%). The absolute number is calculated from
	// the percentage by rounding up. This cannot be 0. The default value is 1. Example: when this is
	// set to 30%, at most 30% of the total number of nodes that should be running the daemon pod
	// (i.e. status.desiredNumberScheduled) can have 2 pods running at any given time. The update
	// starts by starting replacements for at most 30% of those DaemonSet pods. Once the new pods are
	// available it then stops the existing pods before proceeding onto other DaemonSet pods, thus
	// ensuring that at most 130% of the desired final number of DaemonSet  pods are running at all
	// times during the update.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty" protobuf:"bytes,7,opt,name=maxSurge"`
}

// DaemonSetSpec defines the desired state of DaemonSet
type DaemonSetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// A label query over pods that are managed by the daemon set.
	// Must match in order to be controlled.
	// It must match the pod template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector *metav1.LabelSelector `json:"selector" protobuf:"bytes,1,opt,name=selector"`

	// An object that describes the pod that will be created.
	// The DaemonSet will create exactly one copy of this pod on every node
	// that matches the template's node selector (or on every node if no node
	// selector is specified).
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller#pod-template
	Template corev1.PodTemplateSpec `json:"template" protobuf:"bytes,2,opt,name=template"`

	// An update strategy to replace existing DaemonSet pods with new pods.
	// +optional
	UpdateStrategy DaemonSetUpdateStrategy `json:"updateStrategy,omitempty" protobuf:"bytes,3,opt,name=updateStrategy"`

	// The minimum number of seconds for which a newly created DaemonSet pod should
	// be ready without any of its container crashing, for it to be considered
	// available. Defaults to 0 (pod will be considered available as soon as it
	// is ready).
	// +optional
	MinReadySeconds int32 `json:"minReadySeconds,omitempty" protobuf:"varint,4,opt,name=minReadySeconds"`

	// BurstReplicas is a rate limiter for booting pods on a lot of pods.
	// The default value is 250
	BurstReplicas *intstr.IntOrString `json:"burstReplicas,omitempty" protobuf:"bytes,5,opt,name=burstReplicas"`

	// The number of old history to retain to allow rollback.
	// This is a pointer to distinguish between explicit zero and not specified.
	// Defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty" protobuf:"varint,6,opt,name=revisionHistoryLimit"`
}

// DaemonSetStatus defines the observed state of DaemonSet
type DaemonSetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The number of nodes that are running at least 1
	// daemon pod and are supposed to run the daemon pod.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
	CurrentNumberScheduled int32 `json:"currentNumberScheduled" protobuf:"varint,1,opt,name=currentNumberScheduled"`

	// The number of nodes that are running the daemon pod, but are
	// not supposed to run the daemon pod.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
	NumberMisscheduled int32 `json:"numberMisscheduled" protobuf:"varint,2,opt,name=numberMisscheduled"`

	// The total number of nodes that should be running the daemon
	// pod (including nodes correctly running the daemon pod).
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
	DesiredNumberScheduled int32 `json:"desiredNumberScheduled" protobuf:"varint,3,opt,name=desiredNumberScheduled"`

	// The number of nodes that should be running the daemon pod and have one
	// or more of the daemon pod running and ready.
	NumberReady int32 `json:"numberReady" protobuf:"varint,4,opt,name=numberReady"`

	// The most recent generation observed by the daemon set controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,5,opt,name=observedGeneration"`

	// The total number of nodes that are running updated daemon pod
	UpdatedNumberScheduled int32 `json:"updatedNumberScheduled" protobuf:"varint,6,opt,name=updatedNumberScheduled"`

	// The number of nodes that should be running the
	// daemon pod and have one or more of the daemon pod running and
	// available (ready for at least spec.minReadySeconds)
	// +optional
	NumberAvailable int32 `json:"numberAvailable,omitempty" protobuf:"varint,7,opt,name=numberAvailable"`

	// The number of nodes that should be running the
	// daemon pod and have none of the daemon pod running and available
	// (ready for at least spec.minReadySeconds)
	// +optional
	NumberUnavailable int32 `json:"numberUnavailable,omitempty" protobuf:"varint,8,opt,name=numberUnavailable"`

	// Count of hash collisions for the DaemonSet. The DaemonSet controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty" protobuf:"varint,9,opt,name=collisionCount"`

	// Represents the latest available observations of a DaemonSet's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []DaemonSetCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,10,rep,name=conditions"`

	// DaemonSetHash is the controller-revision-hash, which represents the latest version of the DaemonSet.
	DaemonSetHash string `json:"daemonSetHash" protobuf:"bytes,11,opt,name=daemonSetHash"`
}

type DaemonSetConditionType string

// TODO: Add valid condition types of a DaemonSet.

// DaemonSetCondition describes the state of a DaemonSet at a certain point.
type DaemonSetCondition struct {
	// Type of DaemonSet condition.
	Type DaemonSetConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=DaemonSetConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}

const (
	// DefaultDaemonSetUniqueLabelKey is the default label key that is added
	// to existing DaemonSet pods to distinguish between old and new
	// DaemonSet pods during DaemonSet template updates.
	DefaultDaemonSetUniqueLabelKey = ControllerRevisionHashLabelKey
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=daemon
// +kubebuilder:printcolumn:name="DesiredNumber",type="integer",JSONPath=".status.desiredNumberScheduled",description="The desired number of pods."
// +kubebuilder:printcolumn:name="CurrentNumber",type="integer",JSONPath=".status.currentNumberScheduled",description="The current number of pods."
// +kubebuilder:printcolumn:name="UpdatedNumberScheduled",type="integer",JSONPath=".status.updatedNumberScheduled",description="The updated number of pods."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// DaemonSet is the Schema for the daemonsets API
type DaemonSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DaemonSetSpec   `json:"spec,omitempty"`
	Status DaemonSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DaemonSetList contains a list of DaemonSet
type DaemonSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DaemonSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DaemonSet{}, &DaemonSetList{})
}
