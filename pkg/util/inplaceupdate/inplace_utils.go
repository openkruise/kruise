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

package inplaceupdate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/appscode/jsonpatch"
	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var inPlaceUpdatePatchRexp = regexp.MustCompile("^/spec/containers/([0-9]+)/image$")

// Interface for managing pods in-place update.
type Interface interface {
	UpdateInPlace(pod *v1.Pod, oldRevision, newRevision *apps.ControllerRevision) (bool, error)
	UpdateCondition(pod *v1.Pod) error
}

// updateSpec records the images of containers which need to in-place update.
type updateSpec struct {
	revision        string
	containerImages map[string]string
	// TODO(FillZpp): this should be strategic patch if we have to inplace update other fields like labels/annotations
}

type realControl struct {
	adp         adapter
	revisionKey string

	// just for test
	now func() metav1.Time
}

func New(c client.Client, revisionKey string) Interface {
	return &realControl{adp: &adapterRuntimeClient{Client: c}, revisionKey: revisionKey, now: metav1.Now}
}

func NewForTypedClient(c clientset.Interface, revisionKey string) Interface {
	return &realControl{adp: &adapterTypedClient{client: c}, revisionKey: revisionKey, now: metav1.Now}
}

func NewForInformer(informer coreinformers.PodInformer, revisionKey string) Interface {
	return &realControl{adp: &adapterInformer{podInformer: informer}, revisionKey: revisionKey, now: metav1.Now}
}

func NewForTest(c client.Client, revisionKey string, now func() metav1.Time) Interface {
	return &realControl{adp: &adapterRuntimeClient{Client: c}, revisionKey: revisionKey, now: now}
}

func (c *realControl) UpdateInPlace(pod *v1.Pod, oldRevision, newRevision *apps.ControllerRevision) (bool, error) {
	// 1. calculate inplace update spec
	spec := calculateInPlaceUpdateSpec(oldRevision, newRevision)
	if spec == nil {
		return false, nil
	}
	// TODO(FillZpp): maybe we should check if the previous in-place update has completed

	// 2. update condition
	newCondition := v1.PodCondition{
		Type:               appsv1alpha1.InPlaceUpdateReady,
		LastTransitionTime: c.now(),
		Status:             v1.ConditionFalse,
		Reason:             "StartInPlaceUpdate",
	}
	if err := c.updateCondition(pod, newCondition); err != nil {
		return true, err
	}

	// 3. update container images
	if err := c.updatePodInPlace(pod, spec); err != nil {
		return true, err
	}

	return true, nil
}

func (c *realControl) UpdateCondition(pod *v1.Pod) error {
	var containsReadinessGate bool
	for _, r := range pod.Spec.ReadinessGates {
		if r.ConditionType == appsv1alpha1.InPlaceUpdateReady {
			containsReadinessGate = true
			break
		}
	}
	// no need to update condition because of no readiness-gate
	if !containsReadinessGate {
		return nil
	}

	// in-place updating has not completed yet
	if CheckInPlaceUpdateCompleted(pod) != nil {
		return nil
	}

	// already ready
	if existingCondition := GetCondition(pod); existingCondition != nil && existingCondition.Status == v1.ConditionTrue {
		return nil
	}

	newCondition := v1.PodCondition{
		Type:               appsv1alpha1.InPlaceUpdateReady,
		Status:             v1.ConditionTrue,
		LastTransitionTime: c.now(),
	}
	return c.updateCondition(pod, newCondition)
}

func (c *realControl) updateCondition(pod *v1.Pod, condition v1.PodCondition) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := c.adp.getPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		setPodCondition(clone, condition)
		updatePodReadyCondition(clone)
		return c.adp.updatePodStatus(clone)
	})
}

func (c *realControl) updatePodInPlace(pod *v1.Pod, spec *updateSpec) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := c.adp.getPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		// update images
		for i := range clone.Spec.Containers {
			if newImage, ok := spec.containerImages[clone.Spec.Containers[i].Name]; ok {
				clone.Spec.Containers[i].Image = newImage
			}
		}

		// update new revision
		if c.revisionKey != "" {
			clone.Labels[c.revisionKey] = spec.revision
		}

		// record old containerStatuses
		inPlaceUpdateState := appsv1alpha1.InPlaceUpdateState{
			Revision:              spec.revision,
			UpdateTimestamp:       metav1.Now(),
			LastContainerStatuses: make(map[string]appsv1alpha1.InPlaceUpdateContainerStatus, len(spec.containerImages)),
		}
		for _, c := range clone.Status.ContainerStatuses {
			if _, ok := spec.containerImages[c.Name]; ok {
				inPlaceUpdateState.LastContainerStatuses[c.Name] = appsv1alpha1.InPlaceUpdateContainerStatus{
					ImageID: c.ImageID,
				}
			}
		}
		inPlaceUpdateStateJSON, _ := json.Marshal(inPlaceUpdateState)
		if clone.Annotations == nil {
			clone.Annotations = map[string]string{}
		}
		clone.Annotations[appsv1alpha1.InPlaceUpdateStateKey] = string(inPlaceUpdateStateJSON)

		return c.adp.updatePod(clone)
	})
}

