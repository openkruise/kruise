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

package sidecarcontrol

import (
	"encoding/json"
	"fmt"
	"regexp"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
)

const (
	// SidecarSetHashAnnotation represents the key of a sidecarSet hash
	SidecarSetHashAnnotation = "kruise.io/sidecarset-hash"
	// SidecarSetHashWithoutImageAnnotation represents the key of a sidecarset hash without images of sidecar
	SidecarSetHashWithoutImageAnnotation = "kruise.io/sidecarset-hash-without-image"

	// SidecarSetListAnnotation represent sidecarset list that injected pods
	SidecarSetListAnnotation = "kruise.io/sidecarset-injected-list"

	// SidecarEnvKey specifies the environment variable which record a container as injected
	SidecarEnvKey = "IS_INJECTED"

	// SidecarsetInplaceUpdateStateKey records the state of inplace-update.
	// The value of annotation is SidecarsetInplaceUpdateStateKey.
	SidecarsetInplaceUpdateStateKey string = "kruise.io/sidecarset-inplace-update-state"
)

var (
	// SidecarIgnoredNamespaces specifies the namespaces where Pods won't get injected
	SidecarIgnoredNamespaces = []string{"kube-system", "kube-public"}
	// SubPathExprEnvReg format: $(ODD_NAME)、$(POD_NAME)...
	SubPathExprEnvReg, _ = regexp.Compile(`\$\(([-._a-zA-Z][-._a-zA-Z0-9]*)\)`)
)

type SidecarSetUpgradeSpec struct {
	UpdateTimestamp metav1.Time `json:"updateTimestamp"`
	SidecarSetHash  string      `json:"hash"`
	SidecarSetName  string      `json:"sidecarSetName"`
}

// PodMatchSidecarSet determines if pod match Selector of sidecar.
func PodMatchedSidecarSet(pod *corev1.Pod, sidecarSet appsv1alpha1.SidecarSet) (bool, error) {
	//If matchedNamespace is not empty, sidecarSet will only match the pods in the namespace
	if sidecarSet.Spec.Namespace != "" && sidecarSet.Spec.Namespace != pod.Namespace {
		return false, nil
	}
	// if selector not matched, then continue
	selector, err := metav1.LabelSelectorAsSelector(sidecarSet.Spec.Selector)
	if err != nil {
		return false, err
	}

	if !selector.Empty() && selector.Matches(labels.Set(pod.Labels)) {
		return true, nil
	}
	return false, nil
}

// IsActivePod determines the pod whether need be injected and updated
func IsActivePod(pod *corev1.Pod) bool {
	for _, namespace := range SidecarIgnoredNamespaces {
		if pod.Namespace == namespace {
			return false
		}
	}
	if pod.ObjectMeta.GetDeletionTimestamp() != nil {
		return false
	}
	return true
}

func GetSidecarSetRevision(sidecarSet *appsv1alpha1.SidecarSet) string {
	return sidecarSet.Annotations[SidecarSetHashAnnotation]
}

func GetSidecarSetWithoutImageRevision(sidecarSet *appsv1alpha1.SidecarSet) string {
	return sidecarSet.Annotations[SidecarSetHashWithoutImageAnnotation]
}

func GetPodSidecarSetRevision(sidecarSetName string, pod metav1.Object) string {
	annotations := pod.GetAnnotations()
	hashKey := SidecarSetHashAnnotation
	if annotations[hashKey] == "" {
		return ""
	}

	sidecarSetHash := make(map[string]SidecarSetUpgradeSpec)
	if err := json.Unmarshal([]byte(annotations[hashKey]), &sidecarSetHash); err != nil {
		klog.Warningf("parse pod(%s.%s) annotations[%s] value(%s) failed: %s", pod.GetNamespace(), pod.GetName(), hashKey,
			annotations[hashKey], err.Error())
		// to be compatible with older sidecarSet hash struct, map[string]string
		olderSidecarSetHash := make(map[string]string)
		if err = json.Unmarshal([]byte(annotations[hashKey]), &olderSidecarSetHash); err != nil {
			return ""
		}
		for k, v := range olderSidecarSetHash {
			sidecarSetHash[k] = SidecarSetUpgradeSpec{
				SidecarSetHash: v,
			}
		}
	}
	if upgradeSpec, ok := sidecarSetHash[sidecarSetName]; ok {
		return upgradeSpec.SidecarSetHash
	}
	klog.Warningf("parse pod(%s.%s) annotations[%s] sidecarSet(%s) Not Found", pod.GetNamespace(), pod.GetName(), hashKey, sidecarSetName)
	return ""
}

