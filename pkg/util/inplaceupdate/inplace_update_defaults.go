/*
Copyright 2021 The Kruise Authors.

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
	"strconv"
	"strings"

	"github.com/appscode/jsonpatch"
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/features"
	utilcontainermeta "github.com/openkruise/kruise/pkg/util/containermeta"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/klog/v2"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

func SetOptionsDefaults(opts *UpdateOptions) *UpdateOptions {
	if opts == nil {
		opts = &UpdateOptions{}
	}

	if opts.CalculateSpec == nil {
		opts.CalculateSpec = defaultCalculateInPlaceUpdateSpec
	}

	if opts.PatchSpecToPod == nil {
		opts.PatchSpecToPod = defaultPatchUpdateSpecToPod
	}

	if opts.CheckUpdateCompleted == nil {
		opts.CheckUpdateCompleted = DefaultCheckInPlaceUpdateCompleted
	}

	return opts
}

// defaultPatchUpdateSpecToPod returns new pod that merges spec into old pod
func defaultPatchUpdateSpecToPod(pod *v1.Pod, spec *UpdateSpec) (*v1.Pod, error) {
	if spec.MetaDataPatch != nil {
		cloneBytes, _ := json.Marshal(pod)
		modified, err := strategicpatch.StrategicMergePatch(cloneBytes, spec.MetaDataPatch, &v1.Pod{})
		if err != nil {
			return nil, err
		}
		pod = &v1.Pod{}
		if err = json.Unmarshal(modified, pod); err != nil {
			return nil, err
		}
	}

	for i := range pod.Spec.Containers {
		if newImage, ok := spec.ContainerImages[pod.Spec.Containers[i].Name]; ok {
			pod.Spec.Containers[i].Image = newImage
		}
	}
	return pod, nil
}

// defaultCalculateInPlaceUpdateSpec calculates diff between old and update revisions.
// If the diff just contains replace operation of spec.containers[x].image, it will returns an UpdateSpec.
// Otherwise, it returns nil which means can not use in-place update.
func defaultCalculateInPlaceUpdateSpec(oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) *UpdateSpec {
	if oldRevision == nil || newRevision == nil {
		return nil
	}
	opts = SetOptionsDefaults(opts)

	patches, err := jsonpatch.CreatePatch(oldRevision.Data.Raw, newRevision.Data.Raw)
	if err != nil {
		return nil
	}

	oldTemp, err := GetTemplateFromRevision(oldRevision)
	if err != nil {
		return nil
	}
	newTemp, err := GetTemplateFromRevision(newRevision)
	if err != nil {
		return nil
	}

	updateSpec := &UpdateSpec{
		Revision:        newRevision.Name,
		ContainerImages: make(map[string]string),
		GraceSeconds:    opts.GracePeriodSeconds,
	}
	if opts.GetRevision != nil {
		updateSpec.Revision = opts.GetRevision(newRevision)
	}

	// all patches for podSpec can just update images
	var metadataChanged bool
	for _, jsonPatchOperation := range patches {
		jsonPatchOperation.Path = strings.Replace(jsonPatchOperation.Path, "/spec/template", "", 1)

		if !strings.HasPrefix(jsonPatchOperation.Path, "/spec/") {
			metadataChanged = true
			continue
		}
		if jsonPatchOperation.Operation != "replace" || !inPlaceUpdatePatchRexp.MatchString(jsonPatchOperation.Path) {
			return nil
		}
		// for example: /spec/containers/0/image
		words := strings.Split(jsonPatchOperation.Path, "/")
		idx, _ := strconv.Atoi(words[3])
		if len(oldTemp.Spec.Containers) <= idx {
			return nil
		}
		updateSpec.ContainerImages[oldTemp.Spec.Containers[idx].Name] = jsonPatchOperation.Value.(string)
	}
	if metadataChanged {
		oldBytes, _ := json.Marshal(v1.Pod{ObjectMeta: oldTemp.ObjectMeta})
		newBytes, _ := json.Marshal(v1.Pod{ObjectMeta: newTemp.ObjectMeta})
		patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldBytes, newBytes, &v1.Pod{})
		if err != nil {
			return nil
		}
		updateSpec.MetaDataPatch = patchBytes

		if utilfeature.DefaultFeatureGate.Enabled(features.InPlaceUpdateEnvFromMetadata) {
			hasher := utilcontainermeta.NewEnvFromMetadataHasher()
			for i := range newTemp.Spec.Containers {
				c := &newTemp.Spec.Containers[i]
				oldHashWithEnvFromMetadata := hasher.GetExpectHash(c, oldTemp)
				newHashWithEnvFromMetadata := hasher.GetExpectHash(c, newTemp)
				if oldHashWithEnvFromMetadata != newHashWithEnvFromMetadata {
					updateSpec.UpdateEnvFromMetadata = true
					break
				}
			}
		}
	}
	return updateSpec
}

// DefaultCheckInPlaceUpdateCompleted checks whether imageID in pod status has been changed since in-place update.
// If the imageID in containerStatuses has not been changed, we assume that kubelet has not updated
// containers in Pod.
func DefaultCheckInPlaceUpdateCompleted(pod *v1.Pod) error {
	if _, isInGraceState := appspub.GetInPlaceUpdateGrace(pod); isInGraceState {
		return fmt.Errorf("still in grace period of in-place update")
	}

	inPlaceUpdateState := appspub.InPlaceUpdateState{}
	if stateStr, ok := appspub.GetInPlaceUpdateState(pod); !ok {
		return nil
	} else if err := json.Unmarshal([]byte(stateStr), &inPlaceUpdateState); err != nil {
		return err
	}

	runtimeContainerMetaSet, err := appspub.GetRuntimeContainerMetaSet(pod)
	if err != nil {
		return err
	}

	if inPlaceUpdateState.UpdateEnvFromMetadata {
		if runtimeContainerMetaSet == nil {
			return fmt.Errorf("waiting for all containers hash consistent, but runtime-container-meta not found")
		}
		if !checkAllContainersHashConsistent(pod, runtimeContainerMetaSet, extractedEnvFromMetadataHash) {
			return fmt.Errorf("waiting for all containers hash consistent")
		}
	}

	if runtimeContainerMetaSet != nil {
		if checkAllContainersHashConsistent(pod, runtimeContainerMetaSet, plainHash) {
			klog.V(5).Infof("Check Pod %s/%s in-place update completed for all container hash consistent", pod.Namespace, pod.Name)
			return nil
		}
		// If it needs not to update envs from metadata, we don't have to return error here,
		// in case kruise-daemon has broken for some reason and runtime-container-meta is still in an old version.
	}

	containerImages := make(map[string]string, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		containerImages[c.Name] = c.Image
		if len(strings.Split(c.Image, ":")) <= 1 {
			containerImages[c.Name] = fmt.Sprintf("%s:latest", c.Image)
		}
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if oldStatus, ok := inPlaceUpdateState.LastContainerStatuses[cs.Name]; ok {
			// TODO: we assume that users should not update workload template with new image which actually has the same imageID as the old image
			if oldStatus.ImageID == cs.ImageID {
				if containerImages[cs.Name] != cs.Image {
					return fmt.Errorf("container %s imageID not changed", cs.Name)
				}
			}
			delete(inPlaceUpdateState.LastContainerStatuses, cs.Name)
		}
	}

	if len(inPlaceUpdateState.LastContainerStatuses) > 0 {
		return fmt.Errorf("not found statuses of containers %v", inPlaceUpdateState.LastContainerStatuses)
	}

	return nil
}

type hashType string

const (
	plainHash                    hashType = "PlainHash"
	extractedEnvFromMetadataHash hashType = "ExtractedEnvFromMetadataHash"
)

// The requirements for hash consistent:
// 1. all containers in spec.containers should also be in status.containerStatuses and runtime-container-meta
// 2. all containers in status.containerStatuses and runtime-container-meta should have the same containerID
// 3. all containers in spec.containers and runtime-container-meta should have the same hashes
func checkAllContainersHashConsistent(pod *v1.Pod, runtimeContainerMetaSet *appspub.RuntimeContainerMetaSet, hashType hashType) bool {
	for i := range pod.Spec.Containers {
		containerSpec := &pod.Spec.Containers[i]

		var containerStatus *v1.ContainerStatus
		for j := range pod.Status.ContainerStatuses {
			if pod.Status.ContainerStatuses[j].Name == containerSpec.Name {
				containerStatus = &pod.Status.ContainerStatuses[j]
				break
			}
		}
		if containerStatus == nil {
			klog.Warningf("Find no container %s in status for Pod %s/%s", containerSpec.Name, pod.Namespace, pod.Name)
			return false
		}

		var containerMeta *appspub.RuntimeContainerMeta
		for i := range runtimeContainerMetaSet.Containers {
			if runtimeContainerMetaSet.Containers[i].Name == containerSpec.Name {
				containerMeta = &runtimeContainerMetaSet.Containers[i]
				continue
			}
		}
		if containerMeta == nil {
			klog.Warningf("Find no container %s in runtime-container-meta for Pod %s/%s", containerSpec.Name, pod.Namespace, pod.Name)
			return false
		}

		if containerMeta.ContainerID != containerStatus.ContainerID {
			klog.Warningf("Find container %s in runtime-container-meta for Pod %s/%s has different containerID with status %s != %s",
				containerSpec.Name, pod.Namespace, pod.Name, containerMeta.ContainerID, containerStatus.ContainerID)
			return false
		}

		switch hashType {
		case plainHash:
			if expectedHash := kubeletcontainer.HashContainer(containerSpec); containerMeta.Hashes.PlainHash != expectedHash {
				klog.Warningf("Find container %s in runtime-container-meta for Pod %s/%s has different plain hash with spec %v != %v",
					containerSpec.Name, pod.Namespace, pod.Name, containerMeta.Hashes.PlainHash, expectedHash)
				return false
			}
		case extractedEnvFromMetadataHash:
			hasher := utilcontainermeta.NewEnvFromMetadataHasher()
			if expectedHash := hasher.GetExpectHash(containerSpec, pod); containerMeta.Hashes.ExtractedEnvFromMetadataHash != expectedHash {
				klog.Warningf("Find container %s in runtime-container-meta for Pod %s/%s has different extractedEnvFromMetadataHash with spec %v != %v",
					containerSpec.Name, pod.Namespace, pod.Name, containerMeta.Hashes.ExtractedEnvFromMetadataHash, expectedHash)
				return false
			}
		}
	}

	return true
}
