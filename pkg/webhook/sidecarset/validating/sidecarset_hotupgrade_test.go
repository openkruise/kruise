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

package validating

import (
	"fmt"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestSidecarSetUpdateConflict(t *testing.T) {
	oldSidecarset := appsv1alpha1.SidecarSet{
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{Name: "test"},
					UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
						UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
					},
				},
			},
		},
	}
	newSidecarset := &appsv1alpha1.SidecarSet{
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{Name: "test"},
					UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
						UpgradeType: appsv1alpha1.SidecarContainerHotUpgrade,
					},
				},
			},
		},
	}
	allErrs := validateSidecarContainerConflict(oldSidecarset.Spec.Containers, newSidecarset.Spec.Containers, field.NewPath("spec.containers"))
	if len(allErrs) != 1 {
		t.Errorf("expect errors len 1, but got: %v", allErrs)
	} else {
		fmt.Println(allErrs)
	}
}
