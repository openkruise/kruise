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
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/fieldpath"
)

const (
	SidecarSetKindName = "kruise.io/sidecarset-name"

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
	UpdateTimestamp              metav1.Time `json:"updateTimestamp"`
	SidecarSetHash               string      `json:"hash"`
	SidecarSetName               string      `json:"sidecarSetName"`
	SidecarList                  []string    `json:"sidecarList"`                  // sidecarSet container list
	SidecarSetControllerRevision string      `json:"controllerRevision,omitempty"` // sidecarSet controllerRevision name
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
	return kubecontroller.IsPodActive(pod)
}

func GetSidecarSetRevision(sidecarSet *appsv1alpha1.SidecarSet) string {
	return sidecarSet.Annotations[SidecarSetHashAnnotation]
}

func GetSidecarSetWithoutImageRevision(sidecarSet *appsv1alpha1.SidecarSet) string {
	return sidecarSet.Annotations[SidecarSetHashWithoutImageAnnotation]
}

func GetPodSidecarSetRevision(sidecarSetName string, pod metav1.Object) string {
	upgradeSpec := GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetName, SidecarSetHashAnnotation, pod)
	return upgradeSpec.SidecarSetHash
}

func GetPodSidecarSetControllerRevision(sidecarSetName string, pod metav1.Object) string {
	upgradeSpec := GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetName, SidecarSetHashAnnotation, pod)
	return upgradeSpec.SidecarSetControllerRevision
}

func GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetName, annotationKey string, pod metav1.Object) SidecarSetUpgradeSpec {
	annotations := pod.GetAnnotations()
	hashKey := annotationKey
	if annotations[hashKey] == "" {
		return SidecarSetUpgradeSpec{}
	}

	sidecarSetHash := make(map[string]SidecarSetUpgradeSpec)
	if err := json.Unmarshal([]byte(annotations[hashKey]), &sidecarSetHash); err != nil {
		klog.Errorf("parse pod(%s/%s) annotations[%s] value(%s) failed: %s", pod.GetNamespace(), pod.GetName(), hashKey,
			annotations[hashKey], err.Error())
		// to be compatible with older sidecarSet hash struct, map[string]string
		olderSidecarSetHash := make(map[string]string)
		if err = json.Unmarshal([]byte(annotations[hashKey]), &olderSidecarSetHash); err != nil {
			return SidecarSetUpgradeSpec{}
		}
		for k, v := range olderSidecarSetHash {
			sidecarSetHash[k] = SidecarSetUpgradeSpec{
				SidecarSetHash: v,
			}
		}
	}

	return sidecarSetHash[sidecarSetName]
}

func GetPodSidecarSetWithoutImageRevision(sidecarSetName string, pod metav1.Object) string {
	upgradeSpec := GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetName, SidecarSetHashWithoutImageAnnotation, pod)
	return upgradeSpec.SidecarSetHash
}

// whether this pod has been updated based on the latest sidecarSet
func IsPodSidecarUpdated(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) bool {
	return GetSidecarSetRevision(sidecarSet) == GetPodSidecarSetRevision(sidecarSet.Name, pod)
}

