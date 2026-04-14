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
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
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
	restartKey := configmapset.GetConfigMapSetReloadSidecarRestartKey(cms.Name)
	envName := configmapset.GetConfigMapSetEnvRestartAnnotationName(cms.Name)

	configMountPath := configmapset.GetConfigMapSetConfigMountPath(cms.Name)
	configMapMountPath := configmapset.GetConfigMapSetConfigMapMountPath(cms.Name)

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

	reloadSidecar := &corev1.Container{
		Name:  defaultSidecarName,
		Image: defaultReloadImage,
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeName, MountPath: configMountPath},
			{Name: configMapVolume.Name, MountPath: configMapMountPath},
			{Name: "podinfo", MountPath: "/etc/podinfo"},
		},
		Env: []corev1.EnvVar{{
			Name: envName,
			ValueFrom: &corev1.EnvVarSource{ // 可以支持原地升级
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: configmapset.GetConfigMapSetEnvFieldPath(restartKey),
				},
			},
		}, {
			Name:  configmapset.GetConfigMapSetEnvConfigPathName(cms.Name),
			Value: configMapMountPath,
		}, {
			Name:  configmapset.GetConfigMapSetEnvSharePathName(cms.Name),
			Value: configMountPath,
		}},
	}

	if cms.Spec.ReloadSidecarConfig != nil {
		if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.K8sConfigReloadSidecarType {
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
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.CustomerReloadSidecarType {
			if cms.Spec.ReloadSidecarConfig.Config != nil && cms.Spec.ReloadSidecarConfig.Config.ConfigMapRef != nil {
				cmRef := cms.Spec.ReloadSidecarConfig.Config.ConfigMapRef
				customerCM := &corev1.ConfigMap{}
				cmNamespace := cmRef.Namespace
				if cmNamespace == "" {
					cmNamespace = cms.Namespace
				}
				if err := h.Client.Get(ctx, types.NamespacedName{Name: cmRef.Name, Namespace: cmNamespace}, customerCM); err != nil {
					klog.Errorf("failed to get customer sidecar configmap %s/%s: %v", cmNamespace, cmRef.Name, err)
					return err
				}
				if _, exists := customerCM.Data["reload-sidecar"]; !exists {
					klog.Errorf("customer sidecar configmap %s/%s missing key 'reload-sidecar'", cmNamespace, cmRef.Name)
					return fmt.Errorf("customer sidecar configmap %s/%s missing", cmNamespace, cmRef.Name)
				}
				if unmarshalErr := json.Unmarshal([]byte(customerCM.Data["reload-sidecar"]), &reloadSidecar); unmarshalErr != nil {
					klog.Errorf("failed to unmarshal customer sidecar configmap %s/%s data 'reload-sidecar': %v", cmNamespace, cmRef.Name, unmarshalErr)
					return unmarshalErr
				}
			}
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.SidecarSetReloadSidecarType {
			klog.Infof("pod %s/%s will be injected by SidecarSet, skip full sidecar injection in ConfigMapSet webhook, just merge VolumeMounts and Env", pod.Namespace, pod.Name)

			// find reload-sidecar in Pod（Because already injected by sidecarSet）
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

	// 查找是否已有相同的 Sidecar 容器
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

	return h.injectVolumes(pod, configMapVolume)
}

func (h *PodCreateHandler) injectVolumes(pod *corev1.Pod, configMapVolume corev1.Volume) error {
	// 避免重复添加volume
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

	podInfoVolume := corev1.Volume{
		Name: "podinfo",
		VolumeSource: corev1.VolumeSource{
			DownwardAPI: &corev1.DownwardAPIVolumeSource{
				Items: []corev1.DownwardAPIVolumeFile{
					{
						Path: "annotations",
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.annotations",
						},
					},
				},
			},
		},
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
	// 先判断有没有重复的volume
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
				h.applyVM4Container(pod, &c, injectC, volume.Name, cms)
				pod.Spec.Containers[i] = c
				break
			}
		}
	}
	return nil
}

