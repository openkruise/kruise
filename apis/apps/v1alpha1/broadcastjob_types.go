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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// BroadcastJobSpec defines the desired state of BroadcastJob
type BroadcastJobSpec struct {
	// Parallelism specifies the maximum desired number of pods the job should
	// run at any given time. The actual number of pods running in steady state will
	// be less than this number when the work left to do is less than max parallelism.
	// Not setting this value means no limit.
	// +optional
	Parallelism *intstr.IntOrString `json:"parallelism,omitempty" protobuf:"varint,1,opt,name=parallelism"`

	// Template describes the pod that will be created when executing a job.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Template v1.PodTemplateSpec `json:"template" protobuf:"bytes,2,opt,name=template"`

	// CompletionPolicy indicates the completion policy of the job.
	// Default is Always CompletionPolicyType.
	// +optional
	CompletionPolicy CompletionPolicy `json:"completionPolicy" protobuf:"bytes,3,opt,name=completionPolicy"`

	// Paused will pause the job.
	// +optional
	Paused bool `json:"paused,omitempty" protobuf:"bytes,4,opt,name=paused"`

	// FailurePolicy indicates the behavior of the job, when failed pod is found.
	// +optional
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty" protobuf:"bytes,5,opt,name=failurePolicy"`

	// PodFailurePolicy specifies how failed pods influence the backoffLimit.
	// Rules are evaluated in order, and once a rule matches a Pod failure,
	// the remaining rules are ignored. When no rule matches, the default
	// handling applies based on FailurePolicy.
	// This is similar to upstream Kubernetes Job's PodFailurePolicy (GA in k8s 1.31).
	// +optional
	PodFailurePolicy *PodFailurePolicy `json:"podFailurePolicy,omitempty"`

	// PodReplacementPolicy specifies when to create replacement Pods.
	// Possible values are:
	// - TerminatingOrFailed means that we recreate pods
	//   when they are terminating (has a metadata.deletionTimestamp) or failed.
	// - Failed means to wait until a previously created Pod is fully terminated (has phase
	//   Failed or Succeeded) before creating a replacement Pod.
	//
	// When using podFailurePolicy, Failed is the the default.
	// When not using podFailurePolicy, TerminatingOrFailed is the default.
	// This is similar to upstream Kubernetes Job's PodReplacementPolicy (GA in k8s 1.34).
	// +optional
	PodReplacementPolicy *PodReplacementPolicy `json:"podReplacementPolicy,omitempty"`
}

// CompletionPolicy indicates the completion policy for the job
type CompletionPolicy struct {
	// Type indicates the type of the CompletionPolicy.
	// Default is Always.
	Type CompletionPolicyType `json:"type,omitempty" protobuf:"bytes,1,opt,name=type,casttype=CompletionPolicyType"`

	// ActiveDeadlineSeconds specifies the duration in seconds relative to the startTime that the job may be active
	// before the system tries to terminate it; value must be positive integer.
	// Only works for Always type.
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty" protobuf:"varint,2,opt,name=activeDeadlineSeconds"`

	// ttlSecondsAfterFinished limits the lifetime of a Job that has finished
	// execution (either Complete or Failed). If this field is set,
	// ttlSecondsAfterFinished after the Job finishes, it is eligible to be
	// automatically deleted. When the Job is being deleted, its lifecycle
	// guarantees (e.g. finalizers) will be honored. If this field is unset,
	// the Job won't be automatically deleted. If this field is set to zero,
	// the Job becomes eligible to be deleted immediately after it finishes.
	// This field is alpha-level and is only honored by servers that enable the
	// TTLAfterFinished feature.
	// Only works for Always type
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty" protobuf:"varint,4,opt,name=ttlSecondsAfterFinished"`
}

// CompletionPolicyType indicates the type of completion policy
type CompletionPolicyType string

const (
	// Always means the job will eventually finish on these conditions:
	// 1) after all pods on the desired nodes are completed (regardless succeeded or failed),
	// 2) exceeds ActiveDeadlineSeconds,
	// 3) exceeds RestartLimit.
	// This is the default CompletionPolicyType.
	Always CompletionPolicyType = "Always"

	// Never means the job will be kept alive after all pods on the desired nodes are completed.
	// This is useful when new nodes are added after the job completes, the pods will be triggered automatically on those new nodes.
	Never CompletionPolicyType = "Never"
)

