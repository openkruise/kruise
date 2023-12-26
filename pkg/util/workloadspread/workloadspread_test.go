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
	"strconv"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/uuid"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
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

	podDemo2 = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Namespace:   "default",
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "apps/v1",
					Kind:               "Deployment",
					Name:               "workload-xyz",
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
			VersionedSubsetStatuses: map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
				VersionIgnored: {
					{
						Name:            "subset-a",
						MissingReplicas: 5,
						CreatingPods:    map[string]metav1.Time{},
						DeletingPods:    map[string]metav1.Time{},
					},
				},
			},
		},
	}

	workloadSpreadDemo2 = &appsv1alpha1.WorkloadSpread{
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
				Name:       "workload",
			},
			Subsets: []appsv1alpha1.WorkloadSpreadSubset{
				{
					Name:        "subset-a",
					MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				},
				{
					Name:        "subset-b",
					MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				},
				{
					Name: "subset-c",
				},
			},
		},
		Status: appsv1alpha1.WorkloadSpreadStatus{
			SubsetStatuses: []appsv1alpha1.WorkloadSpreadSubsetStatus{
				{
					Name:            "subset-a",
					MissingReplicas: 2,
				},
				{
					Name:            "subset-b",
					MissingReplicas: 3,
				},
				{
					Name:            "subset-c",
					MissingReplicas: -1,
				},
			},
		},
	}

	template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "unit-test",
			Name:      "pod-demo",
			Labels: map[string]string{
				"app": "demo",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "busybox:1.32",
				},
			},
		},
	}

	//nativeStatefulSet = appsv1.StatefulSet{
	//	TypeMeta: metav1.TypeMeta{
	//		APIVersion: appsv1.SchemeGroupVersion.String(),
	//		Kind:       "StatefulSet",
	//	},
	//	ObjectMeta: metav1.ObjectMeta{
	//		Namespace:  "default",
	//		Name:       "native-statefulset-demo",
	//		Generation: 10,
	//		UID:        uuid.NewUUID(),
	//	},
	//	Spec: appsv1.StatefulSetSpec{
	//		Replicas: utilpointer.Int32(10),
	//		Selector: &metav1.LabelSelector{
	//			MatchLabels: map[string]string{
	//				"app": "demo",
	//			},
	//		},
	//		Template: template,
	//		UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
	//			Type: appsv1.RollingUpdateStatefulSetStrategyType,
	//			RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
	//				Partition: utilpointer.Int32(5),
	//			},
	//		},
	//	},
	//	Status: appsv1.StatefulSetStatus{
	//		ObservedGeneration: int64(10),
	//		Replicas:           9,
	//		ReadyReplicas:      8,
	//		UpdatedReplicas:    5,
	//		CurrentReplicas:    4,
	//		AvailableReplicas:  7,
	//		CurrentRevision:    "sts-version1",
	//		UpdateRevision:     "sts-version2",
	//	},
	//}

	advancedStatefulSet = appsv1beta1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1beta1.SchemeGroupVersion.String(),
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "default",
			Name:       "advanced-statefulset-demo",
			Generation: 10,
			UID:        uuid.NewUUID(),
		},
		Spec: appsv1beta1.StatefulSetSpec{
			Replicas: utilpointer.Int32(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "demo",
				},
			},
			Template: template,
			UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
					Partition:      utilpointer.Int32(5),
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
					UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{
						PriorityStrategy: &appspub.UpdatePriorityStrategy{
							OrderPriority: []appspub.UpdatePriorityOrderTerm{
								{
									OrderedKey: "order-key",
								},
							},
						},
					},
				},
			},
		},
		Status: appsv1beta1.StatefulSetStatus{
			ObservedGeneration: int64(10),
			Replicas:           9,
			ReadyReplicas:      8,
			UpdatedReplicas:    5,
			AvailableReplicas:  7,
			CurrentRevision:    "sts-version1",
			UpdateRevision:     "sts-version2",
		},
	}

	cloneset = appsv1alpha1.CloneSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1alpha1.SchemeGroupVersion.String(),
			Kind:       "CloneSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "default",
			Name:       "cloneset-demo",
			Generation: 10,
			UID:        uuid.NewUUID(),
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: utilpointer.Int32(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "demo",
				},
			},
			Template: template,
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
				Partition:      &intstr.IntOrString{Type: intstr.String, StrVal: "20%"},
				MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
				PriorityStrategy: &appspub.UpdatePriorityStrategy{
					OrderPriority: []appspub.UpdatePriorityOrderTerm{
						{
							OrderedKey: "order-key",
						},
					},
				},
			},
		},
		Status: appsv1alpha1.CloneSetStatus{
			ObservedGeneration:   int64(10),
			Replicas:             9,
			ReadyReplicas:        8,
			UpdatedReplicas:      5,
			UpdatedReadyReplicas: 4,
			AvailableReplicas:    7,
			CurrentRevision:      "sts-version1",
			UpdateRevision:       "sts-version2",
		},
	}

	deployment = appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "default",
			Name:       "deployment-demo",
			Generation: 10,
			UID:        uuid.NewUUID(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: utilpointer.Int32(10),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "demo",
				},
			},
			Template: template,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: int64(10),
			Replicas:           9,
			ReadyReplicas:      8,
			UpdatedReplicas:    5,
			AvailableReplicas:  7,
		},
	}
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

