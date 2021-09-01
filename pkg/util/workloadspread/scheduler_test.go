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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

func newResourcePod(usage ...schedulernodeinfo.Resource) *v1.Pod {
	containers := []v1.Container{}
	for _, req := range usage {
		containers = append(containers, v1.Container{
			Resources: v1.ResourceRequirements{Requests: req.ResourceList()},
		})
	}
	return &v1.Pod{
		Spec: v1.PodSpec{
			Containers: containers,
		},
	}
}

func makeResources(milliCPU, memory, pods int64) v1.NodeResources {
	return v1.NodeResources{
		Capacity: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(memory, resource.BinarySI),
			v1.ResourcePods:   *resource.NewQuantity(pods, resource.DecimalSI),
		},
	}
}

func makeAllocatableResources(milliCPU, memory, pods int64) v1.ResourceList {
	return v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(memory, resource.BinarySI),
		v1.ResourcePods:   *resource.NewQuantity(pods, resource.DecimalSI),
	}
}

func TestPredicates(t *testing.T) {
	tests := []struct {
		name     string
		pod      func() *v1.Pod
		node     *v1.Node
		nodeInfo func() *schedulernodeinfo.NodeInfo
		fits     bool
	}{
		{
			pod: func() *v1.Pod {
				return newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1})
			},
			node: &v1.Node{},
			nodeInfo: func() *schedulernodeinfo.NodeInfo {
				return schedulernodeinfo.NewNodeInfo(
					newResourcePod(schedulernodeinfo.Resource{MilliCPU: 10, Memory: 20}))
			},
			fits: false,
			name: "too many resources fails",
		},
		{
			pod: func() *v1.Pod {
				return newResourcePod(schedulernodeinfo.Resource{MilliCPU: 1, Memory: 1})
			},
			node: &v1.Node{},
			nodeInfo: func() *schedulernodeinfo.NodeInfo {
				return schedulernodeinfo.NewNodeInfo(
					newResourcePod(schedulernodeinfo.Resource{MilliCPU: 5, Memory: 5}))
			},
			fits: true,
			name: "both resources fit",
		},
		{
			pod: func() *v1.Pod {
				return &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod0",
					},
				}
			},
			node: &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{{Key: "dedicated", Value: "user1", Effect: "NoSchedule"}},
				},
			},
			nodeInfo: func() *schedulernodeinfo.NodeInfo {
				return schedulernodeinfo.NewNodeInfo(
					newResourcePod(schedulernodeinfo.Resource{MilliCPU: 10, Memory: 20}))
			},
			fits: false,
			name: "A pod having no tolerations can't be scheduled onto a node with nonempty taints",
		},
		{
			pod: func() *v1.Pod {
				return &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
					},
					Spec: v1.PodSpec{
						Containers:  []v1.Container{{Image: "pod1:V1"}},
						Tolerations: []v1.Toleration{{Key: "dedicated", Value: "user1", Effect: "NoSchedule"}},
					},
				}
			},
			node: &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{{Key: "dedicated", Value: "user1", Effect: "NoSchedule"}},
				},
			},
			nodeInfo: func() *schedulernodeinfo.NodeInfo {
				return schedulernodeinfo.NewNodeInfo(
					newResourcePod(schedulernodeinfo.Resource{MilliCPU: 10, Memory: 20}))
			},
			fits: true,
			name: "A pod which can be scheduled on a dedicated node assigned to user1 with effect NoSchedule",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := test.node
			node.Status = v1.NodeStatus{Capacity: makeResources(10, 20, 32).Capacity,
				Allocatable: makeAllocatableResources(10, 20, 32)}
			nodeInfo := test.nodeInfo()
			_ = nodeInfo.SetNode(node)
			fits, err := predicates(test.pod(), nodeInfo)
			if err != nil {
				t.Logf("unexpected error: %v", err)
			}
			if fits != test.fits {
				t.Errorf("expected: %v got %v", test.fits, fits)
			}
		})
	}
}
