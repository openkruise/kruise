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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/fieldpath"
	"net/http"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/configmapset"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

	err = h.injectCMSForPod(ctx, obj)
	if err != nil {
		klog.Errorf("inject cms for pod %s/%s failed, error : %v", obj.Namespace, obj.Name, err)
		return admission.Errored(http.StatusInternalServerError, err)
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

func (h *PodCreateHandler) injectSidecar4Pod(ctx context.Context, pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet, revisionKeys *configmapset.RevisionKeys) error {
	// 注入sidecar 容器
	// 命名为reload-sidecar
	// 挂载名为obj.Name加上后缀hub的configmap
	// 镜像使用最普通的alpine即可
	// 通过envFromMetaData获取pod annotation中"configMapSet/cms1/Revision"所描述的配置版本
	configMapName := cms.Name + "-hub"
	sidecarName := cms.Name + "-reload-sidecar"
	// 先查找pod里面有没有同名的sidecar容器
	// 如果有, 需要更新
	// 否则, 创建新的sidecar容器

	// 新创建的pod, 需要注入一个revision, 不然reload-sidecar启动不了
	// 首先要获取当前的revisionEntry
	re, err := getRevisions(ctx, cms, h.Client)
	if err != nil || len(re) == 0 {
		klog.Errorf("failed to get revisions for cms %s/%s: %v", cms.Namespace, cms.Name, err)
		return fmt.Errorf("failed to get revisions for cms %s/%s: %v", cms.Namespace, cms.Name, err)
	}

	pods, err := configmapset.GetMatchedPods(ctx, h.Client, cms)
	if err != nil {
		klog.Errorf("failed to get matched pods for cms %s/%s: %v", cms.Namespace, cms.Name, err)
		return fmt.Errorf("failed to get matched pods for cms %s/%s: %v", cms.Namespace, cms.Name, err)
	}

	// 打印一下pods的长度, 确认不包含当前的pod
	klog.Infof("matched pods num for cms %s/%s: %d", cms.Namespace, cms.Name, len(pods))

	// 获取需要更新的pod信息
	// 根据更新策略选择要更新的Pod
	updateStrategy := cms.Spec.UpdateStrategy
	var updateRevision, updateCustomVersion string
	var updateInfo *configmapset.UpdateInfo
	var distributions []appsv1alpha1.Distribution
	pods = append(pods, pod) // 模拟分配
	// 定义了partition的情形
	if updateStrategy.Partition != nil {
		// 将partition转为distribution
		distributions, err = configmapset.TransformPartition2Distribution(cms, re, pods)
		if err != nil {
			klog.Errorf("failed to transform partition : %v", err)
			return fmt.Errorf("failed to transform partition : %v", err)
		}
	} else if len(updateStrategy.Distributions) > 0 { // 使用distribution的情形
		distributions = cms.Spec.UpdateStrategy.Distributions
	}
	updateInfo, err = configmapset.FetchUpdateInfoByDistribution(distributions, re, pods, revisionKeys)
	if err != nil {
		klog.Errorf("failed to fetch update info: %v", err)
		return fmt.Errorf("failed to fetch update info: %v", err)
	}
	if updateInfo == nil {
		klog.Errorf("failed to fetch update info: unknown reason")
		return fmt.Errorf("failed to fetch update info: unknown reason")
	}

	// 打印targetRevisions信息
	klog.Infof("target revisions: %v", updateInfo.TargetRevisions)
	klog.Infof("target custom versions: %v", updateInfo.TargetCustomVersions)
	// 遍历podsToUpdate, 找到当前pod
	for i, p := range updateInfo.PodsToUpdate {
		if p == nil { // 不应该出现这种情况
			klog.Errorf("fetch pods failed: pod %d is nil", i)
			return fmt.Errorf("fetch pods failed: pod %d is nil", i)
		}
		if p.Name == pod.Name {
			updateRevision = updateInfo.TargetRevisions[i]
			updateCustomVersion = updateInfo.TargetCustomVersions[i]
			break
		}
	}

	if updateRevision == "" {
		klog.Errorf("failed to find update revision for pod %s/%s", pod.Namespace, pod.Name)
		return fmt.Errorf("failed to find update revision for pod %s/%s", pod.Namespace, pod.Name)
	}

	pod.Annotations[revisionKeys.TargetRevisionKey] = updateRevision
	pod.Annotations[revisionKeys.TargetRevisionTimeStampKey] = "0" // 新建的pod这里设置为0
	pod.Annotations[revisionKeys.CurrentRevisionKey] = updateRevision
	pod.Annotations[revisionKeys.CurrentRevisionTimeStampKey] = strconv.FormatInt(time.Now().UnixMilli(), 10)
	pod.Annotations[revisionKeys.TargetCustomVersionKey] = updateCustomVersion
	pod.Annotations[revisionKeys.CurrentCustomVersionKey] = updateCustomVersion

	// 创建sidecar容器
	newSidecar := corev1.Container{
		Name:  sidecarName,
		Image: "hub.bilibili.co/infra-caster/kratos-demo-wuhao18:reload-sidecar",
		Env: []corev1.EnvVar{
			{
				Name: "REVISION",
				ValueFrom: &corev1.EnvVarSource{ // 可以支持原地升级
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: fmt.Sprintf("metadata.annotations['%s']", revisionKeys.TargetRevisionKey),
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      configMapName + "-volume", // 挂载名为obj.Name-hub的configmap
				MountPath: "/etc/config",             // 挂载路径
			},
			{
				Name:      cms.Name + "-volume",
				MountPath: "/etc/cms",
			},
		},
	}

	// 查找是否已有相同的 Sidecar 容器
	existingIdx := -1
	for i, c := range pod.Spec.Containers {
		if c.Name == sidecarName {
			existingIdx = i
			break
		}
	}

	// 更新逻辑
	if existingIdx != -1 {
		pod.Spec.Containers[existingIdx] = newSidecar
	} else {
		// 添加新的sidecar容器
		if !utilfeature.DefaultFeatureGate.Enabled(features.ConfigMapSidecarContainerCLP) {
			// 作为普通容器
			// 添加sidecar到Pod, 且要作为第一个容器(Container Launch Priority 会保证前面的容器先启动)
			pod.Annotations[ContainerLaunchPriorityAnnotation] = "Ordered"
			cs := make([]corev1.Container, 0, len(pod.Spec.Containers)+1)
			cs = append(cs, newSidecar)
			pod.Spec.Containers = append(cs, pod.Spec.Containers...)
		} else {
			// 作为sidecar容器
			// TODO 内部使用的k8s版本 1.20 不支持设置容器的restart policy
			//cs := make([]corev1.Container, 0, len(pod.Spec.InitContainers)+1)
			//cs = append(cs, sidecar)
			//pod.Spec.InitContainers = append(cs, pod.Spec.InitContainers...)
		}
	}

	// 创建ConfigMap卷
	configMapVolume := corev1.Volume{
		Name: configMapName + "-volume",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName, // ConfigMap 的命名为 CMS+"-hub"
				},
			},
		},
	}

	// 避免重复添加volume
	volumeExists := false
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == configMapVolume.Name {
			volumeExists = true
			break
		}
	}
	if !volumeExists {
		pod.Spec.Volumes = append(pod.Spec.Volumes, configMapVolume)
	}
	return nil
}

