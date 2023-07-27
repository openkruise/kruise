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
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var PubControl pubControl
var recorder record.EventRecorder
var kclient client.Client

type pubControl interface {
	// IsPodReady indicates whether pod is fully ready
	// 1. pod.Status.Phase == v1.PodRunning
	// 2. pod.condition PodReady == true
	IsPodReady(pod *corev1.Pod) bool
	// IsPodStateConsistent indicates whether pod.spec and pod.status are consistent after updating containers
	IsPodStateConsistent(pod *corev1.Pod) bool
	// GetPodsForPub returns Pods protected by the pub object.
	// return two parameters
	// 1. podList
	// 2. expectedCount, the default is workload.Replicas
	GetPodsForPub(pub *policyv1alpha1.PodUnavailableBudget) ([]*corev1.Pod, int32, error)

	// webhook
	// determine if this change to pod might cause unavailability
	IsPodUnavailableChanged(oldPod, newPod *corev1.Pod) bool
	// get pub for pod
	GetPubForPod(pod *corev1.Pod) (*policyv1alpha1.PodUnavailableBudget, error)
	// get pod controller of
	GetPodControllerOf(pod *corev1.Pod) *metav1.OwnerReference
}

func InitPubControl(cli client.Client, finder *controllerfinder.ControllerFinder, rec record.EventRecorder) {
	recorder = rec
	kclient = cli
	PubControl = &commonControl{controllerFinder: finder, Client: cli}
}
