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

package v1beta1

import (
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// MaxMinReadySeconds is the max value of MinReadySeconds
	MaxMinReadySeconds = 300
)

// VolumeClaimUpdateStrategyType defines the update strategy types for volume claims.
// It is an enumerated type that provides two different update strategies.
// +enum
type VolumeClaimUpdateStrategyType string

const (
	// OnPodRollingUpdateVolumeClaimUpdateStrategyType indicates that volume claim updates are triggered when associated Pods undergo rolling updates.
	// This strategy ensures that storage availability and integrity are maintained during the update process.
	OnPodRollingUpdateVolumeClaimUpdateStrategyType VolumeClaimUpdateStrategyType = "OnPodRollingUpdate"

	// OnPVCDeleteVolumeClaimUpdateStrategyType indicates that updates are triggered when a Persistent Volume Claim (PVC) is deleted.
	// This strategy places full control of the update timing in the hands of the user, typically executed after ensuring data has been backed up or there are no data security concerns,
	// allowing for storage resource management that aligns with specific user requirements and security policies.
	OnPVCDeleteVolumeClaimUpdateStrategyType VolumeClaimUpdateStrategyType = "OnDelete"
)

// VolumeClaimStatus describes the status of a volume claim template.
// It provides details about the compatibility and readiness of the volume claim.
type VolumeClaimStatus struct {
	// VolumeClaimName is the name of the volume claim.
	// This is a unique identifier used to reference a specific volume claim.
	VolumeClaimName string `json:"volumeClaimName"`
	// CompatibleReplicas is the number of replicas currently compatible with the volume claim.
	// It indicates how many replicas can function properly, being compatible with this volume claim.
	// Compatibility is determined by whether the PVC spec storage requests are greater than or equal to the template spec storage requests
	CompatibleReplicas int32 `json:"compatibleReplicas"`
	// CompatibleReadyReplicas is the number of replicas that are both ready and compatible with the volume claim.
	// It highlights that these replicas are not only compatible but also ready to be put into service immediately.
	// Compatibility is determined by whether the pvc spec storage requests are greater than or equal to the template spec storage requests
	// The "ready" status is determined by whether the PVC status capacity is greater than or equal to the PVC spec storage requests.
	CompatibleReadyReplicas int32 `json:"compatibleReadyReplicas"`
}

// StatefulSetUpdateStrategy indicates the strategy that the StatefulSet
// controller will use to perform updates. It includes any additional parameters
// necessary to perform the update for the indicated strategy.
type StatefulSetUpdateStrategy struct {
	// Type indicates the type of the StatefulSetUpdateStrategy.
	// Default is RollingUpdate.
	// +optional
	Type apps.StatefulSetUpdateStrategyType `json:"type,omitempty"`
	// RollingUpdate is used to communicate parameters when Type is RollingUpdateStatefulSetStrategyType.
	// +optional
	RollingUpdate *RollingUpdateStatefulSetStrategy `json:"rollingUpdate,omitempty"`
}

// VolumeClaimUpdateStrategy defines the strategy for updating volume claims.
// This structure is used to control how updates to PersistentVolumeClaims are handled during pod rolling updates or PersistentVolumeClaim deletions.
type VolumeClaimUpdateStrategy struct {
	// Type specifies the type of update strategy, possible values include:
	// OnPodRollingUpdateVolumeClaimUpdateStrategyType: Apply the update strategy during pod rolling updates.
	// OnPVCDeleteVolumeClaimUpdateStrategyType: Apply the update strategy when a PersistentVolumeClaim is deleted.
	Type VolumeClaimUpdateStrategyType `json:"type,omitempty"`
}