// BroadcastJobStatus defines the observed state of BroadcastJob
type BroadcastJobStatus struct {
	// The latest available observations of an object's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []JobCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

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

	// The number of actively running pods.
	// +optional
	Active int32 `json:"active" protobuf:"varint,4,opt,name=active"`

	// The number of pods which reached phase Succeeded.
	// +optional
	Succeeded int32 `json:"succeeded" protobuf:"varint,5,opt,name=succeeded"`

	// The number of pods which reached phase Failed.
	// +optional
	Failed int32 `json:"failed" protobuf:"varint,6,opt,name=failed"`

	// The desired number of pods, this is typically equal to the number of nodes satisfied to run pods.
	// +optional
	Desired int32 `json:"desired" protobuf:"varint,7,opt,name=desired"`

	// The phase of the job.
	// +optional
	Phase BroadcastJobPhase `json:"phase" protobuf:"varint,8,opt,name=phase"`
}

// BroadcastJobPhase indicates the phase of the job.
type BroadcastJobPhase string

const (
	// PhaseCompleted means the job is completed.
	PhaseCompleted BroadcastJobPhase = "completed"

	// PhaseRunning means the job is running.
	PhaseRunning BroadcastJobPhase = "running"

	// PhasePaused means the job is paused.
	PhasePaused BroadcastJobPhase = "paused"

	// PhaseFailed means the job is failed.
	PhaseFailed BroadcastJobPhase = "failed"
)

// FailurePolicy indicates the behavior of the job, when failed pod is found.
type FailurePolicy struct {
	// Type indicates the type of FailurePolicyType.
	// Default is FailurePolicyTypeFailFast.
	Type FailurePolicyType `json:"type,omitempty" protobuf:"bytes,1,opt,name=type,casttype=FailurePolicyType"`

	// RestartLimit specifies the number of retries before marking the pod failed.
	RestartLimit int32 `json:"restartLimit,omitempty" protobuf:"varint,2,opt,name=restartLimit"`
}

// PodFailurePolicy describes how failed pods influence the backoffLimit.
// This is similar to upstream Kubernetes Job's PodFailurePolicy.
type PodFailurePolicy struct {
	// A list of pod failure policy rules. The rules are evaluated in order.
	// Once a rule matches a Pod failure, the remaining of the rules are ignored.
	// When no rule matches the Pod failure, the default handling applies - the
	// counter of pod failures is incremented and it is checked against
	// the backoffLimit. At most 20 elements are allowed.
	// +listType=atomic
	Rules []PodFailurePolicyRule `json:"rules"`
}

// PodFailurePolicyRule describes how a pod failure is handled when the requirements are met.
type PodFailurePolicyRule struct {
	// Specifies the action taken on a pod failure when the requirements are satisfied.
	// Possible values are:
	//
	// - FailJob: indicates that the pod's job is marked as Failed and all
	//   running pods are terminated.
	// - Ignore: indicates that the counter towards the .backoffLimit is not
	//   incremented and a replacement pod is created.
	// - Count: indicates that the pod is handled in the default way - the
	//   counter towards the .backoffLimit is incremented.
	// +kubebuilder:validation:Enum=FailJob;Ignore;Count
	Action PodFailurePolicyAction `json:"action"`

	// Represents the requirement on the container exit codes.
	// +optional
	OnExitCodes *PodFailurePolicyOnExitCodesRequirement `json:"onExitCodes,omitempty"`

	// Represents the requirement on the pod conditions. The requirement is represented
	// as a list of pod condition patterns. The requirement is satisfied if at
	// least one pattern matches an actual pod condition. At most 20 elements are allowed.
	// +listType=atomic
	// +optional
	OnPodConditions []PodFailurePolicyOnPodConditionsPattern `json:"onPodConditions,omitempty"`
}

// PodFailurePolicyAction specifies how a pod failure is handled.
// +kubebuilder:validation:Enum=FailJob;Ignore;Count
type PodFailurePolicyAction string

const (
	// PodFailurePolicyActionFailJob indicates that the pod's job is marked as Failed
	// and all running pods are terminated.
	PodFailurePolicyActionFailJob PodFailurePolicyAction = "FailJob"

	// PodFailurePolicyActionIgnore indicates that the counter towards the .backoffLimit
	// is not incremented and a replacement pod is created.
	PodFailurePolicyActionIgnore PodFailurePolicyAction = "Ignore"

	// PodFailurePolicyActionCount indicates that the pod is handled in the default way -
	// the counter towards the .backoffLimit is incremented.
	PodFailurePolicyActionCount PodFailurePolicyAction = "Count"
)

