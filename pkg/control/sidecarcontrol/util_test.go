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
	"context"
	"encoding/json"
	"reflect"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/configuration"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	sch = runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(sch))
}

var (
	sch *runtime.Scheme

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
			Name: "test-sidecarset",
			Labels: map[string]string{
				"app": "sidecar",
			},
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
				UpdatePodSidecarSetHash(pod, control.GetSidecarset())
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
				UpdatePodSidecarSetHash(pod, control.GetSidecarset())
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
				UpdatePodSidecarSetHash(pod, control.GetSidecarset())
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
				UpdatePodSidecarSetHash(pod, control.GetSidecarset())
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
				UpdatePodSidecarSetHash(pod, control.GetSidecarset())
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
			revision := GetPodSidecarSetRevision("test-sidecarset", cs.getPod())
			if cs.exceptRevision != revision {
				t.Fatalf("except sidecar container test-sidecarset revision %s, but get %s", cs.exceptRevision, revision)
			}
			withoutRevision := GetPodSidecarSetWithoutImageRevision("test-sidecarset", cs.getPod())
			if cs.exceptWithoutImageRevision != withoutRevision {
				t.Fatalf("except sidecar container test-sidecarset WithoutImageRevision %s, but get %s", cs.exceptWithoutImageRevision, withoutRevision)
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
			UpdatePodSidecarSetHash(podInput, sidecarSetInput)
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

func TestConvertDownwardAPIFieldLabel(t *testing.T) {
	testCases := []struct {
		version       string
		label         string
		value         string
		expectedErr   bool
		expectedLabel string
		expectedValue string
	}{
		{
			version:     "v2",
			label:       "metadata.name",
			value:       "test-pod",
			expectedErr: true,
		},
		{
			version:     "v1",
			label:       "invalid-label",
			value:       "value",
			expectedErr: true,
		},
		{
			version:     "v1",
			label:       "metadata.name",
			value:       "test-pod",
			expectedErr: true,
		},
		{
			version:     "v1",
			label:       "metadata.annotations",
			value:       "myAnnoValue",
			expectedErr: true,
		},
		{
			version:     "v1",
			label:       "metadata.labels",
			value:       "myLabelValue",
			expectedErr: true,
		},
		{
			version:       "v1",
			label:         "metadata.annotations['myAnnoKey']",
			value:         "myAnnoValue",
			expectedLabel: "metadata.annotations['myAnnoKey']",
			expectedValue: "myAnnoValue",
		},
		{
			version:       "v1",
			label:         "metadata.labels['myLabelKey']",
			value:         "myLabelValue",
			expectedLabel: "metadata.labels['myLabelKey']",
			expectedValue: "myLabelValue",
		},
	}
	for _, tc := range testCases {
		label, value, err := ConvertDownwardAPIFieldLabel(tc.version, tc.label, tc.value)
		if err != nil {
			if tc.expectedErr {
				continue
			}
			t.Errorf("ConvertDownwardAPIFieldLabel(%s, %s, %s) failed: %s",
				tc.version, tc.label, tc.value, err)
		}
		if tc.expectedLabel != label || tc.expectedValue != value {
			t.Errorf("ConvertDownwardAPIFieldLabel(%s, %s, %s) = (%s, %s, nil), expected (%s, %s, nil)",
				tc.version, tc.label, tc.value, label, value, tc.expectedLabel, tc.expectedValue)
		}
	}
}

func TestExtractContainerNameFromFieldPath(t *testing.T) {
	testCases := []struct {
		fieldSelector *corev1.ObjectFieldSelector
		pod           *corev1.Pod
		expectedErr   bool
		expectedName  string
	}{
		{
			fieldSelector: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.labels['test-label']",
			},
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod-label",
				Labels: map[string]string{
					"test-label": "test-pod-label",
				},
			}},
			expectedName: "test-pod-label",
		},
		{
			fieldSelector: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.labels['test-label']",
			},
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod-label",
			}},
			expectedName: "",
		},
		{
			fieldSelector: &corev1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.annotations['test-anno']",
			},
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod-anno",
				Annotations: map[string]string{
					"test-anno": "test-pod-anno",
				},
			}},
			expectedName: "test-pod-anno",
		},
	}
	for _, tc := range testCases {
		containerName, err := ExtractContainerNameFromFieldPath(tc.fieldSelector, tc.pod)
		if err != nil {
			if tc.expectedErr {
				continue
			}
			t.Errorf("ExtractContainerNameFromFieldPath(%s, %s) failed: %s",
				tc.fieldSelector.FieldPath, tc.expectedName, err)
		}
		if tc.expectedName != containerName {
			t.Errorf("ExtractContainerNameFromFieldPath (%s, %s), expected (%s)",
				tc.fieldSelector.FieldPath, containerName, tc.expectedName)
		}
	}
}

