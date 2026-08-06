/*
Copyright 2025 The Kruise Authors.

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

package daemonset

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

// pruneNodesAndOrphanedPods removes ineligible nodes from nodeToDaemonPods so they
// do not consume rolling-update budget, and returns the names of pods on those nodes
// so the caller can delete them via syncNodes.
func (dsc *ReconcileDaemonSet) pruneNodesAndOrphanedPods(ds *appsv1beta1.DaemonSet, nodeToDaemonPods map[string][]*corev1.Pod, nodeList []*corev1.Node) (map[string][]*corev1.Pod, []string) {
	nodeMap := make(map[string]*corev1.Node)
	for _, node := range nodeList {
		nodeMap[node.Name] = node
	}

	podsToDelete := sets.NewString()
	nodeToDelete := sets.NewString()

	for nodeName, pods := range nodeToDaemonPods {
		node, exists := nodeMap[nodeName]
		deleteNode := true
		if exists {
			wantToRun, _ := nodeShouldRunDaemonPod(node, ds)
			if wantToRun {
				deleteNode = false
			}
		}

		if deleteNode {
			nodeToDelete.Insert(nodeName)
			for _, pod := range pods {
				podsToDelete.Insert(pod.Name)
			}
		}
	}
	for _, nodeName := range nodeToDelete.List() {
		delete(nodeToDaemonPods, nodeName)
	}

	return nodeToDaemonPods, podsToDelete.List()
}
