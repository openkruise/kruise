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
	"github.com/openkruise/kruise/pkg/util"
	utilcontainerlaunchpriority "github.com/openkruise/kruise/pkg/util/containerlaunchpriority"
	utilcontainermeta "github.com/openkruise/kruise/pkg/util/containermeta"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/volumeclaimtemplate"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/klog/v2"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

func SetOptionsDefaults(opts *UpdateOptions) *UpdateOptions {
	if opts == nil {
		opts = &UpdateOptions{}
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.InPlaceWorkloadVerticalScaling) {
		if opts.CalculateSpec == nil {
			opts.CalculateSpec = defaultCalculateInPlaceUpdateSpecWithVerticalUpdate
		}

		if opts.PatchSpecToPod == nil {
			opts.PatchSpecToPod = defaultPatchUpdateSpecToPodWithVerticalUpdate
		}

		if opts.CheckPodUpdateCompleted == nil {
			opts.CheckPodUpdateCompleted = DefaultCheckInPlaceUpdateCompletedWithVerticalUpdate
		}

		if opts.CheckContainersUpdateCompleted == nil {
			opts.CheckContainersUpdateCompleted = defaultCheckContainersInPlaceUpdateCompletedWithVerticalUpdate
		}

	} else {
		if opts.CalculateSpec == nil {
			opts.CalculateSpec = defaultCalculateInPlaceUpdateSpec
		}

		if opts.PatchSpecToPod == nil {
			opts.PatchSpecToPod = defaultPatchUpdateSpecToPod
		}

		if opts.CheckPodUpdateCompleted == nil {
			opts.CheckPodUpdateCompleted = DefaultCheckInPlaceUpdateCompleted
		}

		if opts.CheckContainersUpdateCompleted == nil {
			opts.CheckContainersUpdateCompleted = defaultCheckContainersInPlaceUpdateCompleted
		}

	}

	return opts
}

// defaultPatchUpdateSpecToPod returns new pod that merges spec into old pod
func defaultPatchUpdateSpecToPod(pod *v1.Pod, spec *UpdateSpec, state *appspub.InPlaceUpdateState) (*v1.Pod, error) {

	klog.V(5).InfoS("Begin to in-place update pod", "namespace", pod.Namespace, "name", pod.Name, "spec", util.DumpJSON(spec), "state", util.DumpJSON(state))

	state.NextContainerImages = make(map[string]string)
	state.NextContainerRefMetadata = make(map[string]metav1.ObjectMeta)
	state.NextContainerResources = make(map[string]v1.ResourceRequirements)

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

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	// prepare containers that should update this time and next time, according to their priorities
	containersToUpdate := sets.NewString()
	var highestPriority *int
	var containersWithHighestPriority []string
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		_, existImage := spec.ContainerImages[c.Name]
		_, existMetadata := spec.ContainerRefMetadata[c.Name]
		if !existImage && !existMetadata {
			continue
		}
		priority := utilcontainerlaunchpriority.GetContainerPriority(c)
		if priority == nil {
			containersToUpdate.Insert(c.Name)
		} else if highestPriority == nil || *highestPriority < *priority {
			highestPriority = priority
			containersWithHighestPriority = []string{c.Name}
		} else if *highestPriority == *priority {
			containersWithHighestPriority = append(containersWithHighestPriority, c.Name)
		}
	}
	for _, cName := range containersWithHighestPriority {
		containersToUpdate.Insert(cName)
	}
	addMetadataSharedContainersToUpdate(pod, containersToUpdate, spec.ContainerRefMetadata)

	// DO NOT modify the fields in spec for it may have to retry on conflict in updatePodInPlace

	// update images and record current imageIDs for the containers to update
	containersImageChanged := sets.NewString()
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		newImage, exists := spec.ContainerImages[c.Name]
		if !exists {
			continue
		}
		if containersToUpdate.Has(c.Name) {
			pod.Spec.Containers[i].Image = newImage
			containersImageChanged.Insert(c.Name)
		} else {
			state.NextContainerImages[c.Name] = newImage
		}
	}
	for _, c := range pod.Status.ContainerStatuses {
		if containersImageChanged.Has(c.Name) {
			if state.LastContainerStatuses == nil {
				state.LastContainerStatuses = map[string]appspub.InPlaceUpdateContainerStatus{}
			}
			state.LastContainerStatuses[c.Name] = appspub.InPlaceUpdateContainerStatus{ImageID: c.ImageID}
		}
	}

	// update annotations and labels for the containers to update
	for cName, objMeta := range spec.ContainerRefMetadata {
		if containersToUpdate.Has(cName) {
			for k, v := range objMeta.Labels {
				pod.Labels[k] = v
			}
			for k, v := range objMeta.Annotations {
				pod.Annotations[k] = v
			}
		} else {
			state.NextContainerRefMetadata[cName] = objMeta
		}
	}

	// add the containers that update this time into PreCheckBeforeNext, so that next containers can only
	// start to update when these containers have updated ready
	// TODO: currently we only support ContainersRequiredReady, not sure if we have to add ContainersPreferredReady in future
	if len(state.NextContainerImages) > 0 || len(state.NextContainerRefMetadata) > 0 {
		state.PreCheckBeforeNext = &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: containersToUpdate.List()}
	} else {
		state.PreCheckBeforeNext = nil
	}

	state.ContainerBatchesRecord = append(state.ContainerBatchesRecord, appspub.InPlaceUpdateContainerBatch{
		Timestamp:  metav1.NewTime(Clock.Now()),
		Containers: containersToUpdate.List(),
	})

	klog.V(5).InfoS("Decide to in-place update pod", "namespace", pod.Namespace, "name", pod.Name, "state", util.DumpJSON(state))

	inPlaceUpdateStateJSON, _ := json.Marshal(state)
	pod.Annotations[appspub.InPlaceUpdateStateKey] = string(inPlaceUpdateStateJSON)
	return pod, nil
}

