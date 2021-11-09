/*
Copyright 2020 The Kruise Authors.

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

// SidecarSetSpec defines the desired state of SidecarSet
type SidecarSetSpec struct {
	// selector is a label query over pods that should be injected
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Namespace sidecarSet will only match the pods in the namespace
	// otherwise, match pods in all namespaces(in cluster)
	Namespace string `json:"namespace,omitempty"`

	// Containers is the list of init containers to be injected into the selected pod
	// We will inject those containers by their name in ascending order
	// We only inject init containers when a new pod is created, it does not apply to any existing pod
	InitContainers []SidecarContainer `json:"initContainers,omitempty"`

	// Containers is the list of sidecar containers to be injected into the selected pod
	Containers []SidecarContainer `json:"containers,omitempty"`

	// List of volumes that can be mounted by sidecar containers
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// The sidecarset updateStrategy to use to replace existing pods with new ones.
	UpdateStrategy SidecarSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// InjectionStrategy describe the strategy when sidecarset is injected into pods
	InjectionStrategy SidecarSetInjectionStrategy `json:"injectionStrategy,omitempty"`

	// List of the names of secrets required by pulling sidecar container images
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// SidecarContainer defines the container of Sidecar
type SidecarContainer struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	corev1.Container `json:",inline"`

	// The rules that injected SidecarContainer into Pod.spec.containers,
	// not takes effect in initContainers
	// If BeforeAppContainer, the SidecarContainer will be injected in front of the pod.spec.containers
	// otherwise it will be injected into the back.
	// default BeforeAppContainerType
	PodInjectPolicy PodInjectPolicyType `json:"podInjectPolicy,omitempty"`

	// sidecarContainer upgrade strategy, include: ColdUpgrade, HotUpgrade
	UpgradeStrategy SidecarContainerUpgradeStrategy `json:"upgradeStrategy,omitempty"`

	// If ShareVolumePolicy is enabled, the sidecar container will share the other container's VolumeMounts
	// in the pod(don't contains the injected sidecar container).
	ShareVolumePolicy ShareVolumePolicy `json:"shareVolumePolicy,omitempty"`

	// TransferEnv will transfer env info from other container
	// SourceContainerName is pod.spec.container[x].name; EnvName is pod.spec.container[x].Env.name
	TransferEnv []TransferEnvVar `json:"transferEnv,omitempty"`
}

type ShareVolumePolicy struct {
	Type ShareVolumePolicyType `json:"type,omitempty"`
}

type PodInjectPolicyType string

const (
	BeforeAppContainerType PodInjectPolicyType = "BeforeAppContainer"
	AfterAppContainerType  PodInjectPolicyType = "AfterAppContainer"
)

type ShareVolumePolicyType string

const (
	ShareVolumePolicyEnabled  ShareVolumePolicyType = "enabled"
	ShareVolumePolicyDisabled ShareVolumePolicyType = "disabled"
)

type TransferEnvVar struct {
	SourceContainerName string `json:"sourceContainerName,omitempty"`
	EnvName             string `json:"envName,omitempty"`
}

type SidecarContainerUpgradeType string

const (
	SidecarContainerColdUpgrade SidecarContainerUpgradeType = "ColdUpgrade"
	SidecarContainerHotUpgrade  SidecarContainerUpgradeType = "HotUpgrade"
)

type SidecarContainerUpgradeStrategy struct {
	// when sidecar container is stateless, use ColdUpgrade
	// otherwise HotUpgrade are more HotUpgrade.
	// examples for istio envoy container is suitable for HotUpgrade
	// default is ColdUpgrade
	UpgradeType SidecarContainerUpgradeType `json:"upgradeType,omitempty"`

	// when HotUpgrade, HotUpgradeEmptyImage is used to complete the hot upgrading process
	// HotUpgradeEmptyImage is consistent of sidecar container in Command, Args, Liveness probe, etc.
	// but it does no actual work.
	HotUpgradeEmptyImage string `json:"hotUpgradeEmptyImage,omitempty"`
}

// SidecarSetInjectionStrategy indicates the injection strategy of SidecarSet.
type SidecarSetInjectionStrategy struct {
	// Paused indicates that SidecarSet will suspend injection into Pods
	// If Paused is true, the sidecarSet will not be injected to newly created Pods,
	// but the injected sidecar container remains updating and running.
	// default is false
	Paused bool `json:"paused,omitempty"`
}

// SidecarSetUpdateStrategy indicates the strategy that the SidecarSet
// controller will use to perform updates. It includes any additional parameters
// necessary to perform the update for the indicated strategy.
type SidecarSetUpdateStrategy struct {
	// Type is NotUpdate, the SidecarSet don't update the injected pods,
	// it will only inject sidecar container into the newly created pods.
	// Type is RollingUpdate, the SidecarSet will update the injected pods to the latest version on RollingUpdate Strategy.
	// default is RollingUpdate
	Type SidecarSetUpdateStrategyType `json:"type,omitempty"`

	// Paused indicates that the SidecarSet is paused to update the injected pods,
	// but it don't affect the webhook inject sidecar container into the newly created pods.
	// default is false
	Paused bool `json:"paused,omitempty"`

	// If selector is not nil, this upgrade will only update the selected pods.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Partition is the desired number of pods in old revisions. It means when partition
	// is set during pods updating, (replicas - partition) number of pods will be updated.
	// Default value is 0.
	Partition *intstr.IntOrString `json:"partition,omitempty"`

	// The maximum number of SidecarSet pods that can be unavailable during the
	// update. Value can be an absolute number (ex: 5) or a percentage of total
	// number of SidecarSet pods at the start of the update (ex: 10%). Absolute
	// number is calculated from percentage by rounding up.
	// This cannot be 0.
	// Default value is 1.
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// ScatterStrategy defines the scatter rules to make pods been scattered when update.
	// This will avoid pods with the same key-value to be updated in one batch.
	// - Note that pods will be scattered after priority sort. So, although priority strategy and scatter strategy can be applied together, we suggest to use either one of them.
	// - If scatterStrategy is used, we suggest to just use one term. Otherwise, the update order can be hard to understand.
	ScatterStrategy UpdateScatterStrategy `json:"scatterStrategy,omitempty"`
}

type SidecarSetUpdateStrategyType string

const (
	NotUpdateSidecarSetStrategyType     SidecarSetUpdateStrategyType = "NotUpdate"
	RollingUpdateSidecarSetStrategyType SidecarSetUpdateStrategyType = "RollingUpdate"
)

// SidecarSetStatus defines the observed state of SidecarSet
type SidecarSetStatus struct {
	// observedGeneration is the most recent generation observed for this SidecarSet. It corresponds to the
	// SidecarSet's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// matchedPods is the number of Pods whose labels are matched with this SidecarSet's selector and are created after sidecarset creates
	MatchedPods int32 `json:"matchedPods"`

	// updatedPods is the number of matched Pods that are injected with the latest SidecarSet's containers
	UpdatedPods int32 `json:"updatedPods"`

	// readyPods is the number of matched Pods that have a ready condition
	ReadyPods int32 `json:"readyPods"`

	// updatedReadyPods is the number of matched pods that updated and ready
	UpdatedReadyPods int32 `json:"updatedReadyPods,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MATCHED",type="integer",JSONPath=".status.matchedPods",description="The number of pods matched."
// +kubebuilder:printcolumn:name="UPDATED",type="integer",JSONPath=".status.updatedPods",description="The number of pods matched and updated."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyPods",description="The number of pods matched and ready."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// SidecarSet is the Schema for the sidecarsets API
type SidecarSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SidecarSetSpec   `json:"spec,omitempty"`
	Status SidecarSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SidecarSetList contains a list of SidecarSet
type SidecarSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SidecarSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SidecarSet{}, &SidecarSetList{})
}
