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

package utils

import (
	"strings"

	"github.com/docker/distribution/reference"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/integer"
)

func MergeVolumeMounts(original, additional []v1.VolumeMount) []v1.VolumeMount {
	mountpoints := sets.NewString()
	for _, mount := range original {
		mountpoints.Insert(mount.MountPath)
	}

	for _, mount := range additional {
		if mountpoints.Has(mount.MountPath) {
			continue
		}
		original = append(original, mount)
		mountpoints.Insert(mount.MountPath)
	}
	return original
}

func MergeEnvVar(original []v1.EnvVar, additional []v1.EnvVar) []v1.EnvVar {
	exists := sets.NewString()
	for _, env := range original {
		exists.Insert(env.Name)
	}

	for _, env := range additional {
		if exists.Has(env.Name) {
			continue
		}
		original = append(original, env)
		exists.Insert(env.Name)
	}

	return original
}

func MergeVolumes(original []v1.Volume, additional []v1.Volume) []v1.Volume {
	exists := sets.NewString()
	for _, volume := range original {
		exists.Insert(volume.Name)
	}

	for _, volume := range additional {
		if exists.Has(volume.Name) {
			continue
		}
		original = append(original, volume)
		exists.Insert(volume.Name)
	}

	return original
}

func GetContainerEnvVar(container *v1.Container, key string) *v1.EnvVar {
	if container == nil {
		return nil
	}
	for i, e := range container.Env {
		if e.Name == key {
			return &container.Env[i]
		}
	}
	return nil
}

func GetContainerEnvValue(container *v1.Container, key string) string {
	if container == nil {
		return ""
	}
	for i, e := range container.Env {
		if e.Name == key {
			return container.Env[i].Value
		}
	}
	return ""
}

func GetContainerVolumeMount(container *v1.Container, key string) *v1.VolumeMount {
	if container == nil {
		return nil
	}
	for i, m := range container.VolumeMounts {
		if m.MountPath == key {
			return &container.VolumeMounts[i]
		}
	}
	return nil
}

func GetContainer(name string, pod *v1.Pod) *v1.Container {
	if pod == nil {
		return nil
	}
	for i := range pod.Spec.InitContainers {
		v := &pod.Spec.InitContainers[i]
		if v.Name == name {
			return v
		}
	}

	for i := range pod.Spec.Containers {
		v := &pod.Spec.Containers[i]
		if v.Name == name {
			return v
		}
	}
	return nil
}

func GetContainerStatus(name string, pod *v1.Pod) *v1.ContainerStatus {
	if pod == nil {
		return nil
	}
	for i := range pod.Status.ContainerStatuses {
		v := &pod.Status.ContainerStatuses[i]
		if v.Name == name {
			return v
		}
	}
	return nil
}

func IsRunningAndReady(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning && IsPodReady(pod) && pod.DeletionTimestamp.IsZero()
}

func GetPodContainerImageIDs(pod *v1.Pod) map[string]string {
	cImageIDs := make(map[string]string, len(pod.Status.ContainerStatuses))
	for i := range pod.Status.ContainerStatuses {
		c := &pod.Status.ContainerStatuses[i]
		//ImageID format: docker-pullable://busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
		imageID := c.ImageID
		if strings.Contains(imageID, "://") {
			imageID = strings.Split(imageID, "://")[1]
		}
		cImageIDs[c.Name] = imageID
	}
	return cImageIDs
}

// parse container images,
// 1. docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
// repo=docker.io/busybox, tag="", digest=sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
// 2. docker.io/busybox:latest
// repo=docker.io/busybox, tag=latest, digest=""
func ParseImage(image string) (repo, tag, digest string, err error) {
	refer, err := reference.Parse(image)
	if err != nil {
		return "", "", "", err
	}

	if named, ok := refer.(reference.Named); ok {
		repo = named.Name()
	}
	if tagged, ok := refer.(reference.Tagged); ok {
		tag = tagged.Tag()
	}
	if digested, ok := refer.(reference.Digested); ok {
		digest = digested.Digest().String()
	}
	return
}

//whether image is digest format,
//for example: docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
func IsImageDigest(image string) bool {
	_, _, digest, _ := ParseImage(image)
	return digest != ""
}

