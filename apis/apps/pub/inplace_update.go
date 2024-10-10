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

package pub

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// InPlaceUpdateReady must be added into template.spec.readinessGates when pod podUpdatePolicy
	// is InPlaceIfPossible or InPlaceOnly. The condition in podStatus will be updated to False before in-place
	// updating and updated to True after the update is finished. This ensures pod to remain at NotReady state while
	// in-place update is happening.
	InPlaceUpdateReady v1.PodConditionType = "InPlaceUpdateReady"

	// InPlaceUpdateStateKey records the state of inplace-update.
	// The value of annotation is InPlaceUpdateState.
	InPlaceUpdateStateKey string = "apps.kruise.io/inplace-update-state"
	// TODO: will be removed since v1.0.0
	InPlaceUpdateStateKeyOld string = "inplace-update-state"

	// InPlaceUpdateGraceKey records the spec that Pod should be updated when
	// grace period ends.
	InPlaceUpdateGraceKey string = "apps.kruise.io/inplace-update-grace"
	// TODO: will be removed since v1.0.0
	InPlaceUpdateGraceKeyOld string = "inplace-update-grace"

	// RuntimeContainerMetaKey is a key in pod annotations. Kruise-daemon should report the
	// states of runtime containers into its value, which is a structure JSON of RuntimeContainerMetaSet type.
	RuntimeContainerMetaKey = "apps.kruise.io/runtime-containers-meta"
)

// InPlaceUpdateState records latest inplace-update state, including old statuses of containers.
type InPlaceUpdateState struct {
	// Revision is the updated revision hash.
	Revision string `json:"revision"`

	// UpdateTimestamp is the start time when the in-place update happens.
	UpdateTimestamp metav1.Time `json:"updateTimestamp"`

	// LastContainerStatuses records the before-in-place-update container statuses. It is a map from ContainerName
	// to InPlaceUpdateContainerStatus
	LastContainerStatuses map[string]InPlaceUpdateContainerStatus `json:"lastContainerStatuses"`

	// UpdateEnvFromMetadata indicates there are envs from annotations/labels that should be in-place update.
	UpdateEnvFromMetadata bool `json:"updateEnvFromMetadata,omitempty"`

	// UpdateResources indicates there are resources that should be in-place update.
	UpdateResources bool `json:"updateResources,omitempty"`

	// VerticalUpdateOnly indicates there are only vertical update in this revision.
	VerticalUpdateOnly bool `json:"verticalUpdateOnly"`

	// NextContainerImages is the containers with lower priority that waiting for in-place update images in next batch.
	NextContainerImages map[string]string `json:"nextContainerImages,omitempty"`

	// NextContainerRefMetadata is the containers with lower priority that waiting for in-place update labels/annotations in next batch.
	NextContainerRefMetadata map[string]metav1.ObjectMeta `json:"nextContainerRefMetadata,omitempty"`

	// NextContainerResources is the containers with lower priority that waiting for in-place update resources in next batch.
	NextContainerResources map[string]v1.ResourceRequirements `json:"nextContainerResources,omitempty"`

	// PreCheckBeforeNext is the pre-check that must pass before the next containers can be in-place update.
	PreCheckBeforeNext *InPlaceUpdatePreCheckBeforeNext `json:"preCheckBeforeNext,omitempty"`

	// ContainerBatchesRecord records the update batches that have patched in this revision.
	ContainerBatchesRecord []InPlaceUpdateContainerBatch `json:"containerBatchesRecord,omitempty"`
}

// InPlaceUpdatePreCheckBeforeNext contains the pre-check that must pass before the next containers can be in-place update.
type InPlaceUpdatePreCheckBeforeNext struct {
	ContainersRequiredReady []string `json:"containersRequiredReady,omitempty"`
}

// InPlaceUpdateContainerBatch indicates the timestamp and containers for a batch update
type InPlaceUpdateContainerBatch struct {
	// Timestamp is the time for this update batch
	Timestamp metav1.Time `json:"timestamp"`
	// Containers is the name list of containers for this update batch
	Containers []string `json:"containers"`
}

// InPlaceUpdateContainerStatus records the statuses of the container that are mainly used
// to determine whether the InPlaceUpdate is completed.
type InPlaceUpdateContainerStatus struct {
	ImageID   string                  `json:"imageID,omitempty"`
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// InPlaceUpdateStrategy defines the strategies for in-place update.
type InPlaceUpdateStrategy struct {
	// GracePeriodSeconds is the timespan between set Pod status to not-ready and update images in Pod spec
	// when in-place update a Pod.
	GracePeriodSeconds int32 `json:"gracePeriodSeconds,omitempty"`
}

func GetInPlaceUpdateState(obj metav1.Object) (string, bool) {
	if v, ok := obj.GetAnnotations()[InPlaceUpdateStateKey]; ok {
		return v, ok
	}
	v, ok := obj.GetAnnotations()[InPlaceUpdateStateKeyOld]
	return v, ok
}

func GetInPlaceUpdateGrace(obj metav1.Object) (string, bool) {
	if v, ok := obj.GetAnnotations()[InPlaceUpdateGraceKey]; ok {
		return v, ok
	}
	v, ok := obj.GetAnnotations()[InPlaceUpdateGraceKeyOld]
	return v, ok
}

func RemoveInPlaceUpdateGrace(obj metav1.Object) {
	delete(obj.GetAnnotations(), InPlaceUpdateGraceKey)
	delete(obj.GetAnnotations(), InPlaceUpdateGraceKeyOld)
}

// RuntimeContainerMetaSet contains all the containers' meta of the Pod.
type RuntimeContainerMetaSet struct {
	Containers []RuntimeContainerMeta `json:"containers"`
}

// RuntimeContainerMeta contains the meta data of a runtime container.
type RuntimeContainerMeta struct {
	Name         string                 `json:"name"`
	ContainerID  string                 `json:"containerID"`
	RestartCount int32                  `json:"restartCount"`
	Hashes       RuntimeContainerHashes `json:"hashes"`
}

// RuntimeContainerHashes contains the hashes of such container.
type RuntimeContainerHashes struct {
	// PlainHash is the hash that directly calculated from pod.spec.container[x].
	// Usually it is calculated by Kubelet and will be in annotation of each runtime container.
	PlainHash uint64 `json:"plainHash"`
	// PlainHashWithoutResources is the hash that directly calculated from pod.spec.container[x]
	// over fields with Resources field zero'd out.
	// Usually it is calculated by Kubelet and will be in annotation of each runtime container.
	PlainHashWithoutResources uint64 `json:"plainHashWithoutResources"`
	// ExtractedEnvFromMetadataHash is the hash that calculated from pod.spec.container[x],
	// whose envs from annotations/labels have already been extracted to the real values.
	ExtractedEnvFromMetadataHash uint64 `json:"extractedEnvFromMetadataHash,omitempty"`
}

func GetRuntimeContainerMetaSet(obj metav1.Object) (*RuntimeContainerMetaSet, error) {
	str, ok := obj.GetAnnotations()[RuntimeContainerMetaKey]
	if !ok {
		return nil, nil
	}

	s := RuntimeContainerMetaSet{}
	if err := json.Unmarshal([]byte(str), &s); err != nil {
		return nil, err
	}
	return &s, nil
}
