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

package podprobemarker

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme

	demoPodProbeMarker = appsv1alpha1.PodProbeMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ppm-1",
		},
		Spec: appsv1alpha1.PodProbeMarkerSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Probes: []appsv1alpha1.PodContainerProbe{
				{
					Name:          "healthy",
					ContainerName: "main",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/healthy.sh"},
								},
							},
						},
					},
					PodConditionType: "game.kruise.io/healthy",
					MarkerPolicy: []appsv1alpha1.ProbeMarkerPolicy{
						{
							State: appsv1alpha1.ProbeSucceeded,
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "10",
							},
							Labels: map[string]string{
								"server-healthy": "true",
							},
						},
						{
							State: appsv1alpha1.ProbeFailed,
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
	}

	demoPodProbeMarkerForTcpCheck = appsv1alpha1.PodProbeMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ppm-1",
		},
		Spec: appsv1alpha1.PodProbeMarkerSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Probes: []appsv1alpha1.PodContainerProbe{
				{
					Name:          "tcpCheckHealthy",
					ContainerName: "main",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
								},
							},
						},
					},
					PodConditionType: "game.kruise.io/healthy",
					MarkerPolicy: []appsv1alpha1.ProbeMarkerPolicy{
						{
							State: appsv1alpha1.ProbeSucceeded,
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "10",
							},
							Labels: map[string]string{
								"server-healthy": "true",
							},
						},
						{
							State: appsv1alpha1.ProbeFailed,
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
	}

	demoPodProbeMarkerForHttpCheck = appsv1alpha1.PodProbeMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ppm-1",
		},
		Spec: appsv1alpha1.PodProbeMarkerSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Probes: []appsv1alpha1.PodContainerProbe{
				{
					Name:          "httpCheckHealthy",
					ContainerName: "main",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/index.html",
									Port:   intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
									Scheme: corev1.URISchemeHTTP,
								},
							},
						},
					},
					PodConditionType: "game.kruise.io/healthy",
					MarkerPolicy: []appsv1alpha1.ProbeMarkerPolicy{
						{
							State: appsv1alpha1.ProbeSucceeded,
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "10",
							},
							Labels: map[string]string{
								"server-healthy": "true",
							},
						},
						{
							State: appsv1alpha1.ProbeFailed,
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
	}

	demoNodePodProbe = appsv1alpha1.NodePodProbe{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1alpha1.NodePodProbeSpec{
			PodProbes: []appsv1alpha1.PodProbe{
				{
					Name: "pod-1",
					UID:  "pod-1-uid",
					Probes: []appsv1alpha1.ContainerProbe{
						{
							Name:          "ppm-1#healthy",
							ContainerName: "main",
							Probe: appsv1alpha1.ContainerProbeSpec{
								Probe: corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										Exec: &corev1.ExecAction{
											Command: []string{"/bin/sh", "-c", "/healthy.sh"},
										},
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

func TestSyncPodProbeMarker(t *testing.T) {
	cases := []struct {
		name                string
		req                 ctrl.Request
		getPods             func() []*corev1.Pod
		getPodProbeMarkers  func() []*appsv1alpha1.PodProbeMarker
		getNodePodProbes    func() []*appsv1alpha1.NodePodProbe
		expectNodePodProbes func() []*appsv1alpha1.NodePodProbe
	}{
		{
			name: "test1, merge NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarker.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarker.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
						Spec: appsv1alpha1.NodePodProbeSpec{
							PodProbes: []appsv1alpha1.PodProbe{
								{
									Name: "pod-1",
									UID:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "ppm-2#idle",
											ContainerName: "main",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/idle.sh"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
					{
						Name:          "ppm-1#healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/healthy.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},
		{
			name: "test2, no NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarker.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
							UID:  types.UID("pod-2-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarker.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
					},
				}
			},
		},
		{
			name: "test3, remove podProbe from NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarker.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
							UID:  types.UID("pod-2-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				demo := demoPodProbeMarker.DeepCopy()
				now := metav1.Now()
				demo.DeletionTimestamp = &now
				demo.Finalizers = []string{PodProbeMarkerFinalizer}
				ppms := []*appsv1alpha1.PodProbeMarker{
					demo,
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
					{
						Name:          "ppm-1#healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/healthy.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},
		{
			name: "test4, remove podProbe from NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarker.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
							UID:  types.UID("pod-2-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				demo := demoPodProbeMarker.DeepCopy()
				now := metav1.Now()
				demo.DeletionTimestamp = &now
				demo.Finalizers = []string{PodProbeMarkerFinalizer}
				ppms := []*appsv1alpha1.PodProbeMarker{
					demo,
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-1#healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/healthy.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
					},
				}
			},
		},
		{
			name: "test5, merge NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarker.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarker.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
						Spec: appsv1alpha1.NodePodProbeSpec{
							PodProbes: []appsv1alpha1.PodProbe{
								{
									Name: "pod-1",
									UID:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "ppm-2#idle",
											ContainerName: "log",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/idle.sh"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "log",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
					{
						Name:          "ppm-1#healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/healthy.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},
		{
			name: "test6, merge NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarker.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarker.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
						Spec: appsv1alpha1.NodePodProbeSpec{
							PodProbes: []appsv1alpha1.PodProbe{
								{
									Name: "pod-2",
									UID:  "pod-2-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "ppm-2#idle",
											ContainerName: "log",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/idle.sh"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes = []appsv1alpha1.PodProbe{
					{
						Name: "pod-2",
						UID:  "pod-2-uid",
						Probes: []appsv1alpha1.ContainerProbe{
							{
								Name:          "ppm-2#idle",
								ContainerName: "log",
								Probe: appsv1alpha1.ContainerProbeSpec{
									Probe: corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											Exec: &corev1.ExecAction{
												Command: []string{"/bin/sh", "-c", "/idle.sh"},
											},
										},
									},
								},
							},
						},
					},
					{
						Name: "pod-1",
						UID:  "pod-1-uid",
						Probes: []appsv1alpha1.ContainerProbe{
							{
								Name:          "ppm-1#healthy",
								ContainerName: "main",
								Probe: appsv1alpha1.ContainerProbeSpec{
									Probe: corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											Exec: &corev1.ExecAction{
												Command: []string{"/bin/sh", "-c", "/healthy.sh"},
											},
										},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},

		{
			name: "test7, NodePodProbes changed",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarker.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarker.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
						Spec: appsv1alpha1.NodePodProbeSpec{
							PodProbes: []appsv1alpha1.PodProbe{
								{
									Name: "pod-1",
									UID:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "ppm-2#idle",
											ContainerName: "log",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/idle.sh"},
														},
													},
												},
											},
										},
										{
											Name:          "ppm-1#healthy",
											ContainerName: "main",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/home/admin/healthy.sh"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "log",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
					{
						Name:          "ppm-1#healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/healthy.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},

		{
			name: "test8, merge NodePodProbes(failed to convert tcpSocketProbe check port)",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarkerForTcpCheck.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main-v2",
									Ports: []corev1.ContainerPort{
										{
											Name:          "main-port",
											ContainerPort: 9090,
										},
									},
								},
							},
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarkerForTcpCheck.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
						Spec: appsv1alpha1.NodePodProbeSpec{
							PodProbes: []appsv1alpha1.PodProbe{
								{
									Name: "pod-1",
									UID:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "ppm-2#idle",
											ContainerName: "main",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/idle.sh"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},

		{
			name: "test9, merge NodePodProbes(failed to convert httpGetProbe check port)",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarkerForHttpCheck.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "main-v2",
									Ports: []corev1.ContainerPort{
										{
											Name:          "main-port",
											ContainerPort: 9090,
										},
									},
								},
							},
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarkerForHttpCheck.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
						Spec: appsv1alpha1.NodePodProbeSpec{
							PodProbes: []appsv1alpha1.PodProbe{
								{
									Name: "pod-1",
									UID:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "ppm-2#idle",
											ContainerName: "main",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/idle.sh"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},

		{
			name: "test10, update NodePodProbes(failed to convert tcpSocketProbe check port)",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarkerForTcpCheck.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarkerForTcpCheck.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
						Spec: appsv1alpha1.NodePodProbeSpec{
							PodProbes: []appsv1alpha1.PodProbe{
								{
									Name: "pod-1",
									UID:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "ppm-2#idle",
											ContainerName: "log",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/idle.sh"},
														},
													},
												},
											},
										},
										{
											Name:          "ppm-1#healthy",
											ContainerName: "main",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/home/admin/healthy.sh"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "log",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
					{
						Name:          "ppm-1#healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/home/admin/healthy.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},

		{
			name: "test11, update NodePorProbe(failed to convert httpGetProbe check port)",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: demoPodProbeMarkerForHttpCheck.Name,
				},
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodInitialized,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarkerForHttpCheck.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				return []*appsv1alpha1.NodePodProbe{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
						Spec: appsv1alpha1.NodePodProbeSpec{
							PodProbes: []appsv1alpha1.PodProbe{
								{
									Name: "pod-1",
									UID:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "ppm-2#idle",
											ContainerName: "log",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/idle.sh"},
														},
													},
												},
											},
										},
										{
											Name:          "ppm-1#healthy",
											ContainerName: "main",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Probe: corev1.Probe{
													ProbeHandler: corev1.ProbeHandler{
														Exec: &corev1.ExecAction{
															Command: []string{"/bin/sh", "-c", "/home/admin/healthy.sh"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "ppm-2#idle",
						ContainerName: "log",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/idle.sh"},
									},
								},
							},
						},
					},
					{
						Name:          "ppm-1#healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/home/admin/healthy.sh"},
									},
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			builder.WithStatusSubresource(&appsv1alpha1.PodProbeMarker{}, &appsv1alpha1.NodePodProbe{})
			for _, obj := range cs.getPods() {
				builder.WithObjects(obj)
			}
			for _, obj := range cs.getPodProbeMarkers() {
				builder.WithObjects(obj)
			}
			for _, obj := range cs.getNodePodProbes() {
				builder.WithObjects(obj)
			}
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
			}
			builder.WithObjects(node)
			fakeClient := builder.Build()

			controllerfinder.Finder = &controllerfinder.ControllerFinder{Client: fakeClient}
			recon := ReconcilePodProbeMarker{Client: fakeClient}
			_, err := recon.Reconcile(context.TODO(), cs.req)
			if err != nil {
				t.Fatalf("Reconcile failed: %s", err.Error())
			}
			if !checkNodePodProbeEqual(fakeClient, t, cs.expectNodePodProbes()) {
				t.Fatalf("Reconcile failed")
			}
		})
	}
}

func checkNodePodProbeEqual(c client.WithWatch, t *testing.T, expect []*appsv1alpha1.NodePodProbe) bool {
	for i := range expect {
		obj := expect[i]
		npp := &appsv1alpha1.NodePodProbe{}
		err := c.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, npp)
		if err != nil {
			t.Fatalf("get NodePodProbe failed: %s", err.Error())
			return false
		}
		if !reflect.DeepEqual(obj.Spec, npp.Spec) {
			t.Logf("expect: %v --> but: %v", util.DumpJSON(obj.Spec), util.DumpJSON(npp.Spec))
			return false
		}
	}
	return true
}

func TestConvertTcpSocketProbeCheckPort(t *testing.T) {
	cases := []struct {
		name        string
		probe       appsv1alpha1.PodContainerProbe
		pod         *corev1.Pod
		expectErr   error
		exportProbe appsv1alpha1.PodContainerProbe
	}{
		{
			name: "convert tcpProbe port",
			probe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Ports: []corev1.ContainerPort{
								{
									Name:          "main-port",
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
			exportProbe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.Int, IntVal: 80},
							},
						},
					},
				},
			},
		},
		{
			name: "no convert tcpProbe port(int type)",
			probe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.Int, IntVal: 80},
							},
						},
					},
				},
			},
			pod: &corev1.Pod{},
			exportProbe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.Int, IntVal: 80},
							},
						},
					},
				},
			},
		},
		{
			name: "convert tcpProbe port error(no found container)",
			probe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main-fake",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Ports: []corev1.ContainerPort{
								{
									Name:          "main-port",
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
			expectErr: fmt.Errorf("Failed to get container by name: main-fake in pod: sp1/pod1"),
			exportProbe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main-fake",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
							},
						},
					},
				},
			},
		},
		{
			name: "convert tcpProbe port error(failed to extract port)",
			probe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main-fake",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Ports: []corev1.ContainerPort{
								{
									Name:          "main-port",
									ContainerPort: -1,
								},
							},
						},
					},
				},
			},
			exportProbe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main-fake",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
							},
						},
					},
				},
			},
			expectErr: fmt.Errorf("Failed to get container by name: main-fake in pod: sp1/pod1"),
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			get, err := convertTcpSocketProbeCheckPort(cs.probe, cs.pod)
			if !reflect.DeepEqual(cs.expectErr, err) {
				t.Errorf("get probeProbe by container name failed, err: %v", err)
			}
			if !reflect.DeepEqual(get, cs.exportProbe) {
				t.Errorf("expect: %v, but: %v", util.DumpJSON(cs.exportProbe), util.DumpJSON(get))
			}
		})
	}
}

