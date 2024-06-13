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

package framework

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	utilpointer "k8s.io/utils/pointer"
)

const (
	E2eFakeKey         = "kruise-e2e-fake"
	fakeNodeNamePrefix = "fake-node-"
)

type NodeTester struct {
	c clientset.Interface
}

func NewNodeTester(c clientset.Interface) *NodeTester {
	return &NodeTester{
		c: c,
	}
}

func (t *NodeTester) CreateFakeNode(randStr string) (node *v1.Node, err error) {
	name := fakeNodeNamePrefix + randStr
	node = &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{v1.LabelHostname: name, E2eFakeKey: "true"},
		},
		Spec: v1.NodeSpec{
			Taints: []v1.Taint{{Key: E2eFakeKey, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
		},
	}

	node, err = t.c.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	resources := v1.ResourceList{
		v1.ResourceCPU:              resource.MustParse("4"),
		v1.ResourceMemory:           resource.MustParse("8Gi"),
		v1.ResourceEphemeralStorage: resource.MustParse("20Gi"),
		v1.ResourcePods:             resource.MustParse("16"),
	}

	fn := func() error {
		return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			node, err = t.c.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			now := time.Now()
			node.Status = v1.NodeStatus{
				Phase:       v1.NodeRunning,
				Capacity:    resources,
				Allocatable: resources,
				Conditions: []v1.NodeCondition{
					{
						Type:   v1.NodeReady,
						Status: v1.ConditionTrue,
						Reason: "FakeReady",
						// Ready for 10min
						LastTransitionTime: metav1.NewTime(now.Add(-time.Minute * 10)),
						LastHeartbeatTime:  metav1.NewTime(now),
					},
				},
			}
			node, err = t.c.CoreV1().Nodes().UpdateStatus(context.TODO(), node, metav1.UpdateOptions{})
			return err
		})
	}

	err = fn()
	if err != nil {
		return nil, err
	}

	// wait for not-ready taint to be removed
	gomega.Eventually(func() bool {
		node, err = t.c.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for _, t := range node.Spec.Taints {
			if t.Key == v1.TaintNodeNotReady {
				return false
			}
		}
		return true
	}, 10*time.Second, time.Second).Should(gomega.Equal(true))

	// start a goroutine to keep updating ready condition
	go func() {
		var noNode bool
		for {
			time.Sleep(time.Second * 3)
			if !noNode {
				err = fn()
				if err != nil {
					if !errors.IsNotFound(err) {
						Logf("Failed to update status of fake Node %s: %v", name, err)
					}
					noNode = true
				}
			}
			podList, err := t.c.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), metav1.ListOptions{FieldSelector: "spec.nodeName=" + name})
			if err != nil {
				Logf("Failed to get Pods of fake Node %s: %v", name, err)
				return
			}
			for i := range podList.Items {
				pod := &podList.Items[i]
				if pod.DeletionTimestamp != nil && pod.DeletionGracePeriodSeconds != nil {
					t.c.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(0)})
				}
			}
			if len(podList.Items) == 0 && noNode {
				return
			}
		}
	}()

	return node, nil
}

func (t *NodeTester) DeleteFakeNode(randStr string) error {
	name := fakeNodeNamePrefix + randStr
	err := t.c.CoreV1().Nodes().Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (t *NodeTester) ListNodesWithFake() ([]*v1.Node, error) {
	nodeList, err := t.c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var nodes []*v1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (t *NodeTester) ListRealNodesWithFake(tolerations []v1.Toleration) ([]*v1.Node, error) {
	nodeList, err := t.c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	Logf("ListRealNodesWithFake starts check tolerations %v with nodes (%v)", tolerations, len(nodeList.Items))
	var nodes []*v1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		taint, isUntolerated := v1helper.FindMatchingUntoleratedTaint(node.Spec.Taints, tolerations, nil)
		if !isUntolerated {
			nodes = append(nodes, node)
			Logf("ListRealNodesWithFake check node %s matched", node.Name)
		} else {
			Logf("ListRealNodesWithFake check node %s not matched because of %v", node.Name, taint)
		}
	}
	return nodes, nil
}

const (
	// poll is how often to Poll pods, nodes and claims.
	poll = 2 * time.Second

	// singleCallTimeout is how long to try single API calls (like 'get' or 'list'). Used to prevent
	// transient failures from failing tests.
	singleCallTimeout = 5 * time.Minute
)

var (
	// unreachableTaintTemplate is the taint for when a node becomes unreachable.
	// Copied from pkg/controller/nodelifecycle to avoid pulling extra dependencies
	unreachableTaintTemplate = &v1.Taint{
		Key:    v1.TaintNodeUnreachable,
		Effect: v1.TaintEffectNoExecute,
	}

	// notReadyTaintTemplate is the taint for when a node is not ready for executing pods.
	// Copied from pkg/controller/nodelifecycle to avoid pulling extra dependencies
	notReadyTaintTemplate = &v1.Taint{
		Key:    v1.TaintNodeNotReady,
		Effect: v1.TaintEffectNoExecute,
	}

	// updateTaintBackOff contains the maximum retries and the wait interval between two retries.
	updateTaintBackOff = wait.Backoff{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Jitter:   1.0,
	}
)

// checkWaitListSchedulableNodes is a wrapper around listing nodes supporting retries.
func checkWaitListSchedulableNodes(ctx context.Context, c clientset.Interface) (*v1.NodeList, error) {
	nodes, err := waitListSchedulableNodes(c)
	if err != nil {
		return nil, fmt.Errorf("error: %s. Non-retryable failure or timed out while listing nodes for e2e cluster", err)
	}
	return nodes, nil
}

// Filter filters nodes in NodeList in place, removing nodes that do not
// satisfy the given condition
func Filter(nodeList *v1.NodeList, fn func(node v1.Node) bool) {
	var l []v1.Node

	for _, node := range nodeList.Items {
		if fn(node) {
			l = append(l, node)
		}
	}
	nodeList.Items = l
}

// IsConditionSetAsExpected returns a wantTrue value if the node has a match to the conditionType, otherwise returns an opposite value of the wantTrue with detailed logging.
func IsConditionSetAsExpected(node *v1.Node, conditionType v1.NodeConditionType, wantTrue bool) bool {
	return isNodeConditionSetAsExpected(node, conditionType, wantTrue, false)
}

// IsConditionSetAsExpectedSilent returns a wantTrue value if the node has a match to the conditionType, otherwise returns an opposite value of the wantTrue.
func IsConditionSetAsExpectedSilent(node *v1.Node, conditionType v1.NodeConditionType, wantTrue bool) bool {
	return isNodeConditionSetAsExpected(node, conditionType, wantTrue, true)
}

// isConditionUnset returns true if conditions of the given node do not have a match to the given conditionType, otherwise false.
func isConditionUnset(node *v1.Node, conditionType v1.NodeConditionType) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == conditionType {
			return false
		}
	}
	return true
}

