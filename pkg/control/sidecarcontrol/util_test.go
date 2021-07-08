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

package sidecarcontrol

import (
	"encoding/json"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// image.Name -> image.Id
	ImageIds = map[string]string{
		"main:v1":          "4120593193b4",
		"cold-sidecar:v1":  "docker-pullable://cold-sidecar@sha256:9ead06a1362e",
		"cold-sidecar:v2":  "docker-pullable://cold-sidecar@sha256:7223aa0f3a7a",
		"hot-sidecar:v1":   "docker-pullable://hot-sidecar@sha256:86618128c92e",
		"hot-sidecar:v2":   "docker-pullable://hot-sidecar@sha256:74abd85af1e9",
		"hotupgrade:empty": "docker-pullable://hotupgrade@sha256:0e9daf5c02e7",
	}

	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations:     map[string]string{},
			Name:            "test-pod-1",
			Namespace:       "default",
			Labels:          map[string]string{"app": "nginx"},
			ResourceVersion: "495711227",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "main:v1",
				},
				{
					Name:  "cold-sidecar",
					Image: "cold-sidecar:v1",
				},
				{
					Name:  "hot-sidecar-1",
					Image: "hot-sidecar:v1",
				},
				{
					Name:  "hot-sidecar-2",
					Image: "hotupgrade:empty",
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
					Name:    "main",
					Image:   "main:v1",
					ImageID: ImageIds["main:v1"],
					Ready:   true,
				},
				{
					Name:    "cold-sidecar",
					Image:   "cold-sidecar:v1",
					ImageID: ImageIds["cold-sidecar:v1"],
					Ready:   true,
				},
				{
					Name:    "hot-sidecar-1",
					Image:   "hot-sidecar:v1",
					ImageID: ImageIds["hot-sidecar:v1"],
					Ready:   true,
				},
				{
					Name:    "hot-sidecar-2",
					Image:   "hotupgrade:empty",
					ImageID: ImageIds["hotupgrade:empty"],
					Ready:   true,
				},
			},
		},
	}

	sidecarSetDemo = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 123,
			Annotations: map[string]string{
				SidecarSetHashAnnotation:             "bbb",
				SidecarSetHashWithoutImageAnnotation: "without-image-aaa",
			},
			Name:   "test-sidecarset",
			Labels: map[string]string{},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "cold-sidecar",
						Image: "cold-image:v1",
					},
					UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
						UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
					},
				},
				{
					Container: corev1.Container{
						Name:  "hot-sidecar",
						Image: "hot-image:v1",
					},
					UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
						UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
						HotUpgradeEmptyImage: "hotupgrade:empty",
					},
				},
			},
		},
	}
)