func (h *PodCreateHandler) applyVM4Container(pod *corev1.Pod, c *corev1.Container, v appsv1alpha1.ContainerInjectSpec, volumeName string, cms *appsv1alpha1.ConfigMapSet) {
	// 为业务容器注入用于重启的环境变量，每个容器使用独立的 Annotation Key
	restartAnnotationKey := configmapset.GetConfigMapSetContainerRestartKey(cms.Name, c.Name)
	targetRevisionKey := configmapset.GetConfigMapSetUpdateRevisionKey(cms.Name)
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	if pod.Annotations[restartAnnotationKey] == "" {
		// 初始化时使用 md5(pod.Name + "0")，与 Controller 逻辑保持结构一致
		// 由于 mutating 阶段 Pod Name 可能还未分配（例如通过 GenerateName 创建），如果为空则使用固定值或 "0" 的 md5
		podNameForHash := pod.Name
		if podNameForHash == "" {
			podNameForHash = "unnamed"
		}
		hashBytes := md5.Sum([]byte(podNameForHash + pod.Annotations[targetRevisionKey]))
		hashStr := hex.EncodeToString(hashBytes[:])
		pod.Annotations[restartAnnotationKey] = hashStr
	}

	envName := configmapset.GetConfigMapSetEnvRestartAnnotationName(cms.Name)
	envFound := false
	for _, env := range c.Env {
		if env.Name == envName {
			envFound = true
			break
		}
	}
	if !envFound {
		c.Env = append(c.Env, corev1.EnvVar{
			Name: envName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: configmapset.GetConfigMapSetEnvFieldPath(restartAnnotationKey),
				},
			},
		})
	}

	// 检查c里面是否已经有同名的volumeMount了
	for i := range c.VolumeMounts {
		if c.VolumeMounts[i].Name == volumeName {
			// 修改vm挂载的路径即可
			c.VolumeMounts[i].MountPath = v.MountPath
			return
		}
	}
	c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: v.MountPath, // 挂载路径
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

	if !sidecarcontrol.IsActivePod(pod) {
		return true, nil
	}

	// no need inject with configMapSet
	if pod.Annotations == nil || pod.Annotations[configmapset.GetConfigMapSetEnabledKey()] != "true" {
		return true, nil
	}

	cmsList, err := configmapset.GetMatchConfigMapSets(h.Client, pod)
	if err != nil { // 返回error说明不是notfound
		klog.Errorf("get matched cms for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
		return false, fmt.Errorf("get related cms for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
	}

	if len(cmsList) == 0 {
		return true, nil
	}

	copyPod := pod.DeepCopy()
	// sort by cms name, for inject Pod stable
	sort.Slice(cmsList, func(i, j int) bool {
		return cmsList[i].Name < cmsList[j].Name
	})
	for _, cms := range cmsList {
		// 处理创建时的版本注解
		err = h.handlePodRevisionAnnotations(ctx, pod, cms)
		if err != nil {
			klog.Errorf("handle revision annotations for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
			return false, fmt.Errorf("handle revision annotations for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
		}

		// 注入 emptyDir
		klog.Infof("inject emptyDir for pod %s/%s", pod.Namespace, pod.Name)
		err = h.injectEmptyDir4Pod(pod, cms)
		if err != nil {
			klog.Errorf("inject emptyDir for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
			return false, fmt.Errorf("inject emptyDir for pod %s/%s failed, error : %v", pod.Namespace, pod.Name, err)
		}

		// 注入 sidecar 容器
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

func (h *PodCreateHandler) handlePodRevisionAnnotations(ctx context.Context, pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet) error {
	updateRevision, err := configmapset.CalculateHash(cms.Spec.Data)
	if err != nil {
		return fmt.Errorf("failed to compute hash for cms %s/%s: %w", cms.Namespace, cms.Name, err)
	}

	targetRevisionKey := configmapset.GetConfigMapSetUpdateRevisionKey(cms.Name)
	currentRevisionKey := configmapset.GetConfigMapSetCurrentRevisionKey(cms.Name)
	targetCustomVersionKey := configmapset.GetConfigMapSetUpdateCustomVersionKey(cms.Name)
	currentCustomVersionKey := configmapset.GetConfigMapSetCurrentCustomVersionKey(cms.Name)
	currentRevisionTimestampKey := configmapset.GetConfigMapSetCurrentRevisionTimeStampKey(cms.Name)
	updateRevisionTimestampKey := configmapset.GetConfigMapSetUpdateRevisionTimeStampKey(cms.Name)
	restartKey := configmapset.GetConfigMapSetReloadSidecarRestartKey(cms.Name)

	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	// Get related pods to calculate partition correctly
	pods, err := configmapset.GetMatchedPods(ctx, h.Client, cms)
	if err != nil {
		return err
	}

	// Determine if the pod should use the new version or old version based on partition
	shouldUseNewVersion := true

	if cms.Spec.UpdateStrategy.Partition != nil {
		// Only consider pods matching the same MatchLabelKeys if specified
		var groupPods []*corev1.Pod
		if len(cms.Spec.UpdateStrategy.MatchLabelKeys) == 0 {
			groupPods = pods
		} else {
			isMatch := true
			for _, key := range cms.Spec.UpdateStrategy.MatchLabelKeys {
				if pod.Labels[key] != "" {
					// We only need to check if existing pods match this pod's label values
					for _, p := range pods {
						if p.Labels[key] == pod.Labels[key] {
							groupPods = append(groupPods, p)
						}
					}
				} else {
					isMatch = false
					break
				}
			}
			if !isMatch {
				groupPods = pods // fallback to all pods if no labels to match
			}
		}

		// Calculate how many pods should be updated based on partition
		expectedUpdatedCount := len(groupPods) // Default: all pods in group
		partitionStr := cms.Spec.UpdateStrategy.Partition.String()
		if strings.HasSuffix(partitionStr, "%") {
			percentStr := strings.TrimSuffix(partitionStr, "%")
			if percent, err := strconv.Atoi(percentStr); err == nil && percent >= 0 && percent <= 100 {
				// newReplicas = replicas - partition
				expectedUpdatedCount = len(groupPods) - int(math.Ceil(float64(percent)/100.0*float64(len(groupPods))))
			}
		} else if count, err := strconv.Atoi(partitionStr); err == nil && count >= 0 {
			expectedUpdatedCount = len(groupPods) - count
			if expectedUpdatedCount < 0 {
				expectedUpdatedCount = 0
			}
		}

		// Count how many pods in this group are already at the new version
		currentUpdatedCount := 0
		for _, p := range groupPods {
			if p.Annotations[targetRevisionKey] == updateRevision {
				currentUpdatedCount++
			}
		}

		// If we've already reached the target number of updated pods, this new pod should get the old version
		if currentUpdatedCount >= expectedUpdatedCount {
			shouldUseNewVersion = false
		}
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	if shouldUseNewVersion {
		pod.Annotations[targetRevisionKey] = updateRevision
		pod.Annotations[currentRevisionKey] = updateRevision
		pod.Annotations[targetCustomVersionKey] = cms.Spec.CustomVersion
		pod.Annotations[currentCustomVersionKey] = cms.Spec.CustomVersion

		pod.Annotations[currentRevisionTimestampKey] = now
		pod.Annotations[updateRevisionTimestampKey] = now

	} else if cms.Status.CurrentRevision != "" {
		pod.Annotations[targetRevisionKey] = cms.Status.CurrentRevision
		pod.Annotations[currentRevisionKey] = cms.Status.CurrentRevision
		pod.Annotations[targetCustomVersionKey] = cms.Status.CurrentCustomVersion
		pod.Annotations[currentCustomVersionKey] = cms.Status.CurrentCustomVersion

		pod.Annotations[currentRevisionTimestampKey] = now
		pod.Annotations[updateRevisionTimestampKey] = now

	} else {
		// If there is no current revision (first rollout), force new version regardless of partition
		pod.Annotations[targetRevisionKey] = updateRevision
		pod.Annotations[currentRevisionKey] = updateRevision
		pod.Annotations[targetCustomVersionKey] = cms.Spec.CustomVersion
		pod.Annotations[currentCustomVersionKey] = cms.Spec.CustomVersion
		pod.Annotations[currentRevisionTimestampKey] = now
		pod.Annotations[updateRevisionTimestampKey] = now
	}

	if pod.Annotations[restartKey] == "" {
		// 初始化时使用 md5(pod.Name + "0")，与 Controller 逻辑保持结构一致
		// 由于 mutating 阶段 Pod Name 可能还未分配（例如通过 GenerateName 创建），如果为空则使用固定值或 "0" 的 md5
		podNameForHash := pod.Name
		if podNameForHash == "" {
			podNameForHash = "unnamed"
		}
		hashBytes := md5.Sum([]byte(podNameForHash + pod.Annotations[targetRevisionKey]))
		hashStr := hex.EncodeToString(hashBytes[:])
		pod.Annotations[restartKey] = hashStr
	}

	return nil
}
