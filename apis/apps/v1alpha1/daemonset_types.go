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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
)

// DaemonSetUpdateStrategy is a struct used to control the update strategy for a DaemonSet.
type DaemonSetUpdateStrategy struct {
	// Type of daemon set update. Can be "RollingUpdate" or "OnDelete". Default is RollingUpdate.
	// +optional
	Type DaemonSetUpdateStrategyType `json:"type,omitempty"`

	// Rolling update config params. Present only if type = "RollingUpdate".
	// +optional
	RollingUpdate *RollingUpdateDaemonSet `json:"rollingUpdate,omitempty"`
}

type DaemonSetUpdateStrategyType string
type RollingUpdateType string

const (
	// Replace the old daemons by new ones using rolling update i.e replace them on each node one after the other.
	RollingUpdateDaemonSetStrategyType DaemonSetUpdateStrategyType = "RollingUpdate"

	// Replace the old daemons only when it's killed
	OnDeleteDaemonSetStrategyType DaemonSetUpdateStrategyType = "OnDelete"

	// StandardRollingUpdateType is the Standard way that update pods with recreation that sames to the upstream DaemonSet.
	StandardRollingUpdateType RollingUpdateType = "Standard"

	// InplaceRollingUpdateType update container image without killing the pod if possible.
	InplaceRollingUpdateType RollingUpdateType = "InPlaceIfPossible"

	// DeprecatedSurgingRollingUpdateType is a depreciated alias for Standard.
	// Deprecated: Just use Standard instead.
	DeprecatedSurgingRollingUpdateType RollingUpdateType = "Surging"
)

// Spec to control the desired behavior of daemon set rolling update.
type RollingUpdateDaemonSet struct {
	// Type is to specify which kind of rollingUpdate.
	Type RollingUpdateType `json:"rollingUpdateType,omitempty"`

	// The maximum number of DaemonSet pods that can be unavailable during the
	// update. Value can be an absolute number (ex: 5) or a percentage of total
	// number of DaemonSet pods at the start of the update (ex: 10%). Absolute
	// number is calculated from percentage by rounding up.
	// This cannot be 0 if MaxSurge is 0
	// Default value is 1.
	// Example: when this is set to 30%, at most 30% of the total number of nodes
	// that should be running the daemon pod (i.e. status.desiredNumberScheduled)
	// can have their pods stopped for an update at any given time. The update
	// starts by stopping at most 30% of those DaemonSet pods and then brings
	// up new DaemonSet pods in their place. Once the new pods are available,
	// it then proceeds onto other DaemonSet pods, thus ensuring that at least
	// 70% of original number of DaemonSet pods are available at all times during
	// the update.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// The maximum number of nodes with an existing available DaemonSet pod that
	// can have an updated DaemonSet pod during during an update.
	// Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
	// This can not be 0 if MaxUnavailable is 0.
	// Absolute number is calculated from percentage by rounding up to a minimum of 1.
	// Default value is 0.
	// Example: when this is set to 30%, at most 30% of the total number of nodes
	// that should be running the daemon pod (i.e. status.desiredNumberScheduled)
	// can have their a new pod created before the old pod is marked as deleted.
	// The update starts by launching new pods on 30% of nodes. Once an updated
	// pod is available (Ready for at least minReadySeconds) the old DaemonSet pod
	// on that node is marked deleted. If the old pod becomes unavailable for any
	// reason (Ready transitions to false, is evicted, or is drained) an updated
	// pod is immediately created on that node without considering surge limits.
	// Allowing surge implies the possibility that the resources consumed by the
	// daemonset on any given node can double if the readiness check fails, and
	// so resource intensive daemonsets should take into account that they may
	// cause evictions during disruption.
	// This is beta field and enabled/disabled by DaemonSetUpdateSurge feature gate.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`

	// A label query over nodes that are managed by the daemon set RollingUpdate.
	// Must match in order to be controlled.
	// It must match the node's labels.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// The number of DaemonSet pods remained to be old version.
	// Default value is 0.
	// Maximum value is status.DesiredNumberScheduled, which means no pod will be updated.
	// +optional
	Partition *int32 `json:"partition,omitempty"`

	// Indicates that the daemon set is paused and will not be processed by the
	// daemon set controller.
	// +optional
	Paused *bool `json:"paused,omitempty"`
}

