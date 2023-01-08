package mutating

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"testing"
)

func TestRemoveAndBackUpPodContainerLivenessProbe(t *testing.T) {

	testCases := []struct {
		name         string
		pod          *corev1.Pod
		expectResult string
	}{
		{
			name: "testcase1",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
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
								Handler: corev1.Handler{
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
			name: "testcase2",
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
								Handler: corev1.Handler{
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
								Handler: corev1.Handler{
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
			name: "testcase3",
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
								Handler: corev1.Handler{
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
								Handler: corev1.Handler{
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
			name: "testcase4",
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
		str, err := removeAndBackUpPodContainerLivenessProbe(tc.pod)
		if err != nil {
			t.Fatalf("test case %v failed", tc.name)
		}
		if !reflect.DeepEqual(str, tc.expectResult) {
			t.Fatalf("failed to test case: %v, expect: %v, but: %v",
				tc.name, tc.expectResult, str)
		}
	}
}
