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

package util

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// GetPodNames returns names of the given Pods array
func GetPodNames(pods []*v1.Pod) sets.String {
	set := sets.NewString()
	for _, pod := range pods {
		set.Insert(pod.Name)
	}
	return set
}

// MergePods merges two pods arrays
func MergePods(pods1, pods2 []*v1.Pod) []*v1.Pod {
	var ret []*v1.Pod
	names := sets.NewString()

	for _, pod := range pods1 {
		if !names.Has(pod.Name) {
			ret = append(ret, pod)
			names.Insert(pod.Name)
		}
	}
	for _, pod := range pods2 {
		if !names.Has(pod.Name) {
			ret = append(ret, pod)
			names.Insert(pod.Name)
		}
	}
	return ret
}

// DiffPods returns pods in pods1 but not in pods2
func DiffPods(pods1, pods2 []*v1.Pod) (ret []*v1.Pod) {
	names2 := sets.NewString()
	for _, pod := range pods2 {
		names2.Insert(pod.Name)
	}
	for _, pod := range pods1 {
		if names2.Has(pod.Name) {
			continue
		}
		ret = append(ret, pod)
	}
	return
}

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

func GetPodVolume(pod *v1.Pod, volumeName string) *v1.Volume {
	for idx, v := range pod.Spec.Volumes {
		if v.Name == volumeName {
			return &pod.Spec.Volumes[idx]
		}
	}
	return nil
}

func IsRunningAndReady(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning && podutil.IsPodReady(pod) && pod.DeletionTimestamp.IsZero()
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

func IsPodContainerDigestEqual(containers sets.String, pod *v1.Pod) bool {
	cImageIDs := GetPodContainerImageIDs(pod)

	for _, container := range pod.Spec.Containers {
		if !containers.Has(container.Name) {
			continue
		}
		// image must be digest format
		if !IsImageDigest(container.Image) {
			return false
		}
		imageID, ok := cImageIDs[container.Name]
		if !ok {
			return false
		}
		if !IsContainerImageEqual(container.Image, imageID) {
			return false
		}
	}
	return true
}

func MergeVolumeMountsInContainer(origin *v1.Container, other v1.Container) {
	mountExist := make(map[string]bool)
	for _, volume := range origin.VolumeMounts {
		mountExist[volume.MountPath] = true

	}

	for _, volume := range other.VolumeMounts {
		if mountExist[volume.MountPath] {
			continue
		}

		origin.VolumeMounts = append(origin.VolumeMounts, volume)
	}
}

func IsPodOwnedByKruise(pod *v1.Pod) bool {
	ownerRef := metav1.GetControllerOf(pod)
	if ownerRef == nil {
		return false
	}
	gv, _ := schema.ParseGroupVersion(ownerRef.APIVersion)
	return gv.Group == appsv1alpha1.GroupVersion.Group
}

func InjectReadinessGateToPod(pod *v1.Pod, conditionType v1.PodConditionType) {
	for _, g := range pod.Spec.ReadinessGates {
		if g.ConditionType == conditionType {
			return
		}
	}
	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: conditionType})
}

func ContainsObjectRef(slice []v1.ObjectReference, obj v1.ObjectReference) bool {
	for _, o := range slice {
		if o.UID == obj.UID {
			return true
		}
	}
	return false
}

func GetCondition(pod *v1.Pod, cType v1.PodConditionType) *v1.PodCondition {
	if pod == nil {
		return nil
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == cType {
			return &c
		}
	}
	return nil
}

func SetPodCondition(pod *v1.Pod, condition v1.PodCondition) {
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

func SetPodConditionIfMsgChanged(pod *v1.Pod, condition v1.PodCondition) {
	for i, c := range pod.Status.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status || c.Message != condition.Message {
				pod.Status.Conditions[i] = condition
			}
			return
		}
	}
	pod.Status.Conditions = append(pod.Status.Conditions, condition)
}

func SetPodReadyCondition(pod *v1.Pod) {
	podReady := GetCondition(pod, v1.PodReady)
	if podReady == nil {
		return
	}

	containersReady := GetCondition(pod, v1.ContainersReady)
	if containersReady == nil || containersReady.Status != v1.ConditionTrue {
		return
	}

	var unreadyMessages []string
	for _, rg := range pod.Spec.ReadinessGates {
		c := GetCondition(pod, rg.ConditionType)
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

	SetPodCondition(pod, newPodReady)
}

func ExtractPort(param intstr.IntOrString, container v1.Container) (int, error) {
	port := -1
	var err error
	switch param.Type {
	case intstr.Int:
		port = param.IntValue()
	case intstr.String:
		if port, err = findPortByName(container, param.StrVal); err != nil {
			// Last ditch effort - maybe it was an int stored as string?
			klog.ErrorS(err, "failed to find port by name")
			if port, err = strconv.Atoi(param.StrVal); err != nil {
				return port, err
			}
		}
	default:
		return port, fmt.Errorf("intOrString had no kind: %+v", param)
	}
	if port > 0 && port < 65536 {
		return port, nil
	}
	return port, fmt.Errorf("invalid port number: %v", port)
}

// findPortByName is a helper function to look up a port in a container by name.
func findPortByName(container v1.Container, portName string) (int, error) {
	for _, port := range container.Ports {
		if port.Name == portName {
			return int(port.ContainerPort), nil
		}
	}
	return 0, fmt.Errorf("port %s not found", portName)
}

func GetPodContainerByName(cName string, pod *v1.Pod) *v1.Container {
	for _, container := range pod.Spec.Containers {
		if cName == container.Name {
			return &container
		}
	}

	return nil
}

// IsRestartableInitContainer returns true if the initContainer has
// ContainerRestartPolicyAlways.
func IsRestartableInitContainer(initContainer *v1.Container) bool {
	if initContainer.RestartPolicy == nil {
		return false
	}

	return *initContainer.RestartPolicy == v1.ContainerRestartPolicyAlways
}