func GetPodSidecarSetWithoutImageRevision(sidecarSetName string, pod metav1.Object) string {
	annotations := pod.GetAnnotations()
	hashKey := SidecarSetHashWithoutImageAnnotation
	if annotations[hashKey] == "" {
		return ""
	}

	sidecarSetHash := make(map[string]SidecarSetUpgradeSpec)
	if err := json.Unmarshal([]byte(annotations[hashKey]), &sidecarSetHash); err != nil {
		klog.Errorf("parse pod(%s.%s) annotations[%s] value(%s) failed: %s", pod.GetNamespace(), pod.GetName(), hashKey,
			annotations[hashKey], err.Error())
		// to be compatible with older sidecarSet hash struct, map[string]string
		olderSidecarSetHash := make(map[string]string)
		if err = json.Unmarshal([]byte(annotations[hashKey]), &olderSidecarSetHash); err != nil {
			return ""
		}
		for k, v := range olderSidecarSetHash {
			sidecarSetHash[k] = SidecarSetUpgradeSpec{
				SidecarSetHash: v,
			}
		}
	}
	if upgradeSpec, ok := sidecarSetHash[sidecarSetName]; ok {
		return upgradeSpec.SidecarSetHash
	}
	klog.Warningf("parse pod(%s.%s) annotations[%s] sidecarSet(%s) Not Found", pod.GetNamespace(), pod.GetName(), hashKey, sidecarSetName)
	return ""
}

// whether this pod has been updated based on the latest sidecarSet
func IsPodSidecarUpdated(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) bool {
	return GetSidecarSetRevision(sidecarSet) == GetPodSidecarSetRevision(sidecarSet.Name, pod)
}

func updatePodSidecarSetHash(pod *corev1.Pod, sidecarSet *appsv1alpha1.SidecarSet) {
	hashKey := SidecarSetHashAnnotation
	sidecarSetHash := make(map[string]SidecarSetUpgradeSpec)
	if err := json.Unmarshal([]byte(pod.Annotations[hashKey]), &sidecarSetHash); err != nil {
		klog.Errorf("unmarshal pod(%s.%s) annotations[%s] failed: %s", pod.Namespace, pod.Name, hashKey, err.Error())

		// to be compatible with older sidecarSet hash struct, map[string]string
		olderSidecarSetHash := make(map[string]string)
		if err = json.Unmarshal([]byte(pod.Annotations[hashKey]), &olderSidecarSetHash); err == nil {
			for k, v := range olderSidecarSetHash {
				sidecarSetHash[k] = SidecarSetUpgradeSpec{
					SidecarSetHash:  v,
					UpdateTimestamp: metav1.Now(),
					SidecarSetName:  sidecarSet.Name,
				}
			}
		}
		withoutImageHash := make(map[string]SidecarSetUpgradeSpec)
		if err = json.Unmarshal([]byte(pod.Annotations[SidecarSetHashWithoutImageAnnotation]), &olderSidecarSetHash); err == nil {
			for k, v := range olderSidecarSetHash {
				withoutImageHash[k] = SidecarSetUpgradeSpec{
					SidecarSetHash:  v,
					UpdateTimestamp: metav1.Now(),
					SidecarSetName:  sidecarSet.Name,
				}
			}
			newWithoutImageHash, _ := json.Marshal(withoutImageHash)
			pod.Annotations[SidecarSetHashWithoutImageAnnotation] = string(newWithoutImageHash)
		}
		// compatible done
	}

	sidecarSetHash[sidecarSet.Name] = SidecarSetUpgradeSpec{
		UpdateTimestamp: metav1.Now(),
		SidecarSetHash:  GetSidecarSetRevision(sidecarSet),
		SidecarSetName:  sidecarSet.Name,
	}
	newHash, _ := json.Marshal(sidecarSetHash)
	pod.Annotations[hashKey] = string(newHash)
}

