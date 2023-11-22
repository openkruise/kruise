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
)

// NodeImageSpec defines the desired state of NodeImage
type NodeImageSpec struct {
	// Specifies images to be pulled on this node
	// It can not be more than 256 for each NodeImage
	Images map[string]ImageSpec `json:"images,omitempty"`
}

// ImageSpec defines the pulling spec of an image
type ImageSpec struct {
	// PullSecrets is an optional list of references to secrets in the same namespace to use for pulling the image.
	// If specified, these secrets will be passed to individual puller implementations for them to use.  For example,
	// in the case of docker, only DockerConfig type secrets are honored.
	// +optional
	PullSecrets []ReferenceObject `json:"pullSecrets,omitempty"`

	// Tags is a list of versions of this image
	Tags []ImageTagSpec `json:"tags"`

	// SandboxConfig support attach metadata in PullImage CRI interface during ImagePulljobs
	// +optional
	SandboxConfig *SandboxConfig `json:"sandboxConfig,omitempty"`
}

// ReferenceObject comprises a resource name, with a mandatory namespace,
// rendered as "<namespace>/<name>".
type ReferenceObject struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// ImageTagSpec defines the pulling spec of an image tag
type ImageTagSpec struct {
	// Specifies the image tag
	Tag string `json:"tag"`

	// Specifies the create time of this tag
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// PullPolicy is an optional field to set parameters of the pulling task. If not specified,
	// the system will use the default values.
	// +optional
	PullPolicy *ImageTagPullPolicy `json:"pullPolicy,omitempty"`

	// List of objects depended by this object. If this image is managed by a controller,
	// then an entry in this list will point to this controller.
	// +optional
	OwnerReferences []v1.ObjectReference `json:"ownerReferences,omitempty"`

	// An opaque value that represents the internal version of this tag that can
	// be used by clients to determine when objects have changed. May be used for optimistic
	// concurrency, change detection, and the watch operation on a resource or set of resources.
	// Clients must treat these values as opaque and passed unmodified back to the server.
	//
	// Populated by the system.
	// Read-only.
	// Value must be treated as opaque by clients and .
	// +optional
	Version int64 `json:"version,omitempty"`

	// Image pull policy.
	// One of Always, IfNotPresent. Defaults to IfNotPresent.
	// +optional
	ImagePullPolicy ImagePullPolicy `json:"imagePullPolicy,omitempty"`
}

// ImageTagPullPolicy defines the policy of the pulling task
type ImageTagPullPolicy struct {
	// Specifies the timeout of the pulling task.
	// Defaults to 600
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// Specifies the number of retries before marking the pulling task failed.
	// Defaults to 3
	// +optional
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`

	// TTLSecondsAfterFinished limits the lifetime of a pulling task that has finished execution (either Complete or Failed).
	// If this field is set, ttlSecondsAfterFinished after the task finishes, it is eligible to be automatically deleted.
	// If this field is unset, the task won't be automatically deleted.
	// If this field is set to zero, the task becomes eligible to be deleted immediately after it finishes.
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// ActiveDeadlineSeconds specifies the duration in seconds relative to the startTime that the task may be active
	// before the system tries to terminate it; value must be positive integer.
	// if not specified, the system will never terminate it.
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
}

// NodeImageStatus defines the observed state of NodeImage
type NodeImageStatus struct {
	// The desired number of pulling tasks, this is typically equal to the number of images in spec.
	Desired int32 `json:"desired"`

	// The number of pulling tasks which reached phase Succeeded.
	// +optional
	Succeeded int32 `json:"succeeded"`

	// The number of pulling tasks  which reached phase Failed.
	// +optional
	Failed int32 `json:"failed"`

	// The number of pulling tasks which are not finished.
	// +optional
	Pulling int32 `json:"pulling"`

	// all statuses of active image pulling tasks
	ImageStatuses map[string]ImageStatus `json:"imageStatuses,omitempty"`

	// The first of all job has finished on this node. When a node is added to the cluster, we want to know
	// the time when the node's image pulling is completed, and use it to trigger the operation of the upper system.
	// +optional
	FirstSyncStatus *SyncStatus `json:"firstSyncStatus,omitempty"`
}

// ImageStatus defines the pulling status of an image
type ImageStatus struct {
	// Represents statuses of pulling tasks on this node
	Tags []ImageTagStatus `json:"tags"`
}

// ImageTagStatus defines the pulling status of an image tag
type ImageTagStatus struct {
	// Represents the image tag.
	Tag string `json:"tag"`

	// Represents the image pulling task phase.
	Phase ImagePullPhase `json:"phase"`

	// Represents the pulling progress of this tag, which is between 0-100. There is no guarantee
	// of monotonic consistency, and it may be a rollback due to retry during pulling.
	Progress int32 `json:"progress,omitempty"`

	// Represents time when the pulling task was acknowledged by the image puller.
	// It is not guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Represents time when the pulling task was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Represents the internal version of this tag that the daemon handled.
	// +optional
	Version int64 `json:"version,omitempty"`

	// Represents the ID of this image.
	// +optional
	ImageID string `json:"imageID,omitempty"`

	// Represents the summary information of this node
	// +optional
	Message string `json:"message,omitempty"`
}

// ImagePullPhase defines the tasks status
type ImagePullPhase string

const (
	// ImagePhaseWaiting means the task has not started
	ImagePhaseWaiting ImagePullPhase = "Waiting"
	// ImagePhasePulling means the task has been started, but not completed
	ImagePhasePulling ImagePullPhase = "Pulling"
	// ImagePhaseSucceeded means the task has been completed
	ImagePhaseSucceeded ImagePullPhase = "Succeeded"
	// ImagePhaseFailed means the task has failed
	ImagePhaseFailed ImagePullPhase = "Failed"
)

// SyncStatus is summary of the status of all images pulling tasks on the node.
type SyncStatus struct {
	SyncAt  metav1.Time     `json:"syncAt,omitempty"`
	Status  SyncStatusPhase `json:"status,omitempty"`
	Message string          `json:"message,omitempty"`
}

// SyncStatusPhase defines the node status
type SyncStatusPhase string

const (
	// SyncStatusSucceeded means all tasks has succeeded on this node
	SyncStatusSucceeded SyncStatusPhase = "Succeeded"
	// SyncStatusFailed means some task has failed on this node
	SyncStatusFailed SyncStatusPhase = "Failed"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="DESIRED",type="integer",JSONPath=".status.desired",description="Number of all images on this node"
// +kubebuilder:printcolumn:name="PULLING",type="integer",JSONPath=".status.pulling",description="Number of image pull task active"
// +kubebuilder:printcolumn:name="SUCCEED",type="integer",JSONPath=".status.succeeded",description="Number of image pull task succeeded"
// +kubebuilder:printcolumn:name="FAILED",type="integer",JSONPath=".status.failed",description="Number of image pull tasks failed"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// NodeImage is the Schema for the nodeimages API
type NodeImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeImageSpec   `json:"spec,omitempty"`
	Status NodeImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeImageList contains a list of NodeImage
type NodeImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeImage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeImage{}, &NodeImageList{})
}
