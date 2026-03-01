---
title: AdvancedCronJob Crd and Controller
authors:
  - "@rishi-anand"
reviewers:
  - "@Fei-Guo"
  - "@FillZpp"
  - "@jzhoucliqr"
creation-date: 2020-10-25
last-updated: 2020-10-25
status: implementable
---

# Implementing AdvancedCronJob Crd and Controller
- Implementing AdvancedCronJob to support Job/BroadcastJob or any other future CRD and to run it periodically at a given schedule.

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
  - [Implementation History](#implementation-history)

## Summary

This controller will be very generic and will have implementations to help developers to run a job or any CRD in specific schedule.

## Motivation

- Developer may come across a use-case when some job needs to be executed on at a specific schedule.
- Found same use-case requirement in Issues #251 and I got motivated to implement it

### Goals

- Implementing a custom controller for AdvancedCronJob which acts like CronJob but it schedules Job/BroadcastJob or other CRD

## Proposal

- Adding a new CRD and controller for AdvancedCronJob.
- AdvancedCronJob should be able to reconcile Job/BroadcastJob or any other future CRD if required.
- Once AdvancedCronJob is created, spec cannot be modified.
- Adding webhook for validation of AdvancedCronJob.

### User Stories

- Implement a CRD which contains below fields and a controller which honors all the fields and reconciles accordingly.
```

// AdvancedCronJob is the Schema for the advancedcronjobs API
type AdvancedCronJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AdvancedCronJobSpec   `json:"spec,omitempty"`
	Status AdvancedCronJobStatus `json:"status,omitempty"`
}

type AdvancedCronJobSpec struct {
	Schedule string `json:"schedule" protobuf:"bytes,1,opt,name=schedule"`

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
	Paused bool `json:"paused,omitempty" protobuf:"bytes,4,opt,name=paused"`

	// +kubebuilder:validation:Minimum=0

	// The number of successful finished jobs to retain.
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty" protobuf:"varint,5,opt,name=successfulJobsHistoryLimit"`

	// +kubebuilder:validation:Minimum=0

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
	JobTemplate *batchv1beta1.JobTemplateSpec

	// Specifies the broadcastjob that will be created when executing a BroadcastCronJob.
	// +optional
	BroadcastJobTemplate *BroadcastJobTemplateSpec
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
```

#### Story 1
Create a above CRD and implement the controller

#### Story 2
Add unit and integration test cases

## Implementation History

- [ ] 10/24/2020: Proposal discussion in an issue <a href="https://github.com/openkruise/kruise/issues/215#issuecomment-715506813">#215</a>
- [ ] 11/04/2020: Proposal submission
