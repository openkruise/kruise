package mutating

import (
	"context"
	"reflect"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
)

var (
	schema *runtime.Scheme
)

func init() {
	schema = runtime.NewScheme()
	_ = corev1.AddToScheme(schema)
}

func TestRemoveAndBackUpPodContainerLivenessProbe(t *testing.T) {

	testCases := []struct {
		name         string
		pod          *corev1.Pod
		expectResult string
	}{
		{
			name: "livenessProbe configuration for standard container",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "namespace1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "c1",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 3000,
								PeriodSeconds:       100,
								SuccessThreshold:    1,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
										},
										Scheme: corev1.URISchemeHTTP,
									},
								},
							},
						},
					},
				},
			},
			expectResult: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001,"scheme":"HTTP"},"initialDelaySeconds":3000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":1,"failureThreshold":3}}]`,
		},
		{
			name: "livenessProbe configuration for multi-standard containers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "c1",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 3000,
								PeriodSeconds:       100,
								SuccessThreshold:    1,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
										},
										Scheme: corev1.URISchemeHTTP,
									},
								},
							},
						},
						{
							Name: "c2",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 3000,
								PeriodSeconds:       100,
								SuccessThreshold:    1,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"/home/admin/liveness.sh",
										},
									},
								},
							},
						},
					},
				},
			},
			expectResult: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001,"scheme":"HTTP"},"initialDelaySeconds":3000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":1,"failureThreshold":3}},{"name":"c2","livenessProbe":{"exec":{"command":["/home/admin/liveness.sh"]},"initialDelaySeconds":3000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":1,"failureThreshold":3}}]`,
		},
		{
			name: "different livenssProbe configuration for multi-standard containers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod3",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "c1",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 3000,
								PeriodSeconds:       100,
								SuccessThreshold:    1,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
										},
										Scheme: corev1.URISchemeHTTP,
									},
								},
							},
						},
						{
							Name: "c2",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 3000,
								PeriodSeconds:       100,
								SuccessThreshold:    1,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"/home/admin/liveness.sh",
										},
									},
								},
							},
						},
						{
							Name: "c3",
						},
					},
				},
			},
			expectResult: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001,"scheme":"HTTP"},"initialDelaySeconds":3000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":1,"failureThreshold":3}},{"name":"c2","livenessProbe":{"exec":{"command":["/home/admin/liveness.sh"]},"initialDelaySeconds":3000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":1,"failureThreshold":3}}]`,
		},
		{
			name: "no livenessProbe configuration for standard container",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod3",
					Namespace: "sp1",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "c1",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := removeAndBackUpPodContainerLivenessProbe(tc.pod)
			if err != nil {
				t.Errorf("Test case: %v failed, err: %v", tc.name, err)
			}
			if !reflect.DeepEqual(got, tc.expectResult) {
				t.Errorf("Test case: %v failed, expect: %v, but: %v",
					tc.name, tc.expectResult, got)
			}
		})
	}
}

func TestUsingEnhancedLivenessProbe(t *testing.T) {
	testCases := []struct {
		name         string
		pod          *corev1.Pod
		expectResult bool
	}{
		{
			name: "case no exist annotationUsingEnhancedLiveness in pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
				},
			},
			expectResult: false,
		},
		{
			name: "case exist annotationUsingEnhancedLiveness in pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						alpha1.AnnotationUsingEnhancedLiveness: "true",
					},
				},
			},
			expectResult: true,
		},
		{
			name: "case exist reverse annotationUsingEnhancedLiveness in pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
					Annotations: map[string]string{
						alpha1.AnnotationUsingEnhancedLiveness: "false",
					},
				},
			},
			expectResult: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := usingEnhancedLivenessProbe(tc.pod)
			if got != tc.expectResult {
				t.Errorf("Test case: %v failed, expect: %v, but: %v", tc.name, tc.expectResult, got)
			}
		})
	}
}