func TestTestConvertHttpGetProbeCheckPort(t *testing.T) {
	cases := []struct {
		name        string
		probe       appsv1alpha1.PodContainerProbe
		pod         *corev1.Pod
		expectErr   error
		exportProbe appsv1alpha1.PodContainerProbe
	}{
		{
			name: "convert httpProbe port",
			probe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
								Path:   "/index.html",
								Scheme: corev1.URISchemeHTTP,
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Ports: []corev1.ContainerPort{
								{
									Name:          "main-port",
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
			exportProbe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.IntOrString{Type: intstr.Int, IntVal: 80},
								Path:   "/index.html",
								Scheme: corev1.URISchemeHTTP,
							},
						},
					},
				},
			},
		},
		{
			name: "no convert httpProbe port(int type)",
			probe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.IntOrString{Type: intstr.Int, IntVal: 80},
								Path:   "/index.html",
								Scheme: corev1.URISchemeHTTP,
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Ports: []corev1.ContainerPort{
								{
									Name:          "main-port",
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
			exportProbe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.IntOrString{Type: intstr.Int, IntVal: 80},
								Path:   "/index.html",
								Scheme: corev1.URISchemeHTTP,
							},
						},
					},
				},
			},
		},

		{
			name: "convert httpProbe port error(no found container)",
			probe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main-fake",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
								Path:   "/index.html",
								Scheme: corev1.URISchemeHTTP,
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Ports: []corev1.ContainerPort{
								{
									Name:          "main-port",
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
			expectErr: fmt.Errorf("Failed to get container by name: main-fake in pod: sp1/pod1"),
			exportProbe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main-fake",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
								Path:   "/index.html",
								Scheme: corev1.URISchemeHTTP,
							},
						},
					},
				},
			},
		},
		{
			name: "convert httpProbe port error(failed to extract port)",
			probe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
								Path:   "/index.html",
								Scheme: corev1.URISchemeHTTP,
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Ports: []corev1.ContainerPort{
								{
									Name:          "main-port",
									ContainerPort: -1,
								},
							},
						},
					},
				},
			},
			expectErr: fmt.Errorf("Failed to extract port for container: main in pod: sp1/pod1"),
			exportProbe: appsv1alpha1.PodContainerProbe{
				Name:          "probe#main",
				ContainerName: "main",
				Probe: appsv1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
								Path:   "/index.html",
								Scheme: corev1.URISchemeHTTP,
							},
						},
					},
				},
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			get, err := convertHttpGetProbeCheckPort(cs.probe, cs.pod)
			if !reflect.DeepEqual(cs.expectErr, err) {
				t.Errorf("get probeProbe by container name failed, err: %v", err)
			}
			if !reflect.DeepEqual(get, cs.exportProbe) {
				t.Errorf("expect: %v, but: %v", util.DumpJSON(cs.exportProbe), util.DumpJSON(get))
			}
		})
	}
}

