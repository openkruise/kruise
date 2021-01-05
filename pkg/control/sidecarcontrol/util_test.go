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
)

var (
	podDemo = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations:     map[string]string{},
			Name:            "test-pod-1",
			Namespace:       "default",
			Labels:          map[string]string{"app": "nginx"},
			ResourceVersion: "495711227",
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
	}
)

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