func TestWorkloadSpreadCreatePodWithoutFullName(t *testing.T) {
	handler := NewWorkloadSpreadHandler(nil)
	ws := workloadSpreadDemo.DeepCopy()
	ws.Status.SubsetStatuses[0].MissingReplicas = 0
	subset := appsv1alpha1.WorkloadSpreadSubset{
		Name:        "subset-b",
		MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
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
		MissingReplicas: 2,
		CreatingPods:    map[string]metav1.Time{},
		DeletingPods:    map[string]metav1.Time{},
	}
	ws.Status.SubsetStatuses = append(ws.Status.SubsetStatuses, status)
	ws.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{}
	ws.Status.VersionedSubsetStatuses[VersionIgnored] = ws.Status.SubsetStatuses
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
				demo.ResourceVersion = "1"
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				demo.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: demo.Status.SubsetStatuses,
				}
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
				workloadSpread.Status.VersionedSubsetStatuses = map[string][]appsv1alpha1.WorkloadSpreadSubsetStatus{
					VersionIgnored: workloadSpread.Status.SubsetStatuses,
				}
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
				_, err = handler.HandlePodCreation(podIn)
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
			statusby1, _ := json.Marshal(latestWS.Status.VersionedSubsetStatuses)
			statusby2, _ := json.Marshal(cs.expectWorkloadSpread().Status.VersionedSubsetStatuses)
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
			h := Handler{fake.NewClientBuilder().Build()}
			if h.isReferenceEqual(cs.getTargetRef(), cs.getOwnerRef(), "") != cs.expectEqual {
				t.Fatalf("isReferenceEqual failed")
			}
		})
	}
}

