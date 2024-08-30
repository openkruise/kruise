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
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/configuration"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/fieldpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	// SidecarSetUpgradable is a pod condition to indicate whether the pod's sidecarset is upgradable
	SidecarSetUpgradable corev1.PodConditionType = "SidecarSetUpgradable"
)

var (
	// SidecarIgnoredNamespaces specifies the namespaces where Pods won't get injected
	// SidecarIgnoredNamespaces = []string{"kube-system", "kube-public"}
	// SubPathExprEnvReg format: $(ODD_NAME)、$(POD_NAME)...
	SubPathExprEnvReg, _ = regexp.Compile(`\$\(([-._a-zA-Z][-._a-zA-Z0-9]*)\)`)

	UpdateExpectations = expectations.NewUpdateExpectations(RevisionAdapterImpl)
)

type SidecarSetUpgradeSpec struct {
	UpdateTimestamp              metav1.Time `json:"updateTimestamp"`
	SidecarSetHash               string      `json:"hash"`
	SidecarSetName               string      `json:"sidecarSetName"`
	SidecarList                  []string    `json:"sidecarList"`                  // sidecarSet container list
	SidecarSetControllerRevision string      `json:"controllerRevision,omitempty"` // sidecarSet controllerRevision name
}

// PodMatchSidecarSet determines if pod match Selector of sidecar.
func PodMatchedSidecarSet(c client.Client, pod *corev1.Pod, sidecarSet *appsv1alpha1.SidecarSet) (bool, error) {
	podNamespace := pod.Namespace
	if podNamespace == "" {
		podNamespace = "default"
	}
	//If Namespace is not empty, sidecarSet will only match the pods in the namespaces
	if sidecarSet.Spec.Namespace != "" && sidecarSet.Spec.Namespace != podNamespace {
		return false, nil
	}
	if sidecarSet.Spec.NamespaceSelector != nil &&
		!IsSelectorNamespace(c, podNamespace, sidecarSet.Spec.NamespaceSelector) {
		return false, nil
	}

	// if selector not matched, then continue
	selector, err := util.ValidatedLabelSelectorAsSelector(sidecarSet.Spec.Selector)
	if err != nil {
		return false, err
	}

	if !selector.Empty() && selector.Matches(labels.Set(pod.Labels)) {
		return true, nil
	}
	return false, nil
}

func IsSelectorNamespace(c client.Client, ns string, nsSelector *metav1.LabelSelector) bool {
	selector, err := util.ValidatedLabelSelectorAsSelector(nsSelector)
	if err != nil {
		return false
	}
	nsObj := &corev1.Namespace{}
	err = c.Get(context.TODO(), client.ObjectKey{Name: ns}, nsObj)
	if err != nil {
		return false
	}
	return selector.Matches(labels.Set(nsObj.Labels))
}

