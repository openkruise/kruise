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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
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
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/index.html",
									Scheme: corev1.URISchemeHTTP,
									Port:   intstr.FromInt(80),
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

	decoder := admission.NewDecoder(scheme)
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

func TestValidateHandler(t *testing.T) {
	successCases := []corev1.ProbeHandler{
		{Exec: &corev1.ExecAction{Command: []string{"echo"}}},
		{TCPSocket: &corev1.TCPSocketAction{
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8000)},
			Host: "3.3.3.3",
		}},
		{TCPSocket: &corev1.TCPSocketAction{
			Port: intstr.IntOrString{Type: intstr.String, StrVal: "container-port"},
			Host: "3.3.3.3",
		}},
	}
	for _, h := range successCases {
		if errs := validateHandler(&h, field.NewPath("field")); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	errorCases := []corev1.ProbeHandler{
		{},
		{Exec: &corev1.ExecAction{Command: []string{}}},
		{TCPSocket: &corev1.TCPSocketAction{
			Port: intstr.IntOrString{Type: intstr.String, StrVal: "container-port-v2"},
			Host: "3.3.3.3",
		}},
		{TCPSocket: &corev1.TCPSocketAction{
			Port: intstr.IntOrString{Type: intstr.Int, IntVal: -1},
			Host: "3.3.3.3",
		}},
		{HTTPGet: &corev1.HTTPGetAction{Path: "", Port: intstr.FromInt(0), Host: ""}},
		{HTTPGet: &corev1.HTTPGetAction{Path: "/foo", Port: intstr.FromInt(65536), Host: "host"}},
		{HTTPGet: &corev1.HTTPGetAction{Path: "", Port: intstr.FromString(""), Host: ""}},
		{
			Exec: &corev1.ExecAction{Command: []string{}},
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{Type: intstr.String, StrVal: "container-port-v2"},
				Host: "3.3.3.3",
			},
		},
		{
			Exec:    &corev1.ExecAction{Command: []string{}},
			HTTPGet: &corev1.HTTPGetAction{Path: "", Port: intstr.FromString(""), Host: ""},
		},
		{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{Type: intstr.String, StrVal: "container-port-v2"},
				Host: "3.3.3.3",
			},
			HTTPGet: &corev1.HTTPGetAction{Path: "", Port: intstr.FromString(""), Host: ""},
		},
		{
			Exec: &corev1.ExecAction{Command: []string{}},
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{Type: intstr.String, StrVal: "container-port-v2"},
				Host: "3.3.3.3",
			},
			HTTPGet: &corev1.HTTPGetAction{Path: "", Port: intstr.FromString(""), Host: ""},
		},
	}
	for _, h := range errorCases {
		if errs := validateHandler(&h, field.NewPath("field")); len(errs) == 0 {
			t.Errorf("expected failure for %#v", h)
		}
	}
}

func TestValidatePortNumOrName(t *testing.T) {
	successCases := []struct {
		port    intstr.IntOrString
		fldPath *field.Path
	}{
		{
			port: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 80,
			},
			fldPath: field.NewPath("field"),
		},
		{
			port: intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "abc",
			},
			fldPath: field.NewPath("field"),
		},
	}
	for _, cs := range successCases {
		if getErrs := ValidatePortNumOrName(cs.port, cs.fldPath); len(getErrs) != 0 {
			t.Errorf("expect failure for %#v", util.DumpJSON(cs))
		}
	}

	errorCases := []struct {
		port    intstr.IntOrString
		fldPath *field.Path
	}{
		{
			port: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: -1,
			},
			fldPath: field.NewPath("field"),
		},
		{
			port: intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "",
			},
			fldPath: field.NewPath("field"),
		},
		{
			// IsValidPortName check that the argument is valid syntax. It must be
			// non-empty and no more than 15 characters long. It may contain only [-a-z0-9]
			// and must contain at least one letter [a-z].
			port: intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "aaaaabbbbbcccccd", // more than 15 characters
			},
			fldPath: field.NewPath("field"),
		},
		{
			port: intstr.IntOrString{
				Type: 3, // fake type
			},
			fldPath: field.NewPath("field"),
		},
	}
	for _, cs := range errorCases {
		if getErrs := ValidatePortNumOrName(cs.port, cs.fldPath); len(getErrs) == 0 {
			t.Errorf("expect failure for %#v", util.DumpJSON(cs))
		}
	}
}

func TestValidateTCPSocketAction(t *testing.T) {
	successCases := []struct {
		tcp     *corev1.TCPSocketAction
		fldPath *field.Path
	}{
		{
			tcp: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 80,
				},
			},
			fldPath: field.NewPath("field"),
		},
		{
			tcp: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "abc",
				},
			},
			fldPath: field.NewPath("field"),
		},
	}
	for _, cs := range successCases {
		if getErrs := validateTCPSocketAction(cs.tcp, cs.fldPath); len(getErrs) != 0 {
			t.Errorf("expect failure for %#v", util.DumpJSON(cs))
		}
	}

	errorCases := []struct {
		tcp     *corev1.TCPSocketAction
		fldPath *field.Path
	}{
		{
			tcp: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: -1,
				},
			},
			fldPath: field.NewPath("field"),
		},
		{
			tcp: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "",
				},
			},
			fldPath: field.NewPath("field"),
		},
		{
			tcp: &corev1.TCPSocketAction{
				// IsValidPortName check that the argument is valid syntax. It must be
				// non-empty and no more than 15 characters long. It may contain only [-a-z0-9]
				// and must contain at least one letter [a-z].
				Port: intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "aaaaabbbbbcccccd", // more than 15 characters
				},
			},
			fldPath: field.NewPath("field"),
		},
		{
			tcp: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{
					Type: 3, // fake type
				},
			},
			fldPath: field.NewPath("field"),
		},
	}
	for _, cs := range errorCases {
		if getErrs := validateTCPSocketAction(cs.tcp, cs.fldPath); len(getErrs) == 0 {
			t.Errorf("expect failure for %#v", util.DumpJSON(cs))
		}
	}
}
