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

	// number or percentage of maximum unavailable pods
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// number or percentage of minimum available pods
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

	// key-value pairs that indicates that pod is unavailable
	// any pod who carries any matching kv in its label, will be treated as unavailable
	// This is specificly designed for application-level update.
	// During application-level update(like hot fix), application becomes unavailable.
	// But pod status won't change, since pod image does not change.
	// To avoid any violation of MaxUnavailable,
	// app-level update should add an unavailable label to their pod label before doing any update.
	// +optional
	UnavailableLabels map[string]string `json:unavailableLabels",omitempty"`
}

// PodUnavailableBudgetStatus defines the observed state of PodUnavailableBudgetStatus
type PodUnavailableBudgetStatus struct {
	// observedGeneration is the most recent generation observed for this PodUnavailableBudget. It corresponds to the
	// PodUnavailableBudget's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// DeletedPods contains **uids** of pods that are deleted or to be deleted.
	// When pod delete request is audited and approved by PodUnavailableBudget's validating webhook,
	// the pod's uid will be added to this list.
	// Since then, this pod will be treated as both unavailable and deleted by PodUnavailableBudget's controller,
	// even if it may appear to be available for a short while.
	// Note that:
	//  * pods in this list are deleted pods
	//  * not all deleted pods end up in this list.
	//    Those pods that are deleted before PUB starts will not show up here.
	DeletedPods []string `json:"deletedPods,omitempty"`

	// UnavailablePods contains **names** of pods that are unavailable.
	// When a pod becomes unavailable or will be kicked unavailable in a short while,
	// PodUnavailableBudget's validating webhook will add its name to this list.
	// Since then, this pod will be treated as unavailable by PodUnavailableBudget's controller,
	// even if it may appear to be available for a short while.
	UnavailablePods []string `json:"unavailablePods,omitempty"`

	// pod unavailablility allowed (the number of pods that can be kicked unavailable) at this moment.
	UnavailableAllowed int32 `json:"unavailableAllowed"`

	// number of available pods at this moment
	CurrentAvailable int32 `json:"currentAvailable"`

	// number of unavailable pods at this moment
	CurrentUnavailable int32 `json:"currentUnavailable"`

	// total number of pods at this moment
	CurrentPodCount int32 `json:"currentPodCount"`

	// expected number of replicas in total as specified by pods' controllers
	ExpectedTotalReplicas int32 `json:"expectedTotalReplicas"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodUnavailableBudget is the Schema for the PodUnavailableBudget API
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