func TestIsReferenceEqual2(t *testing.T) {
	const mockedAPIVersion = "mock.kruise.io/v1"
	const mockedKindGameServer = "GameServer"
	const mockedKindGameServerSet = "GameServerSet"
	const mockedKindUnknownCRD = "unknownCRD"
	unStruct1, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(&advancedStatefulSet)
	unStruct2, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployment)
	cases := []struct {
		name          string
		TopologyBuild func() (appsv1alpha1.TargetReference, []client.Object)
		Expect        bool
	}{
		{
			name: "pod is owned by cloneset directly, target is cloneset",
			TopologyBuild: func() (appsv1alpha1.TargetReference, []client.Object) {
				father := cloneset.DeepCopy()
				son := podDemo.DeepCopy()
				son.SetOwnerReferences([]metav1.OwnerReference{
					*metav1.NewControllerRef(father, father.GetObjectKind().GroupVersionKind()),
				})
				ref := appsv1alpha1.TargetReference{
					APIVersion: father.APIVersion,
					Kind:       father.Kind,
					Name:       father.Name,
				}
				return ref, []client.Object{father, son}
			},
			Expect: true,
		},
		{
			name: "pod is owned by unstructured-1, unstructured-1 is owned by unstructured-2, target is unstructured-2",
			TopologyBuild: func() (appsv1alpha1.TargetReference, []client.Object) {
				grandfather := &unstructured.Unstructured{Object: unStruct1}
				grandfather.SetGroupVersionKind(schema.FromAPIVersionAndKind(mockedAPIVersion, mockedKindGameServerSet))
				father := &unstructured.Unstructured{Object: unStruct2}
				father.SetGroupVersionKind(schema.FromAPIVersionAndKind(mockedAPIVersion, mockedKindGameServer))
				father.SetOwnerReferences([]metav1.OwnerReference{
					*metav1.NewControllerRef(grandfather, grandfather.GetObjectKind().GroupVersionKind()),
				})
				son := podDemo.DeepCopy()
				son.SetOwnerReferences([]metav1.OwnerReference{
					*metav1.NewControllerRef(father, father.GetObjectKind().GroupVersionKind()),
				})
				ref := appsv1alpha1.TargetReference{
					APIVersion: grandfather.GetAPIVersion(),
					Kind:       grandfather.GetKind(),
					Name:       grandfather.GetName(),
				}
				return ref, []client.Object{grandfather, father, son}
			},
			Expect: true,
		},
		{
			name: "pod is owned by unstructured-1, unstructured-1 is not owned by unstructured-2, target is unstructured-2",
			TopologyBuild: func() (appsv1alpha1.TargetReference, []client.Object) {
				grandfather := &unstructured.Unstructured{Object: unStruct1}
				grandfather.SetGroupVersionKind(schema.FromAPIVersionAndKind(mockedAPIVersion, mockedKindGameServerSet))
				father := &unstructured.Unstructured{Object: unStruct2}
				father.SetGroupVersionKind(schema.FromAPIVersionAndKind(mockedAPIVersion, mockedKindGameServer))
				father.SetOwnerReferences([]metav1.OwnerReference{
					*metav1.NewControllerRef(grandfather, grandfather.GetObjectKind().GroupVersionKind()),
				})
				son := podDemo.DeepCopy()
				son.SetOwnerReferences([]metav1.OwnerReference{
					*metav1.NewControllerRef(cloneset.DeepCopy(), cloneset.GetObjectKind().GroupVersionKind()),
				})
				ref := appsv1alpha1.TargetReference{
					APIVersion: grandfather.GetAPIVersion(),
					Kind:       grandfather.GetKind(),
					Name:       grandfather.GetName(),
				}
				return ref, []client.Object{grandfather, father, son}
			},
			Expect: false,
		},
		{
			name: "pod is owned by unstructured-1, unstructured-1 is owned by unknown CRD, target is unstructured-2",
			TopologyBuild: func() (appsv1alpha1.TargetReference, []client.Object) {
				grandfather := &unstructured.Unstructured{Object: unStruct1}
				grandfather.SetGroupVersionKind(schema.FromAPIVersionAndKind(mockedAPIVersion, mockedKindGameServerSet))
				father := &unstructured.Unstructured{Object: unStruct2}
				father.SetGroupVersionKind(schema.FromAPIVersionAndKind(mockedKindUnknownCRD, mockedKindUnknownCRD))
				father.SetOwnerReferences([]metav1.OwnerReference{
					*metav1.NewControllerRef(grandfather, grandfather.GetObjectKind().GroupVersionKind()),
				})
				son := podDemo.DeepCopy()
				son.SetOwnerReferences([]metav1.OwnerReference{
					*metav1.NewControllerRef(father.DeepCopy(), father.GetObjectKind().GroupVersionKind()),
				})
				ref := appsv1alpha1.TargetReference{
					APIVersion: grandfather.GetAPIVersion(),
					Kind:       grandfather.GetKind(),
					Name:       grandfather.GetName(),
				}
				return ref, []client.Object{grandfather, father, son}
			},
			Expect: false,
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			kruiseConfig := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: webhookutil.GetNamespace(),
					Name:      "kruise-configuration",
				},
				Data: map[string]string{
					"WorkloadSpread_Watch_Custom_Workload_WhiteList": `
   {
      "workloads": [
        {
          "Group": "mock.kruise.io",
          "Version": "v1",
          "Kind": "GameServerSet",
          "replicasPath": "spec.replicas",
          "subResources": [
            {
              "Group": "mock.kruise.io",
              "Version": "v1",
              "Kind": "GameServer"
            }
          ]
        }
      ]
    }`,
				},
			}
			ref, objects := cs.TopologyBuild()
			pod := objects[len(objects)-1]
			cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithObjects(kruiseConfig).Build()
			handler := &Handler{Client: cli}
			workloadsInWhiteListInitialized = false
			initializeWorkloadsInWhiteList(cli)
			result := handler.isReferenceEqual(&ref, metav1.GetControllerOf(pod), pod.GetNamespace())
			if result != cs.Expect {
				t.Fatalf("got unexpected result")
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

func TestGetParentNameAndOrdinal(t *testing.T) {
	for i := 0; i < 500; i++ {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("sample-%d", i),
			},
		}
		_, id := getParentNameAndOrdinal(&pod)
		if id != i {
			t.Fatal("failed to parse pod name")
		}
	}
}

