/*
Copyright 2023 The Kruise Authors.

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

package sidecarterminator

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestInterestingPod(t *testing.T) {
	cases := []struct {
		name     string
		getPod   func() *corev1.Pod
		expected bool
	}{
		{
			name: "Interesting, test1",
			getPod: func() *corev1.Pod {
				return &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
							},
							{
								Name: "normal-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
										Value: "true",
									},
								},
							},
						},
						RestartPolicy: corev1.RestartPolicyNever,
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "main",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode: 127,
									},
								},
							},
							{
								Name: "normal-sidecar1",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Now(),
									},
								},
							},
						},
						Phase: corev1.PodRunning,
					},
				}
			},
			expected: true,
		},
		{
			name: "Interesting, test2",
			getPod: func() *corev1.Pod {
				return &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
							},
							{
								Name: "normal-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
										Value: "true",
									},
								},
							},
							{
								Name: "ignore-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
										Value: "true",
									},
									{
										Name:  appsv1alpha1.KruiseIgnoreContainerExitCodeEnv,
										Value: "true",
									},
								},
							},
						},
						RestartPolicy: corev1.RestartPolicyOnFailure,
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "main",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode: 0,
									},
								},
							},
							{
								Name: "normal-sidecar1",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode: 0,
									},
								},
							},
							{
								Name: "ignore-sidecar1",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Now(),
									},
								},
							},
						},
					},
				}
			},
			expected: true,
		},
		{
			name: "Interesting, test3",
			getPod: func() *corev1.Pod {
				return &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
							},
							{
								Name: "normal-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
										Value: "true",
									},
								},
							},
							{
								Name: "inplace-update-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarWithImageEnv,
										Value: "inplace-update/image",
									},
								},
							},
						},
						RestartPolicy: corev1.RestartPolicyOnFailure,
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "main",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode: 0,
									},
								},
							},
							{
								Name: "normal-sidecar1",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode: 0,
									},
								},
							},
							{
								Name: "inplace-update-sidecar1",
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{
										StartedAt: metav1.Now(),
									},
								},
							},
						},
					},
				}
			},
			expected: true,
		},
	}

	for _, cs := range cases {
		pod := cs.getPod()
		if isInterestingPod(pod) != cs.expected {
			t.Errorf("case %s failed, expected %v, got %v", cs.name, cs.expected, isInterestingPod(pod))
		}
	}
}

func TestGetSidecarContainerNames(t *testing.T) {
	cases := []struct {
		name     string
		getPod   func() *corev1.Pod
		isVK     bool
		expected func() (sets.Set[string], sets.Set[string], sets.Set[string])
	}{
		{
			name: "normal, test1",
			isVK: false,
			getPod: func() *corev1.Pod {
				return &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
							},
							{
								Name: "normal-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
										Value: "true",
									},
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarWithImageEnv,
										Value: "true",
									},
								},
							},
							{
								Name: "ignore-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
										Value: "true",
									},
									{
										Name:  appsv1alpha1.KruiseIgnoreContainerExitCodeEnv,
										Value: "true",
									},
								},
							},
						},
					},
				}
			},
			expected: func() (sets.Set[string], sets.Set[string], sets.Set[string]) {
				return sets.New[string]("normal-sidecar1"), sets.New[string]("ignore-sidecar1"), sets.New[string]()
			},
		},
		{
			name: "vk, test2",
			isVK: true,
			getPod: func() *corev1.Pod {
				return &corev1.Pod{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
							},
							{
								Name: "vk-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
										Value: "true",
									},
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarWithImageEnv,
										Value: "true",
									},
								},
							},
							{
								Name: "ignore-sidecar1",
								Env: []corev1.EnvVar{
									{
										Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
										Value: "true",
									},
									{
										Name:  appsv1alpha1.KruiseIgnoreContainerExitCodeEnv,
										Value: "true",
									},
								},
							},
						},
					},
				}
			},
			expected: func() (sets.Set[string], sets.Set[string], sets.Set[string]) {
				return sets.New[string](), sets.New[string](), sets.New[string]("vk-sidecar1")
			},
		},
	}

	for _, cs := range cases {
		pod := cs.getPod()
		normalSidecarNames, ignoreExitCodeSidecarNames, inplaceUpdateSidecarNames := getSidecarContainerNames(pod, cs.isVK)
		expected1, expected2, expected3 := cs.expected()
		if !normalSidecarNames.Equal(expected1) {
			t.Errorf("case %s failed, expected %v, got %v", cs.name, expected1, normalSidecarNames)
		}
		if !ignoreExitCodeSidecarNames.Equal(expected2) {
			t.Errorf("case %s failed, expected %v, got %v", cs.name, expected2, ignoreExitCodeSidecarNames)
		}
		if !inplaceUpdateSidecarNames.Equal(expected3) {
			t.Errorf("case %s failed, expected %v, got %v", cs.name, expected3, inplaceUpdateSidecarNames)
		}
	}
}
