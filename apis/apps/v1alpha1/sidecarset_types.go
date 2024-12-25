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
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// SidecarSetCustomVersionLabel is designed to record and label the controllerRevision of sidecarSet.
	// This label will be passed from SidecarSet to its corresponding ControllerRevision, users can use
	// this label to selector the ControllerRevision they want.
	// For example, users can update the label from "version-1" to "version-2" when they upgrade the
	// sidecarSet to "version-2", and they write the "version-2" to InjectionStrategy.Revision.CustomVersion
	// when they decided to promote the "version-2", to avoid some risks about gray deployment of SidecarSet.
	SidecarSetCustomVersionLabel = "apps.kruise.io/sidecarset-custom-version"
)

// SidecarSetSpec defines the desired state of SidecarSet
type SidecarSetSpec struct {
	// selector is a label query over pods that should be injected
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Namespace sidecarSet will only match the pods in the namespace
	// otherwise, match pods in all namespaces(in cluster)
	Namespace string `json:"namespace,omitempty"`

	// NamespaceSelector select which namespaces to inject sidecar containers.
	// Default to the empty LabelSelector, which matches everything.
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// InitContainers is the list of init containers to be injected into the selected pod
	// We will inject those containers by their name in ascending order
	// We only inject init containers when a new pod is created, it does not apply to any existing pod
	// +patchMergeKey=name
	// +patchStrategy=merge
	InitContainers []SidecarContainer `json:"initContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name"`

	// Containers is the list of sidecar containers to be injected into the selected pod
	// +patchMergeKey=name
	// +patchStrategy=merge
	Containers []SidecarContainer `json:"containers,omitempty" patchStrategy:"merge" patchMergeKey:"name"`

	// List of volumes that can be mounted by sidecar containers
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +patchMergeKey=name
	// +patchStrategy=merge
	Volumes []corev1.Volume `json:"volumes,omitempty" patchStrategy:"merge" patchMergeKey:"name"`

	// The sidecarset updateStrategy to use to replace existing pods with new ones.
	UpdateStrategy SidecarSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// InjectionStrategy describe the strategy when sidecarset is injected into pods
	InjectionStrategy SidecarSetInjectionStrategy `json:"injectionStrategy,omitempty"`

	// List of the names of secrets required by pulling sidecar container images
	// +patchMergeKey=name
	// +patchStrategy=merge
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty" patchStrategy:"merge" patchMergeKey:"name"`

	// RevisionHistoryLimit indicates the maximum quantity of stored revisions about the SidecarSet.
	// default value is 10
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// SidecarSet support to inject & in-place update metadata in pod.
	PatchPodMetadata []SidecarSetPatchPodMetadata `json:"patchPodMetadata,omitempty"`
}

type SidecarSetPatchPodMetadata struct {
	// annotations
	Annotations map[string]string `json:"annotations,omitempty"`

	// labels map[string]string `json:"labels,omitempty"`
	// patch pod metadata policy, Default is "Retain"
	PatchPolicy SidecarSetPatchPolicyType `json:"patchPolicy,omitempty"`
}

type SidecarSetPatchPolicyType string

var (
	// SidecarSetRetainPatchPolicy indicates if PatchPodFields conflicts with Pod,
	// will ignore PatchPodFields, and retain the corresponding fields of pods.
	// SidecarSet webhook cannot allow the conflict of PatchPodFields between SidecarSets under this policy type.
	// Note: Retain is only supported for injection, and the Metadata will not be updated when upgrading the Sidecar Container in-place.
	SidecarSetRetainPatchPolicy SidecarSetPatchPolicyType = "Retain"

	// SidecarSetOverwritePatchPolicy indicates if PatchPodFields conflicts with Pod,
	// SidecarSet will apply PatchPodFields to overwrite the corresponding fields of pods.
	// SidecarSet webhook cannot allow the conflict of PatchPodFields between SidecarSets under this policy type.
	// Overwrite support to inject and in-place metadata.
	SidecarSetOverwritePatchPolicy SidecarSetPatchPolicyType = "Overwrite"

	// SidecarSetMergePatchJsonPatchPolicy indicate that sidecarSet use application/merge-patch+json to patch annotation value,
	// for example, A patch annotation[oom-score] = '{"log-agent": 1}' and B patch annotation[oom-score] = '{"envoy": 2}'
	// result pod annotation[oom-score] = '{"log-agent": 1, "envoy": 2}'
	// MergePatchJson support to inject and in-place metadata.
	SidecarSetMergePatchJsonPatchPolicy SidecarSetPatchPolicyType = "MergePatchJson"
)

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
	// +optional
	SourceContainerNameFrom *SourceContainerNameSource `json:"sourceContainerNameFrom,omitempty"`
	EnvName                 string                     `json:"envName,omitempty"`
	// +optional
	EnvNames []string `json:"envNames,omitempty"`
}