func addMetadataSharedContainersToUpdate(pod *v1.Pod, containersToUpdate sets.String, containerRefMetadata map[string]metav1.ObjectMeta) {
	labelsToUpdate := sets.NewString()
	annotationsToUpdate := sets.NewString()
	newToUpdate := containersToUpdate
	// We need a for-loop to merge the indirect shared containers
	for newToUpdate.Len() > 0 {
		for _, cName := range newToUpdate.UnsortedList() {
			if objMeta, exists := containerRefMetadata[cName]; exists {
				for key := range objMeta.Labels {
					labelsToUpdate.Insert(key)
				}
				for key := range objMeta.Annotations {
					annotationsToUpdate.Insert(key)
				}
			}
		}
		newToUpdate = sets.NewString()

		for cName, objMeta := range containerRefMetadata {
			if containersToUpdate.Has(cName) {
				continue
			}
			for _, key := range labelsToUpdate.UnsortedList() {
				if _, exists := objMeta.Labels[key]; exists {
					klog.InfoS("Has to in-place update container with lower priority in Pod, for the label it shared has changed",
						"containerName", cName, "namespace", pod.Namespace, "name", pod.Name, "label", key)
					containersToUpdate.Insert(cName)
					newToUpdate.Insert(cName)
					break
				}
			}
			for _, key := range annotationsToUpdate.UnsortedList() {
				if _, exists := objMeta.Annotations[key]; exists {
					klog.InfoS("Has to in-place update container with lower priority in Pod, for the annotation it shared has changed",
						"containerName", cName, "namespace", pod.Namespace, "podName", pod.Name, "annotation", key)
					containersToUpdate.Insert(cName)
					newToUpdate.Insert(cName)
					break
				}
			}
		}
	}
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

	// RecreatePodWhenChangeVCTInCloneSetGate enabled
	if utilfeature.DefaultFeatureGate.Enabled(features.RecreatePodWhenChangeVCTInCloneSetGate) {
		if !opts.IgnoreVolumeClaimTemplatesHashDiff {
			canInPlace := volumeclaimtemplate.CanVCTemplateInplaceUpdate(oldRevision, newRevision)
			if !canInPlace {
				return nil
			}
		}
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
		Revision:             newRevision.Name,
		ContainerImages:      make(map[string]string),
		ContainerRefMetadata: make(map[string]metav1.ObjectMeta),
		GraceSeconds:         opts.GracePeriodSeconds,
	}
	if opts.GetRevision != nil {
		updateSpec.Revision = opts.GetRevision(newRevision)
	}

	// all patches for podSpec can just update images in pod spec
	var metadataPatches []jsonpatch.Operation
	for _, op := range patches {
		op.Path = strings.Replace(op.Path, "/spec/template", "", 1)

		if !strings.HasPrefix(op.Path, "/spec/") {
			if strings.HasPrefix(op.Path, "/metadata/") {
				metadataPatches = append(metadataPatches, op)
				continue
			}
			return nil
		}
		if op.Operation != "replace" || !containerImagePatchRexp.MatchString(op.Path) {
			return nil
		}
		// for example: /spec/containers/0/image
		words := strings.Split(op.Path, "/")
		idx, _ := strconv.Atoi(words[3])
		if len(oldTemp.Spec.Containers) <= idx {
			return nil
		}
		updateSpec.ContainerImages[oldTemp.Spec.Containers[idx].Name] = op.Value.(string)
	}

	if len(metadataPatches) > 0 {
		if utilfeature.DefaultFeatureGate.Enabled(features.InPlaceUpdateEnvFromMetadata) {
			// for example: /metadata/labels/my-label-key
			for _, op := range metadataPatches {
				if op.Operation != "replace" && op.Operation != "add" {
					continue
				}
				words := strings.SplitN(op.Path, "/", 4)
				if len(words) != 4 || (words[2] != "labels" && words[2] != "annotations") {
					continue
				}
				key := rfc6901Decoder.Replace(words[3])

				for i := range newTemp.Spec.Containers {
					c := &newTemp.Spec.Containers[i]
					objMeta := updateSpec.ContainerRefMetadata[c.Name]
					switch words[2] {
					case "labels":
						if !utilcontainermeta.IsContainerReferenceToMeta(c, "metadata.labels", key) {
							continue
						}
						if objMeta.Labels == nil {
							objMeta.Labels = make(map[string]string)
						}
						objMeta.Labels[key] = op.Value.(string)
						delete(oldTemp.ObjectMeta.Labels, key)
						delete(newTemp.ObjectMeta.Labels, key)

					case "annotations":
						if !utilcontainermeta.IsContainerReferenceToMeta(c, "metadata.annotations", key) {
							continue
						}
						if objMeta.Annotations == nil {
							objMeta.Annotations = make(map[string]string)
						}
						objMeta.Annotations[key] = op.Value.(string)
						delete(oldTemp.ObjectMeta.Annotations, key)
						delete(newTemp.ObjectMeta.Annotations, key)
					}

					updateSpec.ContainerRefMetadata[c.Name] = objMeta
					updateSpec.UpdateEnvFromMetadata = true
				}
			}
		}

		oldBytes, _ := json.Marshal(v1.Pod{ObjectMeta: oldTemp.ObjectMeta})
		newBytes, _ := json.Marshal(v1.Pod{ObjectMeta: newTemp.ObjectMeta})
		patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldBytes, newBytes, &v1.Pod{})
		if err != nil {
			return nil
		}
		updateSpec.MetaDataPatch = patchBytes
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
	if len(inPlaceUpdateState.NextContainerImages) > 0 || len(inPlaceUpdateState.NextContainerRefMetadata) > 0 {
		return fmt.Errorf("existing containers to in-place update in next batches")
	}

	return defaultCheckContainersInPlaceUpdateCompleted(pod, &inPlaceUpdateState)
}

