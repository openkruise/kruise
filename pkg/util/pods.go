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
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
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

func GetPodVolume(pod *v1.Pod, volumeName string) *v1.Volume {
	for idx, v := range pod.Spec.Volumes {
		if v.Name == volumeName {
			return &pod.Spec.Volumes[idx]
		}
	}
	return nil
}

func IsRunningAndReady(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning && podutil.IsPodReady(pod)
}

func IsPodContainerDigestEqual(containers sets.String, pod *v1.Pod) bool {
	cStatus := make(map[string]string, len(pod.Status.ContainerStatuses))
	for i := range pod.Status.ContainerStatuses {
		c := &pod.Status.ContainerStatuses[i]
		//ImageID format: docker-pullable://busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
		imageID := c.ImageID
		if strings.Contains(imageID, "://") {
			imageID = strings.Split(imageID, "://")[1]
		}
		cStatus[c.Name] = imageID
	}

	for _, container := range pod.Spec.Containers {
		if !containers.Has(container.Name) {
			continue
		}
		// image must be digest format
		if !IsImageDigest(container.Image) {
			return false
		}
		imageID, ok := cStatus[container.Name]
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
