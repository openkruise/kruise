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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ImagePreDownloadParallelismKey      = "apps.kruise.io/image-predownload-parallelism"
	ImagePreDownloadTimeoutSecondsKey   = "apps.kruise.io/image-predownload-timeout-seconds"
	ImagePreDownloadMinUpdatedReadyPods = "apps.kruise.io/image-predownload-min-updated-ready-pods"
)

// ImagePullPolicy describes a policy for if/when to pull a container image
// +enum
type ImagePullPolicy string

const (
	// PullAlways means that kruise-daemon always attempts to pull the latest image.
	PullAlways ImagePullPolicy = "Always"
	// PullIfNotPresent means that kruise-daemon pulls if the image isn't present on disk.
	PullIfNotPresent ImagePullPolicy = "IfNotPresent"
)

// ImagePullJobSpec defines the desired state of ImagePullJob
type ImagePullJobSpec struct {
	// Image is the image to be pulled by the job
	Image                string `json:"image"`
	ImagePullJobTemplate `json:",inline"`
}

type ImagePullJobTemplate struct {

	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling the image.
	// If specified, these secrets will be passed to individual puller implementations for them to use.  For example,
	// in the case of docker, only DockerConfig type secrets are honored.
	// +optional
	PullSecrets []string `json:"pullSecrets,omitempty"`

	// Selector is a query over nodes that should match the job.
	// nil to match all nodes.
	// +optional
	Selector *ImagePullJobNodeSelector `json:"selector,omitempty"`

	// PodSelector is a query over pods that should pull image on nodes of these pods.
	// Mutually exclusive with Selector.
	// +optional
	PodSelector *ImagePullJobPodSelector `json:"podSelector,omitempty"`

	// Parallelism is the requested parallelism, it can be set to any non-negative value. If it is unspecified,
	// it defaults to 1. If it is specified as 0, then the Job is effectively paused until it is increased.
	// +optional
	Parallelism *intstr.IntOrString `json:"parallelism,omitempty"`

	// PullPolicy is an optional field to set parameters of the pulling task. If not specified,
	// the system will use the default values.
	// +optional
	PullPolicy *PullPolicy `json:"pullPolicy,omitempty"`

	// CompletionPolicy indicates the completion policy of the job.
	// Default is Always CompletionPolicyType.
	CompletionPolicy CompletionPolicy `json:"completionPolicy"`

	// SandboxConfig support attach metadata in PullImage CRI interface during ImagePulljobs
	// +optional
	SandboxConfig *SandboxConfig `json:"sandboxConfig,omitempty"`

	// Image pull policy.
	// One of Always, IfNotPresent. Defaults to IfNotPresent.
	// +optional
	ImagePullPolicy ImagePullPolicy `json:"imagePullPolicy,omitempty"`
}

// ImagePullJobPodSelector is a selector over pods
type ImagePullJobPodSelector struct {
	// LabelSelector is a label query over pods that should match the job.
	// +optional
	metav1.LabelSelector `json:",inline"`
}

// ImagePullJobNodeSelector is a selector over nodes
type ImagePullJobNodeSelector struct {
	// Names specify a set of nodes to execute the job.
	// +optional
	Names []string `json:"names,omitempty"`

	// LabelSelector is a label query over nodes that should match the job.
	// +optional
	metav1.LabelSelector `json:",inline"`
}

// PullPolicy defines the policy of the pulling task
type PullPolicy struct {
	// Specifies the timeout of the pulling task.
	// Defaults to 600
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// Specifies the number of retries before marking the pulling task failed.
	// Defaults to 3
	// +optional
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`
}

// ImagePullJobStatus defines the observed state of ImagePullJob
type ImagePullJobStatus struct {
	// Represents time when the job was acknowledged by the job controller.
	// It is not guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Represents time when the job was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// The desired number of pulling tasks, this is typically equal to the number of nodes satisfied.
	Desired int32 `json:"desired"`

	// The number of actively running pulling tasks.
	// +optional
	Active int32 `json:"active"`

	// The number of pulling tasks which reached phase Succeeded.
	// +optional
	Succeeded int32 `json:"succeeded"`

	// The number of pulling tasks  which reached phase Failed.
	// +optional
	Failed int32 `json:"failed"`

	// The text prompt for job running status.
	// +optional
	Message string `json:"message,omitempty"`

	// The nodes that failed to pull the image.
	// +optional
	FailedNodes []string `json:"failedNodes,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TOTAL",type="integer",JSONPath=".status.desired",description="Number of all nodes matched by this job"
// +kubebuilder:printcolumn:name="ACTIVE",type="integer",JSONPath=".status.active",description="Number of image pull task active"
// +kubebuilder:printcolumn:name="SUCCEED",type="integer",JSONPath=".status.succeeded",description="Number of image pull task succeeded"
// +kubebuilder:printcolumn:name="FAILED",type="integer",JSONPath=".status.failed",description="Number of image pull tasks failed"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// +kubebuilder:printcolumn:name="MESSAGE",type="string",JSONPath=".status.message",description="Summary of status when job is failed"

// ImagePullJob is the Schema for the imagepulljobs API
type ImagePullJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImagePullJobSpec   `json:"spec,omitempty"`
	Status ImagePullJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImagePullJobList contains a list of ImagePullJob
type ImagePullJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImagePullJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImagePullJob{}, &ImagePullJobList{})
}