func defaultCheckContainersInPlaceUpdateCompleted(pod *v1.Pod, inPlaceUpdateState *appspub.InPlaceUpdateState) error {
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
			klog.V(5).InfoS("Check Pod in-place update completed for all container hash consistent", "namespace", pod.Namespace, "name", pod.Name)
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
			klog.InfoS("Find no container in status for Pod", "containerName", containerSpec.Name, "namespace", pod.Namespace, "podName", pod.Name)
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
			klog.InfoS("Find no container in runtime-container-meta for Pod", "containerName", containerSpec.Name, "namespace", pod.Namespace, "podName", pod.Name)
			return false
		}

		if containerMeta.ContainerID != containerStatus.ContainerID {
			klog.InfoS("Find container in runtime-container-meta for Pod has different containerID with status",
				"containerName", containerSpec.Name, "namespace", pod.Namespace, "podName", pod.Name,
				"metaID", containerMeta.ContainerID, "statusID", containerStatus.ContainerID)
			return false
		}

		switch hashType {
		case plainHash:
			if expectedHash := kubeletcontainer.HashContainer(containerSpec); containerMeta.Hashes.PlainHash != expectedHash {
				klog.InfoS("Find container in runtime-container-meta for Pod has different plain hash with spec",
					"containerName", containerSpec.Name, "namespace", pod.Namespace, "podName", pod.Name,
					"metaHash", containerMeta.Hashes.PlainHash, "expectedHash", expectedHash)
				return false
			}
		case extractedEnvFromMetadataHash:
			hasher := utilcontainermeta.NewEnvFromMetadataHasher()
			if expectedHash := hasher.GetExpectHash(containerSpec, pod); containerMeta.Hashes.ExtractedEnvFromMetadataHash != expectedHash {
				klog.InfoS("Find container in runtime-container-meta for Pod has different extractedEnvFromMetadataHash with spec",
					"containerName", containerSpec.Name, "namespace", pod.Namespace, "podName", pod.Name,
					"metaHash", containerMeta.Hashes.ExtractedEnvFromMetadataHash, "expectedHash", expectedHash)
				return false
			}
		}
	}

	return true
}