type SourceContainerNameSource struct {
	// Selects a field of the pod: supports metadata.name, `metadata.labels['<KEY>']`, `metadata.annotations['<KEY>']`,
	FieldRef *corev1.ObjectFieldSelector `json:"fieldRef,omitempty"`
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

	// Revision can help users rolling update SidecarSet safely. If users set
	// this filed, SidecarSet will try to inject specific revision according to
	// different policies.
	Revision *SidecarSetInjectRevision `json:"revision,omitempty"`
}

type SidecarSetInjectRevision struct {
	// CustomVersion corresponds to label 'apps.kruise.io/sidecarset-custom-version' of (History) SidecarSet.
	// SidecarSet will select the specific ControllerRevision via this CustomVersion, and then restore the
	// history SidecarSet to inject specific version of the sidecar to pods.
	// + optional
	CustomVersion *string `json:"customVersion,omitempty"`
	// RevisionName corresponds to a specific ControllerRevision name of SidecarSet that you want to inject to Pods.
	// + optional
	RevisionName *string `json:"revisionName,omitempty"`
	// Policy describes the behavior of revision injection.
	// +kubebuilder:validation:Enum=Always;Partial;
	// +kubebuilder:default=Always
	Policy SidecarSetInjectRevisionPolicy `json:"policy,omitempty"`
}

type SidecarSetInjectRevisionPolicy string

const (
	// AlwaysSidecarSetInjectRevisionPolicy means the SidecarSet will always inject
	// the specific revision to Pods when pod creating, except matching UpdateStrategy.Selector.
	AlwaysSidecarSetInjectRevisionPolicy SidecarSetInjectRevisionPolicy = "Always"

	// PartialSidecarSetInjectRevisionPolicy means the SidecarSet will inject the specific or the latest revision according to UpdateStrategy.
	//
	// If UpdateStrategy.Pause is not true, only when a newly created Pod is **not** selected by the Selector explicitly
	// configured in `UpdateStrategy` will it be injected with the specified version of the Sidecar.
	// Under all other conditions, newly created Pods have a probability of being injected with the latest Sidecar,
	// where the probability is `1 - UpdateStrategy.Partition`.
	// If `Partition` is not a percentage or is not configured, its value is considered to be 0%.
	PartialSidecarSetInjectRevisionPolicy SidecarSetInjectRevisionPolicy = "Partial"
)

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
	// For the impact on the injection behavior for newly created Pods, please refer to the comments of Selector.
	Paused bool `json:"paused,omitempty"`

	// If selector is not nil, this upgrade will only update the selected pods.
	//
	// Starting from Kruise 1.8.0, the updateStrategy.Selector affects the version of the Sidecar container
	// injected into newly created Pods by a SidecarSet configured with an injectionStrategy.
	// In most cases, all newly created Pods are injected with the specified Sidecar version as configured in injectionStrategy.revision,
	// which is consistent with previous versions.
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
	// Priorities are the rules for calculating the priority of updating pods.
	// Each pod to be updated, will pass through these terms and get a sum of weights.
	PriorityStrategy *appspub.UpdatePriorityStrategy `json:"priorityStrategy,omitempty"`
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

	// LatestRevision, if not empty, indicates the latest controllerRevision name of the SidecarSet.
	LatestRevision string `json:"latestRevision,omitempty"`

	// CollisionCount is the count of hash collisions for the SidecarSet. The SidecarSet controller
	// uses this field as a collision avoidance mechanism when it needs to create the name for the
	// newest ControllerRevision.
	CollisionCount *int32 `json:"collisionCount,omitempty"`
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
