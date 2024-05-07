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
	"fmt"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	hotUpgradeEmptyImage   = "hotupgrade:empty"
	hotUpgradeEmptyImageID = "docker-pullable://hotupgrade@sha256:91873971hhf0981293480lhhhgsfgs09809085024lddd092jjj44k4h4vv44"

	sidecarSetHotUpgrade = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 123,
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetHashAnnotation:             "bbb",
				sidecarcontrol.SidecarSetHashWithoutImageAnnotation: "111111111",
			},
			Name:            "test-sidecarset",
			ResourceVersion: "22",
			Labels:          map[string]string{},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "test-sidecar",
						Image: "test-image:v2",
					},
					UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
						HotUpgradeEmptyImage: hotUpgradeEmptyImage,
						UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
					},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			UpdateStrategy:       appsv1alpha1.SidecarSetUpdateStrategy{},
			RevisionHistoryLimit: utilpointer.Int32Ptr(10),
		},
	}

	podHotUpgrade = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				sidecarcontrol.SidecarSetListAnnotation: `test-sidecarset`,
				//hash
				sidecarcontrol.SidecarSetHashAnnotation: `{"test-sidecarset":{"hash":"aaa","sidecarList":["test-sidecar"]}}`,
				//111111111
				sidecarcontrol.SidecarSetHashWithoutImageAnnotation: `{"test-sidecarset":{"hash":"111111111"}}`,
				//sidecar version
				sidecarcontrol.GetPodSidecarSetVersionAnnotation("test-sidecar-1"): "1",
				sidecarcontrol.GetPodSidecarSetVersionAnnotation("test-sidecar-2"): "1",
				//working sidecar container
				sidecarcontrol.SidecarSetWorkingHotUpgradeContainer: `{"test-sidecar":"test-sidecar-1"}`,
			},
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "nginx"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
				{
					Name:  "test-sidecar-1",
					Image: "test-image:v1",
					Env: []corev1.EnvVar{
						{
							Name: sidecarcontrol.SidecarSetVersionEnvKey,
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: fmt.Sprintf("metadata.annotations['%s']", sidecarcontrol.GetPodSidecarSetVersionAnnotation("test-sidecar-1")),
								},
							},
						},
					},
				},
				{
					Name:  "test-sidecar-2",
					Image: hotUpgradeEmptyImage,
					Env: []corev1.EnvVar{
						{
							Name: sidecarcontrol.SidecarSetVersionEnvKey,
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: fmt.Sprintf("metadata.annotations['%s']", sidecarcontrol.GetPodSidecarSetVersionAnnotation("test-sidecar-2")),
								},
							},
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:    "nginx",
					Image:   "nginx:1.15.1",
					ImageID: "docker-pullable://nginx@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
					Ready:   true,
				},
				{
					Name:    "test-sidecar-1",
					Image:   "test-image:v1",
					ImageID: testImageV1ImageID,
					Ready:   true,
				},
				{
					Name:    "test-sidecar-2",
					Image:   hotUpgradeEmptyImage,
					ImageID: hotUpgradeEmptyImageID,
					Ready:   true,
				},
			},
		},
	}
)

func TestUpdateHotUpgradeSidecar(t *testing.T) {
	sidecarSetInput := sidecarSetHotUpgrade.DeepCopy()
	handlers := map[string]HandlePod{
		"test-sidecar-2 container is upgrading": func(pods []*corev1.Pod) {
			pods[0].Status.ContainerStatuses[2].Image = "test-image:v2"
			pods[0].Status.ContainerStatuses[2].ImageID = testImageV2ImageID
		},
		"test-sidecar-2 container upgrade complete, and reset test-sidecar-1 empty image": func(pods []*corev1.Pod) {
			pods[0].Status.ContainerStatuses[1].Image = hotUpgradeEmptyImage
			pods[0].Status.ContainerStatuses[1].ImageID = hotUpgradeEmptyImageID
		},
	}
	testUpdateHotUpgradeSidecar(t, hotUpgradeEmptyImage, sidecarSetInput, handlers)
}