// FetchSidecarSetMatchedNamespace fetch sidecarSet matched namespaces
func FetchSidecarSetMatchedNamespace(c client.Client, sidecarSet *appsv1alpha1.SidecarSet) (sets.String, error) {
	ns := sets.NewString()
	//If Namespace is not empty, sidecarSet will only match the pods in the namespaces
	if sidecarSet.Spec.Namespace != "" {
		return ns.Insert(sidecarSet.Spec.Namespace), nil
	}
	// get more faster selector
	selector, err := util.ValidatedLabelSelectorAsSelector(sidecarSet.Spec.NamespaceSelector)
	if err != nil {
		return nil, err
	}
	nsList := &corev1.NamespaceList{}
	if err = c.List(context.TODO(), nsList, &client.ListOptions{LabelSelector: selector}, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	for _, obj := range nsList.Items {
		ns.Insert(obj.Name)
	}
	return ns, nil
}

// IsActivePod determines the pod whether need be injected and updated
func IsActivePod(pod *corev1.Pod) bool {
	/*for _, namespace := range SidecarIgnoredNamespaces {
		if pod.Namespace == namespace {
			return false
		}
	}*/
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
		klog.ErrorS(err, "Failed to parse pod annotations value", "pod", klog.KObj(pod),
			"annotations", hashKey, "value", annotations[hashKey])
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

// UpdatePodSidecarSetHash when sidecarSet in-place update sidecar container, Update sidecarSet hash in Pod annotations[kruise.io/sidecarset-hash]
func UpdatePodSidecarSetHash(pod *corev1.Pod, sidecarSet *appsv1alpha1.SidecarSet) {
	hashKey := SidecarSetHashAnnotation
	sidecarSetHash := make(map[string]SidecarSetUpgradeSpec)
	if err := json.Unmarshal([]byte(pod.Annotations[hashKey]), &sidecarSetHash); err != nil {
		klog.ErrorS(err, "Failed to unmarshal pod annotations", "pod", klog.KObj(pod), "annotations", hashKey)

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

	sidecarList := listSidecarNameInSidecarSet(sidecarSet)
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
					klog.InfoS("Pod container got nil env", "pod", klog.KObj(pod), "containerName", appContainer.Name, "env", envName)
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
				klog.ErrorS(err, "Failed to get containerName from pod annotations or labels",
					"pod", klog.KObj(pod), "annotationsOrLabels", tEnv.SourceContainerNameFrom.FieldRef)
				continue
			}
			sourceContainerName = containerName
		}
		for _, envName := range envs.List() {
			key := fmt.Sprintf("%v/%v", sourceContainerName, envName)
			env, ok := envsInPod[key]
			if !ok {
				// if sourceContainerName is empty or not found in pod.spec.containers
				klog.InfoS("There was no env in container", "envName", tEnv.EnvName, "containerName", tEnv.SourceContainerName)
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

// PatchPodMetadata patch pod annotations and labels
func PatchPodMetadata(originMetadata *metav1.ObjectMeta, patches []appsv1alpha1.SidecarSetPatchPodMetadata) (skip bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	if originMetadata.Annotations == nil {
		originMetadata.Annotations = map[string]string{}
	}
	oldData := originMetadata.DeepCopy()
	for _, patch := range patches {
		switch patch.PatchPolicy {
		case appsv1alpha1.SidecarSetRetainPatchPolicy, "":
			retainPatchPodMetadata(originMetadata, patch)
		case appsv1alpha1.SidecarSetOverwritePatchPolicy:
			overwritePatchPodMetadata(originMetadata, patch)
		case appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy:
			if err = mergePatchJsonPodMetadata(originMetadata, patch); err != nil {
				return
			}
		}
	}
	if reflect.DeepEqual(oldData.Annotations, originMetadata.Annotations) {
		skip = true
	}
	return
}

func retainPatchPodMetadata(originMetadata *metav1.ObjectMeta, patchPodField appsv1alpha1.SidecarSetPatchPodMetadata) {
	for k, v := range patchPodField.Annotations {
		if _, ok := originMetadata.Annotations[k]; !ok {
			originMetadata.Annotations[k] = v
		}
	}
}

func overwritePatchPodMetadata(originMetadata *metav1.ObjectMeta, patchPodField appsv1alpha1.SidecarSetPatchPodMetadata) {
	for k, v := range patchPodField.Annotations {
		originMetadata.Annotations[k] = v
	}
}

func mergePatchJsonPodMetadata(originMetadata *metav1.ObjectMeta, patchPodField appsv1alpha1.SidecarSetPatchPodMetadata) error {
	for key, patchJSON := range patchPodField.Annotations {
		if origin, ok := originMetadata.Annotations[key]; ok && origin != "" {
			modified, err := jsonpatch.MergePatch([]byte(origin), []byte(patchJSON))
			if err != nil {
				return err
			}
			originMetadata.Annotations[key] = string(modified)
		} else {
			originMetadata.Annotations[key] = patchJSON
		}
	}
	return nil
}

func ValidateSidecarSetPatchMetadataWhitelist(c client.Client, sidecarSet *appsv1alpha1.SidecarSet) error {
	if len(sidecarSet.Spec.PatchPodMetadata) == 0 {
		return nil
	}

	regAnnotations := make([]*regexp.Regexp, 0)
	whitelist, err := configuration.GetSidecarSetPatchMetadataWhiteList(c)
	if err != nil {
		return err
	} else if whitelist == nil {
		if utilfeature.DefaultFeatureGate.Enabled(features.SidecarSetPatchPodMetadataDefaultsAllowed) {
			return nil
		}
		return fmt.Errorf("SidecarSet patch metadata whitelist not found")
	}

	for _, rule := range whitelist.Rules {
		if rule.Selector != nil {
			selector, err := util.ValidatedLabelSelectorAsSelector(rule.Selector)
			if err != nil {
				return err
			}
			if !selector.Matches(labels.Set(sidecarSet.Labels)) {
				continue
			}
		}
		for _, key := range rule.AllowedAnnotationKeyExprs {
			reg, err := regexp.Compile(key)
			if err != nil {
				return err
			}
			regAnnotations = append(regAnnotations, reg)
		}
	}
	if len(regAnnotations) == 0 {
		if utilfeature.DefaultFeatureGate.Enabled(features.SidecarSetPatchPodMetadataDefaultsAllowed) {
			return nil
		}
		return fmt.Errorf("sidecarSet patch metadata annotation is not allowed")
	}
	for _, patch := range sidecarSet.Spec.PatchPodMetadata {
		for key := range patch.Annotations {
			if !matchRegKey(key, regAnnotations) {
				return fmt.Errorf("sidecarSet patch metadata annotation(%s) is not allowed", key)
			}
		}
	}
	return nil
}

func matchRegKey(key string, regs []*regexp.Regexp) bool {
	for _, reg := range regs {
		if reg.MatchString(key) {
			return true
		}
	}
	return false
}

// IsSidecarContainer check whether initContainer is sidecar container in k8s 1.28.
func IsSidecarContainer(container corev1.Container) bool {
	if container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways {
		return true
	}
	return false
}

// listSidecarNameInSidecarSet list always init containers and sidecar containers
func listSidecarNameInSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) sets.String {
	sidecarList := sets.NewString()
	for _, sidecar := range sidecarSet.Spec.InitContainers {
		if IsSidecarContainer(sidecar.Container) {
			sidecarList.Insert(sidecar.Name)
		}
	}
	for _, sidecar := range sidecarSet.Spec.Containers {
		sidecarList.Insert(sidecar.Name)
	}
	return sidecarList
}
