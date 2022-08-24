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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePodProbeSpec defines the desired state of NodePodProbe
type NodePodProbeSpec struct {
	PodContainerProbes []PodContainerProbe `json:"podContainerProbes"`
}

type PodContainerProbe struct {
	// pod name
	PodName string `json:"podName"`
	// Custom container probe, supports Exec, Tcp, and returns the result to Pod yaml
	ContainerProbes []ContainerProbe `json:"containerProbes"`
}

type NodePodProbeStatus struct {
	// observedGeneration is the most recent generation observed for this NodePodProbe. It corresponds to the
	// NodePodProbe's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration"`
	// pod probe results
	PodProbeResults []PodContainerProbeResult `json:"podProbeResults"`
}

type PodContainerProbeResult struct {
	PodName string `json:"podName"`
	// container probe result
	ContainerProbes []ContainerProbeResult `json:"containerProbes"`
}

type ContainerProbeResult struct {
	// container name
	Name string `json:"name"`
	// probe results
	Probes []ProbeResult `json:"probes"`
}

type ProbeResult struct {
	// probe name
	Name string `json:"name"`
	// container probe exec state
	Status ProbeStatus `json:"status"`
	// If Status=True, Message records the return result of Probe.
	// If Status=False, Message records Probe's error message
	Message string `json:"message"`
}

type ProbeStatus string

const (
	ProbeTrue    ProbeStatus = "True"
	ProbeFalse   ProbeStatus = "False"
	ProbeUnknown ProbeStatus = "Unknown"
)

// +genclient
// +kubebuilder:object:root=true
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
