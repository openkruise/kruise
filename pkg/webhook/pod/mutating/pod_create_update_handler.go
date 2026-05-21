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

package mutating

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	kubecontroller "k8s.io/kubernetes/pkg/controller"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/configmapset"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

type RevisionKeys struct {
	CurrentRevisionKey          string
	CurrentRevisionTimeStampKey string
	TargetRevisionKey           string
	TargetRevisionTimeStampKey  string
	TargetCustomVersionKey      string
	CurrentCustomVersionKey     string
}

var defaultReloadImage string

func init() {
	flag.StringVar(&defaultReloadImage, "default-reload-image", "openkruise/reload-sidecar:v1.0.0", "Default reload sidecar image to use if not explicitly specified in ConfigMapSet.")
}

// PodCreateHandler handles Pod
type PodCreateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	Client client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &PodCreateHandler{}

// Handle handles admission requests.
func (h *PodCreateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &corev1.Pod{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	// when pod.namespace is empty, using req.namespace
	if obj.Namespace == "" {
		obj.Namespace = req.Namespace
	}
	oriObj := obj.DeepCopy()
	var changed bool

	if skip := injectPodReadinessGate(req, obj); !skip {
		changed = true
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.WorkloadSpread) {
		if skip, err := h.workloadSpreadMutatingPod(ctx, req, obj); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		} else if !skip {
			changed = true
		}
	}

	if skip, err := h.sidecarsetMutatingPod(ctx, req, obj); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	} else if !skip {
		changed = true
	}

	// configMapSetMutatingPod must after sidecarsetMutatingPod
	if skip, err := h.configMapSetMutatingPod(ctx, req, obj); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	} else if !skip {
		changed = true
	}

	// "the order matters and sidecarsetMutatingPod must precede containerLaunchPriorityInitialization"
	if skip, err := h.containerLaunchPriorityInitialization(ctx, req, obj); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	} else if !skip {
		changed = true
	}

	// patch related-pub annotation in pod
	if utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetUpdateGate) ||
		utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetDeleteGate) {
		if skip, err := h.pubMutatingPod(ctx, req, obj); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		} else if !skip {
			changed = true
		}
	}

	// persistent pod state
	if skip, err := h.persistentPodStateMutatingPod(ctx, req, obj); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	} else if !skip {
		changed = true
	}

	// EnhancedLivenessProbe enabled
	if utilfeature.DefaultFeatureGate.Enabled(features.EnhancedLivenessProbeGate) {
		if skip, err := h.enhancedLivenessProbeWhenPodCreate(ctx, req, obj); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		} else if !skip {
			changed = true
		}
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.EnablePodProbeMarkerOnServerless) {
		if skip, err := h.podProbeMarkerMutatingPod(ctx, req, obj); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		} else if !skip {
			changed = true
		}
	}

	if !changed {
		return admission.Allowed("")
	}
	marshaled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	original, err := json.Marshal(oriObj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(original, marshaled)
}

