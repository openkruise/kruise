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

package validating

import (
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme

	ppmDemo = appsv1alpha1.PodProbeMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ppm-test",
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
)

func TestValidatingPodProbeMarker(t *testing.T) {
	cases := []struct {
		name          string
		getPpm        func() *appsv1alpha1.PodProbeMarker
		expectErrList int
	}{
		{
			name: "test1, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Selector = nil
				return ppm
			},
			expectErrList: 1,
		},
		{
			name: "test2, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Probes = nil
				return ppm
			},
			expectErrList: 1,
		},
		{
			name: "test3, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Probes = append(ppm.Spec.Probes, appsv1alpha1.PodContainerProbe{
					Name:          "healthy",
					ContainerName: "other",
				})
				return ppm
			},
			expectErrList: 1,
		},
		{
			name: "test4, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Probes = append(ppm.Spec.Probes, appsv1alpha1.PodContainerProbe{
					Name:          "check",
					ContainerName: "other",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.FromInt(80),
								},
							},
						},
					},
					PodConditionType: "game.kruise.io/check",
				})
				return ppm
			},
			expectErrList: 1,
		},
		{
			name: "test5, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Probes = append(ppm.Spec.Probes, appsv1alpha1.PodContainerProbe{
					Name:          "check",
					ContainerName: "other",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{},
							},
						},
					},
					PodConditionType: "game.kruise.io/check",
				})
				return ppm
			},
			expectErrList: 1,
		},
		{
			name: "test6, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Probes[0].MarkerPolicy = []appsv1alpha1.ProbeMarkerPolicy{
					{
						State: appsv1alpha1.ProbeUnknown,
						Annotations: map[string]string{
							"controller.kubernetes.io/pod-deletion-cost": "10",
						},
						Labels: map[string]string{
							"server-healthy": "true",
						},
					},
				}
				return ppm
			},
			expectErrList: 1,
		},
		{
			name: "test7, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Probes[0].MarkerPolicy = []appsv1alpha1.ProbeMarkerPolicy{
					{
						State: appsv1alpha1.ProbeSucceeded,
						Annotations: map[string]string{
							"controller.kubernetes.io/pod-deletion-cost": "10",
						},
						Labels: map[string]string{
							"server-/$healthy": "true",
						},
					},
				}
				return ppm
			},
			expectErrList: 2,
		},
		{
			name: "test8, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Probes[0].Name = string(corev1.PodInitialized)
				return ppm
			},
			expectErrList: 1,
		},
		{
			name: "test9, invalid ppm",
			getPpm: func() *appsv1alpha1.PodProbeMarker {
				ppm := ppmDemo.DeepCopy()
				ppm.Spec.Probes[0].PodConditionType = "#5invalid"
				return ppm
			},
			expectErrList: 1,
		},
	}

	decoder, _ := admission.NewDecoder(scheme)
	perHandler := PodProbeMarkerCreateUpdateHandler{
		Decoder: decoder,
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			errList := perHandler.validatingPodProbeMarkerFn(cs.getPpm(), nil)
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %s", cs.expectErrList, len(errList), errList.ToAggregate().Error())
			}
		})
	}
}