// PodFailurePolicyOnExitCodesRequirement describes the requirement for handling
// a failed pod based on its container exit codes.
type PodFailurePolicyOnExitCodesRequirement struct {
	// Restricts the check for exit codes to the container with the
	// specified name. When null, the rule applies to all containers.
	// When specified, it should match one the container or initContainer
	// names in the pod template.
	// +optional
	ContainerName *string `json:"containerName,omitempty"`

	// Represents the relationship between the container exit code(s) and the
	// specified values. Containers completed with success (exit code 0) are
	// excluded from the requirement check.
	// +kubebuilder:validation:Enum=In;NotIn
	Operator PodFailurePolicyOnExitCodesOperator `json:"operator"`

	// Specifies the set of values. Each returned container exit code (might be
	// multiple in case of multiple containers) is checked against this set of
	// values with respect to the operator. The list of values must be ordered
	// and must not contain duplicates. Value '0' cannot be used for the In operator.
	// At least one element is required. At most 255 elements are allowed.
	// +listType=set
	Values []int32 `json:"values"`
}

// PodFailurePolicyOnExitCodesOperator represents the operator for matching container exit codes.
// +kubebuilder:validation:Enum=In;NotIn
type PodFailurePolicyOnExitCodesOperator string

const (
	// PodFailurePolicyOnExitCodesOpIn means the exit code must be in the specified values.
	PodFailurePolicyOnExitCodesOpIn PodFailurePolicyOnExitCodesOperator = "In"
	// PodFailurePolicyOnExitCodesOpNotIn means the exit code must not be in the specified values.
	PodFailurePolicyOnExitCodesOpNotIn PodFailurePolicyOnExitCodesOperator = "NotIn"
)

// PodFailurePolicyOnPodConditionsPattern describes a pattern for matching
// an actual pod condition type.
type PodFailurePolicyOnPodConditionsPattern struct {
	// Specifies the required Pod condition type. To match a pod condition
	// it is required that specified type equals the pod condition type.
	Type v1.PodConditionType `json:"type"`

	// Specifies the required Pod condition status. To match a pod condition
	// it is required that the specified status equals the pod condition status.
	// Defaults to True.
	// +kubebuilder:default=True
	Status v1.ConditionStatus `json:"status"`
}

// PodReplacementPolicy specifies the policy for creating pod replacements.
// +kubebuilder:validation:Enum=TerminatingOrFailed;Failed
type PodReplacementPolicy string

const (
	// TerminatingOrFailed means that we recreate pods
	// when they are terminating (has a metadata.deletionTimestamp) or failed.
	TerminatingOrFailed PodReplacementPolicy = "TerminatingOrFailed"
	// Failed means to wait until a previously created Pod is fully terminated (has phase
	// Failed or Succeeded) before creating a replacement Pod.
	Failed PodReplacementPolicy = "Failed"
)

// FailurePolicyType indicates the type of FailurePolicyType.
type FailurePolicyType string

const (
	// FailurePolicyTypeContinue means the job will be still running, when failed pod is found.
	FailurePolicyTypeContinue FailurePolicyType = "Continue"

	// FailurePolicyTypeFailFast means the job will be failed, when failed pod is found.
	// This is the default FailurePolicyType.
	FailurePolicyTypeFailFast FailurePolicyType = "FailFast"

	// FailurePolicyTypePause means the job will be paused, when failed pod is found.
	FailurePolicyTypePause FailurePolicyType = "Pause"
)

// JobConditionType indicates valid conditions type of a job
type JobConditionType string

// These are valid conditions of a job.
const (
	// JobComplete means the job has completed its execution. A complete job means pods have been deployed on all
	// eligible nodes and all pods have reached succeeded or failed state. Note that the eligible nodes are defined at
	// the beginning of a reconciliation loop. If there are more nodes added within a reconciliation loop, those nodes will
	// not be considered to run pods.
	JobComplete JobConditionType = "Complete"

	// JobFailed means the job has failed its execution. A failed job means the job has either exceeded the
	// ActiveDeadlineSeconds limit, or the aggregated number of container restarts for all pods have exceeded the RestartLimit.
	JobFailed JobConditionType = "Failed"
)

// JobCondition describes current state of a job.
type JobCondition struct {
	// Type of job condition, Complete or Failed.
	Type JobConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=JobConditionType"`
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

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=bcj
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".status.desired",description="The desired number of pods. This is typically equal to the number of nodes satisfied to run pods."
// +kubebuilder:printcolumn:name="Active",type="integer",JSONPath=".status.active",description="The number of actively running pods."
// +kubebuilder:printcolumn:name="Succeeded",type="integer",JSONPath=".status.succeeded",description="The number of pods which reached phase Succeeded."
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.failed",description="The number of pods which reached phase Failed."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// BroadcastJob is the Schema for the broadcastjobs API
type BroadcastJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BroadcastJobSpec   `json:"spec,omitempty"`
	Status BroadcastJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BroadcastJobList contains a list of BroadcastJob
type BroadcastJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BroadcastJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BroadcastJob{}, &BroadcastJobList{})
}
