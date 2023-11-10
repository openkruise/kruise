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

package mutating

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/history"

	admissionv1 "k8s.io/api/admission/v1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// mutate pod based on SidecarSet Object
func (h *PodCreateHandler) sidecarsetMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) (skip bool, err error) {
	if len(req.AdmissionRequest.SubResource) > 0 ||
		(req.AdmissionRequest.Operation != admissionv1.Create && req.AdmissionRequest.Operation != admissionv1.Update) ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}
	// filter out pods that don't require inject
	if !sidecarcontrol.IsActivePod(pod) {
		return true, nil
	}

	var oldPod *corev1.Pod
	var isUpdated bool
	//when Operation is update, decode older object
	if req.AdmissionRequest.Operation == admissionv1.Update {
		isUpdated = true
		oldPod = new(corev1.Pod)
		if err = h.Decoder.Decode(
			admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{Object: req.AdmissionRequest.OldObject}},
			oldPod); err != nil {
			return false, err
		}
	}

	// DisableDeepCopy:true, indicates must be deep copy before update sidecarSet objection
	sidecarsetList := &appsv1alpha1.SidecarSetList{}
	if err = h.Client.List(ctx, sidecarsetList, utilclient.DisableDeepCopy); err != nil {
		return false, err
	}

	matchedSidecarSets := make([]sidecarcontrol.SidecarControl, 0)
	for _, sidecarSet := range sidecarsetList.Items {
		if sidecarSet.Spec.InjectionStrategy.Paused {
			continue
		}
		if matched, err := sidecarcontrol.PodMatchedSidecarSet(h.Client, pod, &sidecarSet); err != nil {
			return false, err
		} else if !matched {
			continue
		}
		// get user-specific revision or the latest revision of SidecarSet
		suitableSidecarSet, err := h.getSuitableRevisionSidecarSet(&sidecarSet, oldPod, pod, req.AdmissionRequest.Operation)
		if err != nil {
			return false, err
		}
		// check whether sidecarSet is active
		// when sidecarSet is not active, it will not perform injections and upgrades process.
		control := sidecarcontrol.New(suitableSidecarSet)
		if !control.IsActiveSidecarSet() {
			continue
		}
		matchedSidecarSets = append(matchedSidecarSets, control)
	}
	if len(matchedSidecarSets) == 0 {
		return true, nil
	}

	// check pod
	if isUpdated {
		if !matchedSidecarSets[0].IsPodAvailabilityChanged(pod, oldPod) {
			klog.V(3).Infof("pod(%s/%s) availability unchanged for sidecarSet, and ignore", pod.Namespace, pod.Name)
			return true, nil
		}
	}

	klog.V(3).Infof("[sidecar inject] begin to operation(%s) pod(%s/%s) resources(%s) subResources(%s)",
		req.Operation, req.Namespace, req.Name, req.Resource, req.SubResource)
	// patch pod metadata, annotations & labels
	// When the Pod main container is upgraded in place, and the sidecarSet configuration does not change at this time,
	// at this point, it can also patch pod metadata
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	skip = true
	for _, control := range matchedSidecarSets {
		sidecarSet := control.GetSidecarset()
		sk, err := sidecarcontrol.PatchPodMetadata(&pod.ObjectMeta, sidecarSet.Spec.PatchPodMetadata)
		if err != nil {
			klog.Errorf("sidecarSet(%s) update pod(%s/%s) metadata failed: %s", sidecarSet.Name, pod.Namespace, pod.Name, err.Error())
			return false, err
		} else if !sk {
			// skip = false
			skip = false
		}
	}
	//build sidecar containers, sidecar initContainers, sidecar volumes, annotations to inject into pod object
	sidecarContainers, sidecarInitContainers, sidecarSecrets, volumesInSidecar, injectedAnnotations, err := buildSidecars(isUpdated, pod, oldPod, matchedSidecarSets)
	if err != nil {
		return false, err
	} else if len(sidecarContainers) == 0 && len(sidecarInitContainers) == 0 {
		klog.V(3).Infof("[sidecar inject] pod(%s/%s) don't have injected containers", pod.Namespace, pod.Name)
		return skip, nil
	}

	klog.V(3).Infof("[sidecar inject] begin inject sidecarContainers(%v) sidecarInitContainers(%v) sidecarSecrets(%v), volumes(%s)"+
		"annotations(%v) into pod(%s/%s)", sidecarContainers, sidecarInitContainers, sidecarSecrets, volumesInSidecar, injectedAnnotations,
		pod.Namespace, pod.Name)
	klog.V(4).Infof("[sidecar inject] before mutating: %v", util.DumpJSON(pod))
	// apply sidecar set info into pod
	// 1. inject init containers, sort by their name, after the original init containers
	sort.SliceStable(sidecarInitContainers, func(i, j int) bool {
		return sidecarInitContainers[i].Name < sidecarInitContainers[j].Name
	})
	for _, initContainer := range sidecarInitContainers {
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, initContainer.Container)
	}
	// 2. inject containers
	pod.Spec.Containers = mergeSidecarContainers(pod.Spec.Containers, sidecarContainers)
	// 3. inject volumes
	pod.Spec.Volumes = util.MergeVolumes(pod.Spec.Volumes, volumesInSidecar)
	// 4. inject imagePullSecrets
	pod.Spec.ImagePullSecrets = mergeSidecarSecrets(pod.Spec.ImagePullSecrets, sidecarSecrets)
	// 5. apply annotations
	for k, v := range injectedAnnotations {
		pod.Annotations[k] = v
	}
	klog.V(4).Infof("[sidecar inject] after mutating: %v", util.DumpJSON(pod))
	return false, nil
}

