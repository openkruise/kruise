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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodProbeMarkerSpec defines the desired state of PodProbeMarker
type PodProbeMarkerSpec struct {
	// Selector is a label query over pods that should exec custom probe
	// It must match the pod template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector *metav1.LabelSelector `json:"selector"`
	// Custom container probe, supports Exec, Tcp, and returns the result to Pod yaml
	ContainerProbes []ContainerProbe `json:"containerProbes"`
	// According to the execution result of ContainerProbe, perform specific actions,
	// such as: patch Pod labels, annotations, ReadinessGate Condition
	MarkerItems []ProbeMarker `json:"markerItems,omitempty"`
}

type ContainerProbe struct {
	// container name
	Name string `json:"name"`
	// container probe spec
	Probes []ContainerProbeSpec `json:"probes"`
}

type ContainerProbeSpec struct {
	// probe name, unique within the Pod
	Name string `json:"name"`
	// The action taken to determine the health of a container
	v1.Handler `json:",inline"`
	// Number of seconds after the container has started before liveness probes are initiated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	// Number of seconds after which the probe times out.
	// Defaults to 1 second. Minimum value is 1.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
	// How often (in seconds) to perform the probe.
	// Default to 10 seconds. Minimum value is 1.
	// +optional
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
	// Minimum consecutive successes for the probe to be considered successful after having failed.
	// Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.
	// +optional
	SuccessThreshold int32 `json:"successThreshold,omitempty"`
	// Minimum consecutive failures for the probe to be considered failed after having succeeded.
	// Defaults to 3. Minimum value is 1.
	// +optional
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
}

type ProbeMarker struct {
	// probe name
	ProbeName string `json:"probeName"`
	// True or False
	Expectation ProbeStatus `json:"expectation"`
	// Labels
	Labels map[string]string `json:"labels,omitempty"`
	// annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

type PodProbeMarkerStatus struct {
	// observedGeneration is the most recent generation observed for this PodProbeMarker. It corresponds to the
	// PodProbeMarker's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PodProbeMarker is the Schema for the PodProbeMarker API
type PodProbeMarker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodProbeMarkerSpec   `json:"spec,omitempty"`
	Status PodProbeMarkerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PodProbeMarkerList contains a list of PodProbeMarker
type PodProbeMarkerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodProbeMarker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodProbeMarker{}, &PodProbeMarkerList{})
}
