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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getCoreVolumes(volumes []v1.Volume, fldPath *field.Path) ([]core.Volume, field.ErrorList) {
	allErrs := field.ErrorList{}

	var coreVolumes []core.Volume
	for _, volume := range volumes {
		coreVolume := core.Volume{}
		if err := corev1.Convert_v1_Volume_To_core_Volume(&volume, &coreVolume, nil); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Root(), volume, fmt.Sprintf("Convert_v1_Volume_To_core_Volume failed: %v", err)))
			return nil, allErrs
		}
		coreVolumes = append(coreVolumes, coreVolume)
	}

	return coreVolumes, allErrs
}

func isSidecarSetNamespaceOverlapping(c client.Client, origin *appsv1alpha1.SidecarSet, other *appsv1alpha1.SidecarSet) bool {
	originNamespace := origin.Spec.Namespace
	otherNamespace := other.Spec.Namespace
	if originNamespace != "" && otherNamespace != "" && originNamespace != otherNamespace {
		return false
	}
	originSelector := origin.Spec.NamespaceSelector
	otherSelector := other.Spec.NamespaceSelector
	if originSelector != nil && otherSelector != nil && !util.IsSelectorOverlapping(originSelector, otherSelector) {
		return false
	}
	if originNamespace != "" && otherSelector != nil && !sidecarcontrol.IsSelectorNamespace(c, originNamespace, otherSelector) {
		return false
	}
	if otherNamespace != "" && originSelector != nil && !sidecarcontrol.IsSelectorNamespace(c, otherNamespace, otherSelector) {
		return false
	}
	return true
}
