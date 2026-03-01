/*
Copyright 2022 The Kruise Authors.

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

package mutating

import (
	"context"
	"reflect"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

const (
	RequiredPodStateNodeAffinityAZLabels  = "topology.kubernetes.io/zone"
	PreferredPodStateNodeAffinityAZLabels = "kubernetes.io/hostname"
)

var (
	// kruise
	KruiseKindSts = appsv1beta1.SchemeGroupVersion.WithKind("StatefulSet")

	ppsDemo = appsv1alpha1.PersistentPodState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kruise-sts",
			Namespace: "ns-test",
		},
		Spec: appsv1alpha1.PersistentPodStateSpec{
			TargetReference: appsv1alpha1.TargetReference{
				APIVersion: KruiseKindSts.GroupVersion().String(),
				Kind:       KruiseKindSts.Kind,
				Name:       "test-kruise-sts",
			},
			RequiredPersistentTopology: &appsv1alpha1.NodeTopologyTerm{
				NodeTopologyKeys: []string{RequiredPodStateNodeAffinityAZLabels},
			},
			PreferredPersistentTopology: []appsv1alpha1.PreferredTopologyTerm{
				{
					Weight: 10,
					Preference: appsv1alpha1.NodeTopologyTerm{
						NodeTopologyKeys: []string{PreferredPodStateNodeAffinityAZLabels},
					},
				},
			},
		},
		Status: appsv1alpha1.PersistentPodStateStatus{
			PodStates: map[string]appsv1alpha1.PodState{
				"test-pod": {
					NodeTopologyLabels: map[string]string{
						RequiredPodStateNodeAffinityAZLabels:  "cn-beijing-a",
						PreferredPodStateNodeAffinityAZLabels: "kube-resource011162007216",
					},
				},
			},
		},
	}

	podDemo = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns-test",
			Name:      "test-pod",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: KruiseKindSts.GroupVersion().String(),
					Kind:       KruiseKindSts.Kind,
					Name:       "test-kruise-sts",
					Controller: pointer.BoolPtr(true),
				},
			},
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
			},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node/gpu",
										Operator: corev1.NodeSelectorOpExists,
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
										Key:      "test-key",
										Operator: corev1.NodeSelectorOpExists,
									},
								},
							},
						},
					},
				},
			},
		},
	}
)

func TestPersistentPodStateMutatingPod(t *testing.T) {
	cases := []struct {
		name        string
		getPod      func() *corev1.Pod
		getPodState func() *appsv1alpha1.PersistentPodState
		exceptPod   func() *corev1.Pod
	}{
		{
			name: "matched PersistentPodState, and required, preferred, labels, annotations",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getPodState: func() *appsv1alpha1.PersistentPodState {
				return ppsDemo.DeepCopy()
			},
			exceptPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations[InjectedPersistentPodStateKey] = ppsDemo.Name
				demo.Spec.NodeSelector = map[string]string{
					RequiredPodStateNodeAffinityAZLabels: "cn-beijing-a",
				}
				demo.Spec.Affinity = &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "node/gpu",
											Operator: corev1.NodeSelectorOpExists,
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
											Key:      "test-key",
											Operator: corev1.NodeSelectorOpExists,
										},
									},
								},
							},
							{
								Weight: 10,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      PreferredPodStateNodeAffinityAZLabels,
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"kube-resource011162007216"},
										},
									},
								},
							},
						},
					},
				}

				return demo
			},
		},
		{
			name: "no matched PersistentPodState",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.OwnerReferences[0].Name = "no-found"
				return demo
			},
			getPodState: func() *appsv1alpha1.PersistentPodState {
				return ppsDemo.DeepCopy()
			},
			exceptPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.OwnerReferences[0].Name = "no-found"
				return demo
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			podIn := cs.getPod()
			decoder := admission.NewDecoder(scheme.Scheme)
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(cs.getPodState()).Build()
			podOut := podIn.DeepCopy()
			podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
			req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
			_, err := podHandler.persistentPodStateMutatingPod(context.Background(), req, podOut)
			if err != nil {
				t.Fatalf("inject sidecar into pod failed, err: %v", err)
			}
			if !reflect.DeepEqual(cs.exceptPod(), podOut) {
				t.Fatalf("pod DeepEqual failed")
			}
		})
	}
}