// RollingUpdateStatefulSetStrategy is used to communicate parameter for RollingUpdateStatefulSetStrategyType.
type RollingUpdateStatefulSetStrategy struct {
	// Partition indicates the number of pods the StatefulSet should be partitioned by default.
	//   - It means controller will update $(replicas - partition) number of pod.
	// Default value is 0.
	// +optional
	Partition *int32 `json:"partition,omitempty"`
	// The maximum number of pods that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// Also, maxUnavailable can just be allowed to work with Parallel podManagementPolicy.
	// Defaults to 1.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	// PodUpdatePolicy indicates how pods should be updated
	// Default value is "ReCreate"
	// +optional
	PodUpdatePolicy PodUpdateStrategyType `json:"podUpdatePolicy,omitempty"`
	// Paused indicates that the StatefulSet is paused.
	// Default value is false
	// +optional
	Paused bool `json:"paused,omitempty"`
	// UnorderedUpdate contains strategies for non-ordered update.
	// If it is not nil, pods will be updated with non-ordered sequence.
	// Noted that UnorderedUpdate can only be allowed to work with Parallel podManagementPolicy
	// +optional
	UnorderedUpdate *UnorderedUpdateStrategy `json:"unorderedUpdate,omitempty"`
	// InPlaceUpdateStrategy contains strategies for in-place update.
	// +optional
	InPlaceUpdateStrategy *appspub.InPlaceUpdateStrategy `json:"inPlaceUpdateStrategy,omitempty"`
	// MinReadySeconds indicates how long will the pod be considered ready after it's updated.
	// MinReadySeconds works with both OrderedReady and Parallel podManagementPolicy.
	// It affects the pod scale up speed when the podManagementPolicy is set to be OrderedReady.
	// Combined with MaxUnavailable, it affects the pod update speed regardless of podManagementPolicy.
	// Default value is 0, max is 300.
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`
}

// UnorderedUpdateStrategy defines strategies for non-ordered update.
type UnorderedUpdateStrategy struct {
	// Priorities are the rules for calculating the priority of updating pods.
	// Each pod to be updated, will pass through these terms and get a sum of weights.
	// +optional
	PriorityStrategy *appspub.UpdatePriorityStrategy `json:"priorityStrategy,omitempty"`
}

// PodUpdateStrategyType is a string enumeration type that enumerates
// all possible ways we can update a Pod when updating application
type PodUpdateStrategyType string

const (
	// RecreatePodUpdateStrategyType indicates that we always delete Pod and create new Pod
	// during Pod update, which is the default behavior
	RecreatePodUpdateStrategyType PodUpdateStrategyType = "ReCreate"
	// InPlaceIfPossiblePodUpdateStrategyType indicates that we try to in-place update Pod instead of
	// recreating Pod when possible. Currently, only image update of pod spec is allowed. Any other changes to the pod
	// spec will fall back to ReCreate PodUpdateStrategyType where pod will be recreated.
	InPlaceIfPossiblePodUpdateStrategyType PodUpdateStrategyType = "InPlaceIfPossible"
	// InPlaceOnlyPodUpdateStrategyType indicates that we will in-place update Pod instead of
	// recreating pod. Currently we only allow image update for pod spec. Any other changes to the pod spec will be
	// rejected by kube-apiserver
	InPlaceOnlyPodUpdateStrategyType PodUpdateStrategyType = "InPlaceOnly"
)

// PersistentVolumeClaimRetentionPolicyType is a string enumeration of the policies that will determine
// when volumes from the VolumeClaimTemplates will be deleted when the controlling StatefulSet is
// deleted or scaled down.
type PersistentVolumeClaimRetentionPolicyType string

const (
	// RetainPersistentVolumeClaimRetentionPolicyType is the default
	// PersistentVolumeClaimRetentionPolicy and specifies that
	// PersistentVolumeClaims associated with StatefulSet VolumeClaimTemplates
	// will not be deleted.
	RetainPersistentVolumeClaimRetentionPolicyType PersistentVolumeClaimRetentionPolicyType = "Retain"
	// DeletePersistentVolumeClaimRetentionPolicyType specifies that
	// PersistentVolumeClaims associated with StatefulSet VolumeClaimTemplates
	// will be deleted in the scenario specified in
	// StatefulSetPersistentVolumeClaimPolicy.
	DeletePersistentVolumeClaimRetentionPolicyType PersistentVolumeClaimRetentionPolicyType = "Delete"
)

// StatefulSetPersistentVolumeClaimRetentionPolicy describes the policy used for PVCs
// created from the StatefulSet VolumeClaims.
type StatefulSetPersistentVolumeClaimRetentionPolicy struct {
	// WhenDeleted specifies what happens to PVCs created from StatefulSet
	// VolumeClaimTemplates when the StatefulSet is deleted. The default policy
	// of `Retain` causes PVCs to not be affected by StatefulSet deletion. The
	// `Delete` policy causes those PVCs to be deleted.
	WhenDeleted PersistentVolumeClaimRetentionPolicyType `json:"whenDeleted,omitempty"`
	// WhenScaled specifies what happens to PVCs created from StatefulSet
	// VolumeClaimTemplates when the StatefulSet is scaled down. The default
	// policy of `Retain` causes PVCs to not be affected by a scaledown. The
	// `Delete` policy causes the associated PVCs for any excess pods above
	// the replica count to be deleted.
	WhenScaled PersistentVolumeClaimRetentionPolicyType `json:"whenScaled,omitempty"`
}

// StatefulSetOrdinals describes the policy used for replica ordinal assignment
// in this StatefulSet.
type StatefulSetOrdinals struct {
	// start is the number representing the first replica's index. It may be used
	// to number replicas from an alternate index (eg: 1-indexed) over the default
	// 0-indexed names, or to orchestrate progressive movement of replicas from
	// one StatefulSet to another.
	// If set, replica indices will be in the range:
	//   [.spec.ordinals.start, .spec.ordinals.start + .spec.replicas).
	// If unset, defaults to 0. Replica indices will be in the range:
	//   [0, .spec.replicas).
	// +optional
	Start int32 `json:"start" protobuf:"varint,1,opt,name=start"`
}

// StatefulSetSpec defines the desired state of StatefulSet
type StatefulSetSpec struct {
	// replicas is the desired number of replicas of the given Template.
	// These are replicas in the sense that they are instantiations of the
	// same Template, but individual replicas also have a consistent identity.
	// If unspecified, defaults to 1.
	// TODO: Consider a rename of this field.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector *metav1.LabelSelector `json:"selector"`

	// template is the object that describes the pod that will be created if
	// insufficient replicas are detected. Each pod stamped out by the StatefulSet
	// will fulfill this Template, but have a unique identity from the rest
	// of the StatefulSet.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Template v1.PodTemplateSpec `json:"template"`

	// volumeClaimTemplates is a list of claims that pods are allowed to reference.
	// The StatefulSet controller is responsible for mapping network identities to
	// claims in a way that maintains the identity of a pod. Every claim in
	// this list must have at least one matching (by name) volumeMount in one
	// container in the template. A claim in this list takes precedence over
	// any volumes in the template, with the same name.
	// TODO: Define the behavior if a claim already exists with the same name.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	VolumeClaimTemplates []v1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`

	// VolumeClaimUpdateStrategy specifies the strategy for updating VolumeClaimTemplates within a StatefulSet.
	// This field is currently only effective if the StatefulSetAutoResizePVCGate is enabled.
	// +optional
	VolumeClaimUpdateStrategy VolumeClaimUpdateStrategy `json:"volumeClaimUpdateStrategy,omitempty"`

	// serviceName is the name of the service that governs this StatefulSet.
	// This service must exist before the StatefulSet, and is responsible for
	// the network identity of the set. Pods get DNS/hostnames that follow the
	// pattern: pod-specific-string.serviceName.default.svc.cluster.local
	// where "pod-specific-string" is managed by the StatefulSet controller.
	ServiceName string `json:"serviceName,omitempty"`

	// podManagementPolicy controls how pods are created during initial scale up,
	// when replacing pods on nodes, or when scaling down. The default policy is
	// `OrderedReady`, where pods are created in increasing order (pod-0, then
	// pod-1, etc) and the controller will wait until each pod is ready before
	// continuing. When scaling down, the pods are removed in the opposite order.
	// The alternative policy is `Parallel` which will create pods in parallel
	// to match the desired scale without waiting, and on scale down will delete
	// all pods at once.
	// +optional
	PodManagementPolicy apps.PodManagementPolicyType `json:"podManagementPolicy,omitempty"`

	// updateStrategy indicates the StatefulSetUpdateStrategy that will be
	// employed to update Pods in the StatefulSet when a revision is made to
	// Template.
	UpdateStrategy StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// revisionHistoryLimit is the maximum number of revisions that will
	// be maintained in the StatefulSet's revision history. The revision history
	// consists of all revisions not represented by a currently applied
	// StatefulSetSpec version. The default value is 10.
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// reserveOrdinals controls the ordinal numbers that should be reserved, and the replicas
	// will always be the expectation number of running Pods.
	// For a sts with replicas=3 and its Pods in [0, 1, 2]:
	// - If you want to migrate Pod-1 and reserve this ordinal, just set spec.reserveOrdinal to [1].
	//   Then controller will delete Pod-1 and create Pod-3 (existing Pods will be [0, 2, 3])
	// - If you just want to delete Pod-1, you should set spec.reserveOrdinal to [1] and spec.replicas to 2.
	//   Then controller will delete Pod-1 (existing Pods will be [0, 2])
	// You can also use ranges along with numbers, such as [1, 3-5], which is a shortcut for [1, 3, 4, 5].
	ReserveOrdinals []intstr.IntOrString `json:"reserveOrdinals,omitempty"`

	// Lifecycle defines the lifecycle hooks for Pods pre-delete, in-place update.
	Lifecycle *appspub.Lifecycle `json:"lifecycle,omitempty"`

	// scaleStrategy indicates the StatefulSetScaleStrategy that will be
	// employed to scale Pods in the StatefulSet.
	ScaleStrategy *StatefulSetScaleStrategy `json:"scaleStrategy,omitempty"`

	// PersistentVolumeClaimRetentionPolicy describes the policy used for PVCs created from
	// the StatefulSet VolumeClaimTemplates. This requires the
	// StatefulSetAutoDeletePVC feature gate to be enabled, which is alpha.
	// +optional
	PersistentVolumeClaimRetentionPolicy *StatefulSetPersistentVolumeClaimRetentionPolicy `json:"persistentVolumeClaimRetentionPolicy,omitempty"`

	// ordinals controls the numbering of replica indices in a StatefulSet. The
	// default ordinals behavior assigns a "0" index to the first replica and
	// increments the index by one for each additional replica requested. Using
	// the ordinals field requires the StatefulSetStartOrdinal feature gate to be
	// enabled, which is beta.
	// +optional
	Ordinals *StatefulSetOrdinals `json:"ordinals,omitempty"`
}