func TestRemoveAndBackUpPodContainerLivenessProbeLink(t *testing.T) {

	testCases := []struct {
		name      string
		pod       *corev1.Pod
		expectPod *corev1.Pod
	}{
		{
			name: "case1: exist using enhanced liveness probe gate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationUsingEnhancedLiveness: "true",
					},
					ResourceVersion: "v1",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         clonesetutils.ControllerKind.GroupVersion().String(),
						Kind:               clonesetutils.ControllerKind.Kind,
						Name:               "cloneSet1",
						UID:                "1111-2222",
						Controller:         func() *bool { v := true; return &v }(),
						BlockOwnerDeletion: func() *bool { v := true; return &v }(),
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "c1",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 1000,
								PeriodSeconds:       100,
								SuccessThreshold:    2,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
										},
									},
								},
							},
						},
						{
							Name: "c2",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 1000,
								PeriodSeconds:       100,
								SuccessThreshold:    2,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
										},
									},
								},
							},
						},
					},
				},
			},
			expectPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "sp1",
					Annotations: map[string]string{
						alpha1.AnnotationUsingEnhancedLiveness:       "true",
						alpha1.AnnotationNativeContainerProbeContext: `[{"name":"c1","livenessProbe":{"httpGet":{"path":"/health","port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}},{"name":"c2","livenessProbe":{"tcpSocket":{"port":7001},"initialDelaySeconds":1000,"timeoutSeconds":5,"periodSeconds":100,"successThreshold":2,"failureThreshold":3}}]`,
					},
					ResourceVersion: "v1",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         clonesetutils.ControllerKind.GroupVersion().String(),
						Kind:               clonesetutils.ControllerKind.Kind,
						Name:               "cloneSet1",
						UID:                "1111-2222",
						Controller:         func() *bool { v := true; return &v }(),
						BlockOwnerDeletion: func() *bool { v := true; return &v }(),
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "c1",
						},
						{
							Name: "c2",
						},
					},
				},
			},
		},
		{
			name: "case2: no exist using enhanced liveness probe gate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "pod1",
					Namespace:       "sp1",
					ResourceVersion: "v1",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         clonesetutils.ControllerKind.GroupVersion().String(),
						Kind:               clonesetutils.ControllerKind.Kind,
						Name:               "cloneSet1",
						UID:                "1111-2222",
						Controller:         func() *bool { v := true; return &v }(),
						BlockOwnerDeletion: func() *bool { v := true; return &v }(),
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "c1",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 1000,
								PeriodSeconds:       100,
								SuccessThreshold:    2,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
										},
									},
								},
							},
						},
						{
							Name: "c2",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 1000,
								PeriodSeconds:       100,
								SuccessThreshold:    2,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
										},
									},
								},
							},
						},
					},
				},
			},
			expectPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "pod1",
					Namespace:       "sp1",
					ResourceVersion: "v1",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         clonesetutils.ControllerKind.GroupVersion().String(),
						Kind:               clonesetutils.ControllerKind.Kind,
						Name:               "cloneSet1",
						UID:                "1111-2222",
						Controller:         func() *bool { v := true; return &v }(),
						BlockOwnerDeletion: func() *bool { v := true; return &v }(),
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "c1",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 1000,
								PeriodSeconds:       100,
								SuccessThreshold:    2,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
										},
									},
								},
							},
						},
						{
							Name: "c2",
							LivenessProbe: &corev1.Probe{
								FailureThreshold:    3,
								InitialDelaySeconds: 1000,
								PeriodSeconds:       100,
								SuccessThreshold:    2,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 7001,
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
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			podIn := tc.pod
			decoder := admission.NewDecoder(schema)
			client := fake.NewClientBuilder().WithScheme(schema).WithObjects(podIn).Build()
			podOut := podIn.DeepCopy()
			podHandler := &PodCreateHandler{Decoder: decoder, Client: client}
			req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
			_, err := podHandler.enhancedLivenessProbeWhenPodCreate(context.Background(), req, podOut)
			if err != nil {
				t.Errorf("enhanced liveness probe when pod create failed, err: %v", err)
			}
			if !reflect.DeepEqual(tc.expectPod, podOut) {
				t.Errorf("pod DeepEqual failed")
			}
		})
	}
}