// IsNodeReady returns true if:
// 1) it's Ready condition is set to true
// 2) doesn't have NetworkUnavailable condition set to true
func IsNodeReady(node *v1.Node) bool {
	nodeReady := IsConditionSetAsExpected(node, v1.NodeReady, true)
	networkReady := isConditionUnset(node, v1.NodeNetworkUnavailable) ||
		IsConditionSetAsExpectedSilent(node, v1.NodeNetworkUnavailable, false)
	return nodeReady && networkReady
}

// IsNodeSchedulable returns true if:
// 1) doesn't have "unschedulable" field set
// 2) it also returns true from IsNodeReady
func IsNodeSchedulable(node *v1.Node) bool {
	if node == nil {
		return false
	}
	return !node.Spec.Unschedulable && IsNodeReady(node)
}

func toleratesTaintsWithNoScheduleNoExecuteEffects(taints []v1.Taint, tolerations []v1.Toleration) bool {
	filteredTaints := []v1.Taint{}
	for _, taint := range taints {
		if taint.Effect == v1.TaintEffectNoExecute || taint.Effect == v1.TaintEffectNoSchedule {
			filteredTaints = append(filteredTaints, taint)
		}
	}

	toleratesTaint := func(taint v1.Taint) bool {
		for _, toleration := range tolerations {
			if toleration.ToleratesTaint(&taint) {
				return true
			}
		}

		return false
	}

	for _, taint := range filteredTaints {
		if !toleratesTaint(taint) {
			return false
		}
	}

	return true
}

// isNodeUntaintedWithNonblocking tests whether a fake pod can be scheduled on "node"
// but allows for taints in the list of non-blocking taints.
func isNodeUntaintedWithNonblocking(node *v1.Node, nonblockingTaints string) bool {
	// Simple lookup for nonblocking taints based on comma-delimited list.
	nonblockingTaintsMap := map[string]struct{}{}
	for _, t := range strings.Split(nonblockingTaints, ",") {
		if strings.TrimSpace(t) != "" {
			nonblockingTaintsMap[strings.TrimSpace(t)] = struct{}{}
		}
	}

	n := node
	if len(nonblockingTaintsMap) > 0 {
		nodeCopy := node.DeepCopy()
		nodeCopy.Spec.Taints = []v1.Taint{}
		for _, v := range node.Spec.Taints {
			if _, isNonblockingTaint := nonblockingTaintsMap[v.Key]; !isNonblockingTaint {
				nodeCopy.Spec.Taints = append(nodeCopy.Spec.Taints, v)
			}
		}
		n = nodeCopy
	}

	return toleratesTaintsWithNoScheduleNoExecuteEffects(n.Spec.Taints, nil)
}

// GetReadySchedulableNodes addresses the common use case of getting nodes you can do work on.
// 1) Needs to be schedulable.
// 2) Needs to be ready.
// If EITHER 1 or 2 is not true, most tests will want to ignore the node entirely.
// If there are no nodes that are both ready and schedulable, this will return an error.
func GetReadySchedulableNodes(ctx context.Context, c clientset.Interface) (nodes *v1.NodeList, err error) {
	nodes, err = checkWaitListSchedulableNodes(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("listing schedulable nodes error: %w", err)
	}
	Filter(nodes, func(node v1.Node) bool {
		return IsNodeSchedulable(&node) && isNodeUntainted(&node)
	})
	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("there are currently no ready, schedulable nodes in the cluster")
	}
	return nodes, nil
}

// GetRandomReadySchedulableNode gets a single randomly-selected node which is available for
// running pods on. If there are no available nodes it will return an error.
func GetRandomReadySchedulableNode(ctx context.Context, c clientset.Interface) (*v1.Node, error) {
	nodes, err := GetReadySchedulableNodes(ctx, c)
	if err != nil {
		return nil, err
	}
	return &nodes.Items[rand.Intn(len(nodes.Items))], nil
}

func WaitForNodeSchedulable(ctx context.Context, c clientset.Interface, name string, timeout time.Duration, wantSchedulable bool) bool {
	Logf("Waiting up to %v for node %s to be schedulable: %t", timeout, name, wantSchedulable)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(poll) {
		node, err := c.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			Logf("Couldn't get node %s", name)
			continue
		}

		if IsNodeSchedulable(node) == wantSchedulable {
			return true
		}
	}
	Logf("Node %s didn't reach desired schedulable status (%t) within %v", name, wantSchedulable, timeout)
	return false
}
