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
package validating

import (
	"strconv"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var (
	scheme             *runtime.Scheme
	kruiseKindCloneSet = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
	handler            = &WorkloadSpreadCreateUpdateHandler{}
	maxReplicasDemo    = intstr.FromInt(50)
	patchResource      = []byte(`{"spec":{"containers":[{"name":"main","resources":{"limits":{"cpu":"2","memory":"8000Mi"}}}]}}`)
	workloadSpreadDemo = &appsv1alpha1.WorkloadSpread{
		ObjectMeta: metav1.ObjectMeta{Name: "workloadSpread", Namespace: metav1.NamespaceDefault},
		Spec: appsv1alpha1.WorkloadSpreadSpec{
			TargetReference: &appsv1alpha1.TargetReference{
				APIVersion: kruiseKindCloneSet.GroupVersion().String(),
				Kind:       kruiseKindCloneSet.Kind,
				Name:       "demo",
			},
			Subsets: []appsv1alpha1.WorkloadSpreadSubset{
				{
					Name:        "subset-a",
					MaxReplicas: &maxReplicasDemo,
					RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "topology.kubernetes.io/zone",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"zone-a"},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "ecs",
							Operator: corev1.TolerationOpExists,
						},
					},
					Patch: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
					},
				},
				{
					Name:        "subset-b",
					MaxReplicas: &maxReplicasDemo,
					RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "topology.kubernetes.io/zone",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"zone-b"},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "ecs",
							Operator: corev1.TolerationOpExists,
						},
					},
					Patch: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
					},
				},
				{
					Name: "subset-c",
					RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "topology.kubernetes.io/zone",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"zone-c"},
							},
						},
					},
				},
			},
		},
	}
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
}

