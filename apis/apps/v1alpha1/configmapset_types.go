/*
Copyright 2024 The Kruise Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ConfigMapSetSpec defines the desired state of ConfigMapSet
type ConfigMapSetSpec struct {
	// CustomVersion alias for the current update revision
	// +optional
	CustomVersion string `json:"customVersion,omitempty"`

	// Selector is a label query over pods that should be updated
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Data contains the configuration data to be updated
	// +optional
	Data map[string]string `json:"data,omitempty"`

	// Containers defines the business containers that need to be updated
	// +optional
	Containers []ConfigMapSetContainer `json:"containers,omitempty"`

	// ReloadSidecarConfig defines the container injected during Pod creation to update configuration files
	// +optional
	ReloadSidecarConfig *ReloadSidecarConfig `json:"reloadSidecarConfig,omitempty"`

	// EffectPolicy defines how the configuration update takes effect
	// +optional
	EffectPolicy *EffectPolicy `json:"effectPolicy,omitempty"`

	// RevisionHistoryLimit indicates the maximum quantity of stored revisions about the ConfigMapSet
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// UpdateStrategy indicates the strategy that the ConfigMapSet controller will use to perform updates
	// +optional
	UpdateStrategy *ConfigMapSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

type ConfigMapSetContainer struct {
	Name      string                     `json:"name,omitempty"`
	NameFrom  *SourceContainerNameSource `json:"nameFrom,omitempty"`
	MountPath string                     `json:"mountPath,omitempty"`
}

type ReloadSidecarConfig struct {
	// Type of the reload sidecar config (k8s, sidecarset, custom)
	Type ReloadSidecarType `json:"type,omitempty"`
	// Config provides the configuration for the chosen type
	Config *ReloadSidecarConfigData `json:"config,omitempty"`
}

type ReloadSidecarType string

const (
	ReloadSidecarTypeK8s        ReloadSidecarType = "k8s"
	ReloadSidecarTypeSidecarSet ReloadSidecarType = "sidecarset"
	ReloadSidecarTypeCustom     ReloadSidecarType = "custom"
)

type ReloadSidecarConfigData struct {
	// Name for k8s type
	Name string `json:"name,omitempty"`
	// Image for k8s type
	Image string `json:"image,omitempty"`
	// RestartPolicy for k8s type
	RestartPolicy corev1.ContainerRestartPolicy `json:"restartPolicy,omitempty"`
	// Command for k8s type
	Command []string `json:"command,omitempty"`

	// SidecarSetRef for sidecarset type
	SidecarSetRef *SidecarSetRef `json:"sidecarSetRef,omitempty"`

	// ConfigMapRef for custom type
	ConfigMapRef *ConfigMapRef `json:"configMapRef,omitempty"`
}

type SidecarSetRef struct {
	Name          string `json:"name,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
}

type ConfigMapRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type EffectPolicy struct {
	Type     EffectPolicyType `json:"type,omitempty"`
	PostHook *PostHookConfig  `json:"postHook,omitempty"`
}

type EffectPolicyType string

const (
	EffectPolicyTypeReStart   EffectPolicyType = "ReStart"
	EffectPolicyTypePostHook  EffectPolicyType = "PostHook"
	EffectPolicyTypeHotUpdate EffectPolicyType = "HotUpdate"
)

type PostHookConfig struct {
	HTTPGet   *corev1.HTTPGetAction   `json:"httpGet,omitempty"`
	TCPSocket *corev1.TCPSocketAction `json:"tcpSocket,omitempty"`
}

type ConfigMapSetUpdateStrategy struct {
	Partition      *intstr.IntOrString `json:"partition,omitempty"`
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	MatchLabelKeys []string            `json:"matchLabelKeys,omitempty"`
}

// ConfigMapSetStatus defines the observed state of ConfigMapSet
type ConfigMapSetStatus struct {
	CurrentCustomVersion    string `json:"currentCustomVersion,omitempty"`
	CurrentRevision         string `json:"currentRevision,omitempty"`
	ExpectedUpdatedReplicas int32  `json:"expectedUpdatedReplicas,omitempty"`
	ObservedGeneration      int64  `json:"observedGeneration,omitempty"`
	ReadyReplicas           int32  `json:"readyReplicas,omitempty"`
	Replicas                int32  `json:"replicas,omitempty"`
	UpdateCustomVersion     string `json:"updateCustomVersion,omitempty"`
	UpdateRevision          string `json:"updateRevision,omitempty"`
	UpdatedReadyReplicas    int32  `json:"updatedReadyReplicas,omitempty"`
	UpdatedReplicas         int32  `json:"updatedReplicas,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="REPLICAS",type="integer",JSONPath=".status.replicas",description="The number of pods matched."
// +kubebuilder:printcolumn:name="UPDATED",type="integer",JSONPath=".status.updatedReplicas",description="The number of pods matched and updated."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyReplicas",description="The number of pods matched and ready."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created."

// ConfigMapSet is the Schema for the configmapsets API
type ConfigMapSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigMapSetSpec   `json:"spec,omitempty"`
	Status ConfigMapSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfigMapSetList contains a list of ConfigMapSet
type ConfigMapSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigMapSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigMapSet{}, &ConfigMapSetList{})
}
