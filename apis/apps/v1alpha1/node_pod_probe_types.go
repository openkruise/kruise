/*
Copyright 2022 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless persistent by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePodProbeSpec defines the desired state of NodePodProbe
type NodePodProbeSpec struct {
	PodProbes []PodProbe `json:"podProbes,omitempty"`
}

type PodProbe struct {
	// pod name
	Name string `json:"name"`
	// pod namespace
	Namespace string `json:"namespace"`
	// pod uid
	UID string `json:"uid"`
	// pod ip
	IP string `json:"IP"`
	// Custom container probe, supports Exec, Tcp, and returns the result to Pod yaml
	Probes []ContainerProbe `json:"probes,omitempty"`
}

type ContainerProbe struct {
	// Name is podProbeMarker.Name#probe.Name
	Name string `json:"name"`
	// container name
	ContainerName string `json:"containerName"`
	// container probe spec
	Probe ContainerProbeSpec `json:"probe"`
}

type NodePodProbeStatus struct {
	// pod probe results
	PodProbeStatuses []PodProbeStatus `json:"podProbeStatuses,omitempty"`
}

type PodProbeStatus struct {
	// pod name
	Name string `json:"name"`
	// pod namespace
	Namespace string `json:"namespace"`
	// pod uid
	UID string `json:"uid"`
	// pod probe result
	ProbeStates []ContainerProbeState `json:"probeStates,omitempty"`
}

type ContainerProbeState struct {
	// Name is podProbeMarker.Name#probe.Name
	Name string `json:"name"`
	// container probe exec state, True or False
	State ProbeState `json:"state"`
	// Last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// If Status=True, Message records the return result of Probe.
	// If Status=False, Message records Probe's error message
	Message string `json:"message,omitempty"`
}

type ProbeState string

const (
	ProbeSucceeded ProbeState = "Succeeded"
	ProbeFailed    ProbeState = "Failed"
	ProbeUnknown   ProbeState = "Unknown"
)

func (p ProbeState) IsEqualPodConditionStatus(status corev1.ConditionStatus) bool {
	switch status {
	case corev1.ConditionTrue:
		return p == ProbeSucceeded
	case corev1.ConditionFalse:
		return p == ProbeFailed
	default:
		return p == ProbeUnknown
	}
}

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status

// NodePodProbe is the Schema for the NodePodProbe API
type NodePodProbe struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePodProbeSpec   `json:"spec,omitempty"`
	Status NodePodProbeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodePodProbeList contains a list of NodePodProbe
type NodePodProbeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePodProbe `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodePodProbe{}, &NodePodProbeList{})
}