func TestValidateWorkloadSpreadCreate(t *testing.T) {
	targetRef := appsv1alpha1.TargetReference{
		APIVersion: kruiseKindCloneSet.GroupVersion().String(),
		Kind:       kruiseKindCloneSet.Kind,
		Name:       "test",
	}
	replicas1 := intstr.FromInt(50)
	replicas2 := intstr.FromString("50%")
	successCases := []appsv1alpha1.WorkloadSpread{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-1", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.WorkloadSpreadSpec{
				TargetReference: &targetRef,
				Subsets: []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &replicas1,
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "topology.kubernetes.io/zone",
									Operator: corev1.NodeSelectorOpNotIn,
									Values:   []string{"zone-a"},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
						},
					},
					{
						Name:        "subset-b",
						MaxReplicas: nil,
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "topology.kubernetes.io/zone",
									Operator: corev1.NodeSelectorOpNotIn,
									Values:   []string{"zone-a"},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
						},
					},
				},
				ScheduleStrategy: appsv1alpha1.WorkloadSpreadScheduleStrategy{
					Type: appsv1alpha1.FixedWorkloadSpreadScheduleStrategyType,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-1", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.WorkloadSpreadSpec{
				TargetReference: &appsv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "demo",
				},
				Subsets: []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &replicas1,
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
						},
					},
					{
						Name:        "subset-b",
						MaxReplicas: nil,
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-1", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.WorkloadSpreadSpec{
				TargetReference: &appsv1alpha1.TargetReference{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "StatefulSet",
					Name:       "demo",
				},
				Subsets: []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &replicas1,
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
						},
					},
					{
						Name:        "subset-b",
						MaxReplicas: nil,
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-1", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.WorkloadSpreadSpec{
				TargetReference: &appsv1alpha1.TargetReference{
					APIVersion: "apps.kruise.io/v1beta1",
					Kind:       "StatefulSet",
					Name:       "demo",
				},
				Subsets: []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &replicas1,
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
						},
					},
					{
						Name:        "subset-b",
						MaxReplicas: nil,
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-2", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.WorkloadSpreadSpec{
				TargetReference: &appsv1alpha1.TargetReference{
					APIVersion: controllerKindDep.GroupVersion().String(),
					Kind:       controllerKindDep.Kind,
					Name:       "test",
				},
				Subsets: []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &replicas1,
						PreferredNodeSelectorTerms: []corev1.PreferredSchedulingTerm{
							{
								Weight: 20,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: corev1.NodeSelectorOpNotIn,
											Values:   []string{"zone-a"},
										},
									},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: patchResource,
						},
					},
					{
						Name:        "subset-b",
						MaxReplicas: nil,
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "topology.kubernetes.io/zone",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-b"},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: patchResource,
						},
					},
				},
				ScheduleStrategy: appsv1alpha1.WorkloadSpreadScheduleStrategy{
					Type: appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType,
					Adaptive: &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
						RescheduleCriticalSeconds: pointer.Int32Ptr(30),
						DisableSimulationSchedule: true,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-3", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.WorkloadSpreadSpec{
				TargetReference: &appsv1alpha1.TargetReference{
					APIVersion: controllerKindJob.GroupVersion().String(),
					Kind:       controllerKindJob.Kind,
					Name:       "test",
				},
				Subsets: []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &replicas2,
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "topology.kubernetes.io/zone",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-a"},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
						},
					},
					{
						Name:        "subset-b",
						MaxReplicas: &replicas2,
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "topology.kubernetes.io/zone",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-b"},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-4", Namespace: metav1.NamespaceDefault},
			Spec: appsv1alpha1.WorkloadSpreadSpec{
				TargetReference: &appsv1alpha1.TargetReference{
					APIVersion: controllerKindJob.GroupVersion().String(),
					Kind:       controllerKindJob.Kind,
					Name:       "test",
				},
				Subsets: []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &replicas2,
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "topology.kubernetes.io/zone",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-a"},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
						},
					},
					{
						Name:        "subset-b",
						MaxReplicas: &replicas2,
						RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "topology.kubernetes.io/zone",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-b"},
								},
							},
						},
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
						},
					},
					{
						Name:        "subset-c",
						MaxReplicas: nil,
						Tolerations: []corev1.Toleration{
							{
								Key:      "ecs",
								Operator: corev1.TolerationOpExists,
							},
						},
						Patch: runtime.RawExtension{
							Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-c"}}}`),
						},
					},
				},
				ScheduleStrategy: appsv1alpha1.WorkloadSpreadScheduleStrategy{
					Type: appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType,
					Adaptive: &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
						RescheduleCriticalSeconds: pointer.Int32Ptr(30),
						DisableSimulationSchedule: true,
					},
				},
			},
		},
	}
	for i, successCase := range successCases {
		t.Run("success case "+strconv.Itoa(i), func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workloadSpreadDemo).Build()
			handler.Client = fakeClient
			if errs := handler.validatingWorkloadSpreadFn(&successCase); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	errorCases := []struct {
		name              string
		getWorkloadSpread func() *appsv1alpha1.WorkloadSpread
		errorSuffix       string
	}{
		{
			name: "targetRef is nil",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference = nil
				return workloadSpread
			},
			errorSuffix: "spec.targetRef",
		},
		{
			name: "targetRef's APIVersion is nil",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference.APIVersion = ""
				return workloadSpread
			},
			errorSuffix: "spec.targetRef",
		},
		{
			name: "targetRef's Kind is nil",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference.APIVersion = ""
				return workloadSpread
			},
			errorSuffix: "spec.targetRef",
		},
		{
			name: "targetRef's Name is nil",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference.APIVersion = ""
				return workloadSpread
			},
			errorSuffix: "spec.targetRef",
		},
		{
			name: "subsets is nil",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets = nil
				return workloadSpread
			},
			errorSuffix: "spec.subsets",
		},
		{
			name: "statefulset group error",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference = &appsv1alpha1.TargetReference{
					APIVersion: "rollouts.kruise.io/v2",
					Kind:       "StatefulSet",
					Name:       "demo",
				}
				return workloadSpread
			},
			errorSuffix: "spec.targetRef",
		},
		{
			name: "statefulset subset is percentage",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference = &appsv1alpha1.TargetReference{
					APIVersion: "apps.kruise.io/v1beta1",
					Kind:       "StatefulSet",
					Name:       "demo",
				}
				workloadSpread.Spec.Subsets[0].MaxReplicas = &replicas2
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].maxReplicas",
		},

		// {
		//	name: "one subset",
		//	getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
		//		workloadSpread := workloadSpreadDemo.DeepCopy()
		//		workloadSpread.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{
		//			{
		//				Name:        "subset-a",
		//				MaxReplicas: nil,
		//			},
		//		}
		//		return workloadSpread
		//	},
		//	errorSuffix: "spec.subsets",
		// },
		{
			name: "subset[0]'name is empty",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].Name = ""
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].name",
		},
		{
			name: "subset[1]'name is empty",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[1].Name = ""
				return workloadSpread
			},
			errorSuffix: "spec.subsets[1].name",
		},
		{
			name: "subset[1]'name is conflict with subsets[0]",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].Name = workloadSpread.Spec.Subsets[1].Name
				return workloadSpread
			},
			errorSuffix: "spec.subsets[1].name",
		},
		// {
		//	name: "subset[0]'s requiredNodeSelectorTerm, preferredNodeSelectorTerms and tolerations are all empty",
		//	getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
		//		workloadSpread := workloadSpreadDemo.DeepCopy()
		//		workloadSpread.Spec.Subsets[0].RequiredNodeSelectorTerm = nil
		//		workloadSpread.Spec.Subsets[0].PreferredNodeSelectorTerms = nil
		//		workloadSpread.Spec.Subsets[0].Tolerations = nil
		//		return workloadSpread
		//	},
		//	errorSuffix: "spec.subsets[0].requiredNodeSelectorTerm",
		// },
		{
			name: "requiredNodeSelectorTerm are not valid",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].RequiredNodeSelectorTerm = &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "key",
							Operator: corev1.NodeSelectorOpExists,
							Values:   []string{"unexpected"},
						},
					},
				}
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].requiredNodeSelectorTerm.matchExpressions[0].values",
		},
		{
			name: "preferredSchedulingTerms' weight are not valid",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].PreferredNodeSelectorTerms = []corev1.PreferredSchedulingTerm{
					{
						Weight: 101,
						Preference: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "key",
									Operator: corev1.NodeSelectorOpExists,
									Values:   []string{"unexpected"},
								},
							},
						},
					},
				}
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].preferredSchedulingTerms[0].weight",
		},
		{
			name: "preferredSchedulingTerms are not valid",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.Subsets[0].PreferredNodeSelectorTerms = []corev1.PreferredSchedulingTerm{
					{
						Weight: 50,
						Preference: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "key",
									Operator: corev1.NodeSelectorOpExists,
									Values:   []string{"unexpected"},
								},
							},
						},
					},
				}
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].preferredSchedulingTerms[0].preference.matchExpressions[0].values",
		},
		{
			name: "subset-a's maxReplicas < 0",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				maxReplicas := intstr.FromInt(-100)
				workloadSpread.Spec.Subsets[0].MaxReplicas = &maxReplicas
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].maxReplicas",
		},
		{
			name: "subset-a's maxReplicas = 0.2%",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				maxReplicas := intstr.FromString("0.2%")
				workloadSpread.Spec.Subsets[0].MaxReplicas = &maxReplicas
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].maxReplicas",
		},
		{
			name: "subset-a's maxReplicas = 50.2%",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				maxReplicas := intstr.FromString("50.2%")
				workloadSpread.Spec.Subsets[0].MaxReplicas = &maxReplicas
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].maxReplicas",
		},
		{
			name: "subset-a's maxReplicas = 101%",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				maxReplicas := intstr.FromString("101%")
				workloadSpread.Spec.Subsets[0].MaxReplicas = &maxReplicas
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].maxReplicas",
		},
		{
			name: "subset-a's maxReplicas = 40%, subset-b's maxReplicas = 70%",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				maxReplicas1 := intstr.FromString("40%")
				maxReplicas2 := intstr.FromString("70%")
				workloadSpread.Spec.Subsets[0].MaxReplicas = &maxReplicas1
				workloadSpread.Spec.Subsets[1].MaxReplicas = &maxReplicas2
				return workloadSpread
			},
			errorSuffix: "spec.subsets[1].maxReplicas",
		},
		{
			name: "subset-a's maxReplicas = 40%, subset-b's maxReplicas = 20%,, subset-c's maxReplicas = 20%",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				maxReplicas1 := intstr.FromString("40%")
				maxReplicas2 := intstr.FromString("20%")
				workloadSpread.Spec.Subsets[0].MaxReplicas = &maxReplicas1
				workloadSpread.Spec.Subsets[1].MaxReplicas = &maxReplicas2
				workloadSpread.Spec.Subsets[2].MaxReplicas = &maxReplicas2
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].maxReplicas",
		},
		{
			name: "subset-a's maxReplicas = -1%",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				maxReplicas := intstr.FromString("-1%")
				workloadSpread.Spec.Subsets[0].MaxReplicas = &maxReplicas
				return workloadSpread
			},
			errorSuffix: "spec.subsets[0].maxReplicas",
		},
		{
			name: "scheduleStrategy's type is not valid",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.ScheduleStrategy.Type = "random"
				return workloadSpread
			},
			errorSuffix: "spec.scheduleStrategy.type",
		},
		{
			name: "rescheduleCriticalSeconds = -1",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.ScheduleStrategy = appsv1alpha1.WorkloadSpreadScheduleStrategy{
					Type: appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType,
					Adaptive: &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
						RescheduleCriticalSeconds: pointer.Int32Ptr(-1),
						DisableSimulationSchedule: true,
					},
				}
				return workloadSpread
			},
			errorSuffix: "spec.scheduleStrategy.adaptive.rescheduleCriticalSeconds",
		},
		{
			name: "rescheduleCriticalSeconds < 0",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.ScheduleStrategy = appsv1alpha1.WorkloadSpreadScheduleStrategy{
					Type: appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType,
					Adaptive: &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
						RescheduleCriticalSeconds: pointer.Int32Ptr(-20),
						DisableSimulationSchedule: true,
					},
				}
				return workloadSpread
			},
			errorSuffix: "spec.scheduleStrategy.adaptive.rescheduleCriticalSeconds",
		},
		{
			name: "scheduleStrategy's type is not matched",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.ScheduleStrategy = appsv1alpha1.WorkloadSpreadScheduleStrategy{
					Type: appsv1alpha1.FixedWorkloadSpreadScheduleStrategyType,
					Adaptive: &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
						RescheduleCriticalSeconds: pointer.Int32Ptr(20),
						DisableSimulationSchedule: true,
					},
				}
				return workloadSpread
			},
			errorSuffix: "spec.scheduleStrategy.type",
		},
		{
			name: "the last subset's maxReplicas is not nil when using adaptive",
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.ScheduleStrategy = appsv1alpha1.WorkloadSpreadScheduleStrategy{
					Type: appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType,
					Adaptive: &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
						RescheduleCriticalSeconds: pointer.Int32Ptr(20),
						DisableSimulationSchedule: true,
					},
				}
				workloadSpread.Spec.Subsets[2].MaxReplicas = &maxReplicasDemo
				return workloadSpread
			},
			errorSuffix: "spec.scheduleStrategy.adaptive",
		},
	}

	for _, errorCase := range errorCases {
		t.Run(errorCase.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workloadSpreadDemo).Build()
			handler.Client = fakeClient
			errs := handler.validatingWorkloadSpreadFn(errorCase.getWorkloadSpread())
			if len(errs) == 0 {
				t.Errorf("expected failure for %s", errorCase.name)
			}

			var exist bool
			for i := range errs {
				f := errs[i].Field
				if f == errorCase.errorSuffix {
					exist = true
					break
				}
			}
			if !exist {
				t.Errorf("%s: missing prefix for: %v", errorCase.name, errs)
			}
		})
	}
}

