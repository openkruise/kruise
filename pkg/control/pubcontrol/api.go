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
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"

	corev1 "k8s.io/api/core/v1"
)

type PubControl interface {
	// Common
	// get PodUnavailableBudget
	GetPodUnavailableBudget() *policyv1alpha1.PodUnavailableBudget
	// IsPodReady indicates whether pod is fully ready
	// 1. pod.Status.Phase == v1.PodRunning
	// 2. pod.condition PodReady == true
	IsPodReady(pod *corev1.Pod) bool
	// IsPodStateConsistent indicates whether pod.spec and pod.status are consistent after updating containers
	IsPodStateConsistent(pod *corev1.Pod) bool

	// webhook
	// determine if this change to pod might cause unavailability
	IsPodUnavailableChanged(oldPod, newPod *corev1.Pod) bool
}

func NewPubControl(pub *policyv1alpha1.PodUnavailableBudget) PubControl {
	return &commonControl{PodUnavailableBudget: pub}
}
