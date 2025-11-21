/*
Copyright 2025 The Kruise Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type PubOperation string

const (
	// Marked the pod will not be pub-protected, solving the scenario of force pod deletion
	PodPubNoProtectionAnnotation = "pub.kruise.io/no-protect"

	// pod webhook operation
	PubUpdateOperation PubOperation = "UPDATE"
	PubDeleteOperation PubOperation = "DELETE"
	PubEvictOperation  PubOperation = "EVICT"
	PubResizeOperation PubOperation = "RESIZE"
)

// PodUnavailableBudgetSpec defines the desired state of PodUnavailableBudget
type PodUnavailableBudgetSpec struct {
	// Selector label query over pods managed by the budget
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// TargetReference contains enough information to let you identify an workload for PodUnavailableBudget
	// Selector and TargetReference are mutually exclusive, TargetReference is priority to take effect
	TargetReference *TargetReference `json:"targetRef,omitempty"`

	// Delete pod, evict pod or update pod specification is allowed if at most "maxUnavailable" pods selected by
	// "selector" or "targetRef"  are unavailable after the above operation for pod.
	// MaxUnavailable and MinAvailable are mutually exclusive, MaxUnavailable is priority to take effect
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// Delete pod, evict pod or update pod specification is allowed if at least "minAvailable" pods selected by
	// "selector" or "targetRef" will still be available after the above operation for pod.
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

	// ProtectOperations indicates the pub protected operations[DELETE,UPDATE,EVICT].
	// If set, only specified operations are protected.
	// If not set, the default DELETE,EVICT,UPDATE are protected.
	// RESIZE: Pod vertical scaling action. If it's enabled, all resize action will be protected. RESIZE
	// is an extension of UPDATE, if RESIZE is disabled and UPDATE is enabled, any UPDATE operation will
	// be protected only as it will definitely cause container restarts.
	// UPDATE: Kruise will carefully differentiate whether this update will cause interruptions. When
	// the FeatureGate InPlacePodVerticalScaling is enabled, pod inplace vertical scaling will be
	// considered non-disruption only when allowedResources(cpu、memory) changes、restartPolicy
	// is not restartContainer、is not static pod and QoS not changed. But if featureGate
	// InPlacePodVerticalScaling is disabled, all resize action will be considered as disruption.
	// +optional
	ProtectOperations []PubOperation `json:"protectOperations,omitempty"`

	// ProtectTotalReplicas is the target replicas.
	// By default, PUB will get the target replicas through workload.spec.replicas. but there are some scenarios that may workload doesn't
	// implement scale subresources or Pod doesn't have workload management. In this scenario, you can set protectTotalReplicas
	// to get the target replicas to realize the same effect of protection ability.
	// +optional
	ProtectTotalReplicas *int32 `json:"protectTotalReplicas,omitempty"`
}

// TargetReference contains enough information to let you identify an workload for PodUnavailableBudget
type TargetReference struct {
	// API version of the referent.
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind of the referent.
	Kind string `json:"kind,omitempty"`
	// Name of the referent.
	Name string `json:"name,omitempty"`
}

// PodUnavailableBudgetStatus defines the observed state of PodUnavailableBudget
type PodUnavailableBudgetStatus struct {
	// Most recent generation observed when updating this PUB status. UnavailableAllowed and other
	// status information is valid only if observedGeneration equals to PUB's object generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`

	// DisruptedPods contains information about pods whose eviction or deletion was
	// processed by the API handler but has not yet been observed by the PodUnavailableBudget.
	// +optional
	DisruptedPods map[string]metav1.Time `json:"disruptedPods,omitempty"`

	// UnavailablePods contains information about pods whose specification changed(inplace-update pod),
	// once pod is available(consistent and ready) again, it will be removed from the list.
	// +optional
	UnavailablePods map[string]metav1.Time `json:"unavailablePods,omitempty"`

	// UnavailableAllowed number of pod unavailable that are currently allowed
	UnavailableAllowed int32 `json:"unavailableAllowed"`

	// CurrentAvailable current number of available pods
	CurrentAvailable int32 `json:"currentAvailable"`

	// DesiredAvailable minimum desired number of available pods
	DesiredAvailable int32 `json:"desiredAvailable"`

	// TotalReplicas total number of pods counted by this unavailable budget
	TotalReplicas int32 `json:"totalReplicas"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=pub
// +kubebuilder:printcolumn:name="Allowed",type="integer",JSONPath=".status.unavailableAllowed",description="UnavailableAllowed number of pod unavailable that are currently allowed"
// +kubebuilder:printcolumn:name="Current",type="integer",JSONPath=".status.currentAvailable",description="CurrentAvailable current number of available pods"
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".status.desiredAvailable",description="DesiredAvailable minimum desired number of available pods"
// +kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.totalReplicas",description="TotalReplicas total number of pods counted by this budget"

// PodUnavailableBudget is the Schema for the podunavailablebudgets API
type PodUnavailableBudget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodUnavailableBudgetSpec   `json:"spec,omitempty"`
	Status PodUnavailableBudgetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PodUnavailableBudgetList contains a list of PodUnavailableBudget
type PodUnavailableBudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodUnavailableBudget `json:"items"`
}
