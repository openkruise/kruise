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
	"fmt"
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/tools/cache"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/securitycontext"
	labelsutil "k8s.io/kubernetes/pkg/util/labels"
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
			name: "shouldIgnoreNodeUpdate",
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
			if got := shouldIgnoreNodeUpdate(tt.args.oldNode, tt.args.curNode); got != tt.want {
				t.Errorf("shouldIgnoreNodeUpdate() = %v, want %v", got, tt.want)
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
			Name:      name,
			Labels:    label,
			Namespace: metav1.NamespaceNone,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("100"),
			},
		},
	}
}

func addNodes(nodeStore cache.Store, startIndex, numNodes int, label map[string]string) {
	for i := startIndex; i < startIndex+numNodes; i++ {
		nodeStore.Add(newNode(fmt.Sprintf("node-%d", i), label))
	}
}

func newPod(podName string, nodeName string, label map[string]string, ds *appsv1alpha1.DaemonSet) *corev1.Pod {
	// Add hash unique label to the pod
	newLabels := label
	var podSpec corev1.PodSpec
	// Copy pod spec from DaemonSet template, or use a default one if DaemonSet is nil
	if ds != nil {
		hash := kubecontroller.ComputeHash(&ds.Spec.Template, ds.Status.CollisionCount)
		newLabels = labelsutil.CloneAndAddLabel(label, apps.DefaultDaemonSetUniqueLabelKey, hash)
		podSpec = ds.Spec.Template.Spec
	} else {
		podSpec = corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Image:                  "foo/bar",
					TerminationMessagePath: corev1.TerminationMessagePathDefault,
					ImagePullPolicy:        corev1.PullIfNotPresent,
					SecurityContext:        securitycontext.ValidSecurityContextWithContainerDefaults(),
				},
			},
		}
	}
	// Add node name to the pod
	if len(nodeName) > 0 {
		podSpec.NodeName = nodeName
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: podName,
			Labels:       newLabels,
			Namespace:    metav1.NamespaceDefault,
		},
		Spec: podSpec,
	}
	pod.Name = names.SimpleNameGenerator.GenerateName(podName)
	if ds != nil {
		pod.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(ds, controllerKind)}
	}
	return pod
}

func TestCreatePodProgressively(t *testing.T) {
	cases := []struct {
		name   string
		ds     *appsv1alpha1.DaemonSet
		expect bool
	}{
		{
			name: "ds with no annotation",
			ds: &appsv1alpha1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ds-with-no-annotation"},
			},
			expect: false,
		},
		{
			name: "ds with annotation, does not contains progressive annotation",
			ds: &appsv1alpha1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ds-with-no-annotation",
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			expect: false,
		},
		{
			name: "ds with annotation, contains progressive annotation, value is not true",
			ds: &appsv1alpha1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ds-with-no-annotation",
					Annotations: map[string]string{
						"foo":                "bar",
						ProgressiveCreatePod: "false",
					},
				},
			},
			expect: false,
		},
		{
			name: "ds with annotation, contains progressive annotation, value is true",
			ds: &appsv1alpha1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ds-with-no-annotation",
					Annotations: map[string]string{
						"foo":                "bar",
						ProgressiveCreatePod: "true",
					},
				},
			},
			expect: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isDaemonSetCreationProgressively(tc.ds)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("isDaemonSetCreationProgressively()=%v, expect=%v", got, tc.expect)
			}
		})
	}
}

