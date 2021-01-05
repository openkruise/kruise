/*
Copyright 2020 The Kruise Authors.

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
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_nodeInSameCondition(t *testing.T) {
	type args struct {
		old []corev1.NodeCondition
		cur []corev1.NodeCondition
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nodeInSameCondition",
			args: args{
				old: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
				},
				cur: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
				},
			},
			want: true,
		},
		{
			name: "nodeInSameCondition2",
			args: args{
				old: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
				},
				cur: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "test-2",
						Status: corev1.ConditionFalse,
					},
				},
			},
			want: true,
		},
		{
			name: "nodeNotInSameCondition",
			args: args{
				old: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "test-3",
						Status: corev1.ConditionTrue,
					},
				},
				cur: []corev1.NodeCondition{
					{
						Type:   "test-1",
						Status: corev1.ConditionTrue,
					},
					{
						Type:   "test-2",
						Status: corev1.ConditionFalse,
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeInSameCondition(tt.args.old, tt.args.cur); got != tt.want {
				t.Errorf("nodeInSameCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldIgnoreNodeUpdate(t *testing.T) {
	type args struct {
		oldNode corev1.Node
		curNode corev1.Node
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "ShouldIgnoreNodeUpdate",
			args: args{
				oldNode: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "node1",
						ResourceVersion: "1111",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{},
					},
				},
				curNode: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "node1",
						ResourceVersion: "1111",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{},
					},
				},
			},
			want: true,
		},
		{
			name: "ShouldNotIgnoreNodeUpdate",
			args: args{
				oldNode: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "node1",
						ResourceVersion: "1111",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   "test-1",
								Status: corev1.ConditionTrue,
							},
							{
								Type:   "test-3",
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				curNode: corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "node1",
						ResourceVersion: "1112",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   "test-1",
								Status: corev1.ConditionTrue,
							},
							{
								Type:   "test-2",
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldIgnoreNodeUpdate(tt.args.oldNode, tt.args.curNode); got != tt.want {
				t.Errorf("ShouldIgnoreNodeUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getBurstReplicas(t *testing.T) {
	type args struct {
		ds *appsv1alpha1.DaemonSet
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "getBurstReplicas",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					Spec: appsv1alpha1.DaemonSetSpec{
						BurstReplicas: &intstr.IntOrString{IntVal: 10},
					},
				},
			},
			want: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getBurstReplicas(tt.args.ds); got != tt.want {
				t.Errorf("getBurstReplicas() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPodRevision(t *testing.T) {
	type args struct {
		pod metav1.Object
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "GetPodRevision",
			args: args{
				pod: &corev1.Pod{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							apps.ControllerRevisionHashLabelKey: "111222333",
						},
					},
					Spec:   corev1.PodSpec{},
					Status: corev1.PodStatus{},
				},
			},
			want: "111222333",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPodRevision("", tt.args.pod); got != tt.want {
				t.Errorf("GetPodRevision() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newNode(name string, label map[string]string) *corev1.Node {
	return &corev1.Node{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: label,
		},
	}
}

func newStandardRollingUpdateStrategy(matchLabels map[string]string) appsv1alpha1.DaemonSetUpdateStrategy {
	one := intstr.FromInt(1)
	strategy := appsv1alpha1.DaemonSetUpdateStrategy{
		Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
			MaxUnavailable: &one,
			Selector:       nil,
			Type:           appsv1alpha1.StandardRollingUpdateType,
		},
	}
	if len(matchLabels) > 0 {
		strategy.RollingUpdate.Selector = &metav1.LabelSelector{MatchLabels: matchLabels}
	}
	return strategy
}

func TestNodeShouldUpdateBySelector(t *testing.T) {
	for _, tt := range []struct {
		Title    string
		Node     *corev1.Node
		Ds       *appsv1alpha1.DaemonSet
		Expected bool
	}{
		{
			"node with no label",
			newNode("node1", nil),
			newDaemonSet("ds1"),
			false,
		},
		{
			"node with label, not selected",
			newNode("node1", map[string]string{
				"key1": "value1",
			}),
			func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("ds1")
				ds.Spec.UpdateStrategy = newStandardRollingUpdateStrategy(map[string]string{
					"key1": "value2",
				})
				return ds
			}(),
			false,
		},
		{
			"node with label, selected",
			newNode("node1", map[string]string{
				"key1": "value1",
			}),
			func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("ds1")
				ds.Spec.UpdateStrategy = newStandardRollingUpdateStrategy(map[string]string{
					"key1": "value1",
				})
				return ds
			}(),
			true,
		},
	} {
		t.Logf("\t%s", tt.Title)
		should := NodeShouldUpdateBySelector(tt.Node, tt.Ds)
		if should != tt.Expected {
			t.Errorf("NodeShouldUpdateBySelector() = %v, want %v", should, tt.Expected)
		}
	}
}

func TestGetSchedulerNodeInfo(t *testing.T) {
	for _, tt := range []struct {
		Title                    string
		Node                     *corev1.Node
		SetExtraAllowedPodNumber int64
		ExpectedAllowedPodNumber int
	}{
		{
			"node with no pod resource, without set extra allowed pod number",
			func() *corev1.Node {
				n := newNode("node1", nil)
				return n
			}(),
			0,
			0,
		},
		{
			"node with no pod resource, with set extra allowed pod number",
			func() *corev1.Node {
				n := newNode("node1", nil)
				return n
			}(),
			9,
			0,
		},
		{
			"node with pod resource, without set extra allowed pod number",
			func() *corev1.Node {
				n := newNode("node1", nil)
				n.Status.Allocatable = corev1.ResourceList{
					corev1.ResourcePods: *resource.NewQuantity(110, resource.DecimalSI),
				}
				return n
			}(),
			0,
			110,
		},
		{
			"node with pod resource, with set extra allowed pod number",
			func() *corev1.Node {
				n := newNode("node1", nil)
				n.Status.Allocatable = corev1.ResourceList{
					corev1.ResourcePods: *resource.NewQuantity(110, resource.DecimalSI),
				}
				return n
			}(),
			9,
			119,
		},
	} {
		t.Logf("\t%s", tt.Title)
		extraAllowedPodNumber = tt.SetExtraAllowedPodNumber
		nodeInfo := newSchedulerNodeInfo(tt.Node)
		if nodeInfo.AllowedPodNumber() != tt.ExpectedAllowedPodNumber {
			t.Errorf("actual allowed pod number = %v, want %v", nodeInfo.AllowedPodNumber(), tt.ExpectedAllowedPodNumber)
		}
	}
}
