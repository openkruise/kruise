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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ConfigMapSetSpec defines the desired state of ConfigMapSet
type ConfigMapSetSpec struct {
	// CustomVersion is an optional string to provide a user-defined version or alias
	// for the current configuration data. It can be used for version tracking.
	// +optional
	CustomVersion string `json:"customVersion,omitempty"`

	// Selector is a label query over Pods that should be managed by this ConfigMapSet.
	// Only Pods matching this selector will have their configurations updated.
	Selector *metav1.LabelSelector `json:"selector"`

	// Data contains the actual configuration files (key-value pairs) to be distributed
	// to the matched Pods. Keys are file names, and values are the file contents.
	// +optional
	Data map[string]string `json:"data,omitempty"`

	// Containers defines the list of business containers within the matched Pods
	// that need to mount the configuration data.
	// +optional
	Containers []ConfigMapSetContainer `json:"containers,omitempty"`

	// ReloadSidecarConfig configures the sidecar container responsible for dynamically
	// receiving configuration updates and sharing them with the business containers.
	// +optional
	ReloadSidecarConfig *ReloadSidecarConfig `json:"reloadSidecarConfig,omitempty"`

	// EffectPolicy defines the strategy for how configuration updates should be applied
	// to the Pods (e.g., restarting containers, hot updating, or triggering webhooks).
	// +optional
	EffectPolicy *EffectPolicy `json:"effectPolicy,omitempty"`

	// RevisionHistoryLimit indicates the maximum number of old configuration revisions
	// to retain for rollback purposes.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// UpdateStrategy defines the rules for rolling out configuration updates to
	// the matched Pods, such as partition, maxUnavailable, and grouping.
	// +optional
	UpdateStrategy *ConfigMapSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

// ConfigMapSetContainer specifies a target container and where the configuration should be mounted.
type ConfigMapSetContainer struct {
	// Name of the target container to mount the configuration.
	Name string `json:"name,omitempty"`
	// NameFrom provides a dynamic way to determine the container name (e.g., via Downward API)
	// to support dynamically injected containers like SidecarSets.
	NameFrom *SourceContainerNameSource `json:"nameFrom,omitempty"`
	// MountPath is the directory path inside the container where the configuration files will be mounted.
	MountPath string `json:"mountPath,omitempty"`
}

// ReloadSidecarConfig specifies the configuration for the reload-sidecar.
type ReloadSidecarConfig struct {
	// Type of the reload sidecar config. Can be "k8s" (native pod template), "sidecarset" (injected via Kruise SidecarSet), or "custom" (referenced via ConfigMap).
	Type ReloadSidecarType `json:"type,omitempty"`
	// Config provides the detailed configuration parameters for the chosen sidecar type.
	Config *ReloadSidecarConfigData `json:"config,omitempty"`
}

// ReloadSidecarType indicates the type of reload sidecar implementation.
type ReloadSidecarType string

const (
	ReloadSidecarTypeK8s        ReloadSidecarType = "k8s"
	ReloadSidecarTypeSidecarSet ReloadSidecarType = "sidecarset"
	ReloadSidecarTypeCustom     ReloadSidecarType = "custom"
)

// ReloadSidecarConfigData contains the specific configuration details for the chosen ReloadSidecarType.
type ReloadSidecarConfigData struct {
	// Name specifies the container name when using the "k8s" type.
	Name string `json:"name,omitempty"`
	// Image specifies the container image when using the "k8s" type.
	Image string `json:"image,omitempty"`
	// RestartPolicy specifies the container restart policy when using the "k8s" type.
	RestartPolicy corev1.ContainerRestartPolicy `json:"restartPolicy,omitempty"`
	// Command specifies the container execution command when using the "k8s" type.
	Command []string `json:"command,omitempty"`

	// SidecarSetRef references an OpenKruise SidecarSet when using the "sidecarset" type.
	SidecarSetRef *SidecarSetRef `json:"sidecarSetRef,omitempty"`

	// ConfigMapRef references a ConfigMap containing the container configuration when using the "custom" type.
	ConfigMapRef *ConfigMapRef `json:"configMapRef,omitempty"`
}

// SidecarSetRef specifies the target sidecar container when using SidecarSet.
type SidecarSetRef struct {
	// Name is the name of the SidecarSet.
	Name string `json:"name,omitempty"`
	// ContainerName is the name of the container within the SidecarSet to be used.
	ContainerName string `json:"containerName,omitempty"`
}

// ConfigMapRef specifies a reference to a ConfigMap.
type ConfigMapRef struct {
	// Name specifies the name of the referenced ConfigMap.
	Name string `json:"name,omitempty"`
	// Namespace allows referencing a ConfigMap from a different namespace.
	// This is by design to allow configuration generalization, where a single ConfigMap
	// can be shared and referenced by ConfigMapSets across multiple namespaces without duplication.
	// Security Implications & RBAC: The kruise-manager requires cluster-level 'get' permission
	// for ConfigMaps to resolve cross-namespace references. This permission is already included
	// in the default controller RBAC manifests.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// EffectPolicy defines how the configuration update takes effect in the pod.
type EffectPolicy struct {
	// Type indicates the method of applying updates.
	// "Restart": Restarts the reload-sidecar first, then restarts business containers.
	// "PostHook": Restarts the reload-sidecar, then triggers HTTP/TCP callbacks to business containers.
	// "HotUpdate": Neither container restarts; reload-sidecar updates shared files and business containers hot-reload.
	Type EffectPolicyType `json:"type,omitempty"`
	// PostHook defines the callback actions to trigger business containers to reload configurations
	// when Type is "PostHook".
	PostHook *PostHookConfig `json:"postHook,omitempty"`
}

// EffectPolicyType specifies the strategy used to apply configuration updates.
type EffectPolicyType string

const (
	EffectPolicyTypeRestart   EffectPolicyType = "Restart"
	EffectPolicyTypePostHook  EffectPolicyType = "PostHook"
	EffectPolicyTypeHotUpdate EffectPolicyType = "HotUpdate"
)

// PostHookConfig specifies actions to be executed after a configuration update.
type PostHookConfig struct {
	// HTTPGet defines an HTTP GET request to perform as a post-hook action.
	HTTPGet []*corev1.HTTPGetAction `json:"httpGet,omitempty"`
	// TCPSocket defines a TCP socket connection to perform as a post-hook action.
	TCPSocket []*corev1.TCPSocketAction `json:"tcpSocket,omitempty"`
}

// ConfigMapSetUpdateStrategy defines the rollout strategy for configuration updates.
type ConfigMapSetUpdateStrategy struct {
	// Partition indicates the desired number or percentage of Pods in old revisions.
	// Pods beyond this partition will be updated to the new revision.
	Partition *intstr.IntOrString `json:"partition,omitempty"`
	// MaxUnavailable is the maximum number of Pods that can be unavailable during the update.
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	// MatchLabelKeys allows grouping matched Pods by label keys. The rollout strategy
	// (Partition/MaxUnavailable) will be applied independently within each group.
	MatchLabelKeys []string `json:"matchLabelKeys,omitempty"`
}

// ConfigMapSetStatus defines the observed state of ConfigMapSet
type ConfigMapSetStatus struct {
	// CurrentCustomVersion tracks the user-defined version alias of the currently applied configuration.
	CurrentCustomVersion string `json:"currentCustomVersion,omitempty"`
	// CurrentRevision is the system-generated hash revision of the currently applied configuration.
	CurrentRevision string `json:"currentRevision,omitempty"`
	// ExpectedUpdatedReplicas is the total number of matched Pods that are expected to be updated.
	ExpectedUpdatedReplicas int32 `json:"expectedUpdatedReplicas,omitempty"`
	// ObservedGeneration is the most recent generation observed by the ConfigMapSet controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ReadyReplicas is the number of matched Pods that have a Ready Condition.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// Replicas is the total number of Pods that match the ConfigMapSet selector.
	Replicas int32 `json:"replicas,omitempty"`
	// UpdateCustomVersion tracks the user-defined version alias of the configuration being rolled out.
	UpdateCustomVersion string `json:"updateCustomVersion,omitempty"`
	// UpdateRevision is the system-generated hash revision of the configuration being rolled out.
	UpdateRevision string `json:"updateRevision,omitempty"`
	// UpdatedReadyReplicas is the number of matched Pods that have been updated to the latest revision and are Ready.
	UpdatedReadyReplicas int32 `json:"updatedReadyReplicas,omitempty"`
	// UpdatedReplicas is the total number of matched Pods that have been updated to the latest revision.
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`
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

	// Spec represents the desired behavior of the ConfigMapSet.
	Spec ConfigMapSetSpec `json:"spec,omitempty"`
	// Status represents the current observed state of the ConfigMapSet.
	Status ConfigMapSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfigMapSetList contains a list of ConfigMapSet
type ConfigMapSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is the list of ConfigMapSets.
	Items []ConfigMapSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigMapSet{}, &ConfigMapSetList{})
}
