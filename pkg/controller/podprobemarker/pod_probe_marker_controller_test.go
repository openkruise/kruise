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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	scheme = runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)
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
			Probes: []appsv1alpha1.ContainerProbe{
				{
					Name:          "healthy",
					ContainerName: "main",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Handler: corev1.Handler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/healthy.sh"},
							},
						},
					},
					MarkerPolicy: []appsv1alpha1.ProbeMarkerPolicy{
						{
							State: appsv1alpha1.ProbeTrue,
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "10",
							},
							Labels: map[string]string{
								"server-healthy": "true",
							},
						},
						{
							State: appsv1alpha1.ProbeFalse,
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
					Uid:  "pod-1-uid",
					Probes: []appsv1alpha1.ContainerProbe{
						{
							Name:          "healthy",
							ContainerName: "main",
							Probe: appsv1alpha1.ContainerProbeSpec{
								Handler: corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/healthy.sh"},
									},
								},
							},
							PodProbeMarkerName: "ppm-1",
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
					Name: fmt.Sprintf("%s#%s", ReconPodProbeMarker, demoPodProbeMarker.Name),
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
									Uid:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "idle",
											ContainerName: "main",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Handler: corev1.Handler{
													Exec: &corev1.ExecAction{
														Command: []string{"/bin/sh", "-c", "/idle.sh"},
													},
												},
											},
											PodProbeMarkerName: "ppm-2",
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
						Name:          "idle",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/idle.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-2",
					},
					{
						Name:          "healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/healthy.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-1",
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},
		{
			name: "test2, no NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: fmt.Sprintf("%s#%s", ReconPodProbeMarker, demoPodProbeMarker.Name),
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
					Name: fmt.Sprintf("%s#%s", ReconPodProbeMarker, demoPodProbeMarker.Name),
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
						Name:          "idle",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/idle.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-2",
					},
					{
						Name:          "healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/healthy.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-1",
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
			expectNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Spec.PodProbes[0].Probes = []appsv1alpha1.ContainerProbe{
					{
						Name:          "idle",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/idle.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-2",
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},
		{
			name: "test4, remove podProbe from NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: fmt.Sprintf("%s#%s", ReconPodProbeMarker, demoPodProbeMarker.Name),
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
						Name:          "healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/healthy.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-1",
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
					Name: fmt.Sprintf("%s#%s", ReconPodProbeMarker, demoPodProbeMarker.Name),
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
									Uid:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "idle",
											ContainerName: "log",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Handler: corev1.Handler{
													Exec: &corev1.ExecAction{
														Command: []string{"/bin/sh", "-c", "/idle.sh"},
													},
												},
											},
											PodProbeMarkerName: "ppm-2",
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
						Name:          "idle",
						ContainerName: "log",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/idle.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-2",
					},
					{
						Name:          "healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/healthy.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-1",
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},
		{
			name: "test6, merge NodePodProbes",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: fmt.Sprintf("%s#%s", ReconPodProbeMarker, demoPodProbeMarker.Name),
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
									Uid:  "pod-2-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "idle",
											ContainerName: "log",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Handler: corev1.Handler{
													Exec: &corev1.ExecAction{
														Command: []string{"/bin/sh", "-c", "/idle.sh"},
													},
												},
											},
											PodProbeMarkerName: "ppm-2",
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
						Uid:  "pod-2-uid",
						Probes: []appsv1alpha1.ContainerProbe{
							{
								Name:          "idle",
								ContainerName: "log",
								Probe: appsv1alpha1.ContainerProbeSpec{
									Handler: corev1.Handler{
										Exec: &corev1.ExecAction{
											Command: []string{"/bin/sh", "-c", "/idle.sh"},
										},
									},
								},
								PodProbeMarkerName: "ppm-2",
							},
						},
					},
					{
						Name: "pod-1",
						Uid:  "pod-1-uid",
						Probes: []appsv1alpha1.ContainerProbe{
							{
								Name:          "healthy",
								ContainerName: "main",
								Probe: appsv1alpha1.ContainerProbeSpec{
									Handler: corev1.Handler{
										Exec: &corev1.ExecAction{
											Command: []string{"/bin/sh", "-c", "/healthy.sh"},
										},
									},
								},
								PodProbeMarkerName: "ppm-1",
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
					Name: fmt.Sprintf("%s#%s", ReconPodProbeMarker, demoPodProbeMarker.Name),
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
									Uid:  "pod-1-uid",
									Probes: []appsv1alpha1.ContainerProbe{
										{
											Name:          "idle",
											ContainerName: "log",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Handler: corev1.Handler{
													Exec: &corev1.ExecAction{
														Command: []string{"/bin/sh", "-c", "/idle.sh"},
													},
												},
											},
											PodProbeMarkerName: "ppm-2",
										},
										{
											Name:          "healthy",
											ContainerName: "main",
											Probe: appsv1alpha1.ContainerProbeSpec{
												Handler: corev1.Handler{
													Exec: &corev1.ExecAction{
														Command: []string{"/bin/sh", "-c", "/home/admin/healthy.sh"},
													},
												},
											},
											PodProbeMarkerName: "ppm-1",
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
						Name:          "idle",
						ContainerName: "log",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/idle.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-2",
					},
					{
						Name:          "healthy",
						ContainerName: "main",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Handler: corev1.Handler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/healthy.sh"},
								},
							},
						},
						PodProbeMarkerName: "ppm-1",
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			for _, obj := range cs.getPods() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create Pod failed: %s", err.Error())
				}
			}
			for _, obj := range cs.getPodProbeMarkers() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create PodProbeMarker failed: %s", err.Error())
				}
			}
			for _, obj := range cs.getNodePodProbes() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create NodePodProbes failed: %s", err.Error())
				}
			}

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
			return false
		}
	}
	return true
}
