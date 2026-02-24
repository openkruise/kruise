/*
Copyright 2026 The Kruise Authors.

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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/openkruise/kruise/apis/policy/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (pub *PodUnavailableBudget) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.PodUnavailableBudget:
		pubv1beta1 := dst.(*v1beta1.PodUnavailableBudget)
		pubv1beta1.ObjectMeta = pub.ObjectMeta

		// spec
		pubv1beta1.Spec = v1beta1.PodUnavailableBudgetSpec{
			Selector:       pub.Spec.Selector,
			MaxUnavailable: pub.Spec.MaxUnavailable,
			MinAvailable:   pub.Spec.MinAvailable,
		}
		if pub.Spec.TargetReference != nil {
			pubv1beta1.Spec.TargetReference = &v1beta1.TargetReference{
				APIVersion: pub.Spec.TargetReference.APIVersion,
				Kind:       pub.Spec.TargetReference.Kind,
				Name:       pub.Spec.TargetReference.Name,
			}
		}

		// status
		pubv1beta1.Status = v1beta1.PodUnavailableBudgetStatus{
			ObservedGeneration: pub.Status.ObservedGeneration,
			UnavailableAllowed: pub.Status.UnavailableAllowed,
			CurrentAvailable:   pub.Status.CurrentAvailable,
			DesiredAvailable:   pub.Status.DesiredAvailable,
			TotalReplicas:      pub.Status.TotalReplicas,
		}
		if pub.Status.DisruptedPods != nil {
			pubv1beta1.Status.DisruptedPods = make(map[string]metav1.Time)
			for k, v := range pub.Status.DisruptedPods {
				pubv1beta1.Status.DisruptedPods[k] = v
			}
		}
		if pub.Status.UnavailablePods != nil {
			pubv1beta1.Status.UnavailablePods = make(map[string]metav1.Time)
			for k, v := range pub.Status.UnavailablePods {
				pubv1beta1.Status.UnavailablePods[k] = v
			}
		}

		return nil

	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func (pub *PodUnavailableBudget) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.PodUnavailableBudget:
		pubv1beta1 := src.(*v1beta1.PodUnavailableBudget)
		pub.ObjectMeta = pubv1beta1.ObjectMeta

		// spec
		pub.Spec = PodUnavailableBudgetSpec{
			Selector:       pubv1beta1.Spec.Selector,
			MaxUnavailable: pubv1beta1.Spec.MaxUnavailable,
			MinAvailable:   pubv1beta1.Spec.MinAvailable,
		}
		if pubv1beta1.Spec.TargetReference != nil {
			pub.Spec.TargetReference = &TargetReference{
				APIVersion: pubv1beta1.Spec.TargetReference.APIVersion,
				Kind:       pubv1beta1.Spec.TargetReference.Kind,
				Name:       pubv1beta1.Spec.TargetReference.Name,
			}
		}

		// status
		pub.Status = PodUnavailableBudgetStatus{
			ObservedGeneration: pubv1beta1.Status.ObservedGeneration,
			UnavailableAllowed: pubv1beta1.Status.UnavailableAllowed,
			CurrentAvailable:   pubv1beta1.Status.CurrentAvailable,
			DesiredAvailable:   pubv1beta1.Status.DesiredAvailable,
			TotalReplicas:      pubv1beta1.Status.TotalReplicas,
		}
		if pubv1beta1.Status.DisruptedPods != nil {
			pub.Status.DisruptedPods = make(map[string]metav1.Time)
			for k, v := range pubv1beta1.Status.DisruptedPods {
				pub.Status.DisruptedPods[k] = v
			}
		}
		if pubv1beta1.Status.UnavailablePods != nil {
			pub.Status.UnavailablePods = make(map[string]metav1.Time)
			for k, v := range pubv1beta1.Status.UnavailablePods {
				pub.Status.UnavailablePods[k] = v
			}
		}

		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}
