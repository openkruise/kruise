package enhancedlivenessprobe2nodeprobe

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	livenessprobeUtils "github.com/openkruise/kruise/pkg/util/livenessprobe"
)

func TestGeneratePodProbeObjByContainersProbeConfig(t *testing.T) {
	testCase := []struct {
		name                          string
		pod                           *v1.Pod
		containersLivenessProbeConfig []livenessprobeUtils.ContainerLivenessProbe
		expect                        appsv1alpha1.PodProbe
	}{
		{
			name: "exist container probe config",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       "11-22-33",
				},
				Status: v1.PodStatus{
					PodIP: "1.1.1.1",
				},
			},
			containersLivenessProbeConfig: []livenessprobeUtils.ContainerLivenessProbe{
				{
					Name: "c1",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							Exec: &v1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/healthy.sh"},
							},
						},
					},
				},
				{
					Name: "c2",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							TCPSocket: &v1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
							},
						},
					},
				},
			},
			expect: appsv1alpha1.PodProbe{
				Name:      "pod1",
				Namespace: "sp1",
				UID:       "11-22-33",
				IP:        "1.1.1.1",
				Probes: []appsv1alpha1.ContainerProbe{
					{
						Name:          "pod1-c1",
						ContainerName: "c1",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: v1.Probe{
								ProbeHandler: v1.ProbeHandler{
									Exec: &v1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/healthy.sh"},
									},
								},
							},
						},
					},
					{
						Name:          "pod1-c2",
						ContainerName: "c2",
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: v1.Probe{
								ProbeHandler: v1.ProbeHandler{
									TCPSocket: &v1.TCPSocketAction{
										Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "no container probe config",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       "11-22-33",
				},
				Status: v1.PodStatus{
					PodIP: "1.1.1.1",
				},
			},
			containersLivenessProbeConfig: []livenessprobeUtils.ContainerLivenessProbe{},
			expect: appsv1alpha1.PodProbe{
				Name:      "pod1",
				Namespace: "sp1",
				UID:       "11-22-33",
				IP:        "1.1.1.1",
				Probes:    []appsv1alpha1.ContainerProbe{},
			},
		},
	}
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			got := generatePodProbeByContainersProbeConfig(tc.pod, tc.containersLivenessProbeConfig)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("Test case: %v failed, expect: %v, but: %v", tc.name, tc.expect, got)
			}
		})
	}
}

func TestGeneratePodContainersProbe(t *testing.T) {
	testCase := []struct {
		name                          string
		pod                           *v1.Pod
		containersLivenessProbeConfig []livenessprobeUtils.ContainerLivenessProbe
		expect                        []appsv1alpha1.ContainerProbe
	}{
		{
			name: "exist container probe config",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       "11-22-33",
				},
				Status: v1.PodStatus{
					PodIP: "1.1.1.1",
				},
			},
			containersLivenessProbeConfig: []livenessprobeUtils.ContainerLivenessProbe{
				{
					Name: "c1",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							Exec: &v1.ExecAction{
								Command: []string{"/bin/sh", "-c", "/healthy.sh"},
							},
						},
					},
				},
				{
					Name: "c2",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							TCPSocket: &v1.TCPSocketAction{
								Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
							},
						},
					},
				},
			},
			expect: []appsv1alpha1.ContainerProbe{
				{
					Name:          "pod1-c1",
					ContainerName: "c1",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: v1.Probe{
							ProbeHandler: v1.ProbeHandler{
								Exec: &v1.ExecAction{
									Command: []string{"/bin/sh", "-c", "/healthy.sh"},
								},
							},
						},
					},
				},
				{
					Name:          "pod1-c2",
					ContainerName: "c2",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: v1.Probe{
							ProbeHandler: v1.ProbeHandler{
								TCPSocket: &v1.TCPSocketAction{
									Port: intstr.IntOrString{Type: intstr.String, StrVal: "main-port"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "no container probe config",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       "11-22-33",
				},
				Status: v1.PodStatus{
					PodIP: "1.1.1.1",
				},
			},
			containersLivenessProbeConfig: []livenessprobeUtils.ContainerLivenessProbe{},
			expect:                        []appsv1alpha1.ContainerProbe{},
		},
	}
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			got := generatePodContainersProbe(tc.pod, tc.containersLivenessProbeConfig)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("Test case: %v failed, expect: %v, but: %v", tc.name, tc.expect, got)
			}
		})
	}
}

func TestGetRawEnhancedLivenessProbeConfig(t *testing.T) {
	testCase := []struct {
		name   string
		pod    *v1.Pod
		expect string
	}{
		{
			name: "exist annotation for pod liveness probe config",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
				},
			},
			expect: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
		},
		{
			name: "no found annotation for pod liveness probe config",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
			},
			expect: "",
		},
	}
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			got := getRawEnhancedLivenessProbeConfig(tc.pod)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("Test case: %v failed, expect: %v, but: %v", tc.name, tc.expect, got)
			}
		})
	}
}

