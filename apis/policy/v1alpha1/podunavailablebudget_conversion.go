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

package v1alpha1

import (
	"fmt"

	"github.com/openkruise/kruise/apis/policy/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *PodUnavailableBudget) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.PodUnavailableBudget:
		dst := dstRaw.(*v1beta1.PodUnavailableBudget)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		if src.Spec.TargetReference != nil {
			dst.Spec.TargetReference = &v1beta1.TargetReference{}
			dst.Spec.TargetReference.APIVersion = src.Spec.TargetReference.APIVersion
			dst.Spec.TargetReference.Kind = src.Spec.TargetReference.Kind
			dst.Spec.TargetReference.Name = src.Spec.TargetReference.Name
		}
		dst.Spec.MaxUnavailable = src.Spec.MaxUnavailable
		dst.Spec.MinAvailable = src.Spec.MinAvailable
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.DisruptedPods = src.Status.DisruptedPods
		dst.Status.UnavailablePods = src.Status.UnavailablePods
		dst.Status.UnavailableAllowed = src.Status.UnavailableAllowed
		dst.Status.CurrentAvailable = src.Status.CurrentAvailable
		dst.Status.DesiredAvailable = src.Status.DesiredAvailable
		dst.Status.TotalReplicas = src.Status.TotalReplicas
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *PodUnavailableBudget) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.PodUnavailableBudget:
		src := srcRaw.(*v1beta1.PodUnavailableBudget)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		if src.Spec.TargetReference != nil {
			dst.Spec.TargetReference = &TargetReference{}
			dst.Spec.TargetReference.APIVersion = src.Spec.TargetReference.APIVersion
			dst.Spec.TargetReference.Kind = src.Spec.TargetReference.Kind
			dst.Spec.TargetReference.Name = src.Spec.TargetReference.Name
		}
		dst.Spec.MaxUnavailable = src.Spec.MaxUnavailable
		dst.Spec.MinAvailable = src.Spec.MinAvailable
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.DisruptedPods = src.Status.DisruptedPods
		dst.Status.UnavailablePods = src.Status.UnavailablePods
		dst.Status.UnavailableAllowed = src.Status.UnavailableAllowed
		dst.Status.CurrentAvailable = src.Status.CurrentAvailable
		dst.Status.DesiredAvailable = src.Status.DesiredAvailable
		dst.Status.TotalReplicas = src.Status.TotalReplicas
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