func updatePodSidecarSetHash(pod *corev1.Pod, sidecarSet *appsv1alpha1.SidecarSet) {
	hashKey := SidecarSetHashAnnotation
	sidecarSetHash := make(map[string]SidecarSetUpgradeSpec)
	if err := json.Unmarshal([]byte(pod.Annotations[hashKey]), &sidecarSetHash); err != nil {
		klog.Errorf("unmarshal pod(%s/%s) annotations[%s] failed: %s", pod.Namespace, pod.Name, hashKey, err.Error())

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

	sidecarList := sets.NewString()
	for _, sidecar := range sidecarSet.Spec.Containers {
		sidecarList.Insert(sidecar.Name)
	}

	sidecarSetHash[sidecarSet.Name] = SidecarSetUpgradeSpec{
		UpdateTimestamp:              metav1.Now(),
		SidecarSetHash:               GetSidecarSetRevision(sidecarSet),
		SidecarSetName:               sidecarSet.Name,
		SidecarList:                  sidecarList.List(),
		SidecarSetControllerRevision: sidecarSet.Status.LatestRevision,
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

func IsPodInjectedSidecarSet(pod *corev1.Pod, sidecarSet *appsv1alpha1.SidecarSet) bool {
	sidecarSetNameStr, ok := pod.Annotations[SidecarSetListAnnotation]
	if !ok || len(sidecarSetNameStr) == 0 {
		return false
	}
	sidecarSetNames := sets.NewString(strings.Split(sidecarSetNameStr, ",")...)
	return sidecarSetNames.Has(sidecarSet.Name)
}

func IsPodConsistentWithSidecarSet(pod *corev1.Pod, sidecarSet *appsv1alpha1.SidecarSet) bool {
	for i := range sidecarSet.Spec.Containers {
		container := &sidecarSet.Spec.Containers[i]
		switch container.UpgradeStrategy.UpgradeType {
		case appsv1alpha1.SidecarContainerHotUpgrade:
			_, exist := GetPodHotUpgradeInfoInAnnotations(pod)[container.Name]
			if !exist || util.GetContainer(fmt.Sprintf("%v-1", container.Name), pod) == nil ||
				util.GetContainer(fmt.Sprintf("%v-2", container.Name), pod) == nil {
				return false
			}
		default:
			if util.GetContainer(container.Name, pod) == nil {
				return false
			}
		}

	}
	return true
}

func IsInjectedSidecarContainerInPod(container *corev1.Container) bool {
	return util.GetContainerEnvValue(container, SidecarEnvKey) == "true"
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
					klog.Warningf("pod(%s/%s) container(%s) get env(%s) is nil", pod.Namespace, pod.Name, appContainer.Name, envName)
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
	// if SourceContainerName is set, use it as source container name
	// if SourceContainerNameFrom.FieldRef, use the fieldref value as source container name
	envsInPod := make(map[string]corev1.EnvVar)
	for _, container := range pod.Spec.Containers {
		for _, env := range container.Env {
			key := fmt.Sprintf("%v/%v", container.Name, env.Name)
			envsInPod[key] = env
		}
	}

	for _, tEnv := range sidecarContainer.TransferEnv {
		envs := sets.NewString()
		if tEnv.EnvName != "" {
			envs.Insert(tEnv.EnvName)
		}
		for _, e := range tEnv.EnvNames {
			envs.Insert(e)
		}

		sourceContainerName := tEnv.SourceContainerName
		if tEnv.SourceContainerNameFrom != nil && tEnv.SourceContainerNameFrom.FieldRef != nil {
			containerName, err := ExtractContainerNameFromFieldPath(tEnv.SourceContainerNameFrom.FieldRef, pod)
			if err != nil {
				klog.Errorf("get containerName from pod(%s/%s) annotations or labels[%s] failed: %s", pod.Namespace, pod.Name, tEnv.SourceContainerNameFrom.FieldRef, err.Error())
				continue
			}
			sourceContainerName = containerName
		}
		for _, envName := range envs.List() {
			key := fmt.Sprintf("%v/%v", sourceContainerName, envName)
			env, ok := envsInPod[key]
			if !ok {
				// if sourceContainerName is empty or not found in pod.spec.containers
				klog.Warningf("there is no env %v in container %v", tEnv.EnvName, tEnv.SourceContainerName)
				continue
			}
			injectedEnvs = append(injectedEnvs, env)
		}
	}
	return
}

func ExtractContainerNameFromFieldPath(fs *corev1.ObjectFieldSelector, pod *corev1.Pod) (string, error) {
	fieldPath := fs.FieldPath
	accessor, err := meta.Accessor(pod)
	if err != nil {
		return "", err
	}
	path, subscript, ok := fieldpath.SplitMaybeSubscriptedPath(fieldPath)
	if ok {
		switch path {
		case "metadata.annotations":
			if errs := validation.IsQualifiedName(strings.ToLower(subscript)); len(errs) != 0 {
				return "", fmt.Errorf("invalid key subscript in %s: %s", fieldPath, strings.Join(errs, ";"))
			}
			return accessor.GetAnnotations()[subscript], nil
		case "metadata.labels":
			if errs := validation.IsQualifiedName(subscript); len(errs) != 0 {
				return "", fmt.Errorf("invalid key subscript in %s: %s", fieldPath, strings.Join(errs, ";"))
			}
			return accessor.GetLabels()[subscript], nil
		default:
			return "", fmt.Errorf("fieldPath %q does not support subscript", fieldPath)
		}
	}
	return "", fmt.Errorf("unsupported fieldPath: %v", fieldPath)
}

// code lifted from https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/core/pods/helpers.go
// ConvertDownwardAPIFieldLabel converts the specified downward API field label
// and its value in the pod of the specified version to the internal version,
// and returns the converted label and value. This function returns an error if
// the conversion fails.
func ConvertDownwardAPIFieldLabel(version, label, value string) (string, string, error) {
	if version != "v1" {
		return "", "", fmt.Errorf("unsupported pod version: %s", version)
	}
	path, _, ok := fieldpath.SplitMaybeSubscriptedPath(label)
	if ok {
		switch path {
		case "metadata.annotations", "metadata.labels":
			return label, value, nil
		default:
			return "", "", fmt.Errorf("field path not supported: %s", path)
		}
	}
	return "", "", fmt.Errorf("field label not supported: %s", label)
}