func TestInitializedSubsetStatuses(t *testing.T) {
	cases := []struct {
		name     string
		spread   func() *appsv1alpha1.WorkloadSpread
		workload func() client.Object
	}{
		{
			name: "workload not found",
			spread: func() *appsv1alpha1.WorkloadSpread {
				return workloadSpreadDemo2.DeepCopy()
			},
			workload: func() client.Object {
				return nil
			},
		},
		{
			name: "cloneSet with absolute number settings",
			spread: func() *appsv1alpha1.WorkloadSpread {
				return workloadSpreadDemo2.DeepCopy()
			},
			workload: func() client.Object {
				clone := cloneset.DeepCopy()
				clone.Name = "workload"
				clone.Spec.Replicas = utilpointer.Int32(5)
				return clone
			},
		},
		{
			name: "cloneSet with percentage settings",
			spread: func() *appsv1alpha1.WorkloadSpread {
				spread := workloadSpreadDemo2.DeepCopy()
				spread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "20%"}
				spread.Spec.Subsets[1].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "30%"}
				spread.Spec.Subsets[2].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "50%"}
				spread.Status.SubsetStatuses[0].MissingReplicas = 2
				spread.Status.SubsetStatuses[1].MissingReplicas = 3
				spread.Status.SubsetStatuses[2].MissingReplicas = 5
				return spread
			},
			workload: func() client.Object {
				clone := cloneset.DeepCopy()
				clone.Name = "workload"
				clone.Spec.Replicas = utilpointer.Int32(10)
				return clone
			},
		},
		{
			name: "deployment with percentage settings",
			spread: func() *appsv1alpha1.WorkloadSpread {
				spread := workloadSpreadDemo2.DeepCopy()
				spread.Spec.TargetReference.Kind = "Deployment"
				spread.Spec.TargetReference.APIVersion = "apps/v1"
				spread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "20%"}
				spread.Spec.Subsets[1].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "30%"}
				spread.Spec.Subsets[2].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "50%"}
				spread.Status.SubsetStatuses[0].MissingReplicas = 2
				spread.Status.SubsetStatuses[1].MissingReplicas = 3
				spread.Status.SubsetStatuses[2].MissingReplicas = 5
				return spread
			},
			workload: func() client.Object {
				clone := deployment.DeepCopy()
				clone.Name = "workload"
				clone.Spec.Replicas = utilpointer.Int32(10)
				return clone
			},
		},
		{
			name: "Other CRD with percentage settings",
			spread: func() *appsv1alpha1.WorkloadSpread {
				spread := workloadSpreadDemo2.DeepCopy()
				spread.Spec.TargetReference.Kind = "GameServerSet"
				spread.Spec.TargetReference.APIVersion = "mock.kruise.io/v1"
				spread.Spec.Subsets[0].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "20%"}
				spread.Spec.Subsets[1].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "30%"}
				spread.Spec.Subsets[2].MaxReplicas = &intstr.IntOrString{Type: intstr.String, StrVal: "50%"}
				spread.Status.SubsetStatuses[0].MissingReplicas = 2
				spread.Status.SubsetStatuses[1].MissingReplicas = 3
				spread.Status.SubsetStatuses[2].MissingReplicas = 5
				return spread
			},
			workload: func() client.Object {
				clone := cloneset.DeepCopy()
				clone.Name = "workload"
				clone.Kind = "GameServerSet"
				clone.APIVersion = "mock.kruise.io/v1"
				clone.Spec.Replicas = utilpointer.Int32(10)
				unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(clone)
				if err != nil {
					panic("err when convert to unstructured object")
				}
				return &unstructured.Unstructured{Object: unstructuredMap}
			},
		},
	}

	kruiseConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: webhookutil.GetNamespace(),
			Name:      "kruise-configuration",
		},
		Data: map[string]string{
			"WorkloadSpread_Watch_Custom_Workload_WhiteList": `
   {
      "workloads": [
        {
          "Group": "mock.kruise.io",
          "Version": "v1",
          "Kind": "GameServerSet",
          "replicasPath": "spec.replicas",
          "subResources": [
            {
              "Group": "mock.kruise.io",
              "Version": "v1",
              "Kind": "GameServer"
            }
          ]
        }
      ]
    }`,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kruiseConfig.DeepCopy())
			if cs.workload() != nil {
				builder = builder.WithObjects(cs.workload())
			}
			spread := cs.spread()
			handler := &Handler{builder.Build()}
			result, err := handler.initializedSubsetStatuses(spread)
			if err != nil {
				t.Fatal(err.Error())
			}
			if !reflect.DeepEqual(result, spread.Status.SubsetStatuses) {
				t.Fatalf("expect %v, but got %v", spread.Status.SubsetStatuses, result)
			}
		})
	}
}

