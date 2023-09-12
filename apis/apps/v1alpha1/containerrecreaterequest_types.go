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
	// We use it mainly to make kruise-daemon do not list watch those ContainerRecreateRequests that have completed.
	ContainerRecreateRequestActiveKey = "crr.apps.kruise.io/active"
	// ContainerRecreateRequestSyncContainerStatusesKey contains the container statuses in current Pod.
	// It is only synchronized during a ContainerRecreateRequest in Recreating phase.
	ContainerRecreateRequestSyncContainerStatusesKey = "crr.apps.kruise.io/sync-container-statuses"
	// ContainerRecreateRequestUnreadyAcquiredKey indicates the Pod has been forced to not-ready.
	// It is required if the unreadyGracePeriodSeconds is set in ContainerRecreateRequests.
	ContainerRecreateRequestUnreadyAcquiredKey = "crr.apps.kruise.io/unready-acquired"
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
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
	// TTLSecondsAfterFinished is the TTL duration after this ContainerRecreateRequest has completed.
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`
}

// ContainerRecreateRequestContainer defines the container that need to recreate.
type ContainerRecreateRequestContainer struct {
	// Name of the container that need to recreate.
	// It must be existing in the real pod.Spec.Containers.
	Name string `json:"name"`
	// PreStop is synced from the real container in Pod spec during this ContainerRecreateRequest creating.
	// Populated by the system.
	// Read-only.
	PreStop *ProbeHandler `json:"preStop,omitempty"`
	// Ports is synced from the real container in Pod spec during this ContainerRecreateRequest creating.
	// Populated by the system.
	// Read-only.
	Ports []v1.ContainerPort `json:"ports,omitempty"`
	// StatusContext is synced from the real Pod status during this ContainerRecreateRequest creating.
	// Populated by the system.
	// Read-only.
	StatusContext *ContainerRecreateRequestContainerContext `json:"statusContext,omitempty"`
}

// ProbeHandler defines a specific action that should be taken
// TODO(FillZpp): improve the definition when openkruise/kruise updates to k8s 1.23
type ProbeHandler struct {
	// One and only one of the following should be specified.
	// Exec specifies the action to take.
	// +optional
	Exec *v1.ExecAction `json:"exec,omitempty" protobuf:"bytes,1,opt,name=exec"`
	// HTTPGet specifies the http request to perform.
	// +optional
	HTTPGet *v1.HTTPGetAction `json:"httpGet,omitempty" protobuf:"bytes,2,opt,name=httpGet"`
	// TCPSocket specifies an action involving a TCP port.
	// TCP hooks not yet supported
	// TODO: implement a realistic TCP lifecycle hook
	// +optional
	TCPSocket *v1.TCPSocketAction `json:"tcpSocket,omitempty" protobuf:"bytes,3,opt,name=tcpSocket"`
}

// ContainerRecreateRequestContainerContext contains context status of the container that need to recreate.
type ContainerRecreateRequestContainerContext struct {
	// Container's ID in the format 'docker://<container_id>'.
	ContainerID string `json:"containerID"`
	// The number of times the container has been restarted, currently based on
	// the number of dead containers that have not yet been removed.
	// Note that this is calculated from dead containers. But those containers are subject to
	// garbage collection. This value will get capped at 5 by GC.
	RestartCount int32 `json:"restartCount"`
}

// ContainerRecreateRequestStrategy contains the strategies for containers recreation.
type ContainerRecreateRequestStrategy struct {
	// FailurePolicy decides whether to continue if one container fails to recreate
	FailurePolicy ContainerRecreateRequestFailurePolicyType `json:"failurePolicy,omitempty"`
	// OrderedRecreate indicates whether to recreate the next container only if the previous one has recreated completely.
	OrderedRecreate bool `json:"orderedRecreate,omitempty"`
	// ForceRecreate indicates whether to force kill the container even if the previous container is starting.
	ForceRecreate bool `json:"forceRecreate,omitempty"`
	// TerminationGracePeriodSeconds is the optional duration in seconds to wait the container terminating gracefully.
	// Value must be non-negative integer. The value zero indicates delete immediately.
	// If this value is nil, we will use pod.Spec.TerminationGracePeriodSeconds as default value.
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`
	// UnreadyGracePeriodSeconds is the optional duration in seconds to mark Pod as not ready over this duration before
	// executing preStop hook and stopping the container.
	UnreadyGracePeriodSeconds *int64 `json:"unreadyGracePeriodSeconds,omitempty"`
	// Minimum number of seconds for which a newly created container should be started and ready
	// without any of its container crashing, for it to be considered Succeeded.
	// Defaults to 0 (container will be considered Succeeded as soon as it is started and ready)
	MinStartedSeconds int32 `json:"minStartedSeconds,omitempty"`
}

type ContainerRecreateRequestFailurePolicyType string

const (
	ContainerRecreateRequestFailurePolicyFail   ContainerRecreateRequestFailurePolicyType = "Fail"
	ContainerRecreateRequestFailurePolicyIgnore ContainerRecreateRequestFailurePolicyType = "Ignore"
)

// ContainerRecreateRequestStatus defines the observed state of ContainerRecreateRequest
type ContainerRecreateRequestStatus struct {
	// Phase of this ContainerRecreateRequest, e.g. Pending, Recreating, Completed
	Phase ContainerRecreateRequestPhase `json:"phase"`
	// Represents time when the ContainerRecreateRequest was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// A human readable message indicating details about this ContainerRecreateRequest.
	Message string `json:"message,omitempty"`
	// ContainerRecreateStates contains the recreation states of the containers.
	ContainerRecreateStates []ContainerRecreateRequestContainerRecreateState `json:"containerRecreateStates,omitempty"`
}

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
	Phase ContainerRecreateRequestPhase `json:"phase"`
	// A human readable message indicating details about this state.
	Message string `json:"message,omitempty"`
	// Containers are killed by kruise daemon
	IsKilled bool `json:"isKilled,omitempty"`
}

// ContainerRecreateRequestSyncContainerStatus only uses in the annotation `crr.apps.kruise.io/sync-container-statuses`.
type ContainerRecreateRequestSyncContainerStatus struct {
	Name string `json:"name"`
	// Specifies whether the container has passed its readiness probe.
	Ready bool `json:"ready"`
	// The number of times the container has been restarted, currently based on
	// the number of dead containers that have not yet been removed.
	RestartCount int32 `json:"restartCount"`
	// Container's ID in the format 'docker://<container_id>'.
	ContainerID string `json:"containerID,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=crr
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase",description="Phase of this ContainerRecreateRequest."
// +kubebuilder:printcolumn:name="POD",type="string",JSONPath=".spec.podName",description="Pod name of this ContainerRecreateRequest."
// +kubebuilder:printcolumn:name="NODE",type="string",JSONPath=".metadata.labels.crr\\.apps\\.kruise\\.io/node-name",description="Pod name of this ContainerRecreateRequest."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

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