func TestValidateWorkloadSpreadTargetRefUpdate(t *testing.T) {
	oldWorkloadSpread := workloadSpreadDemo.DeepCopy()
	errorSuffix := "spec.targetRef"

	workloadSpread1 := workloadSpreadDemo.DeepCopy()
	workloadSpread1.Spec.TargetReference.APIVersion = "apps/v1"
	workloadSpread2 := workloadSpreadDemo.DeepCopy()
	workloadSpread2.Spec.TargetReference.Kind = "Deployment"
	workloadSpread3 := workloadSpreadDemo.DeepCopy()
	workloadSpread3.Spec.TargetReference.Name = "deploy-1"

	errorCases := []struct {
		name              string
		newWorkloadSpread *appsv1alpha1.WorkloadSpread
	}{
		{
			name:              "update group",
			newWorkloadSpread: workloadSpread1,
		},
		{
			name:              "update kind",
			newWorkloadSpread: workloadSpread2,
		},
		{
			name:              "update name",
			newWorkloadSpread: workloadSpread3,
		},
	}
	for _, cs := range errorCases {
		t.Run(cs.name, func(t *testing.T) {
			allErrors := validateWorkloadSpreadTargetRefUpdate(oldWorkloadSpread.Spec.TargetReference, cs.newWorkloadSpread.Spec.TargetReference, field.NewPath("spec"))
			if allErrors[0].Field != errorSuffix {
				t.Errorf("%s: missing prefix for: %v", errorSuffix, allErrors)
			}
		})
	}
}