// DaemonSetSpec defines the desired state of DaemonSet
type DaemonSetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// A label query over pods that are managed by the daemon set.
	// Must match in order to be controlled.
	// It must match the pod template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector *metav1.LabelSelector `json:"selector"`

	// An object that describes the pod that will be created.
	// The DaemonSet will create exactly one copy of this pod on every node
	// that matches the template's node selector (or on every node if no node
	// selector is specified).
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller#pod-template
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Template corev1.PodTemplateSpec `json:"template"`

	// An update strategy to replace existing DaemonSet pods with new pods.
	// +optional
	UpdateStrategy DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// The minimum number of seconds for which a newly created DaemonSet pod should
	// be ready without any of its container crashing, for it to be considered
	// available. Defaults to 0 (pod will be considered available as soon as it
	// is ready).
	// +optional
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// BurstReplicas is a rate limiter for booting pods on a lot of pods.
	// The default value is 250
	BurstReplicas *intstr.IntOrString `json:"burstReplicas,omitempty"`

	// The number of old history to retain to allow rollback.
	// This is a pointer to distinguish between explicit zero and not specified.
	// Defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// Lifecycle defines the lifecycle hooks for Pods pre-delete, in-place update.
	// Currently, we only support pre-delete hook for Advanced DaemonSet.
	// +optional
	Lifecycle *appspub.Lifecycle `json:"lifecycle,omitempty"`
}

// DaemonSetStatus defines the observed state of DaemonSet
type DaemonSetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The number of nodes that are running at least 1
	// daemon pod and are supposed to run the daemon pod.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
	CurrentNumberScheduled int32 `json:"currentNumberScheduled"`

	// The number of nodes that are running the daemon pod, but are
	// not supposed to run the daemon pod.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
	NumberMisscheduled int32 `json:"numberMisscheduled"`

	// The total number of nodes that should be running the daemon
	// pod (including nodes correctly running the daemon pod).
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
	DesiredNumberScheduled int32 `json:"desiredNumberScheduled"`

	// The number of nodes that should be running the daemon pod and have one
	// or more of the daemon pod running and ready.
	NumberReady int32 `json:"numberReady"`

	// The most recent generation observed by the daemon set controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// The total number of nodes that are running updated daemon pod
	UpdatedNumberScheduled int32 `json:"updatedNumberScheduled"`

	// The number of nodes that should be running the
	// daemon pod and have one or more of the daemon pod running and
	// available (ready for at least spec.minReadySeconds)
	// +optional
	NumberAvailable int32 `json:"numberAvailable,omitempty"`

	// The number of nodes that should be running the
	// daemon pod and have none of the daemon pod running and available
	// (ready for at least spec.minReadySeconds)
	// +optional
	NumberUnavailable int32 `json:"numberUnavailable,omitempty"`

	// Count of hash collisions for the DaemonSet. The DaemonSet controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// Represents the latest available observations of a DaemonSet's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []appsv1.DaemonSetCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// DaemonSetHash is the controller-revision-hash, which represents the latest version of the DaemonSet.
	DaemonSetHash string `json:"daemonSetHash"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=daemon;ads
// +kubebuilder:printcolumn:name="DESIRED",type="integer",JSONPath=".status.desiredNumberScheduled",description="The desired number of pods."
// +kubebuilder:printcolumn:name="CURRENT",type="integer",JSONPath=".status.currentNumberScheduled",description="The current number of pods."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.numberReady",description="The ready number of pods."
// +kubebuilder:printcolumn:name="UP-TO-DATE",type="integer",JSONPath=".status.updatedNumberScheduled",description="The updated number of pods."
// +kubebuilder:printcolumn:name="AVAILABLE",type="integer",JSONPath=".status.numberAvailable",description="The updated number of pods."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// +kubebuilder:printcolumn:name="CONTAINERS",type="string",priority=1,JSONPath=".spec.template.spec.containers[*].name",description="The containers of currently  daemonset."
// +kubebuilder:printcolumn:name="IMAGES",type="string",priority=1,JSONPath=".spec.template.spec.containers[*].image",description="The images of currently advanced daemonset."

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
