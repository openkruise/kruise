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

package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	utilpointer "k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
)

type PodProbeMarkerTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
}

func NewPodProbeMarkerTester(c clientset.Interface, kc kruiseclientset.Interface) *PodProbeMarkerTester {
	return &PodProbeMarkerTester{
		c:  c,
		kc: kc,
	}
}

func (s *PodProbeMarkerTester) NewPodProbeMarker(ns, randStr string) []appsv1alpha1.PodProbeMarker {
	nginx := appsv1alpha1.PodProbeMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ppm-nginx",
			Namespace: ns,
		},
		Spec: appsv1alpha1.PodProbeMarkerSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": fmt.Sprintf("probe-%s", randStr),
				},
			},
			Probes: []appsv1alpha1.PodContainerProbe{
				{
					Name:          "healthy",
					ContainerName: "nginx",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "ls /"},
								},
							},
						},
					},
					PodConditionType: "game.kruise.io/healthy",
					MarkerPolicy: []appsv1alpha1.ProbeMarkerPolicy{
						{
							State: appsv1alpha1.ProbeSucceeded,
							Labels: map[string]string{
								"nginx": "healthy",
							},
						},
					},
				},
			},
		},
	}

	main := appsv1alpha1.PodProbeMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ppm-main",
			Namespace: ns,
		},
		Spec: appsv1alpha1.PodProbeMarkerSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": fmt.Sprintf("probe-%s", randStr),
				},
			},
			Probes: []appsv1alpha1.PodContainerProbe{
				{
					Name:          "check",
					ContainerName: "main",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "-c", "ps -ef"},
								},
							},
						},
					},
					PodConditionType: "game.kruise.io/check",
					MarkerPolicy: []appsv1alpha1.ProbeMarkerPolicy{
						{
							State: appsv1alpha1.ProbeSucceeded,
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "10",
							},
						},
						{
							State: appsv1alpha1.ProbeFailed,
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "-10",
							},
						},
					},
				},
			},
		},
	}

	return []appsv1alpha1.PodProbeMarker{nginx, main}
}

func (s *PodProbeMarkerTester) NewPodProbeMarkerForTcpCheck(ns, randStr string) []appsv1alpha1.PodProbeMarker {
	nginx := appsv1alpha1.PodProbeMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ppm-nginx",
			Namespace: ns,
		},
		Spec: appsv1alpha1.PodProbeMarkerSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": fmt.Sprintf("probe-%s", randStr),
				},
			},
			Probes: []appsv1alpha1.PodContainerProbe{
				{
					Name:          "healthy",
					ContainerName: "nginx",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(80)},
								},
							},
						},
					},
					PodConditionType: "game.kruise.io/healthy",
					MarkerPolicy: []appsv1alpha1.ProbeMarkerPolicy{
						{
							State: appsv1alpha1.ProbeSucceeded,
							Labels: map[string]string{
								"nginx": "healthy",
							},
						},
					},
				},
			},
		},
	}

	return []appsv1alpha1.PodProbeMarker{nginx}
}

func (s *PodProbeMarkerTester) NewBaseStatefulSet(namespace, randStr string) *appsv1beta1.StatefulSet {
	return &appsv1beta1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps.kruise.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stateful-test",
			Namespace: namespace,
		},
		Spec: appsv1beta1.StatefulSetSpec{
			PodManagementPolicy: apps.ParallelPodManagement,
			ServiceName:         "fake-service",
			Replicas:            utilpointer.Int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": fmt.Sprintf("probe-%s", randStr),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": fmt.Sprintf("probe-%s", randStr),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "nginx",
							Image:           "nginx:1.15.1",
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
						{
							Name:            "main",
							Image:           "centos:7",
							Command:         []string{"sleep", "999d"},
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
			},
		},
	}
}

func (s *PodProbeMarkerTester) NewPodProbeMarkerWithProbeImg(ns, randStr string) appsv1alpha1.PodProbeMarker {
	return appsv1alpha1.PodProbeMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ppm-minecraft",
			Namespace: ns,
		},
		Spec: appsv1alpha1.PodProbeMarkerSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": fmt.Sprintf("probe-%s", randStr),
				},
			},
			Probes: []appsv1alpha1.PodContainerProbe{
				{
					Name:          "healthy",
					ContainerName: "minecraft",
					Probe: appsv1alpha1.ContainerProbeSpec{
						Probe: corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"bash", "./probe.sh"},
								},
							},
						},
					},
					PodConditionType: "game.kruise.io/healthy",
				},
			},
		},
	}
}

func (s *PodProbeMarkerTester) NewStatefulSetWithProbeImg(namespace, randStr string) *appsv1beta1.StatefulSet {
	return &appsv1beta1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps.kruise.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stateful-test",
			Namespace: namespace,
		},
		Spec: appsv1beta1.StatefulSetSpec{
			PodManagementPolicy: apps.ParallelPodManagement,
			ServiceName:         "fake-service",
			Replicas:            utilpointer.Int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": fmt.Sprintf("probe-%s", randStr),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": fmt.Sprintf("probe-%s", randStr),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "minecraft",
							Image:           "openkruise/minecraft-demo:probe-v0",
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
			},
		},
	}
}

func (s *PodProbeMarkerTester) CreateStatefulSet(sts *appsv1beta1.StatefulSet) {
	Logf("create sts(%s/%s)", sts.Namespace, sts.Name)
	_, err := s.kc.AppsV1beta1().StatefulSets(sts.Namespace).Create(context.TODO(), sts, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForStatefulSetRunning(sts)
}

func (s *PodProbeMarkerTester) WaitForStatefulSetRunning(sts *appsv1beta1.StatefulSet) {
	pollErr := wait.PollImmediate(time.Second, 2*time.Minute,
		func() (bool, error) {
			inner, err := s.kc.AppsV1beta1().StatefulSets(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			if inner.Generation != inner.Status.ObservedGeneration {
				return false, nil
			}
			if *inner.Spec.Replicas == inner.Status.Replicas && *inner.Spec.Replicas == inner.Status.UpdatedReplicas &&
				*inner.Spec.Replicas == inner.Status.ReadyReplicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for statefulset to enter running: %v", pollErr)
	}
}

func (s *PodProbeMarkerTester) ListActivePods(ns string) ([]*corev1.Pod, error) {
	podList, err := s.c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var pods []*corev1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if kubecontroller.IsPodActive(pod) && pod.Spec.NodeName != "" {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}