func TestGetEnhancedLivenessProbeConfig(t *testing.T) {
	testCase := []struct {
		name   string
		pod    *v1.Pod
		expect []livenessprobeUtils.ContainerLivenessProbe
	}{
		{
			name: "exist annotation for pod liveness probe config",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
				},
			},
			expect: []livenessprobeUtils.ContainerLivenessProbe{
				{
					Name: "c1",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromInt(7001),
							},
						},
						InitialDelaySeconds: 1000,
						TimeoutSeconds:      5,
						PeriodSeconds:       100,
						SuccessThreshold:    2,
						FailureThreshold:    3,
					},
				},
				{
					Name: "c2",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							TCPSocket: &v1.TCPSocketAction{
								Port: intstr.FromInt(7001),
							},
						},
						InitialDelaySeconds: 1000,
						TimeoutSeconds:      5,
						PeriodSeconds:       100,
						SuccessThreshold:    2,
						FailureThreshold:    3,
					},
				},
			},
		},
		{
			name: "no found annotation for pod liveness probe config",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
			},
			expect: []livenessprobeUtils.ContainerLivenessProbe{},
		},
	}
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEnhancedLivenessProbeConfig(tc.pod)
			if err != nil {
				t.Errorf("Test case: %v failed, err: %v", tc.name, err)
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("Test case: %v failed, expect: %v, but: %v", tc.name, tc.expect, got)
			}
		})
	}
}

func TestUsingEnhancedLivenessProbe(t *testing.T) {
	testCase := []struct {
		name   string
		pod    *v1.Pod
		expect bool
	}{
		{
			name: "using enhanced liveness probe",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationUsingEnhancedLiveness: "true",
					},
				},
			},
			expect: true,
		},
		{
			name: "no using enhanced liveness probe",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
				},
			},
			expect: false,
		},
		{
			name: "no using enhanced liveness probe v2",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationUsingEnhancedLiveness: "false",
					},
				},
			},
			expect: false,
		},
	}
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			got := usingEnhancedLivenessProbe(tc.pod)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("Test case: %v failed, expect: %v, but: %v", tc.name, tc.expect, got)
			}
		})
	}
}

func TestGenerateNewNodePodProbe(t *testing.T) {
	testCase := []struct {
		name               string
		pod                *v1.Pod
		expectNodePodProbe *appsv1alpha1.NodePodProbe
		expectErr          error
	}{
		{
			name: "correct to generate new node pod probe",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
					UID: types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			expectNodePodProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
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
			name: "no found annotation for pod to generate new node pod probe",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			expectNodePodProbe: &appsv1alpha1.NodePodProbe{},
			expectErr:          fmt.Errorf("Failed to generate pod probe object by containers probe config for pod: %s/%s", "sp1", "pod1"),
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.pod).Build()
			recon := ReconcileEnhancedLivenessProbe2NodeProbe{Client: fakeClient}
			got, err := recon.generateNewNodePodProbe(tc.pod)
			if !reflect.DeepEqual(err, tc.expectErr) {
				t.Errorf("Failed to generate new node pod probe, err: %v, expect: %v", err, tc.expectErr)
			}
			if !reflect.DeepEqual(util.DumpJSON(tc.expectNodePodProbe), util.DumpJSON(got)) {
				t.Errorf("No match, expect: %v, but: %v", util.DumpJSON(tc.expectNodePodProbe), util.DumpJSON(got))
			}
		})
	}
}

