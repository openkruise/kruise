/*
Copyright 2021 The Kruise Authors.

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

package pubcontrol

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	v1qos "k8s.io/kubernetes/pkg/apis/core/v1/helper/qos"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/kubelet/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
)

type commonControl struct {
	client.Client
	controllerFinder *controllerfinder.ControllerFinder
}

func (c *commonControl) IsPodReady(pod *corev1.Pod) bool {
	// 1. pod.Status.Phase == v1.PodRunning
	// 2. pod.condition PodReady == true
	if !util.IsRunningAndReady(pod) {
		return false
	}

	// unavailable label
	return !appspub.HasUnavailableLabel(pod.Labels)
}

func (c *commonControl) IsPodUnavailableChanged(oldPod, newPod *corev1.Pod) bool {
	// If pod.spec changed, pod may be in unavailable condition
	if !reflect.DeepEqual(oldPod.Spec, newPod.Spec) {
		klog.V(3).InfoS("Pod specification changed, and maybe cause unavailability", "pod", klog.KObj(newPod))
		return true
	}
	// pod add unavailable label
	if !appspub.HasUnavailableLabel(oldPod.Labels) && appspub.HasUnavailableLabel(newPod.Labels) {
		klog.V(3).InfoS("Pod add unavailable label, and maybe cause unavailability", "pod", klog.KObj(newPod))
		return true
	}
	// pod other changes will not cause unavailability situation, then return false

	klog.V(3).InfoS("Pod other changes, and maybe not cause unavailability", "pod", klog.KObj(newPod))
	return false
}

// return true if this action won't cause unavailability
func (c *commonControl) CanResizeInplace(oldPod, newPod *corev1.Pod) bool {
	if !appspub.HasUnavailableLabel(oldPod.Labels) && appspub.HasUnavailableLabel(newPod.Labels) {
		klog.V(3).InfoS("Pod add unavailable label, can not resize inplace", "pod", klog.KObj(newPod))
		return false
	}
	// now check if only container resources changed
	oldPodSpecWithoutResourcesHash, err := podSpecWithoutResourcesHash(oldPod)
	if err != nil {
		klog.ErrorS(err, "Failed to get pod hash", "pod", klog.KObj(oldPod))
		return false
	}

	newPodSpecWithoutResourcesHash, err := podSpecWithoutResourcesHash(newPod)
	if err != nil {
		klog.ErrorS(err, "Failed to get pod hash", "pod", klog.KObj(newPod))
		return false
	}

	if oldPodSpecWithoutResourcesHash != newPodSpecWithoutResourcesHash {
		klog.V(3).InfoS("Pod specification without resources changed, and maybe cause unavailability", "pod", klog.KObj(newPod))
		return false
	}

	// now only containerResources changed.
	// check if static pod
	if types.IsStaticPod(newPod) || types.IsStaticPod(oldPod) {
		klog.V(3).InfoS("Static pod resources changed, and maybe cause unavailability", "pod", klog.KObj(newPod))
		return false
	}

	// check if QoS changed
	if v1qos.ComputePodQOS(oldPod) != v1qos.ComputePodQOS(newPod) {
		klog.V(3).InfoS("Pod QoS changed, and maybe cause unavailability", "pod", klog.KObj(newPod))
		return false
	}

	// only containerResources changed, check if container resource resizePolicy is not NotRequired but
	// resource changed, then return true
	if !canInplaceUpdateResources(oldPod, newPod) {
		klog.V(3).InfoS("Pod container resources changed with restartContainer policy, and maybe cause unavailability", "pod", klog.KObj(newPod))
		return false
	}

	// TODO: Check annotations、labels bind with env using downwardAPI and considering `InPlaceUpdateEnvFromMetadata` featureGate in kruise-daemon
	return true
}

// only allowedResizeResourceKey with NotRequired or "" restartPolicy can do inplace update
func canInplaceUpdateResources(oldPod, newPod *corev1.Pod) bool {
	if len(oldPod.Spec.Containers) != len(newPod.Spec.Containers) {
		return false
	}

	nativeInplaceUpdateImpl := inplaceupdate.GetNativeVerticalUpdateImpl()

	oldCtrMap := make(map[string]corev1.Container)
	for _, ctr := range oldPod.Spec.Containers {
		oldCtrMap[ctr.Name] = ctr
	}

	for _, ctr := range newPod.Spec.Containers {
		oldCtr, ok := oldCtrMap[ctr.Name]
		if !ok {
			return false
		}

		// check whether oldCtr to newCtr resource changed
		rsNames := getResourcesNames(oldCtr.Resources.Requests, ctr.Resources.Requests,
			oldCtr.Resources.Limits, ctr.Resources.Limits)
		rsPolicyMap := getContainerRestartPolicyMap(&ctr)
		for rsName := range rsNames {
			if isContainerResourcesChanged(oldCtr, ctr, rsName) {
				if !nativeInplaceUpdateImpl.CanResourcesResizeInPlace(string(rsName)) {
					klog.V(3).InfoS("Pod container resources changed with not allowed resource, and maybe cause unavailability",
						"pod", klog.KObj(newPod), "container", ctr.Name, "resourceName", rsName)
					return false
				}

				// check restartPolicy
				policy, ok := rsPolicyMap[rsName]
				if !ok || policy == corev1.NotRequired || policy == "" {
					continue
				}

				klog.V(3).InfoS("Pod container resources changed with restartContainer policy, and maybe cause unavailability",
					"pod", klog.KObj(newPod), "container", ctr.Name, "restartPolicy", policy, "resourceName", rsName)
				return false
			}
		}
	}
	return true
}

func getResourcesNames(rsLists ...corev1.ResourceList) sets.Set[corev1.ResourceName] {
	names := sets.New[corev1.ResourceName]()
	for _, list := range rsLists {
		for name := range list {
			names.Insert(name)
		}
	}
	return names
}

func getContainerRestartPolicyMap(ctr *corev1.Container) map[corev1.ResourceName]corev1.ResourceResizeRestartPolicy {
	policyMap := make(map[corev1.ResourceName]corev1.ResourceResizeRestartPolicy)
	for _, policy := range ctr.ResizePolicy {
		policyMap[policy.ResourceName] = policy.RestartPolicy
	}
	return policyMap
}

func isContainerResourcesChanged(oldCtr, newCtr corev1.Container, resourceName corev1.ResourceName) bool {
	requestsChanged := isResourceChanged(oldCtr.Resources.Requests, newCtr.Resources.Requests, resourceName)
	limitsChanged := isResourceChanged(oldCtr.Resources.Limits, newCtr.Resources.Limits, resourceName)
	return requestsChanged || limitsChanged
}

func isResourceChanged(a, b corev1.ResourceList, resourceName corev1.ResourceName) bool {
	// Although the scheduler allocates a certain amount of space, such as 100m/200Mi,
	// for null values in advance, this does not mean that the kubelet will do the same.
	// Therefore, our current implementation imposes a strict constraint—requiring the
	// resource values to be explicitly equal.
	oldResource, oldOk := a[resourceName]
	newResource, newOk := b[resourceName]
	if !oldOk && !newOk {
		return false
	}

	if oldOk && !newOk || !oldOk && newOk {
		return true
	}

	// both exist
	return !oldResource.Equal(newResource)
}

func podSpecWithoutResourcesHash(podIn *corev1.Pod) (string, error) {
	pod := podIn.DeepCopy()
	// do not need to process init-containers because they wont change
	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].Resources = corev1.ResourceRequirements{}
	}

	data, err := json.Marshal(pod.Spec)
	if err != nil {
		return "", err
	}

	return rand.SafeEncodeString(hash(string(data))), nil
}

// hash hashes `data` with sha256 and returns the hex string
func hash(data string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

// GetPodsForPub returns Pods protected by the pub object.
// return two parameters
// 1. podList
// 2. expectedCount, the default is workload.Replicas
func (c *commonControl) GetPodsForPub(pub *policyv1alpha1.PodUnavailableBudget) ([]*corev1.Pod, int32, error) {
	return c.getPodsForPubV1alpha1(pub)
}

func (c *commonControl) getPodsForPubV1alpha1(pub *policyv1alpha1.PodUnavailableBudget) ([]*corev1.Pod, int32, error) {
	// if targetReference isn't nil, priority to take effect
	var listOptions *client.ListOptions
	if pub.Spec.TargetReference != nil {
		ref := pub.Spec.TargetReference
		matchedPods, expectedCount, err := c.controllerFinder.GetPodsForRef(ref.APIVersion, ref.Kind, pub.Namespace, ref.Name, true)
		// For v1alpha1, check annotation first for backward compatibility
		if value, _ := pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation]; value != "" {
			count, _ := strconv.ParseInt(value, 10, 32)
			expectedCount = int32(count)
		}
		return matchedPods, expectedCount, err
	} else if pub.Spec.Selector == nil {
		klog.InfoS("Pub spec.Selector could not be empty", "pub", klog.KObj(pub))
		return nil, 0, nil
	}
	// get pods for selector
	labelSelector, err := util.ValidatedLabelSelectorAsSelector(pub.Spec.Selector)
	if err != nil {
		klog.InfoS("Pub ValidatedLabelSelectorAsSelector failed", "pub", klog.KObj(pub), "error", err)
		return nil, 0, nil
	}
	listOptions = &client.ListOptions{Namespace: pub.Namespace, LabelSelector: labelSelector}
	podList := &corev1.PodList{}
	if err = c.List(context.TODO(), podList, listOptions, utilclient.DisableDeepCopy); err != nil {
		return nil, 0, err
	}
	matchedPods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		if kubecontroller.IsPodActive(pod) {
			matchedPods = append(matchedPods, pod)
		}
	}
	// For v1alpha1, check annotation first for backward compatibility
	if value, _ := pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation]; value != "" {
		expectedCount, _ := strconv.ParseInt(value, 10, 32)
		return matchedPods, int32(expectedCount), nil
	}
	expectedCount, err := c.controllerFinder.GetExpectedScaleForPods(matchedPods)
	if err != nil {
		return nil, 0, err
	}
	return matchedPods, expectedCount, nil
}

// GetPodsForPubV1beta1 returns Pods protected by the pub v1beta1 object.
func (c *commonControl) GetPodsForPubV1beta1(pub *policyv1beta1.PodUnavailableBudget) ([]*corev1.Pod, int32, error) {
	// if targetReference isn't nil, priority to take effect
	var listOptions *client.ListOptions
	if pub.Spec.TargetReference != nil {
		ref := pub.Spec.TargetReference
		matchedPods, expectedCount, err := c.controllerFinder.GetPodsForRef(ref.APIVersion, ref.Kind, pub.Namespace, ref.Name, true)
		// For v1beta1, use field instead of annotation
		if pub.Spec.ProtectTotalReplicas != nil {
			expectedCount = *pub.Spec.ProtectTotalReplicas
		}
		return matchedPods, expectedCount, err
	} else if pub.Spec.Selector == nil {
		klog.InfoS("Pub spec.Selector could not be empty", "pub", klog.KObj(pub))
		return nil, 0, nil
	}
	// get pods for selector
	labelSelector, err := util.ValidatedLabelSelectorAsSelector(pub.Spec.Selector)
	if err != nil {
		klog.InfoS("Pub ValidatedLabelSelectorAsSelector failed", "pub", klog.KObj(pub), "error", err)
		return nil, 0, nil
	}
	listOptions = &client.ListOptions{Namespace: pub.Namespace, LabelSelector: labelSelector}
	podList := &corev1.PodList{}
	if err = c.List(context.TODO(), podList, listOptions, utilclient.DisableDeepCopy); err != nil {
		return nil, 0, err
	}
	matchedPods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		if kubecontroller.IsPodActive(pod) {
			matchedPods = append(matchedPods, pod)
		}
	}
	// For v1beta1, use field instead of annotation
	if pub.Spec.ProtectTotalReplicas != nil {
		return matchedPods, *pub.Spec.ProtectTotalReplicas, nil
	}
	expectedCount, err := c.controllerFinder.GetExpectedScaleForPods(matchedPods)
	if err != nil {
		return nil, 0, err
	}
	return matchedPods, expectedCount, nil
}

func (c *commonControl) IsPodStateConsistent(pod *corev1.Pod) bool {
	// if all container image is digest format
	// by comparing status.containers[x].ImageID with spec.container[x].Image can determine whether pod is consistent
	allDigestImage := true
	for _, container := range pod.Spec.Containers {
		//whether image is digest format,
		//for example: docker.io/busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d
		if !util.IsImageDigest(container.Image) {
			allDigestImage = false
			continue
		}

		if !util.IsPodContainerDigestEqual(sets.NewString(container.Name), pod) {
			klog.V(5).InfoS("Pod container image was inconsistent", "pod", klog.KObj(pod), "containerName", container.Name)
			return false
		}
	}
	// If all spec.container[x].image is digest format, only check digest imageId
	if allDigestImage {
		return true
	}

	// check whether injected sidecar container is consistent
	sidecarSets, sidecars := getSidecarSetsInPod(pod)
	if sidecarSets.Len() > 0 && sidecars.Len() > 0 {
		if !sidecarcontrol.IsSidecarContainerUpdateCompleted(pod, sidecarSets, sidecars) {
			klog.V(5).InfoS("PodUnavailableBudget check pod was inconsistent", "pod", klog.KObj(pod))
			return false
		}
	}

	// whether other containers is consistent
	if err := inplaceupdate.DefaultCheckInPlaceUpdateCompleted(pod); err != nil {
		klog.V(5).InfoS("Failed to check pod InPlaceUpdate", "pod", klog.KObj(pod), "error", err)
		return false
	}

	return true
}

func (c *commonControl) GetPubForPod(pod *corev1.Pod) (*policyv1alpha1.PodUnavailableBudget, error) {
	if len(pod.Annotations) == 0 || pod.Annotations[PodRelatedPubAnnotation] == "" {
		return nil, nil
	}
	pubName := pod.Annotations[PodRelatedPubAnnotation]
	pub := &policyv1alpha1.PodUnavailableBudget{}
	err := c.Get(context.TODO(), client.ObjectKey{Namespace: pod.Namespace, Name: pubName}, pub)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("Pod pub was NotFound", "pod", klog.KObj(pod), "pubName", pubName)
			return nil, nil
		}
		return nil, err
	}
	return pub, nil
}

func (c *commonControl) GetPodControllerOf(pod *corev1.Pod) *metav1.OwnerReference {
	return metav1.GetControllerOf(pod)
}

func getSidecarSetsInPod(pod *corev1.Pod) (sidecarSets, containers sets.String) {
	containers = sets.NewString()
	sidecarSets = sets.NewString()
	if setList, ok := pod.Annotations[sidecarcontrol.SidecarSetListAnnotation]; ok && len(setList) > 0 {
		for _, sidecarSetName := range strings.Split(setList, ",") {
			sidecarSets.Insert(sidecarSetName)
		}
	}
	for _, container := range pod.Spec.Containers {
		val := util.GetContainerEnvValue(&container, sidecarcontrol.SidecarEnvKey)
		if val == "true" {
			containers.Insert(container.Name)
		}
	}

	return sidecarSets, containers
}