func TestGetMaxCreateNum(t *testing.T) {
	cases := []struct {
		name             string
		oldPodsNum       int
		newPodsNum       int
		total            int
		partition        int
		progressively    bool
		nodesNeedingPods []string
		expect           []string
	}{
		{
			name:             "no changes applied on daemonset, all pods are new version",
			oldPodsNum:       0,
			newPodsNum:       10,
			total:            10,
			partition:        0,
			nodesNeedingPods: []string{},
			expect:           []string{},
		},
		{
			name:             "create daemonset step1: create a daemonset, partition=10",
			oldPodsNum:       0,
			newPodsNum:       0,
			total:            10,
			partition:        10,
			progressively:    true,
			nodesNeedingPods: []string{"node-0", "node-1", "node-2", "node-3", "node-4", "node-5", "node-6", "node-7", "node-8", "node-9"},
			expect:           []string{},
		},
		{
			name:             "create daemonset step2: decrease the partition, partition=9",
			oldPodsNum:       0,
			newPodsNum:       0,
			total:            10,
			partition:        9,
			progressively:    true,
			nodesNeedingPods: []string{"node-0", "node-1", "node-2", "node-3", "node-4", "node-5", "node-6", "node-7", "node-8", "node-9"},
			expect:           []string{"node-0"},
		},
		{
			name:             "create daemonset step3: decrease the partition, partition=5",
			oldPodsNum:       0,
			newPodsNum:       1,
			total:            10,
			partition:        5,
			progressively:    true,
			nodesNeedingPods: []string{"node-1", "node-2", "node-3", "node-4", "node-5", "node-6", "node-7", "node-8", "node-9"},
			expect:           []string{"node-1", "node-2", "node-3", "node-4"},
		},
		{
			name:             "create daemonset step4: importing 2 new nodes(node-10, node-11), partition=5",
			oldPodsNum:       0,
			newPodsNum:       5,
			total:            12,
			partition:        5,
			progressively:    true,
			nodesNeedingPods: []string{"node-5", "node-6", "node-7", "node-8", "node-9", "node-10", "node-11"},
			expect:           []string{"node-10", "node-11"},
		},
		{
			name:             "create daemonset step5: offline 3 nodes(node-4, node-8, node-11), partition=5",
			oldPodsNum:       0,
			newPodsNum:       4,
			total:            9,
			partition:        5,
			progressively:    true,
			nodesNeedingPods: []string{"node-5", "node-6", "node-7", "node-9", "node-10"},
			expect:           []string{},
		},
		{
			name:             "create daemonset step6: decrease the partition, partition=0",
			oldPodsNum:       0,
			newPodsNum:       4,
			total:            9,
			partition:        0,
			progressively:    true,
			nodesNeedingPods: []string{"node-5", "node-6", "node-7", "node-9", "node-10"},
			expect:           []string{"node-10", "node-5", "node-6", "node-7", "node-9"},
		},
		{
			name:             "create daemonset step7: importing 3 nodes, partition=0",
			oldPodsNum:       0,
			newPodsNum:       9,
			total:            12,
			partition:        0,
			progressively:    true,
			nodesNeedingPods: []string{"node-12", "node-13", "node-14"},
			expect:           []string{"node-12", "node-13", "node-14"},
		},
		{
			name:             "update daemonset step1: update spec and set partition=10",
			oldPodsNum:       10,
			newPodsNum:       0,
			total:            10,
			partition:        10,
			progressively:    false,
			nodesNeedingPods: []string{},
			expect:           []string{},
		},
		{
			name:             "update daemonset step2: decrease partition and pod has not been deleted by rolling update method",
			oldPodsNum:       10,
			newPodsNum:       0,
			total:            10,
			partition:        9,
			nodesNeedingPods: []string{},
			expect:           []string{},
		},
		{
			name:             "update daemonset step3: decrease partition and pod has been deleted by rolling update method",
			oldPodsNum:       9,
			newPodsNum:       0,
			total:            10,
			partition:        9,
			nodesNeedingPods: []string{"node-1"},
			expect:           []string{"node-1"},
		},
		{
			name:             "update daemonset step4: keep decreasing partition and pod has not been deleted by rolling update method",
			oldPodsNum:       9,
			newPodsNum:       1,
			total:            10,
			partition:        5,
			progressively:    false,
			nodesNeedingPods: []string{},
			expect:           []string{},
		},
		{
			name:             "update daemonset step5: keep decreasing partition and pod has been deleted by rolling update method",
			oldPodsNum:       5,
			newPodsNum:       1,
			total:            10,
			partition:        5,
			progressively:    false,
			nodesNeedingPods: []string{"node-2", "node-3", "node-4", "node-5"},
			expect:           []string{"node-2", "node-3", "node-4", "node-5"},
		},
		{
			name:             "update daemonset step6: importing 2 new node",
			oldPodsNum:       5,
			newPodsNum:       5,
			total:            12,
			partition:        5,
			progressively:    false,
			nodesNeedingPods: []string{"node-11", "node-12"},
			expect:           []string{"node-11", "node-12"},
		},
		{
			name:             "update daemonset step7: keep decreasing partition and pod has not been deleted by rolling update method",
			oldPodsNum:       5,
			newPodsNum:       7,
			total:            12,
			partition:        0,
			progressively:    false,
			nodesNeedingPods: []string{},
			expect:           []string{},
		},
		{
			name:             "update daemonset step7: keep decreasing partition and pod has been deleted by rolling update method",
			oldPodsNum:       0,
			newPodsNum:       7,
			total:            12,
			partition:        0,
			progressively:    false,
			nodesNeedingPods: []string{"node-6", "node-7", "node-8", "node-9", "node-10"},
			expect:           []string{"node-10", "node-6", "node-7", "node-8", "node-9"},
		},
		{
			name:             "update daemonset selector step1: update node selector and desire number has changed",
			oldPodsNum:       10,
			newPodsNum:       0,
			total:            20,
			partition:        20,
			progressively:    false,
			nodesNeedingPods: []string{},
			expect:           []string{},
		},
		{
			name:             "update daemonset selector step2: decrease partition",
			oldPodsNum:       10,
			newPodsNum:       0,
			total:            20,
			partition:        15,
			progressively:    true,
			nodesNeedingPods: []string{"node-11", "node-12", "node-13", "node-14", "node-15", "node-16", "node-17", "node-18", "node-19", "node-20"},
			expect:           []string{"node-11", "node-12", "node-13", "node-14", "node-15"},
		},
		{
			name:             "update daemonset selector step3: decrease partition",
			oldPodsNum:       10,
			newPodsNum:       5,
			total:            20,
			partition:        10,
			progressively:    true,
			nodesNeedingPods: []string{"node-16", "node-17", "node-18", "node-19", "node-20"},
			expect:           []string{"node-16", "node-17", "node-18", "node-19", "node-20"},
		},
		{
			name:             "bad case 1: partition is set too large",
			oldPodsNum:       0,
			newPodsNum:       10,
			total:            10,
			partition:        100,
			progressively:    false,
			nodesNeedingPods: []string{},
			expect:           []string{},
		},
		{
			name:             "bad case 2: partition is set too large when create daemonset",
			oldPodsNum:       0,
			newPodsNum:       5,
			total:            10,
			partition:        100,
			progressively:    true,
			nodesNeedingPods: []string{"node-5", "node-6", "node-7", "node-8", "node-9", "node-10"},
			expect:           []string{},
		},
		{
			name:             "bad case 3: an old pod was deleted manually in rolling update",
			oldPodsNum:       4,
			newPodsNum:       5,
			total:            10,
			partition:        5,
			progressively:    false,
			nodesNeedingPods: []string{"node-6"},
			expect:           []string{"node-6"},
		},
		{
			name:             "bad case 4: an old pod was deleted manually in rolling update and new pod has created",
			oldPodsNum:       4,
			newPodsNum:       6,
			total:            10,
			partition:        5,
			progressively:    false,
			nodesNeedingPods: []string{},
			expect:           []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetNodesNeedingPods(tc.newPodsNum, tc.total, tc.partition, tc.progressively, tc.nodesNeedingPods)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("GetMaxCreateNum()=%v, expect=%v", got, tc.expect)
			}
		})
	}
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