func (h *PodCreateHandler) getSuitableRevisionSidecarSet(sidecarSet *appsv1alpha1.SidecarSet, oldPod, newPod *corev1.Pod, operation admissionv1.Operation) (*appsv1alpha1.SidecarSet, error) {
	switch operation {
	case admissionv1.Update:
		// optimization: quickly return if newPod matched the latest sidecarSet
		if sidecarcontrol.GetPodSidecarSetRevision(sidecarSet.Name, newPod) == sidecarcontrol.GetSidecarSetRevision(sidecarSet) {
			return sidecarSet.DeepCopy(), nil
		}

		hc := sidecarcontrol.NewHistoryControl(h.Client)
		revisions, err := history.NewHistory(h.Client).ListControllerRevisions(sidecarcontrol.MockSidecarSetForRevision(sidecarSet), hc.GetRevisionSelector(sidecarSet))
		if err != nil {
			klog.Errorf("Failed to list history controllerRevisions, err %v, name %v", err, sidecarSet.Name)
			return nil, err
		}

		suitableSidecarSet, err := h.getSpecificRevisionSidecarSetForPod(sidecarSet, revisions, newPod)
		if err != nil {
			return nil, err
		} else if suitableSidecarSet != nil {
			return suitableSidecarSet, nil
		}

		suitableSidecarSet, err = h.getSpecificRevisionSidecarSetForPod(sidecarSet, revisions, oldPod)
		if err != nil {
			return nil, err
		} else if suitableSidecarSet != nil {
			return suitableSidecarSet, nil
		}

		return sidecarSet.DeepCopy(), nil

	default:
		revisionInfo := sidecarSet.Spec.InjectionStrategy.Revision
		if revisionInfo == nil || (revisionInfo.RevisionName == nil && revisionInfo.CustomVersion == nil) {
			return sidecarSet.DeepCopy(), nil
		}

		// TODO: support 'PartitionBased' policy to inject old/new revision according to Partition
		switch sidecarSet.Spec.InjectionStrategy.Revision.Policy {
		case "", appsv1alpha1.AlwaysSidecarSetInjectRevisionPolicy:
			return h.getSpecificHistorySidecarSet(sidecarSet, revisionInfo)
		}

		return h.getSpecificHistorySidecarSet(sidecarSet, revisionInfo)
	}
}

func (h *PodCreateHandler) getSpecificRevisionSidecarSetForPod(sidecarSet *appsv1alpha1.SidecarSet, revisions []*apps.ControllerRevision, pod *corev1.Pod) (*appsv1alpha1.SidecarSet, error) {
	var err error
	var matchedSidecarSet *appsv1alpha1.SidecarSet
	for _, revision := range revisions {
		if sidecarcontrol.GetPodSidecarSetControllerRevision(sidecarSet.Name, pod) == revision.Name {
			matchedSidecarSet, err = h.getSpecificHistorySidecarSet(sidecarSet, &appsv1alpha1.SidecarSetInjectRevision{RevisionName: &revision.Name})
			if err != nil {
				return nil, err
			}
			break
		}
	}
	return matchedSidecarSet, nil
}

