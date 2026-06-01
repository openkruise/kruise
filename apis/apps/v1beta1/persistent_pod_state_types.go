/*
Copyright 2026 The Kruise Authors.

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
)

// PersistentPodStateSpec defines the desired state of PersistentPodState
type PersistentPodStateSpec struct {
	// TargetReference contains enough information to let you identify a workload for PersistentPodState
	// Selector and TargetReference are mutually exclusive, TargetReference is priority to take effect
	// current only support StatefulSet
	TargetReference TargetReference `json:"targetRef"`

	// Persist the annotations information of the pods that need to be saved
	PersistentPodAnnotations []PersistentPodAnnotation `json:"persistentPodAnnotations,omitempty"`

	// Pod rebuilt topology required for node labels
	// for example kubernetes.io/hostname, failure-domain.beta.kubernetes.io/zone
	RequiredPersistentTopology *NodeTopologyTerm `json:"requiredPersistentTopology,omitempty"`

	// Pod rebuilt topology preferred for node labels, with xx weight
	// for example  kubernetes.io/hostname, failure-domain.beta.kubernetes.io/zone
	PreferredPersistentTopology []PreferredTopologyTerm `json:"preferredPersistentTopology,omitempty"`

	// PersistentPodStateRetentionPolicy describes the policy used for PodState.
	// The default policy of 'WhenScaled' causes when scale down statefulSet, deleting it.
	// +optional
	PersistentPodStateRetentionPolicy PersistentPodStateRetentionPolicyType `json:"persistentPodStateRetentionPolicy,omitempty"`
}

type PreferredTopologyTerm struct {
	// +kubebuilder:validation:Minimum=0
	Weight     int32            `json:"weight"`
	Preference NodeTopologyTerm `json:"preference"`
}

type NodeTopologyTerm struct {
	// A list of node label keys used for topology persistence.
	Keys []string `json:"keys"`
}

type PersistentPodAnnotation struct {
	Key string `json:"key"`
}

// +kubebuilder:validation:Enum=WhenScaled;WhenDeleted
type PersistentPodStateRetentionPolicyType string

const (
	PersistentPodStateRetentionPolicyWhenScaled  = "WhenScaled"
	PersistentPodStateRetentionPolicyWhenDeleted = "WhenDeleted"
)

type PersistentPodStateStatus struct {
	// observedGeneration is the most recent generation observed for this PersistentPodState. It corresponds to the
	// PersistentPodState's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration"`
	// When the pod is ready, record some status information of the pod, such as: labels, annotations, topologies, etc.
	// map[string]PodState -> map[Pod.Name]PodState
	PodStates map[string]PodState `json:"podStates,omitempty"`
}

type PodState struct {
	// pod.spec.nodeName
	NodeName string `json:"nodeName,omitempty"`
	// node topology labels key=value
	// for example kubernetes.io/hostname=node-1
	NodeTopologyLabels map[string]string `json:"nodeTopologyLabels,omitempty"`
	// pod persistent annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

// TargetReference contains enough information to let you identify a workload
type TargetReference struct {
	// API version of the referent.
	APIVersion string `json:"apiVersion"`
	// Kind of the referent.
	Kind string `json:"kind"`
	// Name of the referent.
	Name string `json:"name"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// PersistentPodState is the Schema for the PersistentPodState API
type PersistentPodState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PersistentPodStateSpec   `json:"spec,omitempty"`
	Status PersistentPodStateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PersistentPodStateList contains a list of PersistentPodState
type PersistentPodStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PersistentPodState `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PersistentPodState{}, &PersistentPodStateList{})
}