func TestGetPodVersion(t *testing.T) {
	cases := []struct {
		name    string
		pod     func() *corev1.Pod
		version string
	}{
		{
			name: "short hash only",
			pod: func() *corev1.Pod {
				pod := podDemo2.DeepCopy()
				pod.Labels = map[string]string{
					appsv1.ControllerRevisionHashLabelKey: "5474f59575",
				}
				return pod
			},
			version: "5474f59575",
		},
		{
			name: "long hash only",
			pod: func() *corev1.Pod {
				pod := podDemo2.DeepCopy()
				pod.Labels = map[string]string{
					appsv1.ControllerRevisionHashLabelKey: "workload-xyz-5474f59575",
				}
				return pod
			},
			version: "5474f59575",
		},
		{
			name: "template hash only",
			pod: func() *corev1.Pod {
				pod := podDemo2.DeepCopy()
				pod.Labels = map[string]string{
					appsv1.DefaultDeploymentUniqueLabelKey: "5474f59575",
				}
				return pod
			},
			version: "5474f59575",
		},
		{
			name: "template hash and long hash",
			pod: func() *corev1.Pod {
				pod := podDemo2.DeepCopy()
				pod.Labels = map[string]string{
					appsv1.ControllerRevisionHashLabelKey:  "workload-xyz-5474f59575",
					appsv1.DefaultDeploymentUniqueLabelKey: "5474f59575",
				}
				return pod
			},
			version: "5474f59575",
		},
		{
			name: "ignored pod",
			pod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Labels = map[string]string{
					appsv1.ControllerRevisionHashLabelKey:  "workload-xyz-5474f59575",
					appsv1.DefaultDeploymentUniqueLabelKey: "version-1",
				}
				return pod
			},
			version: VersionIgnored,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			if version := GetPodVersion(cs.pod()); version != cs.version {
				t.Fatalf("expect %v, but got %v", cs.version, version)
			}
		})
	}
}