func TestValidateWorkloadSpreadConflict(t *testing.T) {
	workloadSpread1 := workloadSpreadDemo.DeepCopy()
	workloadSpread1.Name = "ws-1"
	workloadSpread1.Spec.TargetReference.Name = "test1"

	workloadSpread2 := workloadSpreadDemo.DeepCopy()
	workloadSpread2.Name = "ws-2"
	workloadSpread2.Spec.TargetReference.Name = "test2"

	workloadSpread3 := workloadSpreadDemo.DeepCopy()
	workloadSpread3.Name = "ws-3"
	workloadSpread3.Spec.TargetReference.Name = "test3"

	successCases := []struct {
		name              string
		getOthers         func() []appsv1alpha1.WorkloadSpread
		getWorkloadSpread func() *appsv1alpha1.WorkloadSpread
	}{
		{
			name: "group is different",
			getOthers: func() []appsv1alpha1.WorkloadSpread {
				return []appsv1alpha1.WorkloadSpread{*workloadSpread1, *workloadSpread2, *workloadSpread3}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference.APIVersion = "apps/v1"
				return workloadSpread
			},
		},
		{
			name: "kind is different",
			getOthers: func() []appsv1alpha1.WorkloadSpread {
				return []appsv1alpha1.WorkloadSpread{*workloadSpread1, *workloadSpread2, *workloadSpread3}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference.Kind = "Deployment"
				return workloadSpread
			},
		},
		{
			name: "name is different",
			getOthers: func() []appsv1alpha1.WorkloadSpread {
				return []appsv1alpha1.WorkloadSpread{*workloadSpread1, *workloadSpread2, *workloadSpread3}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				workloadSpread := workloadSpreadDemo.DeepCopy()
				workloadSpread.Spec.TargetReference.Name = "test"
				return workloadSpread
			},
		},
	}

	for i, successCase := range successCases {
		t.Run("success case "+strconv.Itoa(i), func(t *testing.T) {
			if errs := validateWorkloadSpreadConflict(successCase.getWorkloadSpread(), successCase.getOthers(), field.NewPath("spec")); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	workloadSpread1.Spec.TargetReference.Name = "test1"
	workloadSpread2.Spec.TargetReference.Name = "test1"
	workloadSpread3.Spec.TargetReference.Name = "test2"

	others := []appsv1alpha1.WorkloadSpread{*workloadSpread1, *workloadSpread2, *workloadSpread3}
	errorSuffix := "spec.targetRef"

	for i := 0; i < 2; i++ {
		t.Run("", func(t *testing.T) {
			allErrors := validateWorkloadSpreadConflict(&others[i], others, field.NewPath("spec"))
			if allErrors[0].Field != errorSuffix {
				t.Errorf("%s: missing prefix for: %v", errorSuffix, allErrors)
			}
		})
	}
}

func Test_validateWorkloadSpreadSubsets(t *testing.T) {
	cloneset := &appsv1alpha1.CloneSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneSet",
			APIVersion: "apps.kruise.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cs",
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: ptr.To(int32(6)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "img:latest",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "vol-1--0",
									MountPath: "/logs",
									SubPath:   "logs",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vol-1--0",
					},
				},
			},
		},
	}

	sts := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sts",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(int32(6)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "nginx",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "img:latest",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "vol-1--0",
									MountPath: "/logs",
									SubPath:   "logs",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vol-1--0",
					},
				},
			},
		},
	}
	patchData := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{
				"some-key": "some-value",
			},
		},
	}
	patch, _ := json.Marshal(patchData)
	ws := &appsv1alpha1.WorkloadSpread{
		Spec: appsv1alpha1.WorkloadSpreadSpec{
			Subsets: []appsv1alpha1.WorkloadSpreadSubset{
				{
					Name: "test",
					Patch: runtime.RawExtension{
						Raw: patch,
					},
				},
			},
		},
	}

	badCloneSet := cloneset.DeepCopy()
	badCloneSet.Spec.VolumeClaimTemplates[0].Name = "bad-boy"
	badSts := sts.DeepCopy()
	badSts.Spec.VolumeClaimTemplates[0].Name = "bad-boy"

	testCases := []struct {
		name     string
		workload client.Object
		testFunc func(errList field.ErrorList)
	}{
		{
			name:     "good cloneset",
			workload: cloneset,
			testFunc: func(errList field.ErrorList) {
				if len(errList) != 0 {
					t.Fatalf("expected 0 error, got %d, errList = %+v", len(errList), errList)
				}
			},
		}, {
			name:     "bad cloneset",
			workload: badCloneSet,
			testFunc: func(errList field.ErrorList) {
				if len(errList) != 1 {
					t.Fatalf("expected 1 error, got %d, errList = %+v", len(errList), errList)
				}
			},
		}, {
			name:     "good sts",
			workload: sts,
			testFunc: func(errList field.ErrorList) {
				if len(errList) != 0 {
					t.Fatalf("expected 0 error, got %d, errList = %+v", len(errList), errList)
				}
			},
		}, {
			name:     "bad sts",
			workload: badSts,
			testFunc: func(errList field.ErrorList) {
				if len(errList) != 1 {
					t.Fatalf("expected 1 error, got %d, errList = %+v", len(errList), errList)
				}
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.testFunc(
				validateWorkloadSpreadSubsets(ws, ws.Spec.Subsets, tc.workload, field.NewPath("spec").Child("subsets")),
			)
		})
	}
}