func testUpdateHotUpgradeSidecar(t *testing.T, hotUpgradeEmptyImage string, sidecarSetInput *appsv1alpha1.SidecarSet, handlers map[string]HandlePod) {
	podInput := podHotUpgrade.DeepCopy()
	podInput.Name = "liheng-test"
	cases := []struct {
		name          string
		getPods       func() []*corev1.Pod
		getSidecarset func() *appsv1alpha1.SidecarSet
		// container.name -> infos []string
		expectedInfo map[string][]string
		// MatchedPods, UpdatedPods, ReadyPods, AvailablePods, UnavailablePods
		expectedStatus []int32
	}{
		{
			name: "sidecarset hot update test-sidecar container test-image:v2",
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					podInput.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[string][]string{
				"test-sidecar-1": {"test-image:v1"},
				"test-sidecar-2": {"test-image:v2"},
			},
			expectedStatus: []int32{1, 0, 1, 0},
		},
		{
			name: "test-sidecar-2 container is upgrading",
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					podInput.DeepCopy(),
				}
				return pods
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[string][]string{
				"test-sidecar-1": {"test-image:v1"},
				"test-sidecar-2": {"test-image:v2"},
			},
			expectedStatus: []int32{1, 1, 0, 0},
		},
		{
			name: "test-sidecar-2 container upgrade complete, and reset test-sidecar-1 empty image",
			getPods: func() []*corev1.Pod {
				return []*corev1.Pod{podInput.DeepCopy()}
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[string][]string{
				"test-sidecar-1": {hotUpgradeEmptyImage},
				"test-sidecar-2": {"test-image:v2"},
			},
			expectedStatus: []int32{1, 1, 0, 0},
		},
		{
			name: "sidecarset hot update test-sidecar container test-image:v2 complete",
			getPods: func() []*corev1.Pod {
				return []*corev1.Pod{podInput.DeepCopy()}
			},
			getSidecarset: func() *appsv1alpha1.SidecarSet {
				return sidecarSetInput.DeepCopy()
			},
			expectedInfo: map[string][]string{
				"test-sidecar-1": {hotUpgradeEmptyImage},
				"test-sidecar-2": {"test-image:v2"},
			},
			expectedStatus: []int32{1, 1, 1, 1},
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pod := cs.getPods()[0]
			sidecarset := cs.getSidecarset()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidecarset, pod).
				WithStatusSubresource(&appsv1alpha1.SidecarSet{}).Build()
			processor := NewSidecarSetProcessor(fakeClient, record.NewFakeRecorder(10))
			_, err := processor.UpdateSidecarSet(sidecarset)
			if err != nil {
				t.Errorf("processor update sidecarset failed: %s", err.Error())
			}
			podOutput, err := getLatestPod(fakeClient, pod)
			if err != nil {
				t.Errorf("get latest pod(%s) failed: %s", pod.Name, err.Error())
			}
			podInput = podOutput.DeepCopy()
			for cName, infos := range cs.expectedInfo {
				sidecarContainer := util.GetPodContainerByName(cName, podOutput)
				if infos[0] != sidecarContainer.Image {
					t.Fatalf("expect pod(%s) container(%s) image(%s), but get image(%s)", pod.Name, sidecarContainer.Name, infos[0], sidecarContainer.Image)
				}
			}

			sidecarsetOutput, err := getLatestSidecarSet(fakeClient, sidecarset)
			if err != nil {
				t.Errorf("get latest sidecarset(%s) failed: %s", sidecarset.Name, err.Error())
			}
			sidecarSetInput = sidecarsetOutput.DeepCopy()
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
				}

				if v != actualValue {
					t.Fatalf("except sidecarset status(%d:%d), but get value(%d)", k, v, actualValue)
				}
			}
			//handle potInput
			if handle, ok := handlers[cs.name]; ok {
				handle([]*corev1.Pod{podInput})
			}
		})
	}
}
