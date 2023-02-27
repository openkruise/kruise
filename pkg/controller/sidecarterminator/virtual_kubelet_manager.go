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

package sidecarterminator

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultVKLabelKey   = "type"
	DefaultVKLabelValue = "virtual-kubelet"
	DefaultVKTaintKey   = "virtual-kubelet.io/provider"
)

func IsPodRunningOnVirtualKubelet(pod *corev1.Pod, reader client.Reader) (bool, error) {
	node := &corev1.Node{}
	err := reader.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, node)
	if err != nil {
		return false, err
	}

	if isVirtualKubelet(node) {
		return true, nil
	}
	return false, nil
}

func isVirtualKubelet(node *corev1.Node) bool {
	return node.Labels[DefaultVKLabelKey] == DefaultVKLabelValue
}