func TestRetryOnConflictAddNodeProbeConfig(t *testing.T) {
	testCase := []struct {
		name                          string
		pod                           *v1.Pod
		podNodeProbe                  *appsv1alpha1.NodePodProbe
		expectPodNodeProbe            *appsv1alpha1.NodePodProbe
		containersLivenessProbeConfig []livenessprobeUtils.ContainerLivenessProbe
	}{
		{
			name: "case add node probe config",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
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
			expectPodNodeProbe: &appsv1alpha1.NodePodProbe{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NodePodProbe",
					APIVersion: "apps.kruise.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
								},
							},
						},
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
			containersLivenessProbeConfig: []livenessprobeUtils.ContainerLivenessProbe{
				{
					Name: "c1",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromInt(7001),
							},
						},
						InitialDelaySeconds: 1000,
						TimeoutSeconds:      5,
						PeriodSeconds:       100,
						SuccessThreshold:    2,
						FailureThreshold:    3,
					},
				},
				{
					Name: "c2",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							TCPSocket: &v1.TCPSocketAction{
								Port: intstr.FromInt(7001),
							},
						},
						InitialDelaySeconds: 1000,
						TimeoutSeconds:      5,
						PeriodSeconds:       100,
						SuccessThreshold:    2,
						FailureThreshold:    3,
					},
				},
			},
		},
		{
			name: "case add node probe config(exist same config)",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NodePodProbe",
					APIVersion: "apps.kruise.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
								},
							},
						},
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
			expectPodNodeProbe: &appsv1alpha1.NodePodProbe{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NodePodProbe",
					APIVersion: "apps.kruise.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
								},
							},
						},
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
			containersLivenessProbeConfig: []livenessprobeUtils.ContainerLivenessProbe{
				{
					Name: "c1",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromInt(7001),
							},
						},
						InitialDelaySeconds: 1000,
						TimeoutSeconds:      5,
						PeriodSeconds:       100,
						SuccessThreshold:    2,
						FailureThreshold:    3,
					},
				},
				{
					Name: "c2",
					LivenessProbe: v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							TCPSocket: &v1.TCPSocketAction{
								Port: intstr.FromInt(7001),
							},
						},
						InitialDelaySeconds: 1000,
						TimeoutSeconds:      5,
						PeriodSeconds:       100,
						SuccessThreshold:    2,
						FailureThreshold:    3,
					},
				},
			},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			// create pod
			err := fakeClient.Create(context.TODO(), tc.pod)
			if err != nil {
				t.Errorf("Failed to create pod: %s/%s, err: %v", tc.pod.Namespace, tc.pod.Name, err)
			}
			// create nodeProbe
			err = fakeClient.Create(context.TODO(), tc.podNodeProbe)
			if err != nil {
				t.Errorf("Failed to create node pod probe: %s, err: %v", tc.podNodeProbe.Name, err)
			}
			recon := ReconcileEnhancedLivenessProbe2NodeProbe{Client: fakeClient}
			err = recon.retryOnConflictAddNodeProbeConfig(tc.pod, tc.containersLivenessProbeConfig)
			if err != nil {
				t.Errorf("Failed to retry on conflict add node probe config, err: %v", err)
			}
			getNodeProbe := appsv1alpha1.NodePodProbe{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: recon.GetPodNodeName(tc.pod)}, &getNodeProbe)
			if err != nil {
				t.Errorf("Failed to get node probe, err: %v", err)
			}
			if !checkNodePodProbeSpecEqual(tc.expectPodNodeProbe, &getNodeProbe) {
				t.Errorf("No match, expect: %v, but: %v", util.DumpJSON(tc.expectPodNodeProbe), util.DumpJSON(getNodeProbe))
			}
		})
	}

}

func TestRetryOnConflictDelNodeProbeConfig(t *testing.T) {
	testCase := []struct {
		name               string
		pod                *v1.Pod
		podNodeProbe       *appsv1alpha1.NodePodProbe
		expectPodNodeProbe *appsv1alpha1.NodePodProbe
	}{
		{
			name: "case del node probe config",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
								},
							},
						},
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
			expectPodNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
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
		},
		{
			name: "case full del node probe config",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
			expectPodNodeProbe: &appsv1alpha1.NodePodProbe{},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			// create pod
			err := fakeClient.Create(context.TODO(), tc.pod)
			if err != nil {
				t.Errorf("Failed to create pod: %s/%s, err: %v", tc.pod.Namespace, tc.pod.Name, err)
			}
			// create nodeProbe
			err = fakeClient.Create(context.TODO(), tc.podNodeProbe)
			if err != nil {
				t.Errorf("Failed to create node pod probe: %s, err: %v", tc.podNodeProbe.Name, err)
			}
			recon := ReconcileEnhancedLivenessProbe2NodeProbe{Client: fakeClient}
			err = recon.retryOnConflictDelNodeProbeConfig(tc.pod)
			if err != nil {
				t.Errorf("Failed to retry on conflict del node probe config, err: %v", err)
			}
			getNodeProbe := appsv1alpha1.NodePodProbe{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: recon.GetPodNodeName(tc.pod)}, &getNodeProbe)
			if err != nil {
				if !errors.IsNotFound(err) {
					t.Errorf("Failed to get node probe, err: %v", err)
				}
			}
			if !checkNodePodProbeSpecEqual(tc.expectPodNodeProbe, &getNodeProbe) {
				t.Errorf("No match, expect: %v, but: %v", util.DumpJSON(tc.expectPodNodeProbe), util.DumpJSON(getNodeProbe))
			}
		})
	}
}