func (h *PodCreateHandler) injectSidecar4Pod(ctx context.Context, pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet) error {
	configMapName := configmapset.GetConfigMapSetHubName(cms.Name)
	defaultSidecarName := configmapset.GetConfigMapSetDefaultSidecarName(cms.Name)
	volumeName := configmapset.GetConfigMapSetVolumeName(cms.Name)

	// get share path in reload-sidecar
	configMountPath := configmapset.GetConfigMapSetConfigMountPath(cms.Name)
	// get configmap mount path in reload-sidecar
	configMapMountPath := configmapset.GetConfigMapSetConfigMapMountPath(cms.Name)
	// build configmap volume with reload-sidecar
	configMapVolume := corev1.Volume{
		Name: configMapName + "-volume",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}
	// build downward api volume with reload-sidecar
	podInfoVolume := corev1.Volume{
		Name: fmt.Sprintf("cms-%s-config", strings.ToLower(cms.Name)),
		VolumeSource: corev1.VolumeSource{
			DownwardAPI: &corev1.DownwardAPIVolumeSource{
				Items: []corev1.DownwardAPIVolumeFile{
					{
						Path: "target_revision",
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: fmt.Sprintf("metadata.annotations['%s']", configmapset.GetConfigMapSetUpdateRevisionKey(cms.Name)),
						},
					},
					{
						Path: "post_hook_config",
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: fmt.Sprintf("metadata.annotations['%s']", configmapset.GetConfigMapSetPostHookConfigKey(cms.Name)),
						},
					},
				},
			},
		},
	}

	reloadSidecar := &corev1.Container{
		Name:  defaultSidecarName,
		Image: defaultReloadImage,
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeName, MountPath: configMountPath},
			{Name: configMapVolume.Name, MountPath: configMapMountPath},
			{Name: podInfoVolume.Name, MountPath: fmt.Sprintf("/etc/cms_config")},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						configmapset.GetReloadSidecarHealthCheckScript("/etc/execute"),
					},
				},
			},
			InitialDelaySeconds: 1,
			PeriodSeconds:       3,
			TimeoutSeconds:      5,
		},
		Env: []corev1.EnvVar{{
			Name:  configmapset.GetConfigMapSetEnvConfigPathName(),
			Value: configMapMountPath,
		}, {
			Name:  configmapset.GetConfigMapSetEnvSharePathName(),
			Value: configMountPath,
		}},
	}

	if cms.Spec.ReloadSidecarConfig != nil {
		if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.ReloadSidecarTypeK8s {
			if cms.Spec.ReloadSidecarConfig.Config != nil {
				if cms.Spec.ReloadSidecarConfig.Config.Name != "" {
					reloadSidecar.Name = cms.Spec.ReloadSidecarConfig.Config.Name
				}
				if cms.Spec.ReloadSidecarConfig.Config.Image != "" {
					reloadSidecar.Image = cms.Spec.ReloadSidecarConfig.Config.Image
				}
				if len(cms.Spec.ReloadSidecarConfig.Config.Command) > 0 {
					reloadSidecar.Command = cms.Spec.ReloadSidecarConfig.Config.Command
				}
				if cms.Spec.ReloadSidecarConfig.Config.RestartPolicy != "" {
					reloadSidecar.RestartPolicy = &cms.Spec.ReloadSidecarConfig.Config.RestartPolicy
				}
			}
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.ReloadSidecarTypeCustom {
			if cms.Spec.ReloadSidecarConfig.Config != nil && cms.Spec.ReloadSidecarConfig.Config.ConfigMapRef != nil {
				cmRef := cms.Spec.ReloadSidecarConfig.Config.ConfigMapRef
				customerCM := &corev1.ConfigMap{}
				cmNamespace := cmRef.Namespace
				if cmNamespace == "" {
					cmNamespace = cms.Namespace
				}
				if err := h.Client.Get(ctx, types.NamespacedName{Name: cmRef.Name, Namespace: cmNamespace}, customerCM); err != nil {
					klog.Errorf("failed to get custom sidecar configmap %s/%s: %v", cmNamespace, cmRef.Name, err)
					return err
				}
				if _, exists := customerCM.Data["reload-sidecar"]; !exists {
					klog.Errorf("custom sidecar configmap %s/%s missing key 'reload-sidecar'", cmNamespace, cmRef.Name)
					return fmt.Errorf("custom sidecar configmap %s/%s missing key 'reload-sidecar'", cmNamespace, cmRef.Name)
				}
				if unmarshalErr := json.Unmarshal([]byte(customerCM.Data["reload-sidecar"]), &reloadSidecar); unmarshalErr != nil {
					klog.Errorf("failed to unmarshal custom sidecar configmap %s/%s data 'reload-sidecar': %v", cmNamespace, cmRef.Name, unmarshalErr)
					return unmarshalErr
				}
			}
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.ReloadSidecarTypeSidecarSet {
			klog.Infof("pod %s/%s will be injected by SidecarSet, skip full sidecar injection in ConfigMapSet webhook, just merge VolumeMounts and Env", pod.Namespace, pod.Name)

			// find reload-sidecar in Pod (because already injected by sidecarSet)
			targetSidecarName := cms.Spec.ReloadSidecarConfig.Config.SidecarSetRef.ContainerName
			var container *corev1.Container
			for _, c := range pod.Spec.Containers {
				if c.Name == targetSidecarName {
					container = c.DeepCopy()
					break
				}
			}
			if container == nil {
				klog.Errorf("target container %s not found", targetSidecarName)
				return fmt.Errorf("target container %s not found", targetSidecarName)
			}

			// Merge VolumeMounts
			for _, newMount := range reloadSidecar.VolumeMounts {
				mountExists := false
				for _, existingMount := range container.VolumeMounts {
					if existingMount.Name == newMount.Name || existingMount.MountPath == newMount.MountPath {
						mountExists = true
						break
					}
				}
				if !mountExists {
					container.VolumeMounts = append(container.VolumeMounts, newMount)
				}
			}

			// Merge Env
			for _, newEnv := range reloadSidecar.Env {
				envExists := false
				for j, existingEnv := range container.Env {
					if existingEnv.Name == newEnv.Name {
						container.Env[j] = newEnv
						envExists = true
						break
					}
				}
				if !envExists {
					container.Env = append(container.Env, newEnv)
				}
			}
			reloadSidecar = container
		}
	}

	// Find if the same Sidecar container already exists
	containerExistingIdx := -1
	for i, c := range pod.Spec.Containers {
		if c.Name == reloadSidecar.Name {
			containerExistingIdx = i
			break
		}
	}

	// replace reload-sidecar if exist
	if containerExistingIdx != -1 {
		pod.Spec.Containers[containerExistingIdx] = *reloadSidecar
	} else {
		pod.Annotations[pub.ContainerLaunchPriorityKey] = pub.ContainerLaunchOrdered
		pod.Spec.Containers = append([]corev1.Container{*reloadSidecar}, pod.Spec.Containers...)
	}

	return h.injectVolumes(pod, configMapVolume, podInfoVolume)
}

func (h *PodCreateHandler) injectVolumes(pod *corev1.Pod, configMapVolume corev1.Volume, podInfoVolume corev1.Volume) error {
	volumeExistingIdx := -1
	for i, vol := range pod.Spec.Volumes {
		if vol.Name == configMapVolume.Name {
			volumeExistingIdx = i
			break
		}
	}
	if volumeExistingIdx != -1 {
		pod.Spec.Volumes[volumeExistingIdx] = configMapVolume
	} else {
		pod.Spec.Volumes = append(pod.Spec.Volumes, configMapVolume)
	}

	podInfoVolumeExistingIdx := -1
	for i, vol := range pod.Spec.Volumes {
		if vol.Name == podInfoVolume.Name {
			podInfoVolumeExistingIdx = i
			break
		}
	}
	if podInfoVolumeExistingIdx != -1 {
		pod.Spec.Volumes[podInfoVolumeExistingIdx] = podInfoVolume
	} else {
		pod.Spec.Volumes = append(pod.Spec.Volumes, podInfoVolume)
	}

	return nil
}

func (h *PodCreateHandler) injectEmptyDir4Pod(pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet) error {
	volumeName := configmapset.GetConfigMapSetVolumeName(cms.Name)

	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	existingIdx := -1
	// Check if the volume already exists
	for index, v := range pod.Spec.Volumes {
		if v.Name == volume.Name {
			existingIdx = index
			break
		}
	}
	if existingIdx != -1 {
		pod.Spec.Volumes[existingIdx] = volume
	} else {
		pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
	}

	for _, injectC := range cms.Spec.Containers {
		containerName := configmapset.GetContainerName(pod, injectC)
		for i, c := range pod.Spec.Containers {
			if containerName == c.Name {
				h.applyVM4Container(&c, injectC, volume.Name)
				pod.Spec.Containers[i] = c
				break
			}
		}
	}
	return nil
}

func (h *PodCreateHandler) applyVM4Container(c *corev1.Container, v appsv1alpha1.ConfigMapSetContainer, volumeName string) {
	for i := range c.VolumeMounts {
		if c.VolumeMounts[i].Name == volumeName {
			// Update the mount path of the existing volume mount
			c.VolumeMounts[i].MountPath = v.MountPath
			return
		}
	}
	c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: v.MountPath, // Mount path
		ReadOnly:  true,
	})
}

