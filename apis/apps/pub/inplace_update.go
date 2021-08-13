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

	// UpdateTimestamp is the time when the in-place update happens.
	UpdateTimestamp metav1.Time `json:"updateTimestamp"`

	// LastContainerStatuses records the before-in-place-update container statuses. It is a map from ContainerName
	// to InPlaceUpdateContainerStatus
	LastContainerStatuses map[string]InPlaceUpdateContainerStatus `json:"lastContainerStatuses"`
}

// InPlaceUpdateContainerStatus records the statuses of the container that are mainly used
// to determine whether the InPlaceUpdate is completed.
type InPlaceUpdateContainerStatus struct {
	ImageID string `json:"imageID,omitempty"`
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
	// TODO: add ConvertEnvHash here to support inplace update for env from annotation/label
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