func TestUpdateNodePodProbes(t *testing.T) {

	cases := []struct {
		name         string
		ppm          *appsv1alpha1.PodProbeMarker
		nodePodProbe *appsv1alpha1.NodePodProbe
		nodeName     string
		pods         []*corev1.Pod
		getErr       error
	}{
		{
			name: "Failed to convert tcpSocket probe port",
			ppm: &appsv1alpha1.PodProbeMarker{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ppm-1",
				},
				Spec: appsv1alpha1.PodProbeMarkerSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
					Probes: []appsv1alpha1.PodContainerProbe{
						{
							Name:          "tcpCheckHealthy",
							ContainerName: "main",
							Probe: appsv1alpha1.ContainerProbeSpec{
								Probe: corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										TCPSocket: &corev1.TCPSocketAction{
											Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
										},
									},
								},
							},
						},
					},
				},
			},
			nodePodProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "ppm-1#healthy",
									ContainerName: "main",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node-1",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "sp2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod3",
						Namespace: "sp3",
					},
				},
			},
			getErr: nil,
		},
		{
			name: "Failed to convert httpGet probe port",
			ppm: &appsv1alpha1.PodProbeMarker{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ppm-1",
				},
				Spec: appsv1alpha1.PodProbeMarkerSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test",
						},
					},
					Probes: []appsv1alpha1.PodContainerProbe{
						{
							Name:          "httpCheckHealthy",
							ContainerName: "main",
							Probe: appsv1alpha1.ContainerProbeSpec{
								Probe: corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										HTTPGet: &corev1.HTTPGetAction{
											Path:   "/index.html",
											Port:   intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
											Scheme: corev1.URISchemeHTTP,
										},
									},
								},
							},
						},
					},
				},
			},
			nodePodProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name: "pod-1",
							UID:  "pod-1-uid",
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "ppm-1#healthy",
									ContainerName: "main",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												Exec: &corev1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nodeName: "node-1",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "sp2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod3",
						Namespace: "sp3",
					},
				},
			},
			getErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tc.nodePodProbe).Build()
			r := &ReconcilePodProbeMarker{Client: fakeClient}
			get := r.updateNodePodProbes(tc.ppm, tc.nodeName, tc.pods)
			if !reflect.DeepEqual(tc.getErr, get) {
				t.Errorf("expect: %v, but: %v", tc.getErr, get)
			}
		})
	}
}

