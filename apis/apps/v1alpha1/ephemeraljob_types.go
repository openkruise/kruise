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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	EphemeralContainerEnvKey = "KRUISE_EJOB_ID"
)

// EphemeralJobSpec defines the desired state of EphemeralJob
type EphemeralJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Selector is a label query over pods that should match the pod labels.
	Selector *metav1.LabelSelector `json:"selector"`

	// Replicas indicates a part of the quantity from matched pods by selector.
	// Usually it is used for gray scale working.
	// if Replicas exceeded the matched number by selector or not be set, replicas will not work.
	Replicas *int32 `json:"replicas,omitempty"`

	// Parallelism specifies the maximum desired number of pods which matches running ephemeral containers.
	// +optional
	Parallelism *int32 `json:"parallelism,omitempty" protobuf:"varint,1,opt,name=parallelism"`

	// Template describes the ephemeral container that will be created.
	Template EphemeralContainerTemplateSpec `json:"template"`

	// Paused will pause the ephemeral job.
	// +optional
	Paused bool `json:"paused,omitempty" protobuf:"bytes,4,opt,name=paused"`

	// ActiveDeadlineSeconds specifies the duration in seconds relative to the startTime that the job may be active
	// before the system tries to terminate it; value must be positive integer.
	// Only works for Always type.
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty" protobuf:"varint,2,opt,name=activeDeadlineSeconds"`

	// ttlSecondsAfterFinished limits the lifetime of a Job that has finished
	// execution (either Complete or Failed). If this field is set,
	// ttlSecondsAfterFinished after the eJob finishes, it is eligible to be
	// automatically deleted. When the Job is being deleted, its lifecycle
	// guarantees (e.g. finalizers) will be honored.
	// If this field is unset, default value is 1800
	// If this field is set to zero,
	// the Job becomes eligible to be deleted immediately after it finishes.
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty" protobuf:"varint,4,opt,name=ttlSecondsAfterFinished"`
}

// EphemeralContainerTemplateSpec describes template spec of ephemeral containers
type EphemeralContainerTemplateSpec struct {

	// EphemeralContainers defines ephemeral container list in match pods.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +patchMergeKey=name
	// +patchStrategy=merge
	EphemeralContainers []v1.EphemeralContainer `json:"ephemeralContainers" patchStrategy:"merge" patchMergeKey:"name"`
}

// EphemeralJobStatus defines the observed state of EphemeralJob
type EphemeralJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []EphemeralJobCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Represents time when the job was acknowledged by the job controller.
	// It is not guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty" protobuf:"bytes,2,opt,name=startTime"`

	// Represents time when the job was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty" protobuf:"bytes,3,opt,name=completionTime"`

	// The phase of the job.
	// +optional
	Phase EphemeralJobPhase `json:"phase" protobuf:"varint,8,opt,name=phase"`

	// The number of total matched pods.
	// +optional
	Matches int32 `json:"match" protobuf:"varint,4,opt,name=match"`

	// The number of actively running pods.
	// +optional
	Running int32 `json:"running" protobuf:"varint,4,opt,name=running"`

	// The number of pods which reached phase Succeeded.
	// +optional
	Succeeded int32 `json:"succeeded" protobuf:"varint,5,opt,name=completed"`

	// The number of waiting pods.
	// +optional
	Waiting int32 `json:"waiting" protobuf:"varint,4,opt,name=waiting"`

	// The number of pods which reached phase Failed.
	// +optional
	Failed int32 `json:"failed" protobuf:"varint,6,opt,name=failed"`
}

// JobCondition describes current state of a job.
type EphemeralJobCondition struct {
	// Type of job condition, Complete or Failed.
	Type EphemeralJobConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=EphemeralJobConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`
	// Last time the condition was checked.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty" protobuf:"bytes,3,opt,name=lastProbeTime"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,6,opt,name=message"`
}

// JobConditionType indicates valid conditions type of a job
type EphemeralJobConditionType string

// These are valid conditions of a job.
const (
	// EJobSucceeded means the ephemeral job has succeeded completed its execution. A succeeded job means pods have been
	// successful finished on all tasks on eligible nodes.
	EJobSucceeded EphemeralJobConditionType = "JobSucceeded"

	// EJobFailed means there are some ephemeral containers matched by ephemeral job failed.
	EJobFailed EphemeralJobConditionType = "JobFailed"

	// EJobError means some ephemeral containers matched by ephemeral job run error.
	EJobError EphemeralJobConditionType = "JobError"

	// EJobInitialized means ephemeral job has created and initialized by controller.
	EJobInitialized EphemeralJobConditionType = "Initialized"

	// EJobMatchedEmpty means the ephemeral job has not matched the target pods.
	EJobMatchedEmpty EphemeralJobConditionType = "MatchedEmpty"
)

// EphemeralJobPhase indicates the type of EphemeralJobPhase.
type EphemeralJobPhase string

const (
	// EphemeralJobSucceeded means the job has succeeded.
	EphemeralJobSucceeded EphemeralJobPhase = "Succeeded"

	// EphemeralJobFailed means the job has failed.
	EphemeralJobFailed EphemeralJobPhase = "Failed"

	// EphemeralJobWaiting means the job is waiting.
	EphemeralJobWaiting EphemeralJobPhase = "Waiting"

	// EphemeralJobRunning means the job is running.
	EphemeralJobRunning EphemeralJobPhase = "Running"

	// EphemeralJobPause means the ephemeral job paused.
	EphemeralJobPause EphemeralJobPhase = "Paused"

	// EphemeralJobError means the ephemeral  paused.
	EphemeralJobError EphemeralJobPhase = "Error"

	EphemeralJobUnknown EphemeralJobPhase = "Unknown"
)

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ejob
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.phase",description="The status of ephemeral job"
// +kubebuilder:printcolumn:name="MATCH",type="integer",JSONPath=".status.match",description="Number of ephemeral container matched by this job"
// +kubebuilder:printcolumn:name="SUCCEED",type="integer",JSONPath=".status.succeeded",description="Number of succeed ephemeral containers"
// +kubebuilder:printcolumn:name="FAILED",type="integer",JSONPath=".status.failed",description="Number of failed ephemeral containers"
// +kubebuilder:printcolumn:name="RUNNING",type="integer",JSONPath=".status.running",description="Number of running ephemeral containers"
// +kubebuilder:printcolumn:name="WAITING",type="integer",JSONPath=".status.waiting",description="Number of waiting ephemeral containers"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// EphemeralJob is the Schema for the ephemeraljobs API
type EphemeralJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EphemeralJobSpec   `json:"spec,omitempty"`
	Status EphemeralJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EphemeralJobList contains a list of EphemeralJob
type EphemeralJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EphemeralJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EphemeralJob{}, &EphemeralJobList{})
}
