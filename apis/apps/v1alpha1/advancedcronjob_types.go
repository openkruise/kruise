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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const AdvancedCronJobKind = "AdvancedCronJob"

// AdvancedCronJobSpec defines the desired state of AdvancedCronJob
type AdvancedCronJobSpec struct {
	// +kubebuilder:validation:MinLength=0

	// The schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	Schedule string `json:"schedule" protobuf:"bytes,1,opt,name=schedule"`

	// The time zone name for the given schedule, see https://en.wikipedia.org/wiki/List_of_tz_database_time_zones.
	// If not specified, this will default to the time zone of the kruise-controller-manager process.
	// +optional
	TimeZone *string `json:"timeZone,omitempty" protobuf:"bytes,8,opt,name=timeZone"`

	// Optional deadline in seconds for starting the job if it misses scheduled
	// time for any reason.  Missed jobs executions will be counted as failed ones.
	// +optional
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty" protobuf:"varint,2,opt,name=startingDeadlineSeconds"`

	// Specifies how to treat concurrent executions of a Job.
	// Valid values are:
	// - "Allow" (default): allows CronJobs to run concurrently;
	// - "Forbid": forbids concurrent runs, skipping next run if previous run hasn't finished yet;
	// - "Replace": cancels currently running job and replaces it with a new one
	// +optional
	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty" protobuf:"bytes,3,opt,name=concurrencyPolicy"`

	// Paused will pause the cron job.
	// +optional
	Paused *bool `json:"paused,omitempty" protobuf:"bytes,4,opt,name=paused"`

	// The number of successful finished jobs to retain.
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty" protobuf:"varint,5,opt,name=successfulJobsHistoryLimit"`

	// The number of failed finished jobs to retain.
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	FailedJobsHistoryLimit *int32 `json:"failedJobsHistoryLimit,omitempty" protobuf:"varint,6,opt,name=failedJobsHistoryLimit"`

	// Specifies the job that will be created when executing a CronJob.
	Template CronJobTemplate `json:"template" protobuf:"bytes,7,opt,name=template"`
}

type CronJobTemplate struct {
	// Specifies the job that will be created when executing a CronJob.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	JobTemplate *batchv1.JobTemplateSpec `json:"jobTemplate,omitempty" protobuf:"bytes,1,opt,name=jobTemplate"`

	// Specifies the broadcastjob that will be created when executing a BroadcastCronJob.
	// +optional
	BroadcastJobTemplate *BroadcastJobTemplateSpec `json:"broadcastJobTemplate,omitempty" protobuf:"bytes,2,opt,name=broadcastJobTemplate"`
}

type TemplateKind string

const (
	JobTemplate TemplateKind = "Job"

	BroadcastJobTemplate TemplateKind = "BroadcastJob"
)

// JobTemplateSpec describes the data a Job should have when created from a template
type BroadcastJobTemplateSpec struct {
	// Standard object's metadata of the jobs created from this template.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Specification of the desired behavior of the broadcastjob.
	// +optional
	Spec BroadcastJobSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

// ConcurrencyPolicy describes how the job will be handled.
// Only one of the following concurrent policies may be specified.
// If none of the following policies is specified, the default one
// is AllowConcurrent.
// +kubebuilder:validation:Enum=Allow;Forbid;Replace
type ConcurrencyPolicy string

const (
	// AllowConcurrent allows CronJobs to run concurrently.
	AllowConcurrent ConcurrencyPolicy = "Allow"

	// ForbidConcurrent forbids concurrent runs, skipping next run if previous
	// hasn't finished yet.
	ForbidConcurrent ConcurrencyPolicy = "Forbid"

	// ReplaceConcurrent cancels currently running job and replaces it with a new one.
	ReplaceConcurrent ConcurrencyPolicy = "Replace"
)

// AdvancedCronJobStatus defines the observed state of AdvancedCronJob
type AdvancedCronJobStatus struct {
	Type TemplateKind `json:"type,omitempty"`

	// A list of pointers to currently running jobs.
	// +optional
	Active []corev1.ObjectReference `json:"active,omitempty"`

	// Information when was the last time the job was successfully scheduled.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=acj
// +kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.schedule",description="The schedule of advanced cron job."
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type",description="Type of cron job."
// +kubebuilder:printcolumn:name="LastScheduleTime",type="date",JSONPath=".status.lastScheduleTime",description="The last time at which job was scheduled."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// AdvancedCronJob is the Schema for the advancedcronjobs API
type AdvancedCronJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AdvancedCronJobSpec   `json:"spec,omitempty"`
	Status AdvancedCronJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AdvancedCronJobList contains a list of AdvancedCronJob
type AdvancedCronJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AdvancedCronJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AdvancedCronJob{}, &AdvancedCronJobList{})
}
