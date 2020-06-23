/*
Copyright 2019 The Kruise Authors.

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

// PodUnavailableBudgetSpec defines the desired state of PodUnavailableBudget
type PodUnavailableBudgetSpec struct {
	// selector is a label query over pods that should be protected
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// maximum unavailable pods
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

	// key-value pairs that indicates that pod is unavailable
	// any pod who carries any matching kv in its label, will be treated as unavailable
	// This is specificly designed for application-level update.
	// During application-level update(like hot fix), application becomes unavailable.
	// But pod status won't change, since pod image does not change.
	// To avoid any violation of MaxUnavailable,
	// app-level update should add an unavailable label to their pod label before doing any update.
	UnavailableLabels map[string]string `json:unavailableLabels",omitempty"`
}

// PodUnavailableBudgetStatus defines the observed state of PodUnavailableBudgetStatus
type PodUnavailableBudgetStatus struct {
	// observedGeneration is the most recent generation observed for this SidecarSet. It corresponds to the
	// SidecarSet's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	DeletedPods []string `json:"deletedPods,omitempty"`
	UnavailablePods []string `json:"unavailablePods,omitempty"`

	UnavailableAllowed int32 `json:"unavailableAllowed"`

	// number of available pods at this moment
	CurrentAvailable int32 `json:"currentAvailable"`

	CurrentUnavailable int32 `json:"currentUnavailable"`

	TotalReplicas int32 `json:"totalReplicas"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SidecarSet is the Schema for the sidecarsets API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MATCHED",type="integer",JSONPath=".status.matchedPods",description="The number of pods matched."
// +kubebuilder:printcolumn:name="UPDATED",type="integer",JSONPath=".status.updatedPods",description="The number of pods matched and updated."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyPods",description="The number of pods matched and ready."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
type PodUnavailableBudget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodUnavailableBudgetSpec   `json:"spec,omitempty"`
	Status PodUnavailableBudgetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodUnavailableBudgetList contains a list of PodUnavailableBudget
type PodUnavailableBudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodUnavailableBudget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodUnavailableBudget{}, &PodUnavailableBudgetList{})
}