// defaultPatchUpdateSpecToPod returns new pod that merges spec into old pod
func defaultPatchUpdateSpecToPodWithVerticalUpdate(pod *v1.Pod, spec *UpdateSpec, state *appspub.InPlaceUpdateState) (*v1.Pod, error) {
	klog.V(5).Infof("Begin to in-place update pod %s/%s with update spec %v, state %v", pod.Namespace, pod.Name, util.DumpJSON(spec), util.DumpJSON(state))

	state.NextContainerImages = make(map[string]string)
	state.NextContainerRefMetadata = make(map[string]metav1.ObjectMeta)
	state.NextContainerResources = make(map[string]v1.ResourceRequirements)

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

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	// prepare containers that should update this time and next time, according to their priorities
	containersToUpdate := sets.NewString()
	var highestPriority *int
	var containersWithHighestPriority []string
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		_, existImage := spec.ContainerImages[c.Name]
		_, existMetadata := spec.ContainerRefMetadata[c.Name]
		_, existResource := spec.ContainerResources[c.Name]
		if !existImage && !existMetadata && !existResource {
			continue
		}
		priority := utilcontainerlaunchpriority.GetContainerPriority(c)
		if priority == nil {
			containersToUpdate.Insert(c.Name)
		} else if highestPriority == nil || *highestPriority < *priority {
			highestPriority = priority
			containersWithHighestPriority = []string{c.Name}
		} else if *highestPriority == *priority {
			containersWithHighestPriority = append(containersWithHighestPriority, c.Name)
		}
	}
	for _, cName := range containersWithHighestPriority {
		containersToUpdate.Insert(cName)
	}
	addMetadataSharedContainersToUpdate(pod, containersToUpdate, spec.ContainerRefMetadata)

	// DO NOT modify the fields in spec for it may have to retry on conflict in updatePodInPlace

	// update images and record current imageIDs for the containers to update
	containersImageChanged := sets.NewString()
	containersResourceChanged := sets.NewString()
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		newImage, imageExists := spec.ContainerImages[c.Name]
		newResource, resourceExists := spec.ContainerResources[c.Name]
		if !imageExists && !resourceExists {
			continue
		}
		if containersToUpdate.Has(c.Name) {
			if imageExists {
				pod.Spec.Containers[i].Image = newImage
				containersImageChanged.Insert(c.Name)
			}
			if resourceExists {
				for key, quantity := range newResource.Limits {
					c.Resources.Limits[key] = quantity
				}
				for key, quantity := range newResource.Requests {
					c.Resources.Requests[key] = quantity
				}
				containersResourceChanged.Insert(c.Name)
			}
		} else {
			state.NextContainerImages[c.Name] = newImage
			state.NextContainerResources[c.Name] = newResource
		}
	}
	for _, c := range pod.Status.ContainerStatuses {
		if containersImageChanged.Has(c.Name) {
			if state.LastContainerStatuses == nil {
				state.LastContainerStatuses = map[string]appspub.InPlaceUpdateContainerStatus{}
			}
			state.LastContainerStatuses[c.Name] = appspub.InPlaceUpdateContainerStatus{ImageID: c.ImageID}
		}
		// TODO(LavenderQAQ): The status of resource needs to be printed
	}

	// update annotations and labels for the containers to update
	for cName, objMeta := range spec.ContainerRefMetadata {
		if containersToUpdate.Has(cName) {
			for k, v := range objMeta.Labels {
				pod.Labels[k] = v
			}
			for k, v := range objMeta.Annotations {
				pod.Annotations[k] = v
			}
		} else {
			state.NextContainerRefMetadata[cName] = objMeta
		}
	}

	// add the containers that update this time into PreCheckBeforeNext, so that next containers can only
	// start to update when these containers have updated ready
	// TODO: currently we only support ContainersRequiredReady, not sure if we have to add ContainersPreferredReady in future
	if len(state.NextContainerImages) > 0 || len(state.NextContainerRefMetadata) > 0 || len(state.NextContainerResources) > 0 {
		state.PreCheckBeforeNext = &appspub.InPlaceUpdatePreCheckBeforeNext{ContainersRequiredReady: containersToUpdate.List()}
	} else {
		state.PreCheckBeforeNext = nil
	}

	state.ContainerBatchesRecord = append(state.ContainerBatchesRecord, appspub.InPlaceUpdateContainerBatch{
		Timestamp:  metav1.NewTime(Clock.Now()),
		Containers: containersToUpdate.List(),
	})

	klog.V(5).Infof("Decide to in-place update pod %s/%s with state %v", pod.Namespace, pod.Name, util.DumpJSON(state))

	inPlaceUpdateStateJSON, _ := json.Marshal(state)
	pod.Annotations[appspub.InPlaceUpdateStateKey] = string(inPlaceUpdateStateJSON)
	return pod, nil
}

