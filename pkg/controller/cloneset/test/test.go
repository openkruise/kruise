/*
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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

package test

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// NewPVC for unit tests.
func NewPVC(name string) v1.PersistentVolumeClaim {
	return v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: *resource.NewQuantity(10, resource.BinarySI),
				},
			},
		},
	}
}

// NewCloneSetWithVolumes for unit tests.
func NewCloneSetWithVolumes(replicas int, name string, petMounts []v1.VolumeMount, podMounts []v1.VolumeMount) *appsv1alpha1.CloneSet {
	mounts := append(petMounts, podMounts...)
	var claims []v1.PersistentVolumeClaim
	for _, m := range petMounts {
		claims = append(claims, NewPVC(m.Name))
	}

	var vols []v1.Volume
	for _, m := range podMounts {
		vols = append(vols, v1.Volume{
			Name: m.Name,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: fmt.Sprintf("/tmp/%v", m.Name),
				},
			},
		})
	}

	template := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:         "nginx",
					Image:        "nginx",
					VolumeMounts: mounts,
				},
			},
			Volumes: vols,
		},
	}

	template.Labels = map[string]string{"foo": "bar"}

	return &appsv1alpha1.CloneSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneSet",
			APIVersion: "apps.kruise.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: v1.NamespaceDefault,
			UID:       types.UID("test"),
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			Replicas:             func() *int32 { i := int32(replicas); return &i }(),
			Template:             template,
			VolumeClaimTemplates: claims,
			UpdateStrategy:       appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.RecreateCloneSetUpdateStrategyType},
			RevisionHistoryLimit: func() *int32 {
				limit := int32(2)
				return &limit
			}(),
		},
	}
}

// NewCloneSet for unit tests.
func NewCloneSet(replicas int) *appsv1alpha1.CloneSet {
	petMounts := []v1.VolumeMount{
		{Name: "datadir", MountPath: "/tmp/data"},
	}
	podMounts := []v1.VolumeMount{
		{Name: "home", MountPath: "/home"},
	}
	return NewCloneSetWithVolumes(replicas, "foo", petMounts, podMounts)
}
