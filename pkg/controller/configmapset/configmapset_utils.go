/*
Copyright 2019 The Kruise Authors.
Copyright 2019 The Kubernetes Authors.

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

package configmapset

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

const (
	ConfigMapSetAnnotationPrefix                         = "apps.kruise.io/configmapset."
	ConfigMapSetAnnotationEnabled                        = "apps.kruise.io/configmapset-enabled"
	ConfigMapSetAnnotationCrr                            = "apps.kruise.io/configmapset-crr"
	ConfigMapSetAnnotationUpdateRevisionSuffix           = ".updateRevision"
	ConfigMapSetAnnotationCurrentRevisionSuffix          = ".currentRevision"
	ConfigMapSetAnnotationUpdateRevisionTimeStampSuffix  = ".updateRevisionTimestamp"
	ConfigMapSetAnnotationCurrentRevisionTimeStampSuffix = ".currentRevisionTimestamp"
	ConfigMapSetAnnotationUpdateCustomVersionSuffix      = ".updateCustomVersion"
	ConfigMapSetAnnotationCurrentCustomVersionSuffix     = ".currentCustomVersion"
)

func GetConfigMapSetEnabledKey() string {
	return ConfigMapSetAnnotationEnabled
}

func GetConfigMapSetCrrKey() string {
	return ConfigMapSetAnnotationCrr
}

func GetConfigMapSetUpdateRevisionKey(cmsName string) string {
	return ConfigMapSetAnnotationPrefix + cmsName + ConfigMapSetAnnotationUpdateRevisionSuffix
}

func GetConfigMapSetCurrentRevisionKey(cmsName string) string {
	return ConfigMapSetAnnotationPrefix + cmsName + ConfigMapSetAnnotationCurrentRevisionSuffix
}

func GetConfigMapSetUpdateRevisionTimeStampKey(cmsName string) string {
	return ConfigMapSetAnnotationPrefix + cmsName + ConfigMapSetAnnotationUpdateRevisionTimeStampSuffix
}

func GetConfigMapSetCurrentRevisionTimeStampKey(cmsName string) string {
	return ConfigMapSetAnnotationPrefix + cmsName + ConfigMapSetAnnotationCurrentRevisionTimeStampSuffix
}

func GetConfigMapSetUpdateCustomVersionKey(cmsName string) string {
	return ConfigMapSetAnnotationPrefix + cmsName + ConfigMapSetAnnotationUpdateCustomVersionSuffix
}

func GetConfigMapSetCurrentCustomVersionKey(cmsName string) string {
	return ConfigMapSetAnnotationPrefix + cmsName + ConfigMapSetAnnotationCurrentCustomVersionSuffix
}

func GetConfigMapSetReloadSidecarRestartKey(cmsName string) string {
	return fmt.Sprintf("apps.kruise.io/configmapset-%s-reload-sidecar-restart", cmsName)
}

func GetConfigMapSetContainerRestartKey(cmsName, containerName string) string {
	return fmt.Sprintf("apps.kruise.io/configmapset-%s-%s-restart", cmsName, containerName)
}

func GetConfigMapSetHubName(cmsName string) string {
	return fmt.Sprintf("%s-hub", strings.ToLower(cmsName))
}

func GetConfigMapSetDefaultSidecarName(cmsName string) string {
	return fmt.Sprintf("%s-reload-sidecar", strings.ToLower(cmsName))
}

func GetConfigMapSetVolumeName(cmsName string) string {
	return fmt.Sprintf("%s-empty", strings.ToLower(cmsName))
}

func GetConfigMapSetEnvRestartAnnotationName(cmsName string) string {
	return fmt.Sprintf("CMS_%s_RESTART_ANNOTATION", strings.ToUpper(strings.ReplaceAll(cmsName, "-", "_")))
}

func GetConfigMapSetEnvConfigPathName(cmsName string) string {
	return fmt.Sprintf("CMS_%s_CONFIG_PATH", strings.ToUpper(strings.ReplaceAll(cmsName, "-", "_")))
}

func GetConfigMapSetEnvSharePathName(cmsName string) string {
	return fmt.Sprintf("CMS_%s_SHARE_PATH", strings.ToUpper(strings.ReplaceAll(cmsName, "-", "_")))
}

func GetConfigMapSetConfigMountPath(cmsName string) string {
	return fmt.Sprintf("/etc/config/%s", strings.ToLower(cmsName))
}

func GetConfigMapSetConfigMapMountPath(cmsName string) string {
	return fmt.Sprintf("/etc/cms/%s", strings.ToLower(cmsName))
}

func GetConfigMapSetEnvFieldPath(restartKey string) string {
	return fmt.Sprintf("metadata.annotations['%s']", restartKey)
}

func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func removeString(slice []string, str string) []string {
	result := make([]string, 0)
	for _, s := range slice {
		if s != str {
			result = append(result, s)
		}
	}
	return result
}

func GetMatchedPods(ctx context.Context, reader client.Reader, cms *appsv1alpha1.ConfigMapSet) ([]*corev1.Pod, error) {
	// 通过spec.selector获取关联Pod
	// 获取匹配的pods
	podList := &corev1.PodList{}
	labelSelector, err := metav1.LabelSelectorAsSelector(cms.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %v", err)
	}
	if cms.Spec.Selector == nil {
		return nil, fmt.Errorf("ConfigMapSet %s has no selector", cms.Name)
	}
	opts := &client.ListOptions{
		Namespace:     cms.Namespace,
		LabelSelector: labelSelector, // 使用转换后的 LabelSelector
	}
	err = reader.List(ctx, podList, opts)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list pods: %v", err)
	}

	// 过滤不活跃的pod, 新启动的pod不会被过滤
	matchedPods := make([]*corev1.Pod, 0)
	klog.Infof("total pods: %d", len(podList.Items))
	for _, pod := range podList.Items {
		// filter not active Pod if active is true.
		if !kubecontroller.IsPodActive(&pod) {
			klog.Infof("pod %s/%s is not active, skip", pod.Namespace, pod.Name)
			klog.Infof("pod %s/%s 's deletion timestamp: %v", pod.Namespace, pod.Name, pod.DeletionTimestamp)
			continue
		}
		matchedPods = append(matchedPods, &pod)
	}

	return matchedPods, nil
}

func IsPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func GetMatchConfigMapSets(reader client.Reader, pod *corev1.Pod) ([]*appsv1alpha1.ConfigMapSet, error) {
	res := make([]*appsv1alpha1.ConfigMapSet, 0)

	// 查询 ConfigMapSet 对象
	configMapSets := &appsv1alpha1.ConfigMapSetList{}
	err := reader.List(context.Background(), configMapSets, &client.ListOptions{
		Namespace: pod.Namespace,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return res, nil // 有些pod就是没有cms的, 不要报错
		}
		return nil, fmt.Errorf("failed to list ConfigMapSets in namespace %s: %v", pod.Namespace, err)
	}

	// 查找与 Pod 标签匹配的 ConfigMapSet
	// 可能有多个
	for i := range configMapSets.Items {
		cms := configMapSets.Items[i] // 取地址前，先取实际元素
		ls, err := metav1.LabelSelectorAsSelector(cms.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector for ConfigMapSet %s: %v", cms.Name, err)
		}
		if ls.Matches(labels.Set(pod.Labels)) {
			res = append(res, &cms) // 这里的 cms 现在是独立的
		}
	}

	return res, nil
}

func getMatchedPods(reader client.Reader, cms *appsv1alpha1.ConfigMapSet) (*corev1.PodList, error) {
	// 通过spec.selector获取关联Pod
	// 获取匹配的pods
	pods := &corev1.PodList{}
	labelSelector, err := metav1.LabelSelectorAsSelector(cms.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %v", err)
	}
	err = reader.List(context.Background(), pods, &client.ListOptions{
		Namespace:     cms.Namespace,
		LabelSelector: labelSelector, // 使用转换后的 LabelSelector
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %v", err)
	}
	return pods, nil
}