// 1. image1, image2 are digest image, compare repo+digest
// 2. image1, image2 are normal image, compare repo+tag
// 3. image1, image2 are digest+normal image, don't support compare it, return false
func IsContainerImageEqual(image1, image2 string) bool {
	repo1, tag1, digest1, err := ParseImage(image1)
	if err != nil {
		klog.Errorf("parse image %s failed: %s", image1, err.Error())
		return false
	}

	repo2, tag2, digest2, err := ParseImage(image2)
	if err != nil {
		klog.Errorf("parse image %s failed: %s", image2, err.Error())
		return false
	}

	if IsImageDigest(image1) && IsImageDigest(image2) {
		return repo1 == repo2 && digest1 == digest2
	}

	return repo1 == repo2 && tag1 == tag2
}

// GetPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetPodCondition(status *v1.PodStatus, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return GetPodConditionFromList(status.Conditions, conditionType)
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetPodConditionFromList(conditions []v1.PodCondition, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}

// GetPodReadyCondition extracts the pod ready condition from the given status and returns that.
// Returns nil if the condition is not present.
func GetPodReadyCondition(status v1.PodStatus) *v1.PodCondition {
	_, condition := GetPodCondition(&status, v1.PodReady)
	return condition
}

// IsPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func IsPodReadyConditionTrue(status v1.PodStatus) bool {
	condition := GetPodReadyCondition(status)
	return condition != nil && condition.Status == v1.ConditionTrue
}

// IsPodReady returns true if a pod is ready; false otherwise.
func IsPodReady(pod *v1.Pod) bool {
	return IsPodReadyConditionTrue(pod.Status)
}

var (
	podPhaseToOrdinal = map[v1.PodPhase]int{v1.PodPending: 0, v1.PodUnknown: 1, v1.PodRunning: 2}
)

// ActivePods type allows custom sorting of pods so a controller can pick the best ones to delete.
type ActivePods []*v1.Pod

func (s ActivePods) Len() int      { return len(s) }
func (s ActivePods) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s ActivePods) Less(i, j int) bool {
	// 1. Unassigned < assigned
	// If only one of the pods is unassigned, the unassigned one is smaller
	if s[i].Spec.NodeName != s[j].Spec.NodeName && (len(s[i].Spec.NodeName) == 0 || len(s[j].Spec.NodeName) == 0) {
		return len(s[i].Spec.NodeName) == 0
	}
	// 2. PodPending < PodUnknown < PodRunning
	if podPhaseToOrdinal[s[i].Status.Phase] != podPhaseToOrdinal[s[j].Status.Phase] {
		return podPhaseToOrdinal[s[i].Status.Phase] < podPhaseToOrdinal[s[j].Status.Phase]
	}
	// 3. Not ready < ready
	// If only one of the pods is not ready, the not ready one is smaller
	if IsPodReady(s[i]) != IsPodReady(s[j]) {
		return !IsPodReady(s[i])
	}
	// TODO: take availability into account when we push minReadySeconds information from deployment into pods,
	//       see https://github.com/kubernetes/kubernetes/issues/22065
	// 4. Been ready for empty time < less time < more time
	// If both pods are ready, the latest ready one is smaller
	if IsPodReady(s[i]) && IsPodReady(s[j]) {
		readyTime1 := podReadyTime(s[i])
		readyTime2 := podReadyTime(s[j])
		if !readyTime1.Equal(readyTime2) {
			return afterOrZero(readyTime1, readyTime2)
		}
	}
	// 5. Pods with containers with higher restart counts < lower restart counts
	if maxContainerRestarts(s[i]) != maxContainerRestarts(s[j]) {
		return maxContainerRestarts(s[i]) > maxContainerRestarts(s[j])
	}
	// 6. Empty creation time pods < newer pods < older pods
	if !s[i].CreationTimestamp.Equal(&s[j].CreationTimestamp) {
		return afterOrZero(&s[i].CreationTimestamp, &s[j].CreationTimestamp)
	}
	return false
}

func maxContainerRestarts(pod *v1.Pod) int {
	maxRestarts := 0
	for _, c := range pod.Status.ContainerStatuses {
		maxRestarts = integer.IntMax(maxRestarts, int(c.RestartCount))
	}
	return maxRestarts
}

// afterOrZero checks if time t1 is after time t2; if one of them
// is zero, the zero time is seen as after non-zero time.
func afterOrZero(t1, t2 *metav1.Time) bool {
	if t1.Time.IsZero() || t2.Time.IsZero() {
		return t1.Time.IsZero()
	}
	return t1.After(t2.Time)
}

func podReadyTime(pod *v1.Pod) *metav1.Time {
	if IsPodReady(pod) {
		for _, c := range pod.Status.Conditions {
			// we only care about pod ready conditions
			if c.Type == v1.PodReady && c.Status == v1.ConditionTrue {
				return &c.LastTransitionTime
			}
		}
	}
	return &metav1.Time{}
}