func TestIsSidecarContainerUpdateCompleted(t *testing.T) {
	cases := []struct {
		name              string
		getPod            func() *corev1.Pod
		upgradeSidecars   func() (sets.String, sets.String)
		expectedCompleted bool
	}{
		{
			name: "only inject sidecar, not upgrade",
			getPod: func() *corev1.Pod {
				return podDemo.DeepCopy()
			},
			upgradeSidecars: func() (sets.String, sets.String) {
				return sets.NewString(sidecarSetDemo.Name), sets.NewString("cold-sidecar", "hot-sidecar-1", "hot-sidecar-2")
			},
			expectedCompleted: true,
		},
		{
			name: "upgrade cold sidecar, upgrade not completed",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				control := New(sidecarSetDemo.DeepCopy())
				pod.Spec.Containers[1].Image = "cold-sidecar:v2"
				control.UpdatePodAnnotationsInUpgrade([]string{"cold-sidecar"}, pod)
				return pod
			},
			upgradeSidecars: func() (sets.String, sets.String) {
				return sets.NewString(sidecarSetDemo.Name), sets.NewString("cold-sidecar", "hot-sidecar-1", "hot-sidecar-2")
			},
			expectedCompleted: false,
		},
		{
			name: "upgrade cold sidecar, upgrade completed",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				control := New(sidecarSetDemo.DeepCopy())
				pod.Spec.Containers[1].Image = "cold-sidecar:v2"
				control.UpdatePodAnnotationsInUpgrade([]string{"cold-sidecar"}, pod)
				pod.Status.ContainerStatuses[1].ImageID = ImageIds["cold-sidecar:v2"]
				return pod
			},
			upgradeSidecars: func() (sets.String, sets.String) {
				return sets.NewString(sidecarSetDemo.Name), sets.NewString("cold-sidecar", "hot-sidecar-1", "hot-sidecar-2")
			},
			expectedCompleted: true,
		},
		{
			name: "upgrade hot sidecar, upgrade hot-sidecar-2 not completed",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				control := New(sidecarSetDemo.DeepCopy())
				// upgrade cold sidecar completed
				pod.Spec.Containers[1].Image = "cold-sidecar:v2"
				control.UpdatePodAnnotationsInUpgrade([]string{"cold-sidecar"}, pod)
				pod.Status.ContainerStatuses[1].ImageID = ImageIds["cold-sidecar:v2"]
				// start upgrading hot sidecar
				pod.Spec.Containers[3].Image = "hot-sidecar:v2"
				control.UpdatePodAnnotationsInUpgrade([]string{"hot-sidecar-2"}, pod)
				return pod
			},
			upgradeSidecars: func() (sets.String, sets.String) {
				return sets.NewString(sidecarSetDemo.Name), sets.NewString("cold-sidecar", "hot-sidecar-1", "hot-sidecar-2")
			},
			expectedCompleted: false,
		},
		{
			name: "upgrade hot sidecar, upgrade hot-sidecar-1 not completed",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				control := New(sidecarSetDemo.DeepCopy())
				// upgrade cold sidecar completed
				pod.Spec.Containers[1].Image = "cold-sidecar:v2"
				control.UpdatePodAnnotationsInUpgrade([]string{"cold-sidecar"}, pod)
				pod.Status.ContainerStatuses[1].ImageID = ImageIds["cold-sidecar:v2"]
				// start upgrading hot sidecar
				pod.Spec.Containers[3].Image = "hot-sidecar:v2"
				control.UpdatePodAnnotationsInUpgrade([]string{"hot-sidecar-2"}, pod)
				pod.Status.ContainerStatuses[3].ImageID = ImageIds["hot-sidecar:v2"]
				pod.Spec.Containers[2].Image = "hotupgrade:empty"
				control.UpdatePodAnnotationsInUpgrade([]string{"hot-sidecar-1"}, pod)
				return pod
			},
			upgradeSidecars: func() (sets.String, sets.String) {
				return sets.NewString(sidecarSetDemo.Name), sets.NewString("cold-sidecar", "hot-sidecar-1", "hot-sidecar-2")
			},
			expectedCompleted: false,
		},
		{
			name: "upgrade hot sidecar, upgrade hot-sidecar completed",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				control := New(sidecarSetDemo.DeepCopy())
				// upgrade cold sidecar completed
				pod.Spec.Containers[1].Image = "cold-sidecar:v2"
				control.UpdatePodAnnotationsInUpgrade([]string{"cold-sidecar"}, pod)
				pod.Status.ContainerStatuses[1].ImageID = ImageIds["cold-sidecar:v2"]
				// start upgrading hot sidecar
				pod.Spec.Containers[3].Image = "hot-sidecar:v2"
				control.UpdatePodAnnotationsInUpgrade([]string{"hot-sidecar-2"}, pod)
				pod.Status.ContainerStatuses[3].ImageID = ImageIds["hot-sidecar:v2"]
				pod.Spec.Containers[2].Image = "hotupgrade:empty"
				control.UpdatePodAnnotationsInUpgrade([]string{"hot-sidecar-1"}, pod)
				pod.Status.ContainerStatuses[2].ImageID = ImageIds["hotupgrade:empty"]
				return pod
			},
			upgradeSidecars: func() (sets.String, sets.String) {
				return sets.NewString(sidecarSetDemo.Name), sets.NewString("cold-sidecar", "hot-sidecar-1", "hot-sidecar-2")
			},
			expectedCompleted: true,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pod := cs.getPod()
			sidecarSets, containers := cs.upgradeSidecars()
			if IsSidecarContainerUpdateCompleted(pod, sidecarSets, containers) != cs.expectedCompleted {
				t.Fatalf("IsSidecarContainerUpdateCompleted failed: %s", cs.name)
			}
		})
	}
}

func TestGetPodSidecarSetRevision(t *testing.T) {
	cases := []struct {
		name   string
		getPod func() *corev1.Pod
		//sidecarContainer -> sidecarSet.Revision
		exceptRevision             string
		exceptWithoutImageRevision string
	}{
		{
			name: "normal sidecarSet revision",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[SidecarSetHashAnnotation] = `{"test-sidecarset":{"hash":"aaa"}}`
				pod.Annotations[SidecarSetHashWithoutImageAnnotation] = `{"test-sidecarset":{"hash":"without-image-aaa"}}`
				return pod
			},
			exceptRevision:             "aaa",
			exceptWithoutImageRevision: "without-image-aaa",
		},
		{
			name: "older sidecarSet revision",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[SidecarSetHashAnnotation] = `{"test-sidecarset": "aaa"}`
				pod.Annotations[SidecarSetHashWithoutImageAnnotation] = `{"test-sidecarset": "without-image-aaa"}`
				return pod
			},
			exceptRevision:             "aaa",
			exceptWithoutImageRevision: "without-image-aaa",
		},
		{
			name: "failed sidecarSet revision",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[SidecarSetHashAnnotation] = "failed-sidecarset-hash"
				pod.Annotations[SidecarSetHashWithoutImageAnnotation] = "failed-sidecarset-hash"
				return pod
			},
			exceptRevision:             "",
			exceptWithoutImageRevision: "",
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			revison := GetPodSidecarSetRevision("test-sidecarset", cs.getPod())
			if cs.exceptRevision != revison {
				t.Fatalf("except sidecar container test-sidecarset revison %s, but get %s", cs.exceptRevision, revison)
			}
			withoutRevison := GetPodSidecarSetWithoutImageRevision("test-sidecarset", cs.getPod())
			if cs.exceptWithoutImageRevision != withoutRevison {
				t.Fatalf("except sidecar container test-sidecarset WithoutImageRevision %s, but get %s", cs.exceptWithoutImageRevision, withoutRevison)
			}
		})
	}
}