// defaultCalculateInPlaceUpdateSpec calculates diff between old and update revisions.
// If the diff just contains replace operation of spec.containers[x].image, it will returns an UpdateSpec.
// Otherwise, it returns nil which means can not use in-place update.
func defaultCalculateInPlaceUpdateSpecWithVerticalUpdate(oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) *UpdateSpec {
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
		Revision:             newRevision.Name,
		ContainerImages:      make(map[string]string),
		ContainerRefMetadata: make(map[string]metav1.ObjectMeta),
		ContainerResources:   make(map[string]v1.ResourceRequirements),
		GraceSeconds:         opts.GracePeriodSeconds,
	}
	if opts.GetRevision != nil {
		updateSpec.Revision = opts.GetRevision(newRevision)
	}

	// all patches for podSpec can just update images in pod spec
	var metadataPatches []jsonpatch.Operation
	for _, op := range patches {
		op.Path = strings.Replace(op.Path, "/spec/template", "", 1)

		if !strings.HasPrefix(op.Path, "/spec/") {
			if strings.HasPrefix(op.Path, "/metadata/") {
				metadataPatches = append(metadataPatches, op)
				continue
			}
			return nil
		}

		if op.Operation != "replace" {
			return nil
		}

		isImageUpdate := containerImagePatchRexp.MatchString(op.Path)
		isResourceUpdate := containerResourcesPatchRexp.MatchString(op.Path)

		if isImageUpdate {
			// for example: /spec/containers/0/image
			words := strings.Split(op.Path, "/")
			idx, _ := strconv.Atoi(words[3])
			if len(oldTemp.Spec.Containers) <= idx {
				return nil
			}
			updateSpec.ContainerImages[oldTemp.Spec.Containers[idx].Name] = op.Value.(string)
		} else if isResourceUpdate {
			// for example: /spec/containers/0/resources/limits/cpu
			words := strings.Split(op.Path, "/")
			if len(words) != 7 {
				return nil
			}
			idx, err := strconv.Atoi(words[3])
			if err != nil || len(oldTemp.Spec.Containers) <= idx {
				return nil
			}
			quantity, err := resource.ParseQuantity(op.Value.(string))
			if err != nil {
				klog.Errorf("parse quantity error: %v", err)
				return nil
			}

			if _, ok := updateSpec.ContainerResources[oldTemp.Spec.Containers[idx].Name]; !ok {
				updateSpec.ContainerResources[oldTemp.Spec.Containers[idx].Name] = v1.ResourceRequirements{
					Limits:   make(v1.ResourceList),
					Requests: make(v1.ResourceList),
				}
			}
			switch words[5] {
			case "limits":
				updateSpec.ContainerResources[oldTemp.Spec.Containers[idx].Name].Limits[v1.ResourceName(words[6])] = quantity
			case "requests":
				updateSpec.ContainerResources[oldTemp.Spec.Containers[idx].Name].Requests[v1.ResourceName(words[6])] = quantity
			}
		} else {
			return nil
		}
	}

	if len(metadataPatches) > 0 {
		if utilfeature.DefaultFeatureGate.Enabled(features.InPlaceUpdateEnvFromMetadata) {
			// for example: /metadata/labels/my-label-key
			for _, op := range metadataPatches {
				if op.Operation != "replace" && op.Operation != "add" {
					continue
				}
				words := strings.SplitN(op.Path, "/", 4)
				if len(words) != 4 || (words[2] != "labels" && words[2] != "annotations") {
					continue
				}
				key := rfc6901Decoder.Replace(words[3])

				for i := range newTemp.Spec.Containers {
					c := &newTemp.Spec.Containers[i]
					objMeta := updateSpec.ContainerRefMetadata[c.Name]
					switch words[2] {
					case "labels":
						if !utilcontainermeta.IsContainerReferenceToMeta(c, "metadata.labels", key) {
							continue
						}
						if objMeta.Labels == nil {
							objMeta.Labels = make(map[string]string)
						}
						objMeta.Labels[key] = op.Value.(string)
						delete(oldTemp.ObjectMeta.Labels, key)
						delete(newTemp.ObjectMeta.Labels, key)

					case "annotations":
						if !utilcontainermeta.IsContainerReferenceToMeta(c, "metadata.annotations", key) {
							continue
						}
						if objMeta.Annotations == nil {
							objMeta.Annotations = make(map[string]string)
						}
						objMeta.Annotations[key] = op.Value.(string)
						delete(oldTemp.ObjectMeta.Annotations, key)
						delete(newTemp.ObjectMeta.Annotations, key)
					}

					updateSpec.ContainerRefMetadata[c.Name] = objMeta
					updateSpec.UpdateEnvFromMetadata = true
				}
			}
		}

		oldBytes, _ := json.Marshal(v1.Pod{ObjectMeta: oldTemp.ObjectMeta})
		newBytes, _ := json.Marshal(v1.Pod{ObjectMeta: newTemp.ObjectMeta})
		patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldBytes, newBytes, &v1.Pod{})
		if err != nil {
			return nil
		}
		updateSpec.MetaDataPatch = patchBytes
	}
	return updateSpec
}

