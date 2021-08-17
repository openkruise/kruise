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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/apis/core/v1/helper"
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

	node, err = t.c.CoreV1().Nodes().Create(node)
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
			node, err = t.c.CoreV1().Nodes().Get(name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			node.Status = v1.NodeStatus{
				Phase:       v1.NodeRunning,
				Capacity:    resources,
				Allocatable: resources,
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionTrue,
						Reason:             "FakeReady",
						LastTransitionTime: metav1.Now(),
						LastHeartbeatTime:  metav1.Now(),
					},
				},
			}
			node, err = t.c.CoreV1().Nodes().UpdateStatus(node)
			return err
		})
	}

	err = fn()
	if err != nil {
		return nil, err
	}

	// start a goroutine to keep updating ready condition
	go func() {
		var noNode bool
		for {
			time.Sleep(time.Second * 3)
			if !noNode {
				err = fn()
				if err != nil {
					if !errors.IsNotFound(err) {
						klog.Errorf("Failed to update status of fake Node %s: %v", name, err)
					}
					noNode = true
				}
			}
			podList, err := t.c.CoreV1().Pods(v1.NamespaceAll).List(metav1.ListOptions{FieldSelector: "spec.nodeName=" + name})
			if err != nil {
				klog.Errorf("Failed to get Pods of fake Node %s: %v", name, err)
				return
			}
			for i := range podList.Items {
				pod := &podList.Items[i]
				if pod.DeletionTimestamp != nil && pod.DeletionGracePeriodSeconds != nil {
					t.c.CoreV1().Pods(pod.Namespace).Delete(pod.Name, &metav1.DeleteOptions{GracePeriodSeconds: utilpointer.Int64Ptr(0)})
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
	err := t.c.CoreV1().Nodes().Delete(name, &metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (t *NodeTester) ListRealNodesWithFake(tolerations []v1.Toleration) ([]*v1.Node, error) {
	nodeList, err := t.c.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var nodes []*v1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if helper.TolerationsTolerateTaintsWithFilter(tolerations, node.Spec.Taints, nil) {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}
