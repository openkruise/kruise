package util

import (
	corev1 "k8s.io/api/core/v1"
)

func IsNodeReady(node *corev1.Node) bool {
	nodeReady := false
	networkReady := false
	networkReadyFlag := false
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			nodeReady = true
		}
		if condition.Type == corev1.NodeNetworkUnavailable && condition.Status == corev1.ConditionFalse {
			networkReady = true
		}
		if condition.Type == corev1.NodeNetworkUnavailable {
			networkReadyFlag = true
		}
	}
	return nodeReady && (networkReady || !networkReadyFlag)
}
