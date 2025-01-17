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

package util

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestHasOwnerRef(t *testing.T) {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "datadir-foo-id1",
			Labels: map[string]string{
				appsv1alpha1.CloneSetInstanceID: "id1",
				"foo":                           "bar",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "v1",
					Kind:               "Pod",
					Name:               "foo",
					UID:                "test",
					Controller:         func() *bool { a := true; return &a }(),
					BlockOwnerDeletion: func() *bool { a := true; return &a }(),
				},
			},
			ResourceVersion: "1",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("10"),
				},
			},
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
			UID:       "test",
		},
	}

	if !HasOwnerRef(pvc, pod) {
		t.Fatalf("expect pvc %v has pod %v ownerref", pvc, pod)
	}
}

func TestRemoveOwner(t *testing.T) {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "datadir-foo-id1",
			Labels: map[string]string{
				appsv1alpha1.CloneSetInstanceID: "id1",
				"foo":                           "bar",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "v1",
					Kind:               "Pod",
					Name:               "foo",
					UID:                "test",
					Controller:         func() *bool { a := true; return &a }(),
					BlockOwnerDeletion: func() *bool { a := true; return &a }(),
				},
			},
			ResourceVersion: "1",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("10"),
				},
			},
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
			UID:       "test",
		},
	}

	if !RemoveOwnerRef(pvc, pod) {
		t.Fatalf("expect pvc %v to remove pod %v ownerref", pvc, pod)
	}

	if HasOwnerRef(pvc, pod) {
		t.Fatalf("expect pvc %v has no pod %v ownerref", pvc, pod)
	}
}

func TestSetOwnerRef(t *testing.T) {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "datadir-foo-id1",
			Labels: map[string]string{
				appsv1alpha1.CloneSetInstanceID: "id1",
				"foo":                           "bar",
			},
			ResourceVersion: "1",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("10"),
				},
			},
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
			UID:       "test",
		},
	}

	if !SetOwnerRef(pvc, pod, schema.GroupVersionKind{Version: "v1", Kind: "Pod"}) {
		t.Fatalf("expect pvc %v has no pod %v ownerref", pvc, pod)
	}

	if !HasOwnerRef(pvc, pod) {
		t.Fatalf("expect pvc %s has pod %v ownerref", pvc, pod)
	}
}