func (h *PodCreateHandler) injectEmptyDir4Pod(pod *corev1.Pod, cms *appsv1alpha1.ConfigMapSet) error {

	volume := corev1.Volume{
		Name: cms.Name + "-volume",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	exists := false
	// 先判断有没有重复的volume
	for _, v := range pod.Spec.Volumes {
		if v.Name == volume.Name {
			exists = true
			break
		}
	}
	if !exists {
		pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
	}

	templateContainers := pod.Spec.Containers
	for _, v := range cms.Spec.InjectedContainers {
		for j := range templateContainers {
			c := &templateContainers[j] // 直接获取指针
			if c == nil || c.Name == "" {
				// 预期外的情况
				klog.Errorf("pod %s/%s container name is empty", pod.Namespace, pod.Name)
				return fmt.Errorf("pod %s/%s container name is empty", pod.Namespace, pod.Name)
			}
			if v.NameFrom == nil { // 静态注入逻辑
				if v.Name == c.Name {
					h.applyVM4Container(c, v, volume.Name)
					break // 一个pod不会存在多个同名容器
				}
			} else {
				// 动态注入逻辑
				// 获取v.NameFrom.FieldRef
				// 读取pod的相应字段
				// Currently only supports `metadata.labels['<KEY>']`, `metadata.annotations['<KEY>']`
				path, subscript, ok := fieldpath.SplitMaybeSubscriptedPath(v.NameFrom.FieldRef.FieldPath)
				if !ok {
					continue
				}

				annotations := cms.GetAnnotations()
				labels := cms.GetLabels()
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
				if cName == c.Name {
					h.applyVM4Container(c, v, volume.Name)
					break // 一个pod不会存在多个同名容器
				}
			} // end of else
		} // end of for loop
	}
	return nil
}

func (h *PodCreateHandler) applyVM4Container(c *corev1.Container, v appsv1alpha1.InjectedContainerSpec, volumeName string) {
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

func (h *PodCreateHandler) injectCMSForPod(ctx context.Context, obj *corev1.Pod) error {
	// 检查pod能否匹配到cms
	if len(obj.Labels) > 0 {
		cmsList, err := configmapset.GetRelatedConfigMapSets(h.Client, obj)
		if err != nil { // 返回error说明不是notfound
			klog.Errorf("get related cms for pod %s/%s failed, error : %v", obj.Namespace, obj.Name, err)
			return fmt.Errorf("get related cms for pod %s/%s failed, error : %v", obj.Namespace, obj.Name, err)
		}

		// 为pod注入cms相关逻辑
		if len(cmsList) > 0 {
			for _, cms := range cmsList {
				// 获取配置版本信息
				revisionKeys := &configmapset.RevisionKeys{
					CurrentRevisionKey:          fmt.Sprintf("%s%s%s", ConfigMapSetAnnotationPrefix, cms.Name, ConfigMapSetAnnotationCurrentRevisionSuffix),
					CurrentRevisionTimeStampKey: fmt.Sprintf("%s%s%s", ConfigMapSetAnnotationPrefix, cms.Name, ConfigMapSetAnnotationCurrentRevisionTimeStampSuffix),
					TargetRevisionKey:           fmt.Sprintf("%s%s%s", ConfigMapSetAnnotationPrefix, cms.Name, ConfigMapSetAnnotationTargetRevisionSuffix),
					TargetRevisionTimeStampKey:  fmt.Sprintf("%s%s%s", ConfigMapSetAnnotationPrefix, cms.Name, ConfigMapSetAnnotationTargetRevisionTimeStampSuffix),
					TargetCustomVersionKey:      fmt.Sprintf("%s%s%s", ConfigMapSetAnnotationPrefix, cms.Name, ConfigMapSetAnnotationTargetCustomVersionSuffix),
					CurrentCustomVersionKey:     fmt.Sprintf("%s%s%s", ConfigMapSetAnnotationPrefix, cms.Name, ConfigMapSetAnnotationCurrentCustomVersionSuffix),
				}
				// 注入 emptyDir
				klog.Infof("inject emptyDir for pod %s/%s", obj.Namespace, obj.Name)
				err = h.injectEmptyDir4Pod(obj, cms)
				if err != nil {
					klog.Errorf("inject emptyDir for pod %s/%s failed, error : %v", obj.Namespace, obj.Name, err)
					return fmt.Errorf("inject emptyDir for pod %s/%s failed, error : %v", obj.Namespace, obj.Name, err)
				}

				// 注入 sidecar 容器
				klog.Infof("inject sidecar for pod %s/%s", obj.Namespace, obj.Name)
				err = h.injectSidecar4Pod(ctx, obj, cms, revisionKeys)
				if err != nil {
					klog.Errorf("inject sidecar for pod %s/%s failed, error : %v", obj.Namespace, obj.Name, err)
					return fmt.Errorf("inject sidecar for pod %s/%s failed, error : %v", obj.Namespace, obj.Name, err)
				}
			}
		}
	}
	return nil
}

// 获取当前的rmc
func getRevisions(ctx context.Context, cms *appsv1alpha1.ConfigMapSet, r client.Client) ([]configmapset.RevisionEntry, error) {
	klog.Infof("fetching revisions for ConfigMapSet %s/%s", cms.Namespace, cms.Name)
	// ConfigMap 命名：cms.Name + "-hub"
	cmName := fmt.Sprintf("%s-hub", cms.Name)
	cmNamespace := cms.Namespace

	var revisions []configmapset.RevisionEntry

	// 获取最新的 ConfigMap
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cmNamespace}, cm); err != nil {
		if k8serrors.IsNotFound(err) {
			// 如果 ConfigMap 不存在，报错返回
			return nil, fmt.Errorf("ConfigMap %s not found", cmName)
		}
		return nil, fmt.Errorf("failed to get ConfigMap: %v", err)
	}

	// 解析现有 ConfigMap 的 revisions
	if revData, exists := cm.Data["revisions"]; exists {
		if err := json.Unmarshal([]byte(revData), &revisions); err != nil {
			klog.Errorf("Failed to unmarshal revisions from ConfigMap %s: %v, resetting revisions", cmName, err)
			return nil, fmt.Errorf("failed to unmarshal revisions from ConfigMap %s: %v", cmName, err)
		}
	}
	return revisions, nil
}
