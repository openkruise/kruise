/*
Copyright 2020 The Kruise Authors.

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

package sidecarset

import (
	"context"
	"fmt"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/expectations"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	testImageV1ImageID = "docker-pullable://test-image@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d"
	testImageV2ImageID = "docker-pullable://test-image@sha256:f7988fb6c02e0ce69257d9bd9cf37ae20a60f1df7563c3a2a6abe24160306b8d"
)

func TestUpdateColdUpgradeSidecar(t *testing.T) {
	sidecarSetInput := sidecarSetDemo.DeepCopy()
	podInput := podDemo.DeepCopy()
	podInput.Spec.Containers[1].Env = []corev1.EnvVar{
		{
			Name:  "nginx-env",
			Value: "nginx-value",
		},
	}
	podInput.Spec.Containers[1].VolumeMounts = []corev1.VolumeMount{
		{
			MountPath: "/data/nginx",
		},
	}
	handlers := map[string]HandlePod{
		"pod test-pod-1 is upgrading": func(pods []*corev1.Pod) {
			cStatus := &pods[0].Status.ContainerStatuses[1]
			cStatus.Image = "test-image:v2"
			cStatus.ImageID = testImageV2ImageID
		},
		"pod test-pod-2 is upgrading": func(pods []*corev1.Pod) {
			cStatus := &pods[1].Status.ContainerStatuses[1]
			cStatus.Image = "test-image:v2"
			cStatus.ImageID = testImageV2ImageID
		},
	}
	testUpdateColdUpgradeSidecar(t, podInput, sidecarSetInput, handlers)
}

func testUpdateColdUpgradeSidecar(t *testing.T, podDemo *corev1.Pod, sidecarSetInput *appsv1alpha1.SidecarSet, handlers map[string]HandlePod) {
	podInput1 := podDemo.DeepCopy()
	podInput2 := podDemo.DeepCopy()
	podInput2.Name = "test-pod-2"
	cases := []struct {
		name          string
		getPods       func() []*corev1.Pod
		getSidecarset func() *appsv1alpha1.SidecarSet
		// pod.name -> infos []string{Image, Env, volumeMounts}
		expectedInfo map[*corev1.Pod][]string
		// MatchedPods, UpdatedPods, ReadyPods, AvailablePods, UnavailablePods
		expectedStatus []int32
	}{
		{
			name: "sidecarset update pod test-pod-1",
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					podInput1.DeepCopy(), podInput2.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v1"},
			},
			expectedStatus: []int32{2, 0, 2, 0},
		},
		{
			name: "pod test-pod-1 is upgrading",
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					podInput1.DeepCopy(), podInput2.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v1"},
			},
			expectedStatus: []int32{2, 1, 1, 0},
		},
		{
			name: "pod test-pod-1 upgrade complete, and start update pod test-pod-2",
			getPods: func() []*corev1.Pod {
				pod1 := podInput1.DeepCopy()
				pods := []*corev1.Pod{
					pod1, podInput2.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
			},
			expectedStatus: []int32{2, 1, 2, 1},
		},
		{
			name: "pod test-pod-2 is upgrading",
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					podInput1.DeepCopy(), podInput2.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
			},
			expectedStatus: []int32{2, 2, 1, 1},
		},
		{
			name: "pod test-pod-2 upgrade complete",
			getPods: func() []*corev1.Pod {
				pod2 := podInput2.DeepCopy()
				pods := []*corev1.Pod{
					podInput1.DeepCopy(), pod2,
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[*corev1.Pod][]string{
				podInput1: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
				podInput2: {"test-image:v2", "nginx-env", "/data/nginx", "test-sidecarset"},
			},
			expectedStatus: []int32{2, 2, 2, 2},
		},
	}
	exps := expectations.NewUpdateExpectations(sidecarcontrol.GetPodSidecarSetRevision)
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pods := cs.getPods()
			sidecarset := cs.getSidecarset()
			fakeClient := fake.NewFakeClientWithScheme(scheme, sidecarset, pods[0], pods[1])
			processor := NewSidecarSetProcessor(fakeClient, exps, record.NewFakeRecorder(10))
			_, err := processor.UpdateSidecarSet(sidecarset)
			if err != nil {
				t.Errorf("processor update sidecarset failed: %s", err.Error())
			}

			for pod, infos := range cs.expectedInfo {
				podOutput, err := getLatestPod(fakeClient, pod)
				if err != nil {
					t.Errorf("get latest pod(%s) failed: %s", pod.Name, err.Error())
				}
				sidecarContainer := &podOutput.Spec.Containers[1]
				if infos[0] != sidecarContainer.Image {
					t.Fatalf("expect pod(%s) container(%s) image(%s), but get image(%s)", pod.Name, sidecarContainer.Name, infos[0], sidecarContainer.Image)
				}
				if len(infos) >= 2 && util.GetContainerEnvVar(sidecarContainer, infos[1]) == nil {
					t.Fatalf("expect pod(%s) container(%s) env(%s), but get nil", pod.Name, sidecarContainer.Name, infos[1])
				}
				if len(infos) >= 3 && util.GetContainerVolumeMount(sidecarContainer, infos[2]) == nil {
					t.Fatalf("expect pod(%s) container(%s) volumeMounts(%s), but get nil", pod.Name, sidecarContainer.Name, infos[2])
				}
				if len(infos) >= 4 && podOutput.Annotations[sidecarcontrol.SidecarSetListAnnotation] != infos[3] {
					t.Fatalf("expect pod(%s) annotations[%s]=%s, but get %s", pod.Name, sidecarcontrol.SidecarSetListAnnotation, infos[3], podOutput.Annotations[sidecarcontrol.SidecarSetListAnnotation])
				}
				if pod.Name == "test-pod-1" {
					podInput1 = podOutput
				} else {
					podInput2 = podOutput
				}
			}

			sidecarsetOutput, err := getLatestSidecarSet(fakeClient, sidecarset)
			if err != nil {
				t.Errorf("get latest sidecarset(%s) failed: %s", sidecarset.Name, err.Error())
			}
			sidecarSetInput = sidecarsetOutput
			for k, v := range cs.expectedStatus {
				var actualValue int32
				switch k {
				case 0:
					actualValue = sidecarsetOutput.Status.MatchedPods
				case 1:
					actualValue = sidecarsetOutput.Status.UpdatedPods
				case 2:
					actualValue = sidecarsetOutput.Status.ReadyPods
				case 3:
					actualValue = sidecarsetOutput.Status.UpdatedReadyPods
				default:
					//
				}

				if v != actualValue {
					t.Fatalf("except sidecarset status(%d:%d), but get value(%d)", k, v, actualValue)
				}
			}
			//handle potInput
			if handle, ok := handlers[cs.name]; ok {
				handle([]*corev1.Pod{podInput1, podInput2})
			}
		})
	}
}

func TestScopeNamespacePods(t *testing.T) {
	sidecarSet := sidecarSetDemo.DeepCopy()
	sidecarSet.Spec.Namespace = "test-ns"
	fakeClient := fake.NewFakeClientWithScheme(scheme, sidecarSet)
	for i := 0; i < 100; i++ {
		pod := podDemo.DeepCopy()
		pod.Name = fmt.Sprintf("%s-%d", pod.Name, i)
		if i >= 50 {
			pod.Namespace = "test-ns"
		}
		fakeClient.Create(context.TODO(), pod)
	}
	exps := expectations.NewUpdateExpectations(sidecarcontrol.GetPodSidecarSetRevision)
	processor := NewSidecarSetProcessor(fakeClient, exps, record.NewFakeRecorder(10))
	pods, err := processor.getMatchingPods(sidecarSet)
	if err != nil {
		t.Fatalf("getMatchingPods failed: %s", err.Error())
		return
	}

	if len(pods) != 50 {
		t.Fatalf("except matching pods count(%d), but get count(%d)", 50, len(pods))
	}
}

func TestCanUpgradePods(t *testing.T) {
	sidecarSet := factorySidecarSet()
	sidecarSet.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = "without-bbb"
	sidecarSet.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
		Type:   intstr.String,
		StrVal: "50%",
	}
	fakeClient := fake.NewFakeClientWithScheme(scheme, sidecarSet)
	pods := factoryPodsCommon(100, 0, sidecarSet)
	exps := expectations.NewUpdateExpectations(sidecarcontrol.GetPodSidecarSetRevision)
	for i := range pods {
		if i < 50 {
			pods[i].Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = `{"test-sidecarset":{"hash":"without-aaa"}}`
		} else {
			pods[i].Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = `{"test-sidecarset":{"hash":"without-bbb"}}`
		}
		fakeClient.Create(context.TODO(), pods[i])
	}

	processor := NewSidecarSetProcessor(fakeClient, exps, record.NewFakeRecorder(10))
	_, err := processor.UpdateSidecarSet(sidecarSet)
	if err != nil {
		t.Errorf("processor update sidecarset failed: %s", err.Error())
	}

	for i := range pods {
		pod := pods[i]
		podOutput, err := getLatestPod(fakeClient, pod)
		if err != nil {
			t.Errorf("get latest pod(%s) failed: %s", pod.Name, err.Error())
		}
		if i < 50 {
			if podOutput.Spec.Containers[1].Image != "test-image:v1" {
				t.Fatalf("except pod(%d) image(test-image:v1), but get image(%s)", i, podOutput.Spec.Containers[1].Image)
			}
		} else {
			if podOutput.Spec.Containers[1].Image != "test-image:v2" {
				t.Fatalf("except pod(%d) image(test-image:v2), but get image(%s)", i, podOutput.Spec.Containers[1].Image)
			}
		}
	}
}
