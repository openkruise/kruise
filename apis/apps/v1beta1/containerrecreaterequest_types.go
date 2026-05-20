/*
Copyright 2026 The Kruise Authors.

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

package v1beta1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// [Immutable] Pod UID of this ContainerRecreateRequest.
	ContainerRecreateRequestPodUIDKey = "crr.apps.kruise.io/pod-uid"
	// [Immutable] Node name of this ContainerRecreateRequest.
	ContainerRecreateRequestNodeNameKey = "crr.apps.kruise.io/node-name"

	// ContainerRecreateRequestActiveKey indicates if this ContainerRecreateRequest is active.
	// It will be removed in labels since a ContainerRecreateRequest has completed.
	ContainerRecreateRequestActiveKey = "crr.apps.kruise.io/active"

	// Deprecated: ContainerRecreateRequestSyncContainerStatusesKey is superseded by
	// status.containerStatusSnapshot in v1beta1. Kept for compile compatibility only.
	ContainerRecreateRequestSyncContainerStatusesKey = "crr.apps.kruise.io/sync-container-statuses"
	// Deprecated: ContainerRecreateRequestUnreadyAcquiredKey is superseded by
	// status.conditions[type=PodUnreadyAcquired] in v1beta1. Kept for compile compatibility only.
	ContainerRecreateRequestUnreadyAcquiredKey = "crr.apps.kruise.io/unready-acquired"

	// ContainerRecreateRequestPodUnreadyAcquiredType is the condition type written to
	// status.conditions when the controller has forced the Pod not-ready for the
	// unreadyGracePeriodSeconds drain. LastTransitionTime records the exact moment.
	ContainerRecreateRequestPodUnreadyAcquiredType = "PodUnreadyAcquired"
)

// ContainerRecreateRequestSpec defines the desired state of ContainerRecreateRequest
type ContainerRecreateRequestSpec struct {
	// PodName is name of the Pod that owns the recreated containers.
	PodName string `json:"podName"`
	// Containers contains the containers that need to recreate in the Pod.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Containers []ContainerRecreateRequestContainer `json:"containers" patchStrategy:"merge" patchMergeKey:"name"`
	// Strategy defines strategies for containers recreation.
	Strategy *ContainerRecreateRequestStrategy `json:"strategy,omitempty"`
	// ActiveDeadlineSeconds is the deadline duration of this ContainerRecreateRequest.
	// +kubebuilder:validation:Minimum=0
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
	// TTLSecondsAfterFinished is the TTL duration after this ContainerRecreateRequest has completed.
	// +kubebuilder:validation:Minimum=0
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`
}

// ContainerRecreateRequestContainer defines the container that need to recreate.
type ContainerRecreateRequestContainer struct {
	// Name of the container that need to recreate.
	Name string `json:"name"`
	// PreStop is synced from the real container in Pod spec during this ContainerRecreateRequest creating.
	// Populated by the system. Read-only.
	PreStop *CRRProbeHandler `json:"preStop,omitempty"`
	// Ports is synced from the real container in Pod spec during this ContainerRecreateRequest creating.
	// Populated by the system. Read-only.
	Ports []v1.ContainerPort `json:"ports,omitempty"`
	// StatusContext is synced from the real Pod status during this ContainerRecreateRequest creating.
	// Populated by the system. Read-only.
	StatusContext *ContainerRecreateRequestContainerContext `json:"statusContext,omitempty"`
}

// CRRProbeHandler defines a specific action that should be taken for pre-stop.
type CRRProbeHandler struct {
	// +optional
	Exec *v1.ExecAction `json:"exec,omitempty" protobuf:"bytes,1,opt,name=exec"`
	// +optional
	HTTPGet *v1.HTTPGetAction `json:"httpGet,omitempty" protobuf:"bytes,2,opt,name=httpGet"`
	// +optional
	TCPSocket *v1.TCPSocketAction `json:"tcpSocket,omitempty" protobuf:"bytes,3,opt,name=tcpSocket"`
}

// ContainerRecreateRequestContainerContext contains context status of the container that need to recreate.
type ContainerRecreateRequestContainerContext struct {
	// Container's ID in the format 'docker://<container_id>'.
	ContainerID string `json:"containerID"`
	// The number of times the container has been restarted.
	RestartCount int32 `json:"restartCount"`
}

// ContainerRecreateRequestStrategy contains the strategies for containers recreation.
type ContainerRecreateRequestStrategy struct {
	// FailurePolicy decides whether to continue if one container fails to recreate.
	// +kubebuilder:validation:Enum=Fail;Ignore
	FailurePolicy ContainerRecreateRequestFailurePolicyType `json:"failurePolicy,omitempty"`
	// OrderedRecreate indicates whether to recreate the next container only if the previous one has recreated completely.
	OrderedRecreate bool `json:"orderedRecreate,omitempty"`
	// ForceRecreate indicates whether to force kill the container even if the previous container is starting.
	ForceRecreate bool `json:"forceRecreate,omitempty"`
	// TerminationGracePeriodSeconds is the optional duration in seconds to wait the container terminating gracefully.
	// Value must be non-negative integer.
	// +kubebuilder:validation:Minimum=0
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`
	// UnreadyGracePeriodSeconds is the optional duration in seconds to mark Pod as not ready over this duration before
	// executing preStop hook and stopping the container.
	// +kubebuilder:validation:Minimum=0
	UnreadyGracePeriodSeconds *int64 `json:"unreadyGracePeriodSeconds,omitempty"`
	// Minimum number of seconds for which a newly created container should be started and ready
	// without any of its container crashing, for it to be considered Succeeded.
	// +kubebuilder:validation:Minimum=0
	MinStartedSeconds int32 `json:"minStartedSeconds,omitempty"`
}

// +kubebuilder:validation:Enum=Fail;Ignore
type ContainerRecreateRequestFailurePolicyType string

const (
	ContainerRecreateRequestFailurePolicyFail   ContainerRecreateRequestFailurePolicyType = "Fail"
	ContainerRecreateRequestFailurePolicyIgnore ContainerRecreateRequestFailurePolicyType = "Ignore"
)

// ContainerRecreateRequestStatus defines the observed state of ContainerRecreateRequest
type ContainerRecreateRequestStatus struct {
	// Phase of this ContainerRecreateRequest, e.g. Pending, Recreating, Completed
	// +kubebuilder:validation:Enum=Pending;Recreating;Succeeded;Failed;Completed
	Phase ContainerRecreateRequestPhase `json:"phase"`
	// Represents time when the ContainerRecreateRequest was completed.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// A human readable message indicating details about this ContainerRecreateRequest.
	Message string `json:"message,omitempty"`
	// ContainerRecreateStates contains the recreation states of the containers.
	ContainerRecreateStates []ContainerRecreateRequestContainerRecreateState `json:"containerRecreateStates,omitempty"`
	// ContainerStatusSnapshot contains the current container statuses of the Pod, synchronized by
	// the controller during the Recreating phase so the daemon can verify the new container came
	// up with the right ID and is Ready before marking the CRR Succeeded.
	// Replaces the crr.apps.kruise.io/sync-container-statuses annotation.
	ContainerStatusSnapshot []ContainerRecreateRequestSyncContainerStatus `json:"containerStatusSnapshot,omitempty"`
	// Conditions contains condition entries for this ContainerRecreateRequest.
	// The PodUnreadyAcquired condition is written by the controller when it marks the Pod
	// not-ready for the unreadyGracePeriodSeconds drain; LastTransitionTime records the exact moment.
	// Replaces the crr.apps.kruise.io/unready-acquired annotation.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:validation:Enum=Pending;Recreating;Succeeded;Failed;Completed
type ContainerRecreateRequestPhase string

const (
	ContainerRecreateRequestPending    ContainerRecreateRequestPhase = "Pending"
	ContainerRecreateRequestRecreating ContainerRecreateRequestPhase = "Recreating"
	ContainerRecreateRequestSucceeded  ContainerRecreateRequestPhase = "Succeeded"
	ContainerRecreateRequestFailed     ContainerRecreateRequestPhase = "Failed"
	ContainerRecreateRequestCompleted  ContainerRecreateRequestPhase = "Completed"
)

// ContainerRecreateRequestContainerRecreateState contains the recreation state of the container.
type ContainerRecreateRequestContainerRecreateState struct {
	// Name of the container.
	Name string `json:"name"`
	// Phase indicates the recreation phase of the container.
	// +kubebuilder:validation:Enum=Pending;Recreating;Succeeded;Failed;Completed
	Phase ContainerRecreateRequestPhase `json:"phase"`
	// A human readable message indicating details about this state.
	Message string `json:"message,omitempty"`
	// Containers are killed by kruise daemon
	IsKilled bool `json:"isKilled,omitempty"`
}

// ContainerRecreateRequestSyncContainerStatus holds the container status snapshot used to
// verify recreation success. Written to status.containerStatusSnapshot by the controller
// and read by the daemon during the Recreating phase.
type ContainerRecreateRequestSyncContainerStatus struct {
	Name string `json:"name"`
	// Specifies whether the container has passed its readiness probe.
	Ready bool `json:"ready"`
	// The number of times the container has been restarted.
	RestartCount int32 `json:"restartCount"`
	// Container's ID in the format 'docker://<container_id>'.
	ContainerID string `json:"containerID,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=crr
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase",description="Phase of this ContainerRecreateRequest."
// +kubebuilder:printcolumn:name="POD",type="string",JSONPath=".spec.podName",description="Pod name of this ContainerRecreateRequest."
// +kubebuilder:printcolumn:name="NODE",type="string",JSONPath=".metadata.labels.crr\\.apps\\.kruise\\.io/node-name",description="Node name of this ContainerRecreateRequest."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// ContainerRecreateRequest is the Schema for the containerrecreaterequests API
type ContainerRecreateRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContainerRecreateRequestSpec   `json:"spec,omitempty"`
	Status ContainerRecreateRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ContainerRecreateRequestList contains a list of ContainerRecreateRequest
type ContainerRecreateRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContainerRecreateRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ContainerRecreateRequest{}, &ContainerRecreateRequestList{})
}