func (h *PodCreateHandler) getSpecificHistorySidecarSet(sidecarSet *appsv1alpha1.SidecarSet, revisionInfo *appsv1alpha1.SidecarSetInjectRevision) (*appsv1alpha1.SidecarSet, error) {
	// else return its corresponding history revision
	hc := sidecarcontrol.NewHistoryControl(h.Client)
	historySidecarSet, err := hc.GetHistorySidecarSet(sidecarSet, revisionInfo)
	if err != nil {
		klog.Warningf("Failed to restore history revision for SidecarSet %v, ControllerRevision name %v:, error: %v",
			sidecarSet.Name, sidecarSet.Spec.InjectionStrategy.Revision, err)
		return nil, err
	}
	if historySidecarSet == nil {
		historySidecarSet = sidecarSet.DeepCopy()
		klog.Warningf("Failed to restore history revision for SidecarSet %v, will use the latest", sidecarSet.Name)
	}
	return historySidecarSet, nil
}

func mergeSidecarSecrets(secretsInPod, secretsInSidecar []corev1.LocalObjectReference) (allSecrets []corev1.LocalObjectReference) {
	secretFilter := make(map[string]bool)
	for _, podSecret := range secretsInPod {
		if _, ok := secretFilter[podSecret.Name]; !ok {
			secretFilter[podSecret.Name] = true
			allSecrets = append(allSecrets, podSecret)
		}
	}
	for _, sidecarSecret := range secretsInSidecar {
		if _, ok := secretFilter[sidecarSecret.Name]; !ok {
			secretFilter[sidecarSecret.Name] = true
			allSecrets = append(allSecrets, sidecarSecret)
		}
	}
	return allSecrets
}

func mergeSidecarContainers(origins []corev1.Container, injected []*appsv1alpha1.SidecarContainer) []corev1.Container {
	//format: pod.spec.containers[index].name -> index(the index of container in pod)
	containersInPod := make(map[string]int)
	for index, container := range origins {
		containersInPod[container.Name] = index
	}
	var beforeAppContainers []corev1.Container
	var afterAppContainers []corev1.Container
	for _, sidecar := range injected {
		//sidecar container already exist in pod
		//keep the order of pod's original containers unchanged
		if index, ok := containersInPod[sidecar.Name]; ok {
			origins[index] = sidecar.Container
			continue
		}

		switch sidecar.PodInjectPolicy {
		case appsv1alpha1.BeforeAppContainerType:
			beforeAppContainers = append(beforeAppContainers, sidecar.Container)
		case appsv1alpha1.AfterAppContainerType:
			afterAppContainers = append(afterAppContainers, sidecar.Container)
		default:
			beforeAppContainers = append(beforeAppContainers, sidecar.Container)
		}
	}
	origins = append(beforeAppContainers, origins...)
	origins = append(origins, afterAppContainers...)
	return origins
}