func TestAddNodeProbeConfig(t *testing.T) {
	testCase := []struct {
		name               string
		pod                *v1.Pod
		podNodeProbe       *appsv1alpha1.NodePodProbe
		expectPodNodeProbe *appsv1alpha1.NodePodProbe
	}{
		{
			name: "no found node probe config, create it",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
					UID: types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node2", //no found node probe config for this pod
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
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
			expectPodNodeProbe: &appsv1alpha1.NodePodProbe{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NodePodProbe",
					APIVersion: "apps.kruise.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "found node probe config, update it",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
					UID: types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
			expectPodNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
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

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			err := fakeClient.Create(context.TODO(), tc.pod)
			if err != nil {
				t.Errorf("Failed to create pod: %s/%s, err: %v", tc.pod.Namespace, tc.pod.Name, err)
			}
			err = fakeClient.Create(context.TODO(), tc.podNodeProbe)
			if err != nil {
				t.Errorf("Failed to create node pod probe: %s, err: %v", tc.podNodeProbe.Name, err)
			}
			recon := ReconcileEnhancedLivenessProbe2NodeProbe{Client: fakeClient}
			err = recon.addNodeProbeConfig(tc.pod)
			if err != nil {
				t.Errorf("Failed to add node probe config, err: %v", err)
			}
			getNodeProbe := appsv1alpha1.NodePodProbe{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: recon.GetPodNodeName(tc.pod)}, &getNodeProbe)
			if err != nil {
				if !errors.IsNotFound(err) {
					t.Errorf("Failed to get node probe, err: %v", err)
				}
			}
			if !checkNodePodProbeSpecEqual(tc.expectPodNodeProbe, &getNodeProbe) {
				t.Errorf("No match, expect: %v, but: %v", util.DumpJSON(tc.expectPodNodeProbe), util.DumpJSON(getNodeProbe))
			}
		})
	}
}

func TestDelNodeProbeConfig(t *testing.T) {
	testCase := []struct {
		name               string
		pod                *v1.Pod
		podNodeProbe       *appsv1alpha1.NodePodProbe
		expectPodNodeProbe *appsv1alpha1.NodePodProbe
	}{
		{
			name: "case del node probe config",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
													Command: []string{"/bin/sh", "-c", "/healthy.sh"},
												},
											},
										},
									},
								},
							},
						},
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
			expectPodNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod2",
							Namespace: "sp2",
							UID:       fmt.Sprintf("%s", "222-111"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod2-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												Exec: &v1.ExecAction{
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
		},
		{
			name: "case full del node probe config",
			pod: &v1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       types.UID("111-222"),
				},
				Spec: v1.PodSpec{
					NodeName: "node1",
				},
			},
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
				Spec: appsv1alpha1.NodePodProbeSpec{
					PodProbes: []appsv1alpha1.PodProbe{
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												HTTPGet: &v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
								{
									Name:          "pod1-c2",
									ContainerName: "c2",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: v1.Probe{
											ProbeHandler: v1.ProbeHandler{
												TCPSocket: &v1.TCPSocketAction{
													Port: intstr.IntOrString{Type: intstr.Int, IntVal: 7001},
												},
											},
											InitialDelaySeconds: 1000,
											TimeoutSeconds:      5,
											PeriodSeconds:       100,
											SuccessThreshold:    2,
											FailureThreshold:    3,
										},
									},
								},
							},
						},
					},
				},
			},
			expectPodNodeProbe: &appsv1alpha1.NodePodProbe{},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			// create pod
			err := fakeClient.Create(context.TODO(), tc.pod)
			if err != nil {
				t.Errorf("Failed to create pod: %s/%s, err: %v", tc.pod.Namespace, tc.pod.Name, err)
			}
			// create nodeProbe
			err = fakeClient.Create(context.TODO(), tc.podNodeProbe)
			if err != nil {
				t.Errorf("Failed to create node pod probe: %s, err: %v", tc.podNodeProbe.Name, err)
			}
			recon := ReconcileEnhancedLivenessProbe2NodeProbe{Client: fakeClient}
			err = recon.delNodeProbeConfig(tc.pod)
			if err != nil {
				t.Errorf("Failed to retry on conflict del node probe config, err: %v", err)
			}
			getNodeProbe := appsv1alpha1.NodePodProbe{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: recon.GetPodNodeName(tc.pod)}, &getNodeProbe)
			if err != nil {
				if !errors.IsNotFound(err) {
					t.Errorf("Failed to get node probe, err: %v", err)
				}
			}
			if !checkNodePodProbeSpecEqual(tc.expectPodNodeProbe, &getNodeProbe) {
				t.Errorf("No match, expect: %v, but: %v", util.DumpJSON(tc.expectPodNodeProbe), util.DumpJSON(getNodeProbe))
			}
		})
	}
}