// DefaultCheckInPlaceUpdateCompleted checks whether imageID in pod status has been changed since in-place update.
// If the imageID in containerStatuses has not been changed, we assume that kubelet has not updated
// containers in Pod.
func DefaultCheckInPlaceUpdateCompletedWithVerticalUpdate(pod *v1.Pod) error {
	if _, isInGraceState := appspub.GetInPlaceUpdateGrace(pod); isInGraceState {
		return fmt.Errorf("still in grace period of in-place update")
	}

	inPlaceUpdateState := appspub.InPlaceUpdateState{}
	if stateStr, ok := appspub.GetInPlaceUpdateState(pod); !ok {
		return nil
	} else if err := json.Unmarshal([]byte(stateStr), &inPlaceUpdateState); err != nil {
		return err
	}
	if len(inPlaceUpdateState.NextContainerImages) > 0 || len(inPlaceUpdateState.NextContainerRefMetadata) > 0 || len(inPlaceUpdateState.NextContainerResources) > 0 {
		return fmt.Errorf("existing containers to in-place update in next batches")
	}

	return defaultCheckContainersInPlaceUpdateCompletedWithVerticalUpdate(pod, &inPlaceUpdateState)
}

func defaultCheckContainersInPlaceUpdateCompletedWithVerticalUpdate(pod *v1.Pod, inPlaceUpdateState *appspub.InPlaceUpdateState) error {
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
	containerResources := make(map[string]v1.ResourceRequirements, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		containerImages[c.Name] = c.Image
		containerResources[c.Name] = c.Resources
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
			// TODO(LavenderQAQ): Check the vertical updating status of the container
			delete(inPlaceUpdateState.LastContainerStatuses, cs.Name)
		}
	}

	if len(inPlaceUpdateState.LastContainerStatuses) > 0 {
		return fmt.Errorf("not found statuses of containers %v", inPlaceUpdateState.LastContainerStatuses)
	}

	return nil
}
