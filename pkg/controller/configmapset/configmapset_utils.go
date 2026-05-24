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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
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

func GetContainerHash(pod *corev1.Pod, revision string) string {
	podNameForHash := pod.Name
	hashBytes := md5.Sum([]byte(podNameForHash + revision))
	return hex.EncodeToString(hashBytes[:])
}

func GetConfigMapSetContainerRestartKey(cmsName, containerName string) string {
	return fmt.Sprintf("apps.kruise.io/configmapset-%s-%s-restart", cmsName, containerName)
}

func GetConfigMapSetPostHookConfigKey(cmsName string) string {
	return fmt.Sprintf("apps.kruise.io/configmapset-%s-post-hook-config", cmsName)
}

// GetConfigMapSetContainerStartedAtKey gets the container StartedAt annotation key for CRR
func GetConfigMapSetContainerStartedAtKey(containerName string) string {
	return fmt.Sprintf("configmapset.kruise.io/started-at-%s", containerName)
}

func GenerateDerivedName(name, suffix string) string {
	fullName := fmt.Sprintf("%s-%s", name, suffix)
	if len(fullName) <= 63 {
		return fullName
	}
	hash := md5.Sum([]byte(name))
	hashStr := hex.EncodeToString(hash[:])[:8]
	maxNameLen := 53 - len(suffix)
	truncatedName := name
	if maxNameLen > 0 {
		truncatedName = name[:maxNameLen]
		// Trim trailing hyphens to avoid invalid DNS names like `xxx---hash-suffix`
		truncatedName = strings.TrimRight(truncatedName, "-")
	} else {
		truncatedName = ""
	}
	if truncatedName != "" {
		return fmt.Sprintf("%s-%s-%s", truncatedName, hashStr, suffix)
	}
	return fmt.Sprintf("%s-%s", hashStr, suffix)
}

func GetConfigMapSetHubName(cmsName string) string {
	return GenerateDerivedName(strings.ToLower(cmsName), "hub")
}

func GetConfigMapSetDefaultSidecarName(cmsName string) string {
	return GenerateDerivedName(strings.ToLower(cmsName), "reload-sidecar")
}

func GetConfigMapSetVolumeName(cmsName string) string {
	return GenerateDerivedName(strings.ToLower(cmsName), "empty")
}

func GetConfigMapSetHubVolumeName(cmsName string) string {
	return GenerateDerivedName(strings.ToLower(cmsName), "hub-volume")
}

func GetConfigMapSetDownwardAPIVolumeName(cmsName string) string {
	return GenerateDerivedName(fmt.Sprintf("cms-%s", strings.ToLower(cmsName)), "config")
}

func GetConfigMapSetEnvRestartAnnotationName(cmsName string) string {
	return fmt.Sprintf("CMS_%s_RESTART_ANNOTATION", strings.ToUpper(strings.ReplaceAll(cmsName, "-", "_")))
}

func GetConfigMapSetEnvConfigPathName() string {
	return "CMS_CONFIG_PATH"
}