func TestGetSidecarTransferEnvs(t *testing.T) {
	testCases := []struct {
		sidecarContainer *appsv1alpha1.SidecarContainer
		pod              *corev1.Pod
		expectedEnvs     []corev1.EnvVar
	}{
		{
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "cold-sidecar",
					Image: "cold-image:v1",
				},
				UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
					UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
				},
				TransferEnv: []appsv1alpha1.TransferEnvVar{
					{
						EnvName:             "test-env",
						SourceContainerName: "main",
					},
				},
			},
			pod: &corev1.Pod{
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
							Env: []corev1.EnvVar{
								{
									Name:  "test-env",
									Value: "test-value",
								},
							},
						},
					},
				},
			},
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "test-env",
					Value: "test-value",
				},
			},
		},
		{
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "cold-sidecar",
					Image: "cold-image:v1",
				},
				UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
					UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
				},
				TransferEnv: []appsv1alpha1.TransferEnvVar{
					{
						EnvName: "test-env",
						SourceContainerNameFrom: &appsv1alpha1.SourceContainerNameSource{
							FieldRef: &corev1.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.labels['app']",
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations:     map[string]string{},
					Name:            "test-pod-1",
					Namespace:       "default",
					Labels:          map[string]string{"app": "main"},
					ResourceVersion: "495711227",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "main:v1",
							Env: []corev1.EnvVar{
								{
									Name:  "test-env",
									Value: "test-value",
								},
							},
						},
					},
				},
			},
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "test-env",
					Value: "test-value",
				},
			},
		},
		{
			sidecarContainer: &appsv1alpha1.SidecarContainer{
				Container: corev1.Container{
					Name:  "cold-sidecar",
					Image: "cold-image:v1",
				},
				UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
					UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
				},
				TransferEnv: []appsv1alpha1.TransferEnvVar{
					{
						EnvName: "test-env",
						SourceContainerNameFrom: &appsv1alpha1.SourceContainerNameSource{
							FieldRef: &corev1.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.annotations['app']",
							},
						},
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations:     map[string]string{"app": "main"},
					Name:            "test-pod-1",
					Namespace:       "default",
					Labels:          map[string]string{"app": "main"},
					ResourceVersion: "495711227",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "main:v1",
							Env: []corev1.EnvVar{
								{
									Name:  "test-env",
									Value: "test-value",
								},
							},
						},
					},
				},
			},
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "test-env",
					Value: "test-value",
				},
			},
		},
	}
	for _, tc := range testCases {
		injectedEnvs := GetSidecarTransferEnvs(tc.sidecarContainer, tc.pod)
		if len(injectedEnvs) != len(tc.expectedEnvs) {
			t.Errorf("GetSidecarTransferEnv failed, expected envs %s, got %s",
				tc.expectedEnvs, injectedEnvs)
		}
	}
}

