/*
Copyright 2025 The Kruise Authors.

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

package podprobemarker

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsalphav1.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme
)

func TestGetPodProbeMarkerForPod(t *testing.T) {

	cases := []struct {
		name      string
		ppmList   *appsalphav1.PodProbeMarkerList
		pod       *corev1.Pod
		expect    []*appsalphav1.PodProbeMarker
		expectErr error
	}{
		{
			name: "get pod probe marker list with selector",
			ppmList: &appsalphav1.PodProbeMarkerList{
				Items: []appsalphav1.PodProbeMarker{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v1",
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v1",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v2",
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v2",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v3",
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v2",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Labels: map[string]string{
						"app": "test-v2",
					},
				},
			},
			expect: []*appsalphav1.PodProbeMarker{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PodProbeMarker",
						APIVersion: "apps.kruise.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "game-server-probe-v2",
						Namespace:       "sp1",
						ResourceVersion: "1",
					},
					Spec: appsalphav1.PodProbeMarkerSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test-v2",
							},
						},
						Probes: []appsalphav1.PodContainerProbe{
							{
								Name:          "healthy",
								ContainerName: "main",
								Probe: appsalphav1.ContainerProbeSpec{
									Probe: corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											Exec: &corev1.ExecAction{
												Command: []string{"/bin/sh", "-c", "/healthy.sh"},
											},
										},
									},
								},
								PodConditionType: "game.kruise.io/healthy",
								MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
									{
										State: appsalphav1.ProbeSucceeded,
										Annotations: map[string]string{
											"controller.kubernetes.io/pod-deletion-cost": "10",
										},
										Labels: map[string]string{
											"server-healthy": "true",
										},
									},
									{
										State: appsalphav1.ProbeFailed,
										Annotations: map[string]string{
											"controller.kubernetes.io/pod-deletion-cost": "-10",
										},
										Labels: map[string]string{
											"server-healthy": "false",
										},
									},
								},
							},
						},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PodProbeMarker",
						APIVersion: "apps.kruise.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "game-server-probe-v3",
						Namespace:       "sp1",
						ResourceVersion: "1",
					},
					Spec: appsalphav1.PodProbeMarkerSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "test-v2",
							},
						},
						Probes: []appsalphav1.PodContainerProbe{
							{
								Name:          "healthy",
								ContainerName: "main",
								Probe: appsalphav1.ContainerProbeSpec{
									Probe: corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											Exec: &corev1.ExecAction{
												Command: []string{"/bin/sh", "-c", "/healthy.sh"},
											},
										},
									},
								},
								PodConditionType: "game.kruise.io/healthy",
								MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
									{
										State: appsalphav1.ProbeSucceeded,
										Annotations: map[string]string{
											"controller.kubernetes.io/pod-deletion-cost": "10",
										},
										Labels: map[string]string{
											"server-healthy": "true",
										},
									},
									{
										State: appsalphav1.ProbeFailed,
										Annotations: map[string]string{
											"controller.kubernetes.io/pod-deletion-cost": "-10",
										},
										Labels: map[string]string{
											"server-healthy": "false",
										},
									},
								},
							},
						},
					},
				},
			},
			expectErr: nil,
		},
		{
			name: "no found pod probe marker list resources",
			ppmList: &appsalphav1.PodProbeMarkerList{
				Items: []appsalphav1.PodProbeMarker{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "game-server-probe-v1",
							Namespace:       "sp1",
							ResourceVersion: "01",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v1",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "game-server-probe-v2",
							Namespace:       "sp1",
							ResourceVersion: "01",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v2",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:            "game-server-probe-v3",
							Namespace:       "sp1",
							ResourceVersion: "01",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "test-v2",
								},
							},
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Labels: map[string]string{
						"app": "test",
					},
				},
			},
			expect:    nil,
			expectErr: nil,
		},
		{
			name: "get pod probe marker list with pod kruise.io/podprobemarker-list annotation",
			ppmList: &appsalphav1.PodProbeMarkerList{
				Items: []appsalphav1.PodProbeMarker{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v1",
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v2",
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "game-server-probe-v3",
							Namespace: "sp1",
						},
						Spec: appsalphav1.PodProbeMarkerSpec{
							Probes: []appsalphav1.PodContainerProbe{
								{
									Name:          "healthy",
									ContainerName: "main",
									Probe: appsalphav1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
									PodConditionType: "game.kruise.io/healthy",
									MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
										{
											State: appsalphav1.ProbeSucceeded,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "10",
											},
											Labels: map[string]string{
												"server-healthy": "true",
											},
										},
										{
											State: appsalphav1.ProbeFailed,
											Annotations: map[string]string{
												"controller.kubernetes.io/pod-deletion-cost": "-10",
											},
											Labels: map[string]string{
												"server-healthy": "false",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Labels: map[string]string{
						"app": "test-v2",
					},
					Annotations: map[string]string{
						appsalphav1.PodProbeMarkerListAnnotationKey: "game-server-probe-v1,game-server-probe-v2",
					},
				},
			},
			expect: []*appsalphav1.PodProbeMarker{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PodProbeMarker",
						APIVersion: "apps.kruise.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "game-server-probe-v1",
						Namespace:       "sp1",
						ResourceVersion: "1",
					},
					Spec: appsalphav1.PodProbeMarkerSpec{
						Probes: []appsalphav1.PodContainerProbe{
							{
								Name:          "healthy",
								ContainerName: "main",
								Probe: appsalphav1.ContainerProbeSpec{
									Probe: corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											Exec: &corev1.ExecAction{
												Command: []string{"/bin/sh", "-c", "/healthy.sh"},
											},
										},
									},
								},
								PodConditionType: "game.kruise.io/healthy",
								MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
									{
										State: appsalphav1.ProbeSucceeded,
										Annotations: map[string]string{
											"controller.kubernetes.io/pod-deletion-cost": "10",
										},
										Labels: map[string]string{
											"server-healthy": "true",
										},
									},
									{
										State: appsalphav1.ProbeFailed,
										Annotations: map[string]string{
											"controller.kubernetes.io/pod-deletion-cost": "-10",
										},
										Labels: map[string]string{
											"server-healthy": "false",
										},
									},
								},
							},
						},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PodProbeMarker",
						APIVersion: "apps.kruise.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "game-server-probe-v2",
						Namespace:       "sp1",
						ResourceVersion: "1",
					},
					Spec: appsalphav1.PodProbeMarkerSpec{
						Probes: []appsalphav1.PodContainerProbe{
							{
								Name:          "healthy",
								ContainerName: "main",
								Probe: appsalphav1.ContainerProbeSpec{
									Probe: corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											Exec: &corev1.ExecAction{
												Command: []string{"/bin/sh", "-c", "/healthy.sh"},
											},
										},
									},
								},
								PodConditionType: "game.kruise.io/healthy",
								MarkerPolicy: []appsalphav1.ProbeMarkerPolicy{
									{
										State: appsalphav1.ProbeSucceeded,
										Annotations: map[string]string{
											"controller.kubernetes.io/pod-deletion-cost": "10",
										},
										Labels: map[string]string{
											"server-healthy": "true",
										},
									},
									{
										State: appsalphav1.ProbeFailed,
										Annotations: map[string]string{
											"controller.kubernetes.io/pod-deletion-cost": "-10",
										},
										Labels: map[string]string{
											"server-healthy": "false",
										},
									},
								},
							},
						},
					},
				},
			},
			expectErr: nil,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			for _, ppm := range cs.ppmList.Items {
				fakeClient.Create(context.TODO(), &ppm)
			}
			get, err := GetPodProbeMarkerForPod(fakeClient, cs.pod)
			if !reflect.DeepEqual(cs.expectErr, err) {
				t.Errorf("expectErr: %v, but: %v", cs.expectErr, err)
			}
			for i := range get {
				get[i].TypeMeta.Kind = "PodProbeMarker"
				get[i].TypeMeta.APIVersion = "apps.kruise.io/v1alpha1"
			}
			if !reflect.DeepEqual(util.DumpJSON(cs.expect), util.DumpJSON(get)) {
				t.Errorf("expectGet: %v, but: %v", util.DumpJSON(cs.expect), util.DumpJSON(get))
			}
		})
	}
}
