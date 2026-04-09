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
	"fmt"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/fieldpath"
	"net/http"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

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

const (
	//DefaultVMName                     = "configmapset-volume"

	ContainerLaunchPriorityAnnotation                    = "apps.kruise.io/container-launch-priority"
	ConfigMapSetAnnotationPrefix                         = "apps.kruise.io/configmapset."
	ConfigMapSetAnnotationTargetRevisionSuffix           = ".revision"
	ConfigMapSetAnnotationCurrentRevisionSuffix          = ".currentRevision"
	ConfigMapSetAnnotationTargetRevisionTimeStampSuffix  = ".revisionTimestamp"
	ConfigMapSetAnnotationCurrentRevisionTimeStampSuffix = ".currentRevisionTimestamp"
	ConfigMapSetAnnotationTargetCustomVersionSuffix      = ".customVersion"
	ConfigMapSetAnnotationCurrentCustomVersionSuffix     = ".currentCustomVersion"
)

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
	configMapName := fmt.Sprintf("%s-%s", strings.ToLower(cms.Name), "hub")
	defaultSidecarName := fmt.Sprintf("%s-%s", strings.ToLower(cms.Name), "reload-sidecar")
	volumeName := fmt.Sprintf("%s-%s-empty", strings.ToLower(cms.Namespace), strings.ToLower(cms.Name))
	revisionKey := fmt.Sprintf("%s%s%s", ConfigMapSetAnnotationPrefix, cms.Name, ConfigMapSetAnnotationTargetRevisionSuffix)
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

	reloadSidecar := corev1.Container{
		Name: defaultSidecarName,
		VolumeMounts: []corev1.VolumeMount{
			{Name: volumeName, MountPath: "/etc/config"},
			{Name: configMapVolume.Name, MountPath: "/etc/cms"},
		},
		Env: []corev1.EnvVar{{
			Name: "CMS_REVISION",
			ValueFrom: &corev1.EnvVarSource{ // 可以支持原地升级
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fmt.Sprintf("metadata.annotations['%s']", revisionKey),
				},
			},
		}},
	}

	if cms.Spec.ReloadSidecarConfig != nil {
		if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.K8sConfigReloadSidecarType {
			if cms.Spec.ReloadSidecarConfig.Config != nil {
				reloadSidecar.Name = cms.Spec.ReloadSidecarConfig.Config.Name
				reloadSidecar.Image = cms.Spec.ReloadSidecarConfig.Config.Image
				if len(cms.Spec.ReloadSidecarConfig.Config.Command) > 0 {
					reloadSidecar.Command = cms.Spec.ReloadSidecarConfig.Config.Command
				}
				if cms.Spec.ReloadSidecarConfig.Config.RestartPolicy != "" {
					reloadSidecar.RestartPolicy = &cms.Spec.ReloadSidecarConfig.Config.RestartPolicy
				}
				// 强制为 Always 的逻辑通常由用户配置，或者这里不覆盖，这里遵循 ConfigMapSet 声明
			}
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.CustomerReloadSidecarType {
			// customer 类型从 ConfigMap 读取
			// 由于此处为 mutator 阶段，且在 webhook 中读取 configmap 可能会增加耗时，
			// 若实现需从 webhook Client 尝试 Get ConfigMap 并 Unmarshal 为 Container
			// 为简化流程或遵循现有设计，这里给出获取 customer sidecar 的占位逻辑
			if cms.Spec.ReloadSidecarConfig.Config != nil && cms.Spec.ReloadSidecarConfig.Config.ConfigMapRef != nil {
				cmRef := cms.Spec.ReloadSidecarConfig.Config.ConfigMapRef
				customerCM := &corev1.ConfigMap{}
				if err := h.Client.Get(ctx, types.NamespacedName{Name: cmRef.Name, Namespace: cmRef.Namespace}, customerCM); err == nil {
					// 解析 custom sidecar configuration
					for _, v := range customerCM.Data {
						if unmarshalErr := json.Unmarshal([]byte(v), &reloadSidecar); unmarshalErr == nil {
							break
						}
					}
				} else {
					klog.Errorf("failed to get customer sidecar configmap %s/%s: %v", cmRef.Namespace, cmRef.Name, err)
					return err
				}
			}
		} else if cms.Spec.ReloadSidecarConfig.Type == appsv1alpha1.SidecarSetReloadSidecarType {
			// 引用 SidecarSet 对象，由 SidecarSet 负责注入和管理，ConfigMapSet webhook 中不进行注入
			klog.Infof("pod %s/%s will be injected by SidecarSet, skip sidecar injection in ConfigMapSet webhook", pod.Namespace, pod.Name)
			return nil
		}
	}

	if reloadSidecar.Image == "" {
		reloadSidecar.Image = "hub.bilibili.co/infra-caster/kratos-demo-wuhao18:reload-sidecar"
	}

	// 查找是否已有相同的 Sidecar 容器
	containerExistingIdx := -1
	for i, c := range pod.Spec.Containers {
		if c.Name == defaultSidecarName {
			containerExistingIdx = i
			break
		}
	}

	// replace reload-sidecar if exist
	if containerExistingIdx != -1 {
		pod.Spec.Containers[containerExistingIdx] = reloadSidecar
	} else {
		pod.Annotations[ContainerLaunchPriorityAnnotation] = "Ordered"
		pod.Spec.Containers = append([]corev1.Container{reloadSidecar}, pod.Spec.Containers...)
	}

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
	return nil
}

func (h *PodCreateHandler) injectEmptyDir4Pod(pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet) error {
	volumeName := fmt.Sprintf("%s-%s-empty", strings.ToLower(cms.Namespace), strings.ToLower(cms.Name))
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
		for _, c := range pod.Spec.Containers {
			if injectC.NameFrom == nil {
				// inject from static conf
				if injectC.Name == c.Name {
					h.applyVM4Container(&c, injectC, volume.Name)
					break
				}
			} else {
				// inject from metadata
				path, subscript, ok := fieldpath.SplitMaybeSubscriptedPath(injectC.NameFrom.FieldRef.FieldPath)
				if !ok {
					return fmt.Errorf("prase fieldPath failed with:%s", injectC.NameFrom.FieldRef.FieldPath)
				}

				annotations := pod.GetAnnotations()
				labels := pod.GetLabels()
				var cName string
				switch path {
				case "metadata.annotations":
					if annotations != nil {
						cName = annotations[subscript]
					}
				case "metadata.labels":
					if labels != nil {
						cName = labels[subscript]
					}
				}
				if cName != "" && cName == c.Name {
					h.applyVM4Container(&c, injectC, volume.Name)
					break
				}
			}
		}
	}
	return nil
}

func (h *PodCreateHandler) applyVM4Container(c *corev1.Container, v appsv1alpha1.ContainerInjectSpec, volumeName string) {
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

	if pod.Annotations == nil {
		return true, nil
	}

	// no need inject
	if pod.Annotations["apps.kruise.io/configmapset-enabled"] != "true" {
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
