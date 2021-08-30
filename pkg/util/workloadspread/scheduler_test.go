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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
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

func newNode(nodeName string, labels map[string]string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: labels,
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

func TestGetNodesForSubset(t *testing.T) {
	cases := []struct {
		name        string
		pod         *v1.Pod
		getNodes    func() []*v1.Node
		expectNodes sets.String
		expectWS    *appsv1alpha1.WorkloadSpread
	}{
		{
			name: "match node_1, node_2",
			expectWS: &appsv1alpha1.WorkloadSpread{
				Spec: appsv1alpha1.WorkloadSpreadSpec{
					Subsets: []appsv1alpha1.WorkloadSpreadSubset{
						{
							Name: "subset-a",
							RequiredNodeSelectorTerm: &v1.NodeSelectorTerm{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "foo",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"bar"},
									},
								},
							},
							Tolerations: []v1.Toleration{
								{
									Key:      "node.kubernetes.io/not-ready",
									Operator: v1.TolerationOpExists,
									Effect:   v1.TaintEffectNoExecute,
								},
							},
							Patch: runtime.RawExtension{
								Raw: []byte(`{"metadata":{"labels":{"subset":"subset-a"},"annotations":{"subset":"subset-a"}}}`),
							},
							MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
						},
					},
				},
			},
			pod: &v1.Pod{},
			getNodes: func() []*v1.Node {
				node1 := newNode("node_1", map[string]string{
					"foo": "bar",
				})
				node2 := newNode("node_2", map[string]string{
					"foo": "bar",
				})
				node3 := newNode("node_3", map[string]string{})
				return []*v1.Node{node1, node2, node3}
			},
			expectNodes: sets.String{"node_1": sets.Empty{}, "node_2": sets.Empty{}},
		},
		{
			name: "match node_2, node_3",
			expectWS: &appsv1alpha1.WorkloadSpread{
				Spec: appsv1alpha1.WorkloadSpreadSpec{
					Subsets: []appsv1alpha1.WorkloadSpreadSubset{
						{
							Name: "subset-a",
							RequiredNodeSelectorTerm: &v1.NodeSelectorTerm{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "foo",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"bar"},
									},
								},
							},
							Tolerations: []v1.Toleration{
								{
									Key:      "node.kubernetes.io/not-ready",
									Operator: v1.TolerationOpExists,
									Effect:   v1.TaintEffectNoExecute,
								},
							},
							Patch: runtime.RawExtension{
								Raw: []byte(`{"metadata":{"labels":{"subset":"subset-a"},"annotations":{"subset":"subset-a"}}}`),
							},
							MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
						},
					},
				},
			},
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: v1.NodeSelectorOpIn,
												Values:   []string{"bar"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			getNodes: func() []*v1.Node {
				node1 := newNode("node_1", map[string]string{
					"foo": "not_bar",
				})
				node2 := newNode("node_2", map[string]string{
					"foo": "bar",
				})
				node3 := newNode("node_3", map[string]string{
					"foo": "bar",
				})
				return []*v1.Node{node1, node2, node3}
			},
			expectNodes: sets.String{"node_2": sets.Empty{}, "node_3": sets.Empty{}},
		},
		{
			name: "no match node",
			expectWS: &appsv1alpha1.WorkloadSpread{
				Spec: appsv1alpha1.WorkloadSpreadSpec{
					Subsets: []appsv1alpha1.WorkloadSpreadSubset{
						{
							Name: "subset-a",
							RequiredNodeSelectorTerm: &v1.NodeSelectorTerm{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "foo",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"bar"},
									},
								},
							},
							Tolerations: []v1.Toleration{
								{
									Key:      "node.kubernetes.io/not-ready",
									Operator: v1.TolerationOpExists,
									Effect:   v1.TaintEffectNoExecute,
								},
							},
							Patch: runtime.RawExtension{
								Raw: []byte(`{"metadata":{"labels":{"subset":"subset-a"},"annotations":{"subset":"subset-a"}}}`),
							},
							MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
						},
					},
				},
			},
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "foo",
												Operator: v1.NodeSelectorOpIn,
												Values:   []string{"bar"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			getNodes: func() []*v1.Node {
				node1 := newNode("node_1", map[string]string{
					"foo": "not_bar",
				})
				node2 := newNode("node_2", map[string]string{
					"foo": "not_bar",
				})
				node3 := newNode("node_3", map[string]string{
					"foo": "not_bar",
				})
				return []*v1.Node{node1, node2, node3}
			},
			expectNodes: sets.String{},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(scheme)
			for _, node := range cs.getNodes() {
				err := fakeClient.Create(context.TODO(), node)
				if err != nil {
					t.Errorf("failed to create node")
				}
			}

			handler := Handler{Client: fakeClient}

			clone := cs.pod.DeepCopy()
			injectWorkloadSpreadIntoPod(cs.expectWS, clone, "subset-a", "")
			nodes, err := handler.getNodesForSubset(clone)
			if err != nil {
				t.Errorf("failed to get nodes")
			}

			for _, node := range nodes {
				if !cs.expectNodes.Has(node.Name) {
					t.Errorf("match node failed")
				}
			}
		})
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
			fits, err := Predicates(test.pod(), nodeInfo)
			if err != nil {
				t.Logf("unexpected error: %v", err)
			}
			if fits != test.fits {
				t.Errorf("expected: %v got %v", test.fits, fits)
			}
		})
	}
}