func TestGetWorkloadVersion(t *testing.T) {
	restored := EnabledWorkloadSetForVersionedStatus
	EnabledWorkloadSetForVersionedStatus = sets.NewString("deployment", "replicaset", "cloneset")
	defer func() {
		EnabledWorkloadSetForVersionedStatus = restored
	}()

	cases := []struct {
		name         string
		workload     func() client.Object
		subWorkloads func() []client.Object
		version      string
	}{
		{
			name: "replicaset",
			workload: func() client.Object {
				return &appsv1.ReplicaSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps/v1",
						Kind:       "ReplicaSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "workload",
						Labels: map[string]string{
							appsv1.DefaultDeploymentUniqueLabelKey: "5474f59575",
						},
					},
				}
			},
			subWorkloads: func() []client.Object {
				return []client.Object{}
			},
			version: "5474f59575",
		},
		{
			name: "cloneset with consistent generation",
			workload: func() client.Object {
				return &appsv1alpha1.CloneSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps.kruise.io/v1alpha1",
						Kind:       "CloneSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "workload",
						Generation: 5,
					},
					Status: appsv1alpha1.CloneSetStatus{
						ObservedGeneration: 5,
						UpdateRevision:     "workload-5474f59575",
					},
				}
			},
			subWorkloads: func() []client.Object {
				return []client.Object{}
			},
			version: "5474f59575",
		},
		{
			name: "cloneset with inconsistent generation",
			workload: func() client.Object {
				return &appsv1alpha1.CloneSet{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps.kruise.io/v1alpha1",
						Kind:       "CloneSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "workload",
						Generation: 6,
					},
					Status: appsv1alpha1.CloneSetStatus{
						ObservedGeneration: 5,
						UpdateRevision:     "workload-5474f59575",
					},
				}
			},
			subWorkloads: func() []client.Object {
				return []client.Object{}
			},
			version: "",
		},
		{
			name: "deployment with latest rs",
			workload: func() client.Object {
				latestVersion := template.DeepCopy()
				latestVersion.Labels["version"] = "v5"
				return &appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "workload",
						Namespace: "test",
						UID:       "workload-uid",
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test",
							},
						},
						Template: *latestVersion.DeepCopy(),
					},
				}
			},
			subWorkloads: func() []client.Object {
				var objects []client.Object
				for i := 1; i <= 5; i++ {
					version := template.DeepCopy()
					version.Labels["version"] = "v" + strconv.Itoa(i)
					objects = append(objects, &appsv1.ReplicaSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "workload" + strconv.Itoa(i),
							Namespace: "test",
							Labels: map[string]string{
								"app":                                  "test",
								appsv1.DefaultDeploymentUniqueLabelKey: "version-" + strconv.Itoa(i),
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: "apps/v1",
									Kind:       "Deployment",
									Name:       "workload",
									UID:        "workload-uid",
									Controller: utilpointer.Bool(true),
								},
							},
						},
						Spec: appsv1.ReplicaSetSpec{
							Template: *version.DeepCopy(),
						},
					})
				}
				return objects
			},
			version: "version-5",
		},
		{
			name: "deployment without latest rs",
			workload: func() client.Object {
				latestVersion := template.DeepCopy()
				latestVersion.Labels["version"] = "v5"
				return &appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "workload",
						Namespace: "test",
						UID:       "workload-uid",
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test",
							},
						},
						Template: *latestVersion.DeepCopy(),
					},
				}
			},
			subWorkloads: func() []client.Object {
				var objects []client.Object
				for i := 1; i <= 4; i++ {
					version := template.DeepCopy()
					version.Labels["version"] = "v" + strconv.Itoa(i)
					objects = append(objects, &appsv1.ReplicaSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "workload" + strconv.Itoa(i),
							Namespace: "test",
							Labels: map[string]string{
								"app":                                  "test",
								appsv1.DefaultDeploymentUniqueLabelKey: "version-" + strconv.Itoa(i),
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: "apps/v1",
									Kind:       "Deployment",
									Name:       "workload",
									UID:        "workload-uid",
									Controller: utilpointer.Bool(true),
								},
							},
						},
						Spec: appsv1.ReplicaSetSpec{
							Template: *version.DeepCopy(),
						},
					})
				}
				return objects
			},
			version: "",
		},
		{
			name: "un-support workload",
			workload: func() client.Object {
				return advancedStatefulSet.DeepCopy()
			},
			subWorkloads: func() []client.Object {
				return []client.Object{}
			},
			version: VersionIgnored,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, object := range cs.subWorkloads() {
				builder.WithObjects(object)
			}
			fc := builder.Build()
			if version, _ := GetWorkloadVersion(fc, cs.workload()); version != cs.version {
				t.Fatalf("expect %v, but got %v", cs.version, version)
			}
		})
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