func TestPatchPodMetadata(t *testing.T) {
	cases := []struct {
		name              string
		getPod            func() *corev1.Pod
		patches           func() []appsv1alpha1.SidecarSetPatchPodMetadata
		expectAnnotations map[string]string
		expectErr         bool
		skip              bool
	}{
		{
			name: "add pod annotation",
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{}
				return demo
			},
			patches: func() []appsv1alpha1.SidecarSetPatchPodMetadata {
				patch := []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetRetainPatchPolicy,
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
					{
						PatchPolicy: appsv1alpha1.SidecarSetOverwritePatchPolicy,
						Annotations: map[string]string{
							"key2": "value2",
						},
					},
				}
				return patch
			},
			expectAnnotations: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			skip:      false,
			expectErr: false,
		},
		{
			name: "add pod annotation, exist",
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key1": "old",
							"key2": "old",
						},
					},
				}
				return demo
			},
			patches: func() []appsv1alpha1.SidecarSetPatchPodMetadata {
				patch := []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetRetainPatchPolicy,
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
					{
						PatchPolicy: appsv1alpha1.SidecarSetOverwritePatchPolicy,
						Annotations: map[string]string{
							"key2": "value2",
						},
					},
				}
				return patch
			},
			expectAnnotations: map[string]string{
				"key1": "old",
				"key2": "value2",
			},
			skip:      false,
			expectErr: false,
		},
		{
			name: "json merge pod annotation",
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key1": `{"log-agent":1}`,
							"key2": `{"log-agent":1}`,
						},
					},
				}
				return demo
			},
			patches: func() []appsv1alpha1.SidecarSetPatchPodMetadata {
				patch := []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy,
						Annotations: map[string]string{
							"key1": `{"log-agent":1}`,
							"key2": `{"envoy":2}`,
							"key3": `{"probe":5}`,
						},
					},
				}
				return patch
			},
			expectAnnotations: map[string]string{
				"key1": `{"log-agent":1}`,
				"key2": `{"envoy":2,"log-agent":1}`,
				"key3": `{"probe":5}`,
			},
			skip:      false,
			expectErr: false,
		},
		{
			name: "json merge pod annotation, skip",
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"key1": `{"log-agent":1}`,
						},
					},
				}
				return demo
			},
			patches: func() []appsv1alpha1.SidecarSetPatchPodMetadata {
				patch := []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy,
						Annotations: map[string]string{
							"key1": `{"log-agent":1}`,
						},
					},
				}
				return patch
			},
			expectAnnotations: map[string]string{
				"key1": `{"log-agent":1}`,
			},
			skip:      true,
			expectErr: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pod := cs.getPod()
			skip, err := PatchPodMetadata(&pod.ObjectMeta, cs.patches())
			if cs.expectErr && err == nil {
				t.Fatalf("PatchPodMetadata failed")
			} else if !cs.expectErr && err != nil {
				t.Fatalf("PatchPodMetadata failed: %v", err)
			} else if skip != cs.skip {
				t.Fatalf("expect %v, but get %v", cs.skip, skip)
			} else if !reflect.DeepEqual(cs.expectAnnotations, pod.Annotations) {
				t.Fatalf("expect %v, but get %v", cs.expectAnnotations, pod.Annotations)
			}
		})
	}
}

