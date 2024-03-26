package livenessprobemapnodeprobe

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	"github.com/openkruise/kruise/pkg/util"
)

var (
	scheme *runtime.Scheme
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
}

func TestAddOrRemoveNodePodProbeConfig(t *testing.T) {
	testCase := []struct {
		name            string
		pod             *corev1.Pod
		op              string
		podNodeProbe    *appsv1alpha1.NodePodProbe
		expectNodeProbe *appsv1alpha1.NodePodProbe
	}{
		{
			name: "test: add node probe config",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
					UID: types.UID("111-222"),
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
				},
			},
			op: AddNodeProbeConfigOpType,
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "node1",
					ResourceVersion: "111",
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
			expectNodeProbe: &appsv1alpha1.NodePodProbe{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NodePodProbe",
					APIVersion: "apps.kruise.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "node1",
					ResourceVersion: "112",
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
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												HTTPGet: &corev1.HTTPGetAction{
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
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												TCPSocket: &corev1.TCPSocketAction{
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
			name: "test: remove node probe config",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
					UID: types.UID("111-222"),
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
				},
			},
			op: DelNodeProbeConfigOpType,
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "node1",
					ResourceVersion: "111",
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
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												HTTPGet: &corev1.HTTPGetAction{
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
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												TCPSocket: &corev1.TCPSocketAction{
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
			expectNodeProbe: &appsv1alpha1.NodePodProbe{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NodePodProbe",
					APIVersion: "apps.kruise.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "node1",
					ResourceVersion: "112",
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
		},
		{
			name: "test: add node probe config(change different config)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
					UID: types.UID("111-222"),
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
				},
			},
			op: AddNodeProbeConfigOpType,
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "node1",
					ResourceVersion: "111",
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
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												HTTPGet: &corev1.HTTPGetAction{
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
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												TCPSocket: &corev1.TCPSocketAction{
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
			expectNodeProbe: &appsv1alpha1.NodePodProbe{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NodePodProbe",
					APIVersion: "apps.kruise.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "node1",
					ResourceVersion: "112",
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
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												HTTPGet: &corev1.HTTPGetAction{
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
		},
		{
			name: "test: add node probe config(no found config)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					UID:       types.UID("111-222"),
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
				},
			},
			op: AddNodeProbeConfigOpType,
			podNodeProbe: &appsv1alpha1.NodePodProbe{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "node1",
					ResourceVersion: "111",
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
			expectNodeProbe: &appsv1alpha1.NodePodProbe{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NodePodProbe",
					APIVersion: "apps.kruise.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "node1",
					ResourceVersion: "112",
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
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			fakeClient := builder.WithObjects(tc.pod).WithObjects(tc.podNodeProbe).Build()
			r := ReconcileEnhancedLivenessProbeMapNodeProbe{Client: fakeClient}
			err := r.addOrRemoveNodePodProbeConfig(tc.pod, tc.op)
			if err != nil {
				t.Errorf("Test case: %v failed, err: %v", tc.name, err)
			}
			// get NodeProbe
			getNodeProbe := appsv1alpha1.NodePodProbe{}
			err = r.Get(context.TODO(), types.NamespacedName{Name: tc.pod.Spec.NodeName}, &getNodeProbe)
			if err != nil {
				t.Errorf("Failed to get node probe object, err: %v", err)
			}
			t.Logf("GetNodeProbe: %v", util.DumpJSON(getNodeProbe))
			if !reflect.DeepEqual(util.DumpJSON(tc.expectNodeProbe), util.DumpJSON(getNodeProbe)) {
				t.Errorf("Expect: %v, but: %v", util.DumpJSON(tc.expectNodeProbe), util.DumpJSON(getNodeProbe))
			}

		})
	}
}

func TestSyncPodContainersLivenessProbe(t *testing.T) {
	testCase := []struct {
		name               string
		request            reconcile.Request
		pod                *corev1.Pod
		podNodeProbe       *appsv1alpha1.NodePodProbe
		expectPod          *corev1.Pod
		expectPodNodeProbe *appsv1alpha1.NodePodProbe
	}{
		{
			name: "test reconcile: add node probe config",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "pod1",
					Namespace: "sp1",
				},
			},
			pod: &corev1.Pod{
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
				Spec: corev1.PodSpec{
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
			expectPod: &corev1.Pod{
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
					Finalizers: []string{
						FinalizerPodEnhancedLivenessProbe,
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
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
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												HTTPGet: &corev1.HTTPGetAction{
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
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												TCPSocket: &corev1.TCPSocketAction{
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
			name: "test reconcile: remove node probe config",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "pod1",
					Namespace: "sp1",
				},
			},
			pod: &corev1.Pod{
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
					UID:               types.UID("111-222"),
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers: []string{
						FinalizerPodEnhancedLivenessProbe,
						"no-delete-protection",
					},
				},
				Spec: corev1.PodSpec{
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
						{
							Name:      "pod1",
							Namespace: "sp1",
							UID:       fmt.Sprintf("%s", "111-222"),
							Probes: []appsv1alpha1.ContainerProbe{
								{
									Name:          "pod1-c1",
									ContainerName: "c1",
									Probe: appsv1alpha1.ContainerProbeSpec{
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												HTTPGet: &corev1.HTTPGetAction{
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
										Probe: corev1.Probe{
											ProbeHandler: corev1.ProbeHandler{
												TCPSocket: &corev1.TCPSocketAction{
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
			expectPod: &corev1.Pod{
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
					Finalizers: []string{
						"no-delete-protection",
					},
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Spec: corev1.PodSpec{
					NodeName: "node1",
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
				t.Errorf("Failed to create pod node probe: %s, err: %v", tc.podNodeProbe.Name, err)
			}
			recon := ReconcileEnhancedLivenessProbeMapNodeProbe{Client: fakeClient}
			_, err = recon.Reconcile(context.TODO(), tc.request)
			if err != nil {
				t.Errorf("Reconcile failed: %s", err.Error())
			}
			// get pod and check
			getPod := corev1.Pod{}
			err = fakeClient.Get(context.TODO(), tc.request.NamespacedName, &getPod)
			if err != nil {
				t.Errorf("Failed to get pod: %s/%s, err: %v", tc.request.Namespace, tc.request.Name, err)
			}
			t.Logf("getPod: %v", util.DumpJSON(getPod))
			// check pod
			if !checkPodFinalizerEqual(tc.expectPod, &getPod) {
				t.Errorf("No match, expect: %v, get: %v", util.DumpJSON(tc.expectPod), util.DumpJSON(getPod))
			}
			// get nodeProbe and check
			getPodNodeProbe := appsv1alpha1.NodePodProbe{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: tc.pod.Spec.NodeName}, &getPodNodeProbe)
			if err != nil {
				t.Errorf("Failed to get pod node probe: %s, err: %v", tc.pod.Spec.NodeName, err)
			}
			t.Logf("getPodNodeProbe: %v", util.DumpJSON(getPodNodeProbe))
			// check modePodProbe
			if !checkNodePodProbeSpecEqual(tc.expectPodNodeProbe, &getPodNodeProbe) {
				t.Errorf("No match, expect: %v, get: %v", util.DumpJSON(tc.expectPodNodeProbe), util.DumpJSON(getPodNodeProbe))
			}
		})
	}

}

func checkNodePodProbeSpecEqual(expectNodePodProbe, getNodePodProbe *appsv1alpha1.NodePodProbe) bool {
	if expectNodePodProbe == nil || getNodePodProbe == nil {
		return false
	}
	return reflect.DeepEqual(expectNodePodProbe.Spec, getNodePodProbe.Spec)
}

func checkPodFinalizerEqual(expectPod, getPod *corev1.Pod) bool {
	if expectPod == nil || getPod == nil {
		return false
	}
	return reflect.DeepEqual(expectPod.Finalizers, getPod.Finalizers)
}
