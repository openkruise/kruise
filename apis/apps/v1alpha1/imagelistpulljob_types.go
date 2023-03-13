/*
Copyright 2023 The Kruise Authors.

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
)

// ImageListPullJobSpec defines the desired state of ImageListPullJob
type ImageListPullJobSpec struct {
	// Images is the image list to be pulled by the job
	Images []string `json:"images"`

	ImagePullJobTemplate `json:",inline"`
}

// ImageListPullJobStatus defines the observed state of ImageListPullJob
type ImageListPullJobStatus struct {
	// Represents time when the job was acknowledged by the job controller.
	// It is not guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Represents time when the all the image pull job was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// The desired number of ImagePullJobs, this is typically equal to the number of len(spec.Images).
	Desired int32 `json:"desired"`

	// The number of running ImagePullJobs which are acknowledged by the imagepulljob controller.
	// +optional
	Active int32 `json:"active"`

	// The number of ImagePullJobs which are finished
	// +optional
	Completed int32 `json:"completed"`

	// The number of image pull job which are finished and status.Succeeded==status.Desired.
	// +optional
	Succeeded int32 `json:"succeeded"`

	// The status of ImagePullJob which has the failed nodes(status.Failed>0) .
	// +optional
	FailedImageStatuses []*FailedImageStatus `json:"failedImageStatuses,omitempty"`
}

// FailedImageStatus the state of ImagePullJob which has the failed nodes(status.Failed>0)
type FailedImageStatus struct {
	// The name of ImagePullJob which has the failed nodes(status.Failed>0)
	// +optional
	ImagePullJob string `json:"imagePullJob,omitempty"`

	// Name of the image
	// +optional
	Name string `json:"name,omitempty"`

	// The text prompt for job running status.
	// +optional
	Message string `json:"message,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TOTAL",type="integer",JSONPath=".status.desired",description="Number of image pull job"
// +kubebuilder:printcolumn:name="SUCCEEDED",type="integer",JSONPath=".status.succeeded",description="Number of image pull job succeeded"
// +kubebuilder:printcolumn:name="COMPLETED",type="integer",JSONPath=".status.completed",description="Number of ImagePullJobs which are finished"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// ImageListPullJob is the Schema for the imagelistpulljobs API
type ImageListPullJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageListPullJobSpec   `json:"spec,omitempty"`
	Status ImageListPullJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageListPullJobList contains a list of ImageListPullJob
type ImageListPullJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageListPullJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageListPullJob{}, &ImageListPullJobList{})
}