// calculateInPlaceUpdateSpec calculates diff between old and update revisions.
// If the diff just contains replace operation of spec.containers[x].image, it will returns an UpdateSpec.
// Otherwise, it returns nil which means can not use in-place update.
func calculateInPlaceUpdateSpec(oldRevision, newRevision *apps.ControllerRevision) *updateSpec {
	if oldRevision == nil || newRevision == nil {
		return nil
	}

	patches, err := jsonpatch.CreatePatch(oldRevision.Data.Raw, newRevision.Data.Raw)
	if err != nil {
		return nil
	}

	var patchObj *struct {
		Spec struct {
			Template v1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}
	if err = json.Unmarshal(oldRevision.Data.Raw, &patchObj); err != nil {
		return nil
	}
	temp := patchObj.Spec.Template

	updateSpec := &updateSpec{
		revision:        newRevision.Name,
		containerImages: make(map[string]string),
	}
	// all patches can just update images
	for _, jsonPatchOperation := range patches {
		jsonPatchOperation.Path = strings.Replace(jsonPatchOperation.Path, "/spec/template", "", 1)

		if !strings.HasPrefix(jsonPatchOperation.Path, "/spec/") {
			continue
		}
		if jsonPatchOperation.Operation != "replace" || !inPlaceUpdatePatchRexp.MatchString(jsonPatchOperation.Path) {
			return nil
		}
		// for example: /spec/containers/0/image
		words := strings.Split(jsonPatchOperation.Path, "/")
		idx, _ := strconv.Atoi(words[3])
		if len(temp.Spec.Containers) <= idx {
			return nil
		}
		updateSpec.containerImages[temp.Spec.Containers[idx].Name] = jsonPatchOperation.Value.(string)
	}
	return updateSpec
}

// CheckInPlaceUpdateCompleted checks whether imageID in pod status has been changed since in-place update.
// If the imageID in containerStatuses has not been changed, we assume that kubelet has not updated
// containers in Pod.
func CheckInPlaceUpdateCompleted(pod *v1.Pod) error {
	inPlaceUpdateState := appsv1alpha1.InPlaceUpdateState{}
	if stateStr, ok := pod.Annotations[appsv1alpha1.InPlaceUpdateStateKey]; !ok {
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
			// TODO: we assume that users should not update workload template with new image which actually has the same imageID as the old image
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

// InjectReadinessGate injects InPlaceUpdateReady into pod.spec.readinessGates
func InjectReadinessGate(pod *v1.Pod) {
	for _, r := range pod.Spec.ReadinessGates {
		if r.ConditionType == appsv1alpha1.InPlaceUpdateReady {
			return
		}
	}
	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appsv1alpha1.InPlaceUpdateReady})
}

// GetCondition returns the InPlaceUpdateReady condition in Pod.
func GetCondition(pod *v1.Pod) *v1.PodCondition {
	return getCondition(pod, appsv1alpha1.InPlaceUpdateReady)
}

func getCondition(pod *v1.Pod, cType v1.PodConditionType) *v1.PodCondition {
	for _, c := range pod.Status.Conditions {
		if c.Type == cType {
			return &c
		}
	}
	return nil
}

func setPodCondition(pod *v1.Pod, condition v1.PodCondition) {
	for i, c := range pod.Status.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status {
				pod.Status.Conditions[i] = condition
			}
			return
		}
	}
	pod.Status.Conditions = append(pod.Status.Conditions, condition)
}

func updatePodReadyCondition(pod *v1.Pod) {
	podReady := getCondition(pod, v1.PodReady)
	if podReady == nil {
		return
	}

	containersReady := getCondition(pod, v1.ContainersReady)
	if containersReady == nil || containersReady.Status != v1.ConditionTrue {
		return
	}

	var unreadyMessages []string
	for _, rg := range pod.Spec.ReadinessGates {
		c := getCondition(pod, rg.ConditionType)
		if c == nil {
			unreadyMessages = append(unreadyMessages, fmt.Sprintf("corresponding condition of pod readiness gate %q does not exist.", string(rg.ConditionType)))
		} else if c.Status != v1.ConditionTrue {
			unreadyMessages = append(unreadyMessages, fmt.Sprintf("the status of pod readiness gate %q is not \"True\", but %v", string(rg.ConditionType), c.Status))
		}
	}

	newPodReady := v1.PodCondition{
		Type:               v1.PodReady,
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
	}
	// Set "Ready" condition to "False" if any readiness gate is not ready.
	if len(unreadyMessages) != 0 {
		unreadyMessage := strings.Join(unreadyMessages, ", ")
		newPodReady = v1.PodCondition{
			Type:    v1.PodReady,
			Status:  v1.ConditionFalse,
			Reason:  "ReadinessGatesNotReady",
			Message: unreadyMessage,
		}
	}

	setPodCondition(pod, newPodReady)
}
