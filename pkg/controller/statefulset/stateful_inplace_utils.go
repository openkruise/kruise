/*
Copyright 2019 The Kruise Authors.

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

package statefulset

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/appscode/jsonpatch"
	jsonpatch2 "github.com/evanphx/json-patch"
	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var inPlaceUpdatePatchRexp = regexp.MustCompile("/spec/containers/([0-9]+)/image")

// InPlaceUpdateSpec records the images of containers which need to in-place update.
type InPlaceUpdateSpec struct {
	revision        string
	patches         []jsonpatch.JsonPatchOperation
	containerImages map[string]string
}

func isInPlaceOnly(set *appsv1alpha1.StatefulSet) bool {
	return set.Spec.UpdateStrategy.RollingUpdate != nil &&
		set.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy == appsv1alpha1.InPlaceOnlyPodUpdateStrategyType
}

// shouldDoInPlaceUpdate check if the patches can be patched to existing pods.
// Currently kube-apiserver just allow to modify spec.containers[*].image/spec.initContainers[*].image/spec.activeDeadlineSeconds in pod.
func shouldDoInPlaceUpdate(
	set *appsv1alpha1.StatefulSet,
	updateRevision *apps.ControllerRevision,
	oldRevisionName string,
	revisions []*apps.ControllerRevision,
) (bool, *InPlaceUpdateSpec) {
	if set.Spec.UpdateStrategy.RollingUpdate == nil {
		return false, nil
	}
	if set.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy != appsv1alpha1.InPlaceIfPossiblePodUpdateStrategyType &&
		set.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy != appsv1alpha1.InPlaceOnlyPodUpdateStrategyType {
		return false, nil
	}

	var oldRevision *apps.ControllerRevision
	for _, r := range revisions {
		if r.Name == oldRevisionName {
			oldRevision = r
			break
		}
	}
	// the old ControllerRevision might be already deleted
	if oldRevision == nil {
		return false, nil
	}

	patches, err := jsonpatch.CreatePatch(oldRevision.Data.Raw, updateRevision.Data.Raw)
	if err != nil {
		return false, nil
	}

	var patchObj *struct {
		Spec struct {
			Template v1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}
	if err = json.Unmarshal(oldRevision.Data.Raw, &patchObj); err != nil {
		return false, nil
	}
	temp := patchObj.Spec.Template

	inPlaceUpdateSpec := &InPlaceUpdateSpec{
		revision:        updateRevision.Name,
		containerImages: make(map[string]string),
	}
	// all patches can just update images
	for _, jsonPatchOperation := range patches {
		jsonPatchOperation.Path = strings.Replace(jsonPatchOperation.Path, "/spec/template", "", 1)
		inPlaceUpdateSpec.patches = append(inPlaceUpdateSpec.patches, jsonPatchOperation)

		if !strings.HasPrefix(jsonPatchOperation.Path, "/spec/") {
			continue
		}
		if jsonPatchOperation.Operation != "replace" || !inPlaceUpdatePatchRexp.MatchString(jsonPatchOperation.Path) {
			return false, nil
		}
		// for example: /spec/containers/0/image
		words := strings.Split(jsonPatchOperation.Path, "/")
		idx, _ := strconv.Atoi(words[3])
		if len(temp.Spec.Containers) <= idx {
			return false, nil
		}
		inPlaceUpdateSpec.containerImages[temp.Spec.Containers[idx].Name] = jsonPatchOperation.Value.(string)
	}

	return true, inPlaceUpdateSpec
}

// checkInPlaceUpdateCompleted checks whether imageID in pod status has been changed since in-place update.
// If the imageID in containerStatuses has not been changed, we assume that kubelet has not updated
// containers in Pod.
func checkInPlaceUpdateCompleted(pod *v1.Pod) error {
	inPlaceUpdateState := appsv1alpha1.InPlaceUpdateState{}
	if stateStr, ok := pod.Annotations[appsv1alpha1.StatefulSetInPlaceUpdateStateAnnotation]; !ok {
		return nil
	} else if err := json.Unmarshal([]byte(stateStr), &inPlaceUpdateState); err != nil {
		return err
	}

	// this should not happen, unless someone modified pod revision label
	if inPlaceUpdateState.Revision != pod.Labels[apps.StatefulSetRevisionLabel] {
		return fmt.Errorf("currently revision %s not equal to in-place update revision %s",
			pod.Labels[apps.StatefulSetRevisionLabel], inPlaceUpdateState.Revision)
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if oldStatus, ok := inPlaceUpdateState.LastContainerStatuses[cs.Name]; ok {
			// TODO: we assume that users should not update StatefulSet with new image which actually has the same imageID as the old image
			if oldStatus.ImageID == cs.ImageID {
				return fmt.Errorf("container %s imageID not changed", cs.Name)
			}
			delete(inPlaceUpdateState.LastContainerStatuses, cs.Name)
		}
	}

	if len(inPlaceUpdateState.LastContainerStatuses) > 0 {
		return fmt.Errorf("not found statuses of containers %v", inPlaceUpdateState.LastContainerStatuses)
	}

	return nil
}

// podInPlaceUpdate updates a pod object in-place, including revision label, spec images and
// annotation of in-place update state.
func podInPlaceUpdate(pod *v1.Pod, spec *InPlaceUpdateSpec) (*v1.Pod, error) {
	patchJSON, _ := json.Marshal(spec.patches)
	patchClient, err := jsonpatch2.DecodePatch(patchJSON)
	if err != nil {
		return nil, fmt.Errorf("failed DecodePatch patches %v : %v", string(patchJSON), err)
	}

	podJSON, _ := json.Marshal(pod)
	newPodJSON, err := patchClient.Apply(podJSON)
	if err != nil {
		return nil, fmt.Errorf("failed apply patches %v to pod: %v", string(patchJSON), err)
	}

	var newPod *v1.Pod
	if err := json.Unmarshal(newPodJSON, &newPod); err != nil {
		return nil, fmt.Errorf("failed unmarshal pod after patch: %v", err)
	}

	// update new revision
	newPod.Labels[apps.StatefulSetRevisionLabel] = spec.revision

	// record old containerStatuses
	inPlaceUpdateState := appsv1alpha1.InPlaceUpdateState{
		Revision:              spec.revision,
		UpdateTimestamp:       metav1.NewTime(time.Now()),
		LastContainerStatuses: make(map[string]appsv1alpha1.InPlaceUpdateContainerStatus, len(spec.containerImages)),
	}
	for _, c := range newPod.Status.ContainerStatuses {
		if _, ok := spec.containerImages[c.Name]; ok {
			inPlaceUpdateState.LastContainerStatuses[c.Name] = appsv1alpha1.InPlaceUpdateContainerStatus{
				ImageID: c.ImageID,
			}
		}
	}
	inPlaceUpdateStateJSON, _ := json.Marshal(inPlaceUpdateState)
	if newPod.Annotations == nil {
		newPod.Annotations = map[string]string{}
	}
	newPod.Annotations[appsv1alpha1.StatefulSetInPlaceUpdateStateAnnotation] = string(inPlaceUpdateStateJSON)

	return newPod, nil
}

func shouldUpdateInPlaceReady(pod *v1.Pod) bool {
	var containsReadinessGate bool
	for _, r := range pod.Spec.ReadinessGates {
		if r.ConditionType == appsv1alpha1.StatefulSetInPlaceUpdateReady {
			containsReadinessGate = true
			break
		}
	}
	// no need to update condition because of no readiness-gate
	if !containsReadinessGate {
		return false
	}

	// in-place updating has not completed yet
	if checkInPlaceUpdateCompleted(pod) != nil {
		return false
	}

	existingCondition := getInPlaceUpdateReadyCondition(pod)
	return existingCondition == nil || existingCondition.Status == v1.ConditionFalse
}

func getInPlaceUpdateReadyCondition(pod *v1.Pod) *v1.PodCondition {
	for _, c := range pod.Status.Conditions {
		if c.Type == appsv1alpha1.StatefulSetInPlaceUpdateReady {
			return &c
		}
	}
	return nil
}

func updatePodCondition(pod *v1.Pod, condition v1.PodCondition) {
	for i, c := range pod.Status.Conditions {
		if c.Type == condition.Type {
			pod.Status.Conditions[i] = condition
			return
		}
	}
	pod.Status.Conditions = append(pod.Status.Conditions, condition)
}