func (h *PodCreateHandler) configMapSetMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) (skip bool, err error) {
	// just only inject in pod create
	if len(req.AdmissionRequest.SubResource) > 0 ||
		req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}

	if !kubecontroller.IsPodActive(pod) {
		return true, nil
	}

	// no need inject with configMapSet
	if pod.Annotations == nil || pod.Annotations[configmapset.GetConfigMapSetEnabledKey()] != "true" {
		return true, nil
	}

	cmsList, err := configmapset.GetMatchConfigMapSets(h.Client, pod)
	if err != nil { // Return error if it's not a NotFound error
		klog.Errorf("get matched cms for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
		return false, fmt.Errorf("get related cms for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
	}

	if len(cmsList) == 0 {
		return true, nil
	}

	// Check for conflicts among matched ConfigMapSets
	if err := h.checkConfigMapSetConflicts(ctx, pod, cmsList); err != nil {
		klog.Errorf("ConfigMapSet conflict for pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return false, err
	}

	copyPod := pod.DeepCopy()
	// sort by cms name, for inject Pod stable
	sort.Slice(cmsList, func(i, j int) bool {
		return cmsList[i].Name < cmsList[j].Name
	})
	for _, cms := range cmsList {
		if cms.DeletionTimestamp != nil {
			continue
		}
		// Handle version annotations on creation
		err = h.handlePodRevisionAnnotations(ctx, pod, cms)
		if err != nil {
			klog.Errorf("handle revision annotations for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
			return false, fmt.Errorf("handle revision annotations for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
		}

		// inject share emptyDir volume
		klog.Infof("inject emptyDir for pod %s/%s", pod.Namespace, pod.Name)
		err = h.injectEmptyDir4Pod(pod, cms)
		if err != nil {
			klog.Errorf("inject emptyDir for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
			return false, fmt.Errorf("inject emptyDir for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
		}

		// inject reload sidecar
		klog.Infof("inject sidecar for pod %s/%s", pod.Namespace, pod.Name)
		err = h.injectSidecar4Pod(ctx, pod, cms)
		if err != nil {
			klog.Errorf("inject sidecar for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
			return false, fmt.Errorf("inject sidecar for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
		}
	}

	if equality.Semantic.DeepEqual(copyPod, pod) {
		return true, nil
	}

	return false, nil
}

func (h *PodCreateHandler) checkConfigMapSetConflicts(ctx context.Context, pod *corev1.Pod, cmsList []*appsv1alpha1.ConfigMapSet) error {
	if len(cmsList) <= 1 {
		return nil
	}

	sidecarNames := make(map[string]string)          // Sidecar Container Name -> ConfigMapSet Name
	mountPaths := make(map[string]map[string]string) // ContainerName -> MountPath -> ConfigMapSet Name

	for _, cms := range cmsList {
		// 1. Check for reload-sidecar name conflicts
		sidecarName, err := h.getReloadSidecarName(ctx, cms)
		if err != nil {
			return err
		}
		if sidecarName != "" {
			if existingCmsName, exists := sidecarNames[sidecarName]; exists {
				return fmt.Errorf("ConfigMapSet conflict: both %s and %s attempt to inject reload-sidecar with the same name '%s' into pod %s", existingCmsName, cms.Name, sidecarName, pod.Name)
			}
			sidecarNames[sidecarName] = cms.Name
		}

		// 2. Check for container mount path conflicts
		for _, c := range cms.Spec.Containers {
			targetContainerName := configmapset.GetContainerName(pod, c)
			if targetContainerName == "" {
				continue
			}
			if mountPaths[targetContainerName] == nil {
				mountPaths[targetContainerName] = make(map[string]string)
			}
			if existingCmsName, exists := mountPaths[targetContainerName][c.MountPath]; exists {
				return fmt.Errorf("ConfigMapSet conflict: both %s and %s attempt to mount to '%s' on container '%s' in pod %s", existingCmsName, cms.Name, c.MountPath, targetContainerName, pod.Name)
			}
			mountPaths[targetContainerName][c.MountPath] = cms.Name
		}
	}

	return nil
}

func (h *PodCreateHandler) getReloadSidecarName(ctx context.Context, cms *appsv1alpha1.ConfigMapSet) (string, error) {
	if cms.Spec.ReloadSidecarConfig == nil || cms.Spec.ReloadSidecarConfig.Config == nil {
		return configmapset.GetConfigMapSetDefaultSidecarName(cms.Name), nil
	}

	config := cms.Spec.ReloadSidecarConfig.Config
	switch cms.Spec.ReloadSidecarConfig.Type {
	case appsv1alpha1.ReloadSidecarTypeK8s:
		if config.Name != "" {
			return config.Name, nil
		}
	case appsv1alpha1.ReloadSidecarTypeSidecarSet:
		if config.SidecarSetRef != nil {
			return config.SidecarSetRef.ContainerName, nil
		}
	case appsv1alpha1.ReloadSidecarTypeCustom:
		if config.ConfigMapRef != nil {
			cmRef := config.ConfigMapRef
			customerCM := &corev1.ConfigMap{}
			cmNamespace := cmRef.Namespace
			if cmNamespace == "" {
				cmNamespace = cms.Namespace
			}
			if err := h.Client.Get(ctx, types.NamespacedName{Name: cmRef.Name, Namespace: cmNamespace}, customerCM); err == nil {
				if containerData, exists := customerCM.Data["reload-sidecar"]; exists {
					var reloadSidecar corev1.Container
					if unmarshalErr := json.Unmarshal([]byte(containerData), &reloadSidecar); unmarshalErr == nil {
						if reloadSidecar.Name != "" {
							return reloadSidecar.Name, nil
						}
					}
				}
			}
			return fmt.Sprintf("custom-sidecar-%s", cms.Name), nil
		}
	}
	return configmapset.GetConfigMapSetDefaultSidecarName(cms.Name), nil
}

func (h *PodCreateHandler) handlePodRevisionAnnotations(ctx context.Context, pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet) error {
	targetRevisionKey := configmapset.GetConfigMapSetUpdateRevisionKey(cms.Name)
	currentRevisionKey := configmapset.GetConfigMapSetCurrentRevisionKey(cms.Name)
	targetCustomVersionKey := configmapset.GetConfigMapSetUpdateCustomVersionKey(cms.Name)
	currentCustomVersionKey := configmapset.GetConfigMapSetCurrentCustomVersionKey(cms.Name)
	currentRevisionTimestampKey := configmapset.GetConfigMapSetCurrentRevisionTimeStampKey(cms.Name)
	updateRevisionTimestampKey := configmapset.GetConfigMapSetUpdateRevisionTimeStampKey(cms.Name)

	reloadSidecarRestartKey := configmapset.GetConfigMapSetReloadSidecarRestartKey(cms.Name)

	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	// Strategy 1: Prioritize using CurrentRevision. If empty, it means this is the first release, so use the target version calculated from Data.
	targetVersion := cms.Status.CurrentRevision
	if targetVersion == "" {
		hash, err := configmapset.CalculateHash(cms.Spec.Data)
		if err == nil {
			targetVersion = hash
		}
	}

	pod.Annotations[targetRevisionKey] = targetVersion
	pod.Annotations[currentRevisionKey] = targetVersion
	pod.Annotations[targetCustomVersionKey] = cms.Status.CurrentCustomVersion
	pod.Annotations[currentCustomVersionKey] = cms.Status.CurrentCustomVersion
	pod.Annotations[currentRevisionTimestampKey] = now
	pod.Annotations[updateRevisionTimestampKey] = now

	expectHash := configmapset.GetContainerHash(pod, targetVersion)
	pod.Annotations[reloadSidecarRestartKey] = expectHash
	for _, container := range cms.Spec.Containers {
		containerRestartKey := configmapset.GetConfigMapSetContainerRestartKey(cms.Name, container.Name)
		pod.Annotations[containerRestartKey] = expectHash
	}

	// Inject PostHook config into Pod Annotations
	if cms.Spec.EffectPolicy != nil && cms.Spec.EffectPolicy.Type == appsv1alpha1.EffectPolicyTypePostHook && cms.Spec.EffectPolicy.PostHook != nil {
		hookConfigBytes, err := json.Marshal(cms.Spec.EffectPolicy.PostHook)
		if err == nil {
			pod.Annotations[configmapset.GetConfigMapSetPostHookConfigKey(cms.Name)] = string(hookConfigBytes)
		}
	}

	return nil
}