// StatefulSetScaleStrategy defines strategies for pods scale.
type StatefulSetScaleStrategy struct {
	// The maximum number of pods that can be unavailable during scaling.
	// Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// It can just be allowed to work with Parallel podManagementPolicy.
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// StatefulSetStatus defines the observed state of StatefulSet
type StatefulSetStatus struct {
	// observedGeneration is the most recent generation observed for this StatefulSet. It corresponds to the
	// StatefulSet's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// replicas is the number of Pods created by the StatefulSet controller.
	Replicas int32 `json:"replicas"`

	// readyReplicas is the number of Pods created by the StatefulSet controller that have a Ready Condition.
	ReadyReplicas int32 `json:"readyReplicas"`

	// AvailableReplicas is the number of Pods created by the StatefulSet controller that have been ready for
	//minReadySeconds.
	AvailableReplicas int32 `json:"availableReplicas"`

	// currentReplicas is the number of Pods created by the StatefulSet controller from the StatefulSet version
	// indicated by currentRevision.
	CurrentReplicas int32 `json:"currentReplicas"`

	// updatedReplicas is the number of Pods created by the StatefulSet controller from the StatefulSet version
	// indicated by updateRevision.
	UpdatedReplicas int32 `json:"updatedReplicas"`

	// updatedReadyReplicas is the number of updated Pods created by the StatefulSet controller that have a Ready Condition.
	UpdatedReadyReplicas int32 `json:"updatedReadyReplicas,omitempty"`

	// updatedAvailableReplicas is the number of updated Pods created by the StatefulSet controller that have a Ready condition
	//for atleast minReadySeconds.
	UpdatedAvailableReplicas int32 `json:"updatedAvailableReplicas,omitempty"`

	// currentRevision, if not empty, indicates the version of the StatefulSet used to generate Pods in the
	// sequence [0,currentReplicas).
	CurrentRevision string `json:"currentRevision,omitempty"`

	// updateRevision, if not empty, indicates the version of the StatefulSet used to generate Pods in the sequence
	// [replicas-updatedReplicas,replicas)
	UpdateRevision string `json:"updateRevision,omitempty"`

	// collisionCount is the count of hash collisions for the StatefulSet. The StatefulSet controller
	// uses this field as a collision avoidance mechanism when it needs to create the name for the
	// newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// Represents the latest available observations of a statefulset's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []apps.StatefulSetCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// LabelSelector is label selectors for query over pods that should match the replica count used by HPA.
	LabelSelector string `json:"labelSelector,omitempty"`

	// VolumeClaims represents the status of compatibility between existing PVCs
	// and their respective templates. It tracks whether the PersistentVolumeClaims have been updated
	// to match any changes made to the volumeClaimTemplates, ensuring synchronization
	// between the defined templates and the actual PersistentVolumeClaims in use.
	VolumeClaims []VolumeClaimStatus `json:"volumeClaims,omitempty"`
}

// These are valid conditions of a statefulset.
const (
	FailedCreatePod apps.StatefulSetConditionType = "FailedCreatePod"
	FailedUpdatePod apps.StatefulSetConditionType = "FailedUpdatePod"
)

// +genclient
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.labelSelector
// +kubebuilder:resource:shortName=sts;asts
// +kubebuilder:storageversion
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.labelSelector
// +kubebuilder:printcolumn:name="DESIRED",type="integer",JSONPath=".spec.replicas",description="The desired number of pods."
// +kubebuilder:printcolumn:name="CURRENT",type="integer",JSONPath=".status.replicas",description="The number of currently all pods."
// +kubebuilder:printcolumn:name="UPDATED",type="integer",JSONPath=".status.updatedReplicas",description="The number of pods updated."
// +kubebuilder:printcolumn:name="READY",type="integer",JSONPath=".status.readyReplicas",description="The number of pods ready."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."
// +kubebuilder:printcolumn:name="CONTAINERS",type="string",priority=1,JSONPath=".spec.template.spec.containers[*].name",description="The containers of currently advanced statefulset."
// +kubebuilder:printcolumn:name="IMAGES",type="string",priority=1,JSONPath=".spec.template.spec.containers[*].image",description="The images of currently advanced statefulset."

// StatefulSet is the Schema for the statefulsets API
type StatefulSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StatefulSetSpec   `json:"spec,omitempty"`
	Status StatefulSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StatefulSetList contains a list of StatefulSet
type StatefulSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StatefulSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StatefulSet{}, &StatefulSetList{})
}