func GetConfigMapSetEnvSharePathName() string {
	return "CMS_SHARE_PATH"
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

// GetReloadSidecarHealthCheckScript returns the health check script for the reload-sidecar
func GetReloadSidecarHealthCheckScript(path string) string {
	return fmt.Sprintf(`
TARGET_REVISION=$(cat /etc/cms_config/target_revision 2>/dev/null || true)
if [ -z "$TARGET_REVISION" ]; then exit 0; fi

# 1. Verify target revision matches local identity (requires sidecar to write .current_revision)
# Retaining compatibility for content hash check here. Because the hash algorithm (json.marshal) 
# differs across languages and OS, the standard practice is:
# After reload-sidecar copies/updates the files from configmap-hub to the shared directory,
# it generates a hidden file '.current_revision' in the shared directory to record the updated Hash.
if [ -f %[1]s/.current_revision ]; then
    CURRENT_REVISION=$(cat %[1]s/.current_revision)
    if [ "$TARGET_REVISION" != "$CURRENT_REVISION" ]; then
        exit 1
    fi
else
    exit 1
fi

# 2. If PostHook is configured, check for success marker
POST_HOOK_CONFIG=$(cat /etc/cms_config/post_hook_config 2>/dev/null || true)
if [ -n "$POST_HOOK_CONFIG" ] && [ "$POST_HOOK_CONFIG" != "null" ]; then
    if [ ! -f "%[1]s/.post_hook_success_${TARGET_REVISION}" ]; then
        exit 1
    fi
fi

exit 0
`, path)
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
	// Get related Pods by spec.selector
	// Get matched pods
	podList := &corev1.PodList{}
	if cms.Spec.Selector == nil {
		return nil, fmt.Errorf("ConfigMapSet %s has no selector", cms.Name)
	}
	labelSelector, err := metav1.LabelSelectorAsSelector(cms.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %v", err)
	}
	opts := &client.ListOptions{
		Namespace:     cms.Namespace,
		LabelSelector: labelSelector, // Use converted LabelSelector
	}
	err = reader.List(ctx, podList, opts)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list pods: %v", err)
	}

	// Filter inactive pods, newly started pods will not be filtered
	matchedPods := make([]*corev1.Pod, 0)
	klog.Infof("total pods: %d", len(podList.Items))
	for i, pod := range podList.Items {
		if !kubecontroller.IsPodActive(&pod) {
			klog.V(4).Infof("Pod %s/%s is not active, skipping", pod.Namespace, pod.Name)
			continue
		}
		if pod.DeletionTimestamp != nil {
			klog.V(4).Infof("Pod %s/%s is being deleted, skipping", pod.Namespace, pod.Name)
			continue
		}
		matchedPods = append(matchedPods, &podList.Items[i])
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

	// Query ConfigMapSet objects
	configMapSets := &appsv1alpha1.ConfigMapSetList{}
	err := reader.List(context.Background(), configMapSets, &client.ListOptions{
		Namespace: pod.Namespace,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return res, nil // Some pods may not have cms, do not return error
		}
		return nil, fmt.Errorf("failed to list ConfigMapSets in namespace %s: %v", pod.Namespace, err)
	}

	// Find ConfigMapSets that match the Pod's labels
	// There may be multiple
	for i := range configMapSets.Items {
		cms := configMapSets.Items[i] // Get actual element before taking address
		if cms.DeletionTimestamp != nil {
			continue
		}
		ls, err := metav1.LabelSelectorAsSelector(cms.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector for ConfigMapSet %s: %v", cms.Name, err)
		}
		if ls.Matches(labels.Set(pod.Labels)) {
			res = append(res, &cms) // The cms here is now independent
		}
	}

	return res, nil
}

// GroupPodsByMatchLabelKeys groups pods by matchLabelKeys values.
// Missing key and empty value are treated equivalently.
// Pods form a group if and only if they have the same value for ALL keys.
// Returns groups in deterministic order.
func GroupPodsByMatchLabelKeys(pods []*corev1.Pod, matchLabelKeys []string) [][]*corev1.Pod {
	if len(matchLabelKeys) == 0 {
		return [][]*corev1.Pod{pods}
	}
	groupMap := make(map[string][]*corev1.Pod)
	for _, pod := range pods {
		groupKey := podGroupKey(pod, matchLabelKeys)
		groupMap[groupKey] = append(groupMap[groupKey], pod)
	}
	keys := make([]string, 0, len(groupMap))
	for k := range groupMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	groups := make([][]*corev1.Pod, 0, len(keys))
	for _, k := range keys {
		groups = append(groups, groupMap[k])
	}
	return groups
}

// GetPodGroupByMatchLabelKeys returns the subset of pods that belong to the same group as targetPod.
func GetPodGroupByMatchLabelKeys(pods []*corev1.Pod, targetPod *corev1.Pod, matchLabelKeys []string) []*corev1.Pod {
	if len(matchLabelKeys) == 0 {
		return pods
	}
	var group []*corev1.Pod
	for _, p := range pods {
		allMatch := true
		for _, key := range matchLabelKeys {
			if p.Labels[key] != targetPod.Labels[key] {
				allMatch = false
				break
			}
		}
		if allMatch {
			group = append(group, p)
		}
	}
	return group
}

func podGroupKey(pod *corev1.Pod, matchLabelKeys []string) string {
	keyVals := make([]string, len(matchLabelKeys))
	for i, key := range matchLabelKeys {
		keyVals[i] = fmt.Sprintf("%d=%s", len(pod.Labels[key]), pod.Labels[key])
	}
	return strings.Join(keyVals, "\x00")
}

func getMatchedPods(reader client.Reader, cms *appsv1alpha1.ConfigMapSet) (*corev1.PodList, error) {
	// Get related Pods by spec.selector
	// Get matched pods
	pods := &corev1.PodList{}
	labelSelector, err := metav1.LabelSelectorAsSelector(cms.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %v", err)
	}
	err = reader.List(context.Background(), pods, &client.ListOptions{
		Namespace:     cms.Namespace,
		LabelSelector: labelSelector, // Use converted LabelSelector
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %v", err)
	}
	return pods, nil
}
