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

const (
	// PodProbeMarkerAnnotationKey records the Probe Spec, mainly used for serverless Pod scenarios, as follows:
	//  annotations:
	//    kruise.io/podprobe: |
	//      [
	//          {
	//              "containerName": "minecraft",
	//              "name": "healthy",
	//              "podConditionType": "game.kruise.io/healthy",
	//              "probe": {
	//                  "exec": {
	//                      "command": [
	//                          "bash",
	//                          "/data/probe.sh"
	//                      ]
	//                  }
	//              }
	//          }
	//      ]
	PodProbeMarkerAnnotationKey = "kruise.io/podprobe"
	// PodProbeMarkerListAnnotationKey records the injected PodProbeMarker Name List
	// example: kruise.io/podprobemarker-list="probe-marker-1,probe-marker-2"
	PodProbeMarkerListAnnotationKey = "kruise.io/podprobemarker-list"
)

// PodProbeMarkerSpec defines the desired state of PodProbeMarker
type PodProbeMarkerSpec struct {
	// Selector is a label query over pods that should exec custom probe
	// It must match the pod template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector *metav1.LabelSelector `json:"selector"`
	// Custom container probe, current only support Exec().
	// Probe Result will record in Pod.Status.Conditions, and condition.type=probe.name.
	// condition.status=True indicates probe success
	// condition.status=False indicates probe fails
	// +patchMergeKey=name
	// +patchStrategy=merge
	Probes []PodContainerProbe `json:"probes" patchStrategy:"merge" patchMergeKey:"name"`
}

type PodContainerProbe struct {
	// probe name, unique within the Pod(Even between different containers, they cannot be the same)
	Name string `json:"name"`
	// container name
	ContainerName string `json:"containerName"`
	// container probe spec
	Probe ContainerProbeSpec `json:"probe"`
	// According to the execution result of ContainerProbe, perform specific actions,
	// such as: patch Pod labels, annotations, ReadinessGate Condition
	// It cannot be null at the same time as PodConditionType.
	// +patchMergeKey=state
	// +patchStrategy=merge
	MarkerPolicy []ProbeMarkerPolicy `json:"markerPolicy,omitempty"  patchStrategy:"merge" patchMergeKey:"state"`
	// If it is not empty, the Probe execution result will be recorded on the Pod condition.
	// It cannot be null at the same time as MarkerPolicy.
	// For example PodConditionType=game.kruise.io/healthy, pod.status.condition.type = game.kruise.io/healthy.
	// When probe is Succeeded, pod.status.condition.status = True. Otherwise, when the probe fails to execute, pod.status.condition.status = False.
	PodConditionType string `json:"podConditionType,omitempty"`
}

type ContainerProbeSpec struct {
	v1.Probe `json:",inline"`
}

type ProbeMarkerPolicy struct {
	// probe status, True or False
	// For example: State=Succeeded, annotations[controller.kubernetes.io/pod-deletion-cost] = '10'.
	// State=Failed, annotations[controller.kubernetes.io/pod-deletion-cost] = '-10'.
	// In addition, if State=Failed is not defined, Exec execution fails, and the annotations[controller.kubernetes.io/pod-deletion-cost] will be Deleted
	State ProbeState `json:"state"`
	// Patch Labels pod.labels
	Labels map[string]string `json:"labels,omitempty"`
	// Patch annotations pod.annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

type PodProbeMarkerStatus struct {
	// observedGeneration is the most recent generation observed for this PodProbeMarker. It corresponds to the
	// PodProbeMarker's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration"`
	// matched Pods
	MatchedPods int64 `json:"matchedPods,omitempty"`
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