func GetSidecarContainersInPod(sidecarSet *appsv1alpha1.SidecarSet) sets.String {
	names := sets.NewString()
	for _, sidecarContainer := range sidecarSet.Spec.Containers {
		if IsHotUpgradeContainer(&sidecarContainer) {
			name1, name2 := GetHotUpgradeContainerName(sidecarContainer.Name)
			names.Insert(name2)
			names.Insert(name1)
		} else {
			names.Insert(sidecarContainer.Name)
		}
	}
	return names
}

func GetPodsSortFunc(pods []*corev1.Pod, waitUpdateIndexes []int) func(i, j int) bool {
	// not-ready < ready, unscheduled < scheduled, and pending < running
	return func(i, j int) bool {
		return kubecontroller.ActivePods(pods).Less(waitUpdateIndexes[i], waitUpdateIndexes[j])
	}
}

func IsInjectedSidecarContainerInPod(container *corev1.Container) bool {
	if util.GetContainerEnvValue(container, SidecarEnvKey) == "true" {
		return true
	}
	return false
}

func IsSharePodVolumeMounts(container *appsv1alpha1.SidecarContainer) bool {
	return container.ShareVolumePolicy.Type == appsv1alpha1.ShareVolumePolicyEnabled
}

func GetInjectedVolumeMountsAndEnvs(control SidecarControl, sidecarContainer *appsv1alpha1.SidecarContainer, pod *corev1.Pod) ([]corev1.VolumeMount, []corev1.EnvVar) {
	if !IsSharePodVolumeMounts(sidecarContainer) {
		return nil, nil
	}

	// injected volumeMounts
	var injectedMounts []corev1.VolumeMount
	// injected EnvVar
	var injectedEnvs []corev1.EnvVar
	for _, appContainer := range pod.Spec.Containers {
		// ignore the injected sidecar container
		if IsInjectedSidecarContainerInPod(&appContainer) {
			continue
		}

		for _, volumeMount := range appContainer.VolumeMounts {
			if !control.NeedToInjectVolumeMount(volumeMount) {
				continue
			}
			injectedMounts = append(injectedMounts, volumeMount)
			//If volumeMounts.SubPathExpr contains expansions, copy environment
			//for example: SubPathExpr=foo/$(ODD_NAME)/$(POD_NAME), we need copy environment ODD_NAME、POD_NAME
			//envs = [$(ODD_NAME) $(POD_NAME)]
			envs := SubPathExprEnvReg.FindAllString(volumeMount.SubPathExpr, -1)
			for _, env := range envs {
				// $(ODD_NAME) -> ODD_NAME
				envName := env[2 : len(env)-1]
				// get envVar in container
				eVar := util.GetContainerEnvVar(&appContainer, envName)
				if eVar == nil {
					klog.Warningf("pod(%s.%s) container(%s) get env(%s) is nil", pod.Namespace, pod.Name, appContainer.Name, envName)
					continue
				}
				injectedEnvs = append(injectedEnvs, *eVar)
			}
		}
	}
	return injectedMounts, injectedEnvs
}

func GetSidecarTransferEnvs(sidecarContainer *appsv1alpha1.SidecarContainer, pod *corev1.Pod) (injectedEnvs []corev1.EnvVar) {
	// pre-process envs in pod, format: container.name/env.name -> container.env
	envsInPod := make(map[string]corev1.EnvVar)
	for _, container := range pod.Spec.Containers {
		for _, env := range container.Env {
			key := fmt.Sprintf("%v/%v", container.Name, env.Name)
			envsInPod[key] = env
		}
	}

	for _, tEnv := range sidecarContainer.TransferEnv {
		key := fmt.Sprintf("%v/%v", tEnv.SourceContainerName, tEnv.EnvName)
		env, ok := envsInPod[key]
		if !ok {
			klog.Warningf("there is no env %v in container %v", tEnv.EnvName, tEnv.SourceContainerName)
			continue
		}
		injectedEnvs = append(injectedEnvs, env)
	}
	return
}