func TestUpdatePodSidecarSetHash(t *testing.T) {
	cases := []struct {
		name                       string
		getPod                     func() *corev1.Pod
		getSidecarSet              func() *appsv1alpha1.SidecarSet
		exceptRevision             map[string]SidecarSetUpgradeSpec
		exceptWithoutImageRevision map[string]SidecarSetUpgradeSpec
	}{
		{
			name: "normal sidecarSet revision",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[SidecarSetHashAnnotation] = `{"test-sidecarset":{"hash":"aaa"}}`
				pod.Annotations[SidecarSetHashWithoutImageAnnotation] = `{"test-sidecarset":{"hash":"without-image-aaa"}}`
				return pod
			},
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return sidecarSetDemo.DeepCopy()
			},
			exceptRevision: map[string]SidecarSetUpgradeSpec{
				"test-sidecarset": {
					SidecarSetHash: "bbb",
				},
			},
			exceptWithoutImageRevision: map[string]SidecarSetUpgradeSpec{
				"test-sidecarset": {
					SidecarSetHash: "without-image-aaa",
				},
			},
		},
		{
			name: "older sidecarSet revision",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[SidecarSetHashAnnotation] = `{"test-sidecarset": "aaa"}`
				pod.Annotations[SidecarSetHashWithoutImageAnnotation] = `{"test-sidecarset": "without-image-aaa"}`
				return pod
			},
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return sidecarSetDemo.DeepCopy()
			},
			exceptRevision: map[string]SidecarSetUpgradeSpec{
				"test-sidecarset": {
					SidecarSetHash: "bbb",
				},
			},
			exceptWithoutImageRevision: map[string]SidecarSetUpgradeSpec{
				"test-sidecarset": {
					SidecarSetHash: "without-image-aaa",
				},
			},
		},
		{
			name: "failed sidecarSet revision",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[SidecarSetHashAnnotation] = "failed-sidecarset-hash"
				pod.Annotations[SidecarSetHashWithoutImageAnnotation] = "failed-sidecarset-hash"
				return pod
			},
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return sidecarSetDemo.DeepCopy()
			},
			exceptRevision: map[string]SidecarSetUpgradeSpec{
				"test-sidecarset": {
					SidecarSetHash: "bbb",
				},
			},
			exceptWithoutImageRevision: map[string]SidecarSetUpgradeSpec{},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			podInput := cs.getPod()
			sidecarSetInput := cs.getSidecarSet()
			updatePodSidecarSetHash(podInput, sidecarSetInput)
			// sidecarSet hash
			sidecarSetHash := make(map[string]SidecarSetUpgradeSpec)
			err := json.Unmarshal([]byte(podInput.Annotations[SidecarSetHashAnnotation]), &sidecarSetHash)
			if err != nil {
				t.Fatalf("parse pod sidecarSet hash failed: %s", err.Error())
			}
			for k, o := range sidecarSetHash {
				eo := cs.exceptRevision[k]
				if o.SidecarSetHash != eo.SidecarSetHash {
					t.Fatalf("except sidecar container %s revision %s, but get revision %s", k, eo.SidecarSetHash, o.SidecarSetHash)
				}
			}
			if len(cs.exceptWithoutImageRevision) == 0 {
				return
			}
			// without image sidecarSet hash
			sidecarSetHash = make(map[string]SidecarSetUpgradeSpec)
			err = json.Unmarshal([]byte(podInput.Annotations[SidecarSetHashWithoutImageAnnotation]), &sidecarSetHash)
			if err != nil {
				t.Fatalf("parse pod sidecarSet hash failed: %s", err.Error())
			}
			for k, o := range sidecarSetHash {
				eo := cs.exceptWithoutImageRevision[k]
				if o.SidecarSetHash != eo.SidecarSetHash {
					t.Fatalf("except sidecar container %s revision %s, but get revision %s", k, eo.SidecarSetHash, o.SidecarSetHash)
				}
			}
		})
	}
}
