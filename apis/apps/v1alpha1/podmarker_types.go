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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PodMarkerSpec defines the desired state of PodMarker
type PodMarkerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	//
	// Strategy defines the behaviors when marking Pods.
	Strategy PodMarkerStrategy `json:"strategy,omitempty"`
	// MatchRequirements describe the requirements for Pods that users want to mark.
	MatchRequirements PodMarkerRequirements `json:"matchRequirements"`
	// MatchPreferences defines the different priorities for the Pods that could be marked;
	// The Pods having higher priority will be marked preferentially;
	// MatchPreferences is an sequential list, where the Pods satisfying
	// MatchPreferences[i] will have higher priority than the Pods satisfying MatchPreferences[j] if i < j.
	// If len(MatchPreferences) > 1, the sorting rule of Pods when marking is similar to lexicographic order.
	MatchPreferences []PodMarkerPreference `json:"matchPreferences,omitempty"`
	// MarkItems indicates the marks users want to mark, support Labels and Annotations.
	MarkItems PodMarkerMarkItems `json:"markItems"`
}

// PodMarkerStrategy defines some behaviors when marking Pods
type PodMarkerStrategy struct {
	// Replicas is the number/percentage of Pods that should be marked;
	// If Replicas is nil, will mark all matched Pods;
	// default is nil
	Replicas *intstr.IntOrString `json:"replicas,omitempty"`
	// ConflictPolicy describes the behavior of PodMarker when some matched Pods that already have
	// the marks with the same keys but different values as spec.markItems.
	// ConflictPolicy must be "Ignore" or "Overwrite".
	// "Ignore" indicates that PodMarker will treat these conflicting Pods as unmatched Pods.
	// "Overwrite" indicates that PodMarker will over-write these conflicting marks.
	// default is "Ignore"
	ConflictPolicy PodMarkerConflictPolicyType `json:"conflictPolicy,omitempty"`
}

type PodMarkerConflictPolicyType string

const (
	// PodMarkerConflictIgnore indicates that PodMarker will treat these conflicting Pods as unmatched Pods.
	PodMarkerConflictIgnore PodMarkerConflictPolicyType = "Ignore"
	// PodMarkerConflictOverwrite indicates that the labels or annotations,
	// which have the same keys but different values as spec.MarkItems, will be over-written.
	PodMarkerConflictOverwrite PodMarkerConflictPolicyType = "Overwrite"
)

// PodMarkerRequirements defines the requirements for the Pods that users want to mark.
type PodMarkerRequirements struct {
	// PodLabelSelector describe the pod label requirement for matched Pods.
	//+optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// NodeLabelSelector describe the node label requirement for matched Pods.
	// The matched Pods must run in these requirement-satisfied nodes.
	//+optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// PodMarkerPreference actually describes the priority of Pods.
// PodMarkerPreference tells PodMarker what the Pods having this priority should look like.
type PodMarkerPreference struct {
	// If PodReady is true, the Pods having this priority should be Ready, and vice versa.
	// default is nil
	//+optional
	PodReady *bool `json:"podReady,omitempty"`
	// The Pods having this priority should match this PodSelector.
	// default is nil
	//+optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// The Pods having this priority should run in the nodes that match this NodeSelector.
	// default is nil
	//+optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// PodMarkerMarkItems is the marks that users want to mark.
// PodMarkerMarkItems supports labels and annotations.
type PodMarkerMarkItems struct {
	// The Labels users want to mark
	Labels map[string]string `json:"labels,omitempty"`
	// The Annotations users want to mark
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PodMarkerStatus defines the observed state of PodMarker.
type PodMarkerStatus struct {
	// Desired denotes the number of the Pods that users want to mark.
	// Conflicting Pods ARE NOT counted by this field.
	Desired int32 `json:"desired,omitempty"`
	// Succeeded denotes the number of Pods that have been marked successful.
	Succeeded int32 `json:"succeeded,omitempty"`
	// Failed denotes the number of failures when marking.
	Failed int32 `json:"failed,omitempty"`
	// ObservedGeneration is the most recent generation observed for this PodMarker. It corresponds to the
	// PodMarker's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=marker
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".status.desired",description="The desired number of pods that should be marked."
// +kubebuilder:printcolumn:name="Succeeded",type="integer",JSONPath=".status.succeeded",description="The number of pods that have been marked successful."
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=".status.failed",description="The number of failures when marking."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// PodMarker is the Schema for the podmarkers API
type PodMarker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodMarkerSpec   `json:"spec,omitempty"`
	Status PodMarkerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodMarkerList contains a list of PodMarker
type PodMarkerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodMarker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodMarker{}, &PodMarkerList{})
}
