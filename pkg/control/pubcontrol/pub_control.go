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
	"reflect"
	"strings"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type commonControl struct {
	client.Client
	*policyv1alpha1.PodUnavailableBudget
	controllerFinder *controllerfinder.ControllerFinder
}

func (c *commonControl) GetPodUnavailableBudget() *policyv1alpha1.PodUnavailableBudget {
	return c.PodUnavailableBudget
}

func (c *commonControl) IsPodReady(pod *corev1.Pod) bool {
	// 1. pod.Status.Phase == v1.PodRunning
	// 2. pod.condition PodReady == true
	return util.IsRunningAndReady(pod)
}

func (c *commonControl) IsPodUnavailableChanged(oldPod, newPod *corev1.Pod) bool {
	// kruise workload in-place situation
	if newPod == nil || oldPod == nil {
		return true
	}
	// If pod.spec changed, pod will be in unavailable condition
	if !reflect.DeepEqual(oldPod.Spec, newPod.Spec) {
		klog.V(3).Infof("pod(%s.%s) specification changed, and maybe cause unavailability", newPod.Namespace, newPod.Name)
		return true
	}
	// pod other changes will not cause unavailability situation, then return false
	return false
}

// GetPodsForPub returns Pods protected by the pub object.
// return two parameters
// 1. podList
// 2. expectedCount, the default is workload.Replicas
func (c *commonControl) GetPodsForPub() ([]*corev1.Pod, int32, error) {
	pub := c.GetPodUnavailableBudget()
	// if targetReference isn't nil, priority to take effect
	var listOptions *client.ListOptions
	if pub.Spec.TargetReference != nil {
		ref := pub.Spec.TargetReference
		matchedPods, _, expectedCount, err := c.controllerFinder.GetPodsForRef(ref.APIVersion, ref.Kind, ref.Name, pub.Namespace, true)
		return matchedPods, expectedCount, err
	} else if pub.Spec.Selector == nil {
		klog.Warningf("pub(%s/%s) spec.Selector cannot be empty", pub.Namespace, pub.Name)
		return nil, 0, nil
	}
	// get pods for selector
	labelSelector, err := util.GetFastLabelSelector(pub.Spec.Selector)
	if err != nil {
		klog.Warningf("pub(%s/%s) GetFastLabelSelector failed: %s", pub.Namespace, pub.Name, err.Error())
		return nil, 0, nil
	}
	listOptions = &client.ListOptions{Namespace: pub.Namespace, LabelSelector: labelSelector}
	podList := &corev1.PodList{}
	if err := c.List(context.TODO(), podList, listOptions); err != nil {
		return nil, 0, err
	}

	matchedPods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		if kubecontroller.IsPodActive(pod) {
			matchedPods = append(matchedPods, pod)
		}
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
			klog.V(5).Infof("pod(%s.%s) container(%s) image is inconsistent", pod.Namespace, pod.Name, container.Name)
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
			return false
		}
	}

	// whether other containers is consistent
	if err := inplaceupdate.DefaultCheckInPlaceUpdateCompleted(pod); err != nil {
		klog.V(5).Infof("check pod(%s.%s) InPlaceUpdate failed: %s", pod.Namespace, pod.Name, err.Error())
		return false
	}

	return true
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
