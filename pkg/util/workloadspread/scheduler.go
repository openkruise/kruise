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

package workloadspread

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/noderesources"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	pluginhelper "k8s.io/kubernetes/pkg/scheduler/framework/plugins/helper"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

func (h *Handler) canScheduleOnSubset(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod, subsetName string) (bool, error) {
	// Make a deep copy because we just simulate Pod schedule process and
	// don't really want to inject subset into this Pod currently.
	clone := pod.DeepCopy()
	if inject, err := injectWorkloadSpreadIntoPod(ws, clone, subsetName, ""); !inject || err != nil {
		klog.Errorf("failed to inject pod(%s/%s) subset(%s) data for workloadSpread(%s/%s)",
			pod.Namespace, pod.Name, subsetName, ws.Namespace, ws.Name)
		return false, err
	}

	subsetNodes, err := h.getNodesForSubset(clone)
	if err != nil {
		return false, err
	}
	if len(subsetNodes) == 0 {
		return false, nil
	}

	for _, node := range subsetNodes {
		canRun := NodeShouldRunPod(ws, clone, node, subsetName)
		if canRun {
			return true, nil
		}
	}

	return false, nil
}

func NodeShouldRunPod(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod, node *corev1.Node, subsetName string) bool {
	fit, err := Predicates(pod, newSchedulerNodeInfo(node))
	if err != nil {
		klog.Warningf("Subset %s predicates failed on Node %s for WorkloadSpread (%s/%s) due to unexpected error: %v",
			subsetName, node.Name, ws.Namespace, ws.Name, err)
	}
	return fit
}

// getNodesForSubset returns nodeList belongs to this subset using nodeAffinity.
func (h *Handler) getNodesForSubset(pod *corev1.Pod) ([]*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	err := h.Client.List(context.TODO(), nodeList)
	if err != nil {
		return nil, err
	}

	subsetNodes := make([]*corev1.Node, 0)
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if pluginhelper.PodMatchesNodeSelectorAndAffinityTerms(pod, node) {
			subsetNodes = append(subsetNodes, node)
		}
	}
	return subsetNodes, nil
}

func newSchedulerNodeInfo(node *corev1.Node) *schedulernodeinfo.NodeInfo {
	nodeInfo := schedulernodeinfo.NewNodeInfo()
	_ = nodeInfo.SetNode(node)
	return nodeInfo
}

// Predicates checks if a Pod can be scheduled on a node of subset.
// the predicates include:
//   - PodToleratesNodeTaints: exclude tainted node unless pod has specific toleration
//   - PodFitsResources: checks if a node has sufficient resources, such as cpu, memory, gpu, opaque int resources etc to run a pod.
func Predicates(pod *corev1.Pod, nodeInfo *schedulernodeinfo.NodeInfo) (bool, error) {
	node := nodeInfo.Node()

	filterPredicate := func(t *corev1.Taint) bool {
		// PodToleratesNodeTaints is only interested in NoSchedule and NoExecute taints.
		return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
	}
	taint, isUntolerated := v1helper.FindMatchingUntoleratedTaint(node.Spec.Taints, pod.Spec.Tolerations, filterPredicate)
	if isUntolerated {
		errReason := fmt.Sprintf("node(s) had taint {%s: %s}, that the pod didn't tolerate",
			taint.Key, taint.Value)
		return logPredicateFailedReason(node, framework.NewStatus(framework.UnschedulableAndUnresolvable, errReason))
	}

	insufficientResources := noderesources.Fits(pod, nodeInfo, sets.String{})
	if len(insufficientResources) != 0 {
		// We will keep all failure reasons.
		failureReasons := make([]string, 0, len(insufficientResources))
		for _, r := range insufficientResources {
			failureReasons = append(failureReasons, r.Reason)
		}
		return logPredicateFailedReason(node, framework.NewStatus(framework.Unschedulable, failureReasons...))
	}

	return true, nil
}

func logPredicateFailedReason(node *corev1.Node, status *framework.Status) (bool, error) {
	if status.IsSuccess() {
		return true, nil
	}
	for _, reason := range status.Reasons() {
		klog.V(6).Infof("Failed predicate on node %s : %s ", node.Name, reason)
	}
	return status.IsSuccess(), status.AsError()
}