func TestMarkerServerlessPod(t *testing.T) {
	cases := []struct {
		name                string
		getPod              func() *corev1.Pod
		markers             map[string][]appsv1alpha1.ProbeMarkerPolicy
		expectedLabels      map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name: "probe success",
			getPod: func() *corev1.Pod {
				obj := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-1",
						Annotations: map[string]string{
							"app": "web",
						},
						Labels: map[string]string{
							"app": "web",
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodInitialized,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodConditionType("game.io/idle"),
								Status: corev1.ConditionTrue,
							},
							{
								Type:   corev1.PodConditionType("game.io/healthy"),
								Status: corev1.ConditionFalse,
							},
						},
					},
				}

				return obj
			},
			markers: map[string][]appsv1alpha1.ProbeMarkerPolicy{
				"game.io/idle": {
					{
						State: appsv1alpha1.ProbeSucceeded,
						Labels: map[string]string{
							"gameserver-idle": "true",
						},
						Annotations: map[string]string{
							"controller.kubernetes.io/pod-deletion-cost": "-10",
						},
					},
					{
						State: appsv1alpha1.ProbeFailed,
						Labels: map[string]string{
							"gameserver-idle": "false",
						},
						Annotations: map[string]string{
							"controller.kubernetes.io/pod-deletion-cost": "10",
						},
					},
				},
				"game.io/healthy": {
					{
						State: appsv1alpha1.ProbeSucceeded,
						Labels: map[string]string{
							"gameserver-healthy": "true",
						},
						Annotations: map[string]string{
							"controller.kubernetes.io/ingress": "true",
						},
					},
					{
						State: appsv1alpha1.ProbeFailed,
						Labels: map[string]string{
							"gameserver-healthy": "false",
						},
						Annotations: map[string]string{
							"controller.kubernetes.io/ingress": "false",
						},
					},
				},
			},
			expectedLabels: map[string]string{
				"gameserver-healthy": "false",
				"gameserver-idle":    "true",
				"app":                "web",
			},
			expectedAnnotations: map[string]string{
				"controller.kubernetes.io/ingress":           "false",
				"controller.kubernetes.io/pod-deletion-cost": "-10",
				"app": "web",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pod := tc.getPod()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
			r := &ReconcilePodProbeMarker{Client: fakeClient}
			err := r.markServerlessPod(tc.getPod(), tc.markers)
			if err != nil {
				t.Fatalf(err.Error())
			}
			newPod := &corev1.Pod{}
			err = fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(pod), newPod)
			if err != nil {
				t.Fatalf(err.Error())
			}
			if !reflect.DeepEqual(tc.expectedLabels, newPod.Labels) {
				t.Errorf("expect: %v, but: %v", tc.expectedLabels, newPod.Labels)
			}
			if !reflect.DeepEqual(tc.expectedAnnotations, newPod.Annotations) {
				t.Errorf("expect: %v, but: %v", tc.expectedAnnotations, newPod.Annotations)
			}
		})
	}

}
