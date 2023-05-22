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

package core

import (
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	v1 "k8s.io/api/core/v1"
)

type Control interface {
	// common
	IsInitializing() bool
	SetRevisionTemplate(revisionSpec map[string]interface{}, template map[string]interface{})
	ApplyRevisionPatch(patched []byte) (*appsv1alpha1.CloneSet, error)

	// scale
	IsReadyToScale() bool
	NewVersionedPods(currentCS, updateCS *appsv1alpha1.CloneSet,
		currentRevision, updateRevision string,
		expectedCreations, expectedCurrentCreations int,
		availableIDs []string,
	) ([]*v1.Pod, error)
	GetPodSpreadConstraint() []clonesetutils.PodSpreadConstraint

	// update
	IsPodUpdatePaused(pod *v1.Pod) bool
	IsPodUpdateReady(pod *v1.Pod, minReadySeconds int32) bool
	GetPodsSortFunc(pods []*v1.Pod, waitUpdateIndexes []int) func(i, j int) bool
	GetUpdateOptions() *inplaceupdate.UpdateOptions
	ExtraStatusCalculation(status *appsv1alpha1.CloneSetStatus, pods []*v1.Pod) error

	// validation
	ValidateCloneSetUpdate(oldCS, newCS *appsv1alpha1.CloneSet) error

	// event handler
	IgnorePodUpdateEvent(oldPod, newPod *v1.Pod) bool
}

func New(cs *appsv1alpha1.CloneSet) Control {
	return &commonControl{CloneSet: cs}
}