func buildSidecars(isUpdated bool, pod *corev1.Pod, oldPod *corev1.Pod, matchedSidecarSets []sidecarcontrol.SidecarControl) (
	sidecarContainers, sidecarInitContainers []*appsv1alpha1.SidecarContainer, sidecarSecrets []corev1.LocalObjectReference,
	volumesInSidecars []corev1.Volume, injectedAnnotations map[string]string, err error) {

	// injected annotations
	injectedAnnotations = make(map[string]string)
	// get sidecarSet annotations from pods
	// sidecarSet.name -> sidecarSet hash struct
	sidecarSetHash := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
	// sidecarSet.name -> sidecarSet hash(without image) struct
	sidecarSetHashWithoutImage := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
	// parse sidecar hash in pod annotations
	if oldHashStr := pod.Annotations[sidecarcontrol.SidecarSetHashAnnotation]; len(oldHashStr) > 0 {
		if err = json.Unmarshal([]byte(oldHashStr), &sidecarSetHash); err != nil {
			// to be compatible with older sidecarSet hash struct, map[string]string
			olderSidecarSetHash := make(map[string]string)
			if err = json.Unmarshal([]byte(oldHashStr), &olderSidecarSetHash); err != nil {
				return nil, nil, nil, nil, nil,
					fmt.Errorf("pod(%s/%s) invalid annotations[%s] value %v, unmarshal failed: %v", pod.Namespace, pod.Name, sidecarcontrol.SidecarSetHashAnnotation, oldHashStr, err)
			}
			for k, v := range olderSidecarSetHash {
				sidecarSetHash[k] = sidecarcontrol.SidecarSetUpgradeSpec{
					SidecarSetHash: v,
					SidecarSetName: k,
				}
			}
		}
	}
	if oldHashStr := pod.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation]; len(oldHashStr) > 0 {
		if err = json.Unmarshal([]byte(oldHashStr), &sidecarSetHashWithoutImage); err != nil {
			// to be compatible with older sidecarSet hash struct, map[string]string
			olderSidecarSetHash := make(map[string]string)
			if err = json.Unmarshal([]byte(oldHashStr), &olderSidecarSetHash); err != nil {
				return nil, nil, nil, nil, nil,
					fmt.Errorf("pod(%s/%s) invalid annotations[%s] value %v, unmarshal failed: %v", pod.Namespace, pod.Name, sidecarcontrol.SidecarSetHashWithoutImageAnnotation, oldHashStr, err)
			}
			for k, v := range olderSidecarSetHash {
				sidecarSetHashWithoutImage[k] = sidecarcontrol.SidecarSetUpgradeSpec{
					SidecarSetHash: v,
					SidecarSetName: k,
				}
			}
		}
	}
	// hotUpgrade work info, sidecarSet.spec.container[x].name -> pod.spec.container[x].name
	// for example: mesh -> mesh-1, envoy -> envoy-2
	hotUpgradeWorkInfo := sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(pod)
	// SidecarSet Name List, for example: log-sidecarset,envoy-sidecarset
	sidecarSetNames := sets.NewString()
	if sidecarSetListStr := pod.Annotations[sidecarcontrol.SidecarSetListAnnotation]; sidecarSetListStr != "" {
		sidecarSetNames.Insert(strings.Split(sidecarSetListStr, ",")...)
	}

	for _, control := range matchedSidecarSets {
		sidecarSet := control.GetSidecarset()
		klog.V(3).Infof("build pod(%s/%s) sidecar containers for sidecarSet(%s)", pod.Namespace, pod.Name, sidecarSet.Name)
		// sidecarSet List
		sidecarSetNames.Insert(sidecarSet.Name)
		// pre-process volumes only in sidecar
		volumesMap := getVolumesMapInSidecarSet(sidecarSet)
		// process sidecarset hash
		setUpgrade1 := sidecarcontrol.SidecarSetUpgradeSpec{
			UpdateTimestamp:              metav1.Now(),
			SidecarSetHash:               sidecarcontrol.GetSidecarSetRevision(sidecarSet),
			SidecarSetName:               sidecarSet.Name,
			SidecarSetControllerRevision: sidecarSet.Status.LatestRevision,
		}
		setUpgrade2 := sidecarcontrol.SidecarSetUpgradeSpec{
			UpdateTimestamp: metav1.Now(),
			SidecarSetHash:  sidecarcontrol.GetSidecarSetWithoutImageRevision(sidecarSet),
			SidecarSetName:  sidecarSet.Name,
		}

		isInjecting := false
		//process initContainers
		//only when created pod, inject initContainer and pullSecrets
		if !isUpdated {
			for i := range sidecarSet.Spec.InitContainers {
				initContainer := &sidecarSet.Spec.InitContainers[i]
				// volumeMounts that injected into sidecar container
				// when volumeMounts SubPathExpr contains expansions, then need copy container EnvVars(injectEnvs)
				injectedMounts, injectedEnvs := sidecarcontrol.GetInjectedVolumeMountsAndEnvs(control, initContainer, pod)
				// get injected env & mounts explicitly so that can be compared with old ones in pod
				transferEnvs := sidecarcontrol.GetSidecarTransferEnvs(initContainer, pod)
				// append volumeMounts SubPathExpr environments
				transferEnvs = util.MergeEnvVar(transferEnvs, injectedEnvs)
				klog.Infof("try to inject initContainer sidecar %v@%v/%v, with injected envs: %v, volumeMounts: %v",
					initContainer.Name, pod.Namespace, pod.Name, transferEnvs, injectedMounts)
				// insert volumes that initContainers used
				for _, mount := range initContainer.VolumeMounts {
					volumesInSidecars = append(volumesInSidecars, *volumesMap[mount.Name])
				}
				// merge VolumeMounts from sidecar.VolumeMounts and shared VolumeMounts
				initContainer.VolumeMounts = util.MergeVolumeMounts(initContainer.VolumeMounts, injectedMounts)
				// add "IS_INJECTED" env in initContainer's envs
				initContainer.Env = append(initContainer.Env, corev1.EnvVar{Name: sidecarcontrol.SidecarEnvKey, Value: "true"})
				// merged Env from sidecar.Env and transfer envs
				initContainer.Env = util.MergeEnvVar(initContainer.Env, transferEnvs)
				isInjecting = true
				sidecarInitContainers = append(sidecarInitContainers, initContainer)
			}
			//process imagePullSecrets
			sidecarSecrets = append(sidecarSecrets, sidecarSet.Spec.ImagePullSecrets...)
		}

		sidecarList := sets.NewString()
		//process containers
		for i := range sidecarSet.Spec.Containers {
			sidecarContainer := &sidecarSet.Spec.Containers[i]
			sidecarList.Insert(sidecarContainer.Name)
			// volumeMounts that injected into sidecar container
			// when volumeMounts SubPathExpr contains expansions, then need copy container EnvVars(injectEnvs)
			injectedMounts, injectedEnvs := sidecarcontrol.GetInjectedVolumeMountsAndEnvs(control, sidecarContainer, pod)
			// get injected env & mounts explicitly so that can be compared with old ones in pod
			transferEnvs := sidecarcontrol.GetSidecarTransferEnvs(sidecarContainer, pod)
			// append volumeMounts SubPathExpr environments
			transferEnvs = util.MergeEnvVar(transferEnvs, injectedEnvs)
			klog.Infof("try to inject Container sidecar %v@%v/%v, with injected envs: %v, volumeMounts: %v",
				sidecarContainer.Name, pod.Namespace, pod.Name, transferEnvs, injectedMounts)
			//when update pod object
			if isUpdated {
				// judge whether inject sidecar container into pod
				needInject, existSidecars, existVolumes := control.NeedToInjectInUpdatedPod(pod, oldPod, sidecarContainer, transferEnvs, injectedMounts)
				if !needInject {
					sidecarContainers = append(sidecarContainers, existSidecars...)
					volumesInSidecars = append(volumesInSidecars, existVolumes...)
					continue
				}

				klog.V(3).Infof("upgrade or insert sidecar container %v during upgrade in pod %v/%v",
					sidecarContainer.Name, pod.Namespace, pod.Name)
				//when created pod object, need inject sidecar container into pod
			} else {
				klog.V(3).Infof("inject new sidecar container %v during creation in pod %v/%v",
					sidecarContainer.Name, pod.Namespace, pod.Name)
			}
			isInjecting = true
			// insert volume that sidecar container used
			for _, mount := range sidecarContainer.VolumeMounts {
				volumesInSidecars = append(volumesInSidecars, *volumesMap[mount.Name])
			}
			// merge VolumeMounts from sidecar.VolumeMounts and shared VolumeMounts
			sidecarContainer.VolumeMounts = util.MergeVolumeMounts(sidecarContainer.VolumeMounts, injectedMounts)
			// add the "Injected" env to the sidecar container
			sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{Name: sidecarcontrol.SidecarEnvKey, Value: "true"})
			// merged Env from sidecar.Env and transfer envs
			sidecarContainer.Env = util.MergeEnvVar(sidecarContainer.Env, transferEnvs)

			// when sidecar container UpgradeStrategy is HotUpgrade
			if sidecarcontrol.IsHotUpgradeContainer(sidecarContainer) {
				hotContainers, annotations := injectHotUpgradeContainers(hotUpgradeWorkInfo, sidecarContainer)
				sidecarContainers = append(sidecarContainers, hotContainers...)
				for k, v := range annotations {
					injectedAnnotations[k] = v
				}
			} else {
				sidecarContainers = append(sidecarContainers, sidecarContainer)
			}
		}
		// the container was (re)injected and the annotations need to be updated
		if isInjecting {
			setUpgrade1.SidecarList = sidecarList.List()
			setUpgrade2.SidecarList = sidecarList.List()
			sidecarSetHash[sidecarSet.Name] = setUpgrade1
			sidecarSetHashWithoutImage[sidecarSet.Name] = setUpgrade2
		}
	}

	// store sidecarset hash in pod annotations
	by, _ := json.Marshal(sidecarSetHash)
	injectedAnnotations[sidecarcontrol.SidecarSetHashAnnotation] = string(by)
	by, _ = json.Marshal(sidecarSetHashWithoutImage)
	injectedAnnotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = string(by)
	sidecarSetNameList := strings.Join(sidecarSetNames.List(), ",")
	// store matched sidecarset list in pod annotations
	injectedAnnotations[sidecarcontrol.SidecarSetListAnnotation] = sidecarSetNameList
	return sidecarContainers, sidecarInitContainers, sidecarSecrets, volumesInSidecars, injectedAnnotations, nil
}

func getVolumesMapInSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) map[string]*corev1.Volume {
	volumesMap := make(map[string]*corev1.Volume)
	for idx, volume := range sidecarSet.Spec.Volumes {
		volumesMap[volume.Name] = &sidecarSet.Spec.Volumes[idx]
	}
	return volumesMap
}
