/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file expect in compliance with the License.
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
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

var (
	scheme      *runtime.Scheme
	defaultTime = time.Now()

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod-1",
			Namespace:   "default",
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "apps.kruise.io/v1alpha1",
					Kind:               "CloneSet",
					Name:               "cloneset-test",
					Controller:         utilpointer.BoolPtr(true),
					UID:                types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
					BlockOwnerDeletion: utilpointer.BoolPtr(true),
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
			},
		},
	}

	workloadSpreadDemo = &appsv1alpha1.WorkloadSpread{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps.kruise.io/v1alpha1",
			Kind:       "WorkloadSpread",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ws",
			Namespace: "default",
		},
		Spec: appsv1alpha1.WorkloadSpreadSpec{
			TargetReference: &appsv1alpha1.TargetReference{
				APIVersion: "apps.kruise.io/v1alpha1",
				Kind:       "CloneSet",
				Name:       "cloneset-test",
			},
			Subsets: []appsv1alpha1.WorkloadSpreadSubset{
				{
					Name: "subset-a",
					RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "topology.kubernetes.io/zone",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"ack"},
							},
							{
								Key:      "sigma.ali/resource-pool",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"lark"},
							},
						},
						MatchFields: []corev1.NodeSelectorRequirement{
							{
								Key:      "metadata.name",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"i-8vbhlh6rte2w5d1k2sgn", "i-8vbhlh6rte2vnxlq8ztc"},
							},
						},
					},
					PreferredNodeSelectorTerms: []corev1.PreferredSchedulingTerm{
						{
							Weight: 5,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "topology.kubernetes.io/zone",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"cn-zhangjiakou-a"},
									},
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "node.kubernetes.io/not-ready",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoExecute,
						},
					},
					Patch: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"labels":{"subset":"subset-a"},"annotations":{"subset":"subset-a"}}}`),
					},
					MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
				},
			},
		},
		Status: appsv1alpha1.WorkloadSpreadStatus{
			ObservedGeneration: 10,
			SubsetStatuses: []appsv1alpha1.WorkloadSpreadSubsetStatus{
				{
					Name:            "subset-a",
					MissingReplicas: 5,
					CreatingPods:    map[string]metav1.Time{},
					DeletingPods:    map[string]metav1.Time{},
				},
			},
		},
	}
)

func init() {
	scheme = runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
}

func TestWorkloadSpreadCreatePodWithoutFullName(t *testing.T) {
	handler := NewWorkloadSpreadHandler(nil)
	ws := workloadSpreadDemo.DeepCopy()
	ws.Status.SubsetStatuses[0].MissingReplicas = 0
	subset := appsv1alpha1.WorkloadSpreadSubset{
		Name: "subset-b",
		RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "topology.kubernetes.io/zone",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"cn-zhangjiakou-b"},
				},
			},
		},
	}
	ws.Spec.Subsets = append(ws.Spec.Subsets, subset)
	status := appsv1alpha1.WorkloadSpreadSubsetStatus{
		Name:            "subset-b",
		MissingReplicas: -1,
		CreatingPods:    map[string]metav1.Time{},
		DeletingPods:    map[string]metav1.Time{},
	}
	ws.Status.SubsetStatuses = append(ws.Status.SubsetStatuses, status)
	pod := podDemo.DeepCopy()
	pod.Name = ""
	_, suitableSubset, generatedUID, _ := handler.updateSubsetForPod(ws, pod, nil, CreateOperation)
	if generatedUID == "" {
		t.Fatalf("generate id failed")
	}
	if _, exist := suitableSubset.CreatingPods[pod.Name]; exist {
		t.Fatalf("inject map failed")
	}
}

func TestWorkloadSpreadMutatingPod(t *testing.T) {
	cases := []struct {
		name                 string
		getPod               func() *corev1.Pod
		getWorkloadSpread    func() *appsv1alpha1.WorkloadSpread
		getOperation         func() Operation
		expectPod            func() *corev1.Pod
		expectWorkloadSpread func() *appsv1alpha1.WorkloadSpread
	}{
		{
			name: "operation = create, matched workloadSpread, MissingReplicas = 5",
			getPod: func() *corev1.Pod {
				return podDemo.DeepCopy()
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				return workloadSpreadDemo.DeepCopy()
			},
			getOperation: func() Operation {
				return CreateOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels = map[string]string{
					"subset": "subset-a",
				}
				pod.Annotations = map[string]string{
					"subset":                               "subset-a",
					MatchedWorkloadSpreadSubsetAnnotations: `{"name":"test-ws","subset":"subset-a"}`,
				}
				pod.Spec.Tolerations = []corev1.Toleration{
					{
						Key:      "node.kubernetes.io/not-ready",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoExecute,
					},
				}
				pod.Spec.Affinity = &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"ack"},
										},
										{
											Key:      "sigma.ali/resource-pool",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"lark"},
										},
									},
									MatchFields: []corev1.NodeSelectorRequirement{
										{
											Key:      "metadata.name",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"i-8vbhlh6rte2w5d1k2sgn", "i-8vbhlh6rte2vnxlq8ztc"},
										},
									},
								},
							},
						},
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
							{
								Weight: 5,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"cn-zhangjiakou-a"},
										},
									},
								},
							},
						},
					},
				}

				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.ResourceVersion = "1"
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 4
				workloadSpread.Status.SubsetStatuses[0].CreatingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return workloadSpread
			},
		},
		{
			name: "operation = create, matched workloadSpread, subset-a MissingReplicas = 0, subset-b MissingReplicas = -1",
			getPod: func() *corev1.Pod {
				return podDemo.DeepCopy()
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 0
				subset := appsv1alpha1.WorkloadSpreadSubset{
					Name: "subset-b",
					RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "topology.kubernetes.io/zone",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"cn-zhangjiakou-b"},
							},
						},
					},
				}
				demo.Spec.Subsets = append(demo.Spec.Subsets, subset)
				status := appsv1alpha1.WorkloadSpreadSubsetStatus{
					Name:            "subset-b",
					MissingReplicas: -1,
					CreatingPods:    map[string]metav1.Time{},
					DeletingPods:    map[string]metav1.Time{},
				}
				demo.Status.SubsetStatuses = append(demo.Status.SubsetStatuses, status)
				return demo
			},
			getOperation: func() Operation {
				return CreateOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations = map[string]string{
					MatchedWorkloadSpreadSubsetAnnotations: `{"name":"test-ws","subset":"subset-b"}`,
				}
				pod.Spec.Affinity = &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"cn-zhangjiakou-b"},
										},
									},
								},
							},
						},
					},
				}

				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 0
				subset := appsv1alpha1.WorkloadSpreadSubset{
					Name: "subset-b",
					RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "topology.kubernetes.io/zone",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"cn-zhangjiakou-b"},
							},
						},
					},
				}
				demo.Spec.Subsets = append(demo.Spec.Subsets, subset)
				status := appsv1alpha1.WorkloadSpreadSubsetStatus{
					Name:            "subset-b",
					MissingReplicas: -1,
					CreatingPods:    map[string]metav1.Time{},
					DeletingPods:    map[string]metav1.Time{},
				}
				demo.Status.SubsetStatuses = append(demo.Status.SubsetStatuses, status)
				demo.ResourceVersion = "1"
				demo.Status.SubsetStatuses[1].CreatingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return demo
			},
		},
		{
			name: "operation = create, matched workloadSpread, MissingReplicas = 0",
			getPod: func() *corev1.Pod {
				return podDemo.DeepCopy()
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 0
				return demo
			},
			getOperation: func() Operation {
				return CreateOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 0
				return workloadSpread
			},
		},
		{
			name: "operation = create, not matched workloadSpread, MissingReplicas = 5",
			getPod: func() *corev1.Pod {
				return podDemo.DeepCopy()
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Spec.TargetReference.Name = "not found"
				return demo
			},
			getOperation: func() Operation {
				return CreateOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				return workloadSpread
			},
		},
		{
			name: "operation = create, matched workloadSpread, MissingReplicas = -1",
			getPod: func() *corev1.Pod {
				return podDemo.DeepCopy()
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = -1
				return demo
			},
			getOperation: func() Operation {
				return CreateOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels = map[string]string{
					"subset": "subset-a",
				}
				pod.Annotations = map[string]string{
					"subset":                               "subset-a",
					MatchedWorkloadSpreadSubsetAnnotations: `{"name":"test-ws","subset":"subset-a"}`,
				}
				pod.Spec.Tolerations = []corev1.Toleration{
					{
						Key:      "node.kubernetes.io/not-ready",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoExecute,
					},
				}
				pod.Spec.Affinity = &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"ack"},
										},
										{
											Key:      "sigma.ali/resource-pool",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"lark"},
										},
									},
									MatchFields: []corev1.NodeSelectorRequirement{
										{
											Key:      "metadata.name",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"i-8vbhlh6rte2w5d1k2sgn", "i-8vbhlh6rte2vnxlq8ztc"},
										},
									},
								},
							},
						},
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
							{
								Weight: 5,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"cn-zhangjiakou-a"},
										},
									},
								},
							},
						},
					},
				}

				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.ResourceVersion = "1"
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = -1
				workloadSpread.Status.SubsetStatuses[0].CreatingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return workloadSpread
			},
		},
		{
			name: "operation = create, matched workloadSpread, MissingReplicas = 4, creatingPods[test-pod-1]",
			getPod: func() *corev1.Pod {
				return podDemo.DeepCopy()
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 4
				demo.Status.SubsetStatuses[0].CreatingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return demo
			},
			getOperation: func() Operation {
				return CreateOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels = map[string]string{
					"subset": "subset-a",
				}
				pod.Annotations = map[string]string{
					"subset":                               "subset-a",
					MatchedWorkloadSpreadSubsetAnnotations: `{"name":"test-ws","subset":"subset-a"}`,
				}
				pod.Spec.Tolerations = []corev1.Toleration{
					{
						Key:      "node.kubernetes.io/not-ready",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoExecute,
					},
				}
				pod.Spec.Affinity = &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"ack"},
										},
										{
											Key:      "sigma.ali/resource-pool",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"lark"},
										},
									},
									MatchFields: []corev1.NodeSelectorRequirement{
										{
											Key:      "metadata.name",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"i-8vbhlh6rte2w5d1k2sgn", "i-8vbhlh6rte2vnxlq8ztc"},
										},
									},
								},
							},
						},
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
							{
								Weight: 5,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"cn-zhangjiakou-a"},
										},
									},
								},
							},
						},
					},
				}

				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.ResourceVersion = "1"
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 4
				workloadSpread.Status.SubsetStatuses[0].CreatingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return workloadSpread
			},
		},
		{
			name: "operation = delete, matched workloadSpread, MissingReplicas = 0",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return demo
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 0
				return demo
			},
			getOperation: func() Operation {
				return DeleteOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 1
				workloadSpread.Status.SubsetStatuses[0].DeletingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return workloadSpread
			},
		},
		{
			name: "operation = eviction, matched workloadSpread, MissingReplicas = 0",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return demo
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 0
				return demo
			},
			getOperation: func() Operation {
				return EvictionOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 1
				workloadSpread.Status.SubsetStatuses[0].DeletingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return workloadSpread
			},
		},
		{
			name: "operation = delete, not matched workloadSpread, MissingReplicas = 0",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				//demo.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return demo
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 0
				return demo
			},
			getOperation: func() Operation {
				return DeleteOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				//pod.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 0
				return workloadSpread
			},
		},
		{
			name: "operation = delete, not matched workloadSpread subset, MissingReplicas = 0",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"Name":"test-ws","Subset":"subset-b"}`
				return demo
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 0
				return demo
			},
			getOperation: func() Operation {
				return DeleteOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"Name":"test-ws","Subset":"subset-b"}`
				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 0
				return workloadSpread
			},
		},
		{
			name: "operation = delete, matched workloadSpread, MissingReplicas = 0, DeletingPods[test-pod-1]",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return demo
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = 1
				demo.Status.SubsetStatuses[0].DeletingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return demo
			},
			getOperation: func() Operation {
				return DeleteOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = 1
				workloadSpread.Status.SubsetStatuses[0].DeletingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return workloadSpread
			},
		},
		{
			name: "operation = delete, matched workloadSpread, MissingReplicas = -1",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return demo
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				demo := workloadSpreadDemo.DeepCopy()
				demo.Status.SubsetStatuses[0].MissingReplicas = -1
				return demo
			},
			getOperation: func() Operation {
				return DeleteOperation
			},
			expectPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = `{"name":"test-ws","subset":"subset-a"}`
				return pod
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Status.SubsetStatuses[0].MissingReplicas = -1
				workloadSpread.Status.SubsetStatuses[0].DeletingPods[podDemo.Name] = metav1.Time{Time: defaultTime}
				return workloadSpread
			},
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			podIn := cs.getPod()
			workloadSpreadIn := cs.getWorkloadSpread()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workloadSpreadIn).Build()
			handler := NewWorkloadSpreadHandler(fakeClient)

			var err error
			switch cs.getOperation() {
			case CreateOperation:
				err = handler.HandlePodCreation(podIn)
			case DeleteOperation:
				err = handler.HandlePodDeletion(podIn, DeleteOperation)
			case EvictionOperation:
				err = handler.HandlePodDeletion(podIn, EvictionOperation)
			}
			//err := handler.WorkloadSpreadMutatingPod(cs.getOperation(), podIn)
			if err != nil {
				t.Fatalf("WorkloadSpreadMutatingPod failed: %s", err.Error())
			}
			podInBy, _ := json.Marshal(podIn)
			expectPodBy, _ := json.Marshal(cs.expectPod())
			if !reflect.DeepEqual(podInBy, expectPodBy) {
				fmt.Println(podIn.Annotations)
				fmt.Println(cs.expectPod().Annotations)
				t.Fatalf("pod DeepEqual failed")
			}
			latestWS, err := getLatestWorkloadSpread(fakeClient, workloadSpreadIn)
			if err != nil {
				t.Fatalf("getLatestWorkloadSpread failed: %s", err.Error())
			}
			setWorkloadSpreadSubset(latestWS)
			statusby1, _ := json.Marshal(latestWS.Status)
			statusby2, _ := json.Marshal(cs.expectWorkloadSpread().Status)
			if !reflect.DeepEqual(statusby1, statusby2) {
				fmt.Println(latestWS.Status)
				fmt.Println(cs.expectWorkloadSpread().Status)
				t.Fatalf("workloadSpread DeepEqual failed")
			}
			util.GlobalCache.Delete(workloadSpreadIn)
		})
	}
}

func TestIsReferenceEqual(t *testing.T) {
	cases := []struct {
		name         string
		getTargetRef func() *appsv1alpha1.TargetReference
		getOwnerRef  func() *metav1.OwnerReference
		expectEqual  bool
	}{
		{
			name: "ApiVersion, Kind, Name equals",
			getTargetRef: func() *appsv1alpha1.TargetReference {
				return &appsv1alpha1.TargetReference{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "CloneSet",
					Name:       "test-1",
				}
			},
			getOwnerRef: func() *metav1.OwnerReference {
				return &metav1.OwnerReference{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "CloneSet",
					Name:       "test-1",
				}
			},
			expectEqual: true,
		},
		{
			name: "Group, Kind, Name equal, but Version not equal",
			getTargetRef: func() *appsv1alpha1.TargetReference {
				return &appsv1alpha1.TargetReference{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "CloneSet",
					Name:       "test-1",
				}
			},
			getOwnerRef: func() *metav1.OwnerReference {
				return &metav1.OwnerReference{
					APIVersion: "apps.kruise.io/v1beta1",
					Kind:       "CloneSet",
					Name:       "test-1",
				}
			},
			expectEqual: true,
		},
		{
			name: "Kind, Name equals, but ApiVersion not equal",
			getTargetRef: func() *appsv1alpha1.TargetReference {
				return &appsv1alpha1.TargetReference{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "CloneSet",
					Name:       "test-1",
				}
			},
			getOwnerRef: func() *metav1.OwnerReference {
				return &metav1.OwnerReference{
					APIVersion: "apps/v1",
					Kind:       "CloneSet",
					Name:       "test-1",
				}
			},
			expectEqual: false,
		},
		{
			name: "ApiVersion, Kind equals, but name not equal",
			getTargetRef: func() *appsv1alpha1.TargetReference {
				return &appsv1alpha1.TargetReference{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "CloneSet",
					Name:       "test-1",
				}
			},
			getOwnerRef: func() *metav1.OwnerReference {
				return &metav1.OwnerReference{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "CloneSet",
					Name:       "test-2",
				}
			},
			expectEqual: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			h := Handler{}
			if h.isReferenceEqual(cs.getTargetRef(), cs.getOwnerRef(), "") != cs.expectEqual {
				t.Fatalf("isReferenceEqual failed")
			}
		})
	}
}

func TestPatchMetadata(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
			},
		},
	}
	cloneBytes, _ := json.Marshal(pod)
	patch := runtime.RawExtension{
		Raw: []byte(`{"metadata":{"labels":{"subset":"subset-a"},"annotations":{"subset":"subset-a"}}}`),
	}
	modified, err := strategicpatch.StrategicMergePatch(cloneBytes, patch.Raw, &corev1.Pod{})
	if err != nil {
		t.Fatalf("failed to merge patch raw %s", patch.Raw)
	}
	if err = json.Unmarshal(modified, pod); err != nil {
		t.Fatalf("failed to unmarshal %s to Pod", modified)
	}

	if v := pod.GetAnnotations()["subset"]; v != "subset-a" {
		t.Fatal("failed to patch metadata to Pod")
	}
	if v := pod.GetLabels()["subset"]; v != "subset-a" {
		t.Fatal("failed to patch metadata to Pod")
	}
}

func TestPatchContainerResource(t *testing.T) {
	//spec:
	//  containers:
	//	- name: sidecar
	//    resources:
	//	    limits:
	//		  cpu: "2"
	//		  memory: 800Mi
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
				{
					Name:  "sidecar",
					Image: "nginx:1.15.1",
				},
			},
		},
	}
	cloneBytes, _ := json.Marshal(pod)
	patch := runtime.RawExtension{
		Raw: []byte(`{"spec":{"containers":[{"name":"sidecar","resources":{"limits":{"cpu":"2","memory":"8000Mi"}}}]}}`),
	}
	modified, err := strategicpatch.StrategicMergePatch(cloneBytes, patch.Raw, &corev1.Pod{})
	if err != nil {
		t.Fatalf("failed to merge patch raw %s", patch.Raw)
	}
	if err = json.Unmarshal(modified, pod); err != nil {
		t.Fatalf("failed to unmarshal %s to Pod", modified)
	}

	if pod.Spec.Containers[1].Name != "sidecar" && pod.Spec.Containers[0].Name != "nginx" {
		t.Fatal("failed to patch container resources to Pod")
	}

	cpuQuantity, _ := resource.ParseQuantity("2")
	memQuantity, _ := resource.ParseQuantity("8000Mi")
	if !pod.Spec.Containers[1].Resources.Limits.Cpu().Equal(cpuQuantity) ||
		!pod.Spec.Containers[1].Resources.Limits.Memory().Equal(memQuantity) {
		t.Fatal("failed to patch container resources to Pod")
	}
}

func TestFilterReference(t *testing.T) {
	csRef := &metav1.OwnerReference{
		APIVersion:         "apps.kruise.io/v1alpha1",
		Kind:               "CloneSet",
		Name:               "cloneset-test",
		Controller:         utilpointer.BoolPtr(true),
		UID:                types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
		BlockOwnerDeletion: utilpointer.BoolPtr(true),
	}

	rsRef := &metav1.OwnerReference{
		APIVersion:         "apps/v1",
		Kind:               "ReplicaSet",
		Name:               "rs-test",
		Controller:         utilpointer.BoolPtr(true),
		UID:                types.UID("a03eb001-27eb-4713-b634-7c46f6861758"),
		BlockOwnerDeletion: utilpointer.BoolPtr(true),
	}

	refs := []*metav1.OwnerReference{csRef, rsRef}
	for _, ref := range refs {
		if matched, _ := matchReference(ref); !matched {
			t.Fatalf("error")
		}
	}
}

func setWorkloadSpreadSubset(workloadSpread *appsv1alpha1.WorkloadSpread) {
	for i := range workloadSpread.Status.SubsetStatuses {
		subset := &workloadSpread.Status.SubsetStatuses[i]
		if subset.DeletingPods == nil {
			subset.DeletingPods = map[string]metav1.Time{}
		}
		if subset.CreatingPods == nil {
			subset.CreatingPods = map[string]metav1.Time{}
		}
		for k := range subset.CreatingPods {
			subset.CreatingPods[k] = metav1.Time{Time: defaultTime}
		}
		for k := range subset.DeletingPods {
			subset.DeletingPods[k] = metav1.Time{Time: defaultTime}
		}
	}
}

func getLatestWorkloadSpread(client client.Client, workloadSpread *appsv1alpha1.WorkloadSpread) (*appsv1alpha1.WorkloadSpread, error) {
	newWS := &appsv1alpha1.WorkloadSpread{}
	Key := types.NamespacedName{
		Name:      workloadSpread.Name,
		Namespace: workloadSpread.Namespace,
	}
	err := client.Get(context.TODO(), Key, newWS)
	return newWS, err
}