func TestValidateSidecarSetPatchMetadataWhitelist(t *testing.T) {
	cases := []struct {
		name          string
		getSidecarSet func() *appsv1alpha1.SidecarSet
		getKruiseCM   func() *corev1.ConfigMap
		expectErr     bool
	}{
		{
			name: "validate sidecarSet no patch Metadata",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				return nil
			},
			expectErr: false,
		},
		{
			name: "validate sidecarSet whitelist failed-1",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				return nil
			},
			expectErr: true,
		},
		{
			name: "validate sidecarSet whitelist success-1",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				demo := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"allowedAnnotationKeyExprs":["key.*"]}]}`,
					},
				}
				return demo
			},
			expectErr: false,
		},
		{
			name: "validate sidecarSet whitelist failed-2",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				demo := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"allowedAnnotationKeyExprs":["key2"]}]}`,
					},
				}
				return demo
			},
			expectErr: true,
		},
		{
			name: "validate sidecarSet whitelist failed-3",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				demo := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"allowedAnnotationKeyExprs":["key.*"],"selector":{"matchLabels":{"app":"other"}}}]}`,
					},
				}
				return demo
			},
			expectErr: true,
		},
		{
			name: "validate sidecarSet whitelist success-2",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				demo := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"allowedAnnotationKeyExprs":["key.*"],"selector":{"matchLabels":{"app":"sidecar"}}}]}`,
					},
				}
				return demo
			},
			expectErr: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).Build()
			if cs.getKruiseCM() != nil {
				fakeClient.Create(context.TODO(), cs.getKruiseCM())
			}
			err := ValidateSidecarSetPatchMetadataWhitelist(fakeClient, cs.getSidecarSet())
			if cs.expectErr && err == nil {
				t.Fatalf("ValidateSidecarSetPatchMetadataWhitelist failed")
			} else if !cs.expectErr && err != nil {
				t.Fatalf("ValidateSidecarSetPatchMetadataWhitelist failed: %s", err.Error())
			}
		})
	}
}

func TestPodMatchedSidecarSet(t *testing.T) {
	cases := []struct {
		name          string
		getSidecarSet func() *appsv1alpha1.SidecarSet
		getPod        func() *corev1.Pod
		getNs         func() []*corev1.Namespace
		expect        bool
	}{
		{
			name: "test1",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := &appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset-test"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
					},
				}
				return demo
			},
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Labels:    map[string]string{"app": "nginx"},
						Namespace: "app1",
					},
				}
				return demo
			},
			getNs: func() []*corev1.Namespace {
				return nil
			},
			expect: true,
		},
		{
			name: "test2",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := &appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset-test"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						Namespace: "app1",
					},
				}
				return demo
			},
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Labels:    map[string]string{"app": "nginx"},
						Namespace: "app1",
					},
				}
				return demo
			},
			getNs: func() []*corev1.Namespace {
				return nil
			},
			expect: true,
		},
		{
			name: "test3",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := &appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset-test"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						Namespace: "app2",
					},
				}
				return demo
			},
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Labels:    map[string]string{"app": "nginx"},
						Namespace: "app1",
					},
				}
				return demo
			},
			getNs: func() []*corev1.Namespace {
				return nil
			},
			expect: false,
		},
		{
			name: "test4",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := &appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset-test"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "app1"},
						},
					},
				}
				return demo
			},
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Labels:    map[string]string{"app": "nginx"},
						Namespace: "app1",
					},
				}
				return demo
			},
			getNs: func() []*corev1.Namespace {
				demo := []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "app1",
							Labels: map[string]string{"app": "app1"},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "app2",
							Labels: map[string]string{"app": "app2"},
						},
					},
				}
				return demo
			},
			expect: true,
		},
		{
			name: "test5",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := &appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset-test"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "app2"},
						},
					},
				}
				return demo
			},
			getPod: func() *corev1.Pod {
				demo := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Labels:    map[string]string{"app": "nginx"},
						Namespace: "app1",
					},
				}
				return demo
			},
			getNs: func() []*corev1.Namespace {
				demo := []*corev1.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "app1",
							Labels: map[string]string{"app": "app1"},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "app2",
							Labels: map[string]string{"app": "app2"},
						},
					},
				}
				return demo
			},
			expect: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).Build()
			for _, ns := range cs.getNs() {
				_ = fakeClient.Create(context.TODO(), ns)
			}
			matched, err := PodMatchedSidecarSet(fakeClient, cs.getPod(), cs.getSidecarSet())
			if err != nil {
				t.Fatalf("PodMatchedSidecarSet failed: %s", err.Error())
			}
			if cs.expect != matched {
				t.Fatalf("expect(%v), but get(%v)", cs.expect, matched)
			}
		})
	}
}
