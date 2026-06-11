package daemonset

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func (dsc *ReconcileDaemonSet) pruneNodesAndOrphanedPods(ds *appsv1beta1.DaemonSet, nodeToDaemonPods map[string][]*corev1.Pod, nodeList []*corev1.Node) map[string][]*corev1.Pod {
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

	return nodeToDaemonPods
}
