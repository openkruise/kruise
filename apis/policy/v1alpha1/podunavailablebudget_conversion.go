/*
Copyright 2025 The Kruise Authors.

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
	"strconv"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
)

func (pub *PodUnavailableBudget) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.PodUnavailableBudget:
		v := dst.(*v1beta1.PodUnavailableBudget)
		v.ObjectMeta = pub.ObjectMeta

		// Convert Spec
		v.Spec = v1beta1.PodUnavailableBudgetSpec{
			Selector:        pub.Spec.Selector,
			TargetReference: convertTargetRefToV1Beta1(pub.Spec.TargetReference),
			MaxUnavailable:  pub.Spec.MaxUnavailable,
			MinAvailable:    pub.Spec.MinAvailable,
		}

		// Convert annotation kruise.io/pub-protect-operations to protectOperations field
		if operations, ok := pub.Annotations[PubProtectOperationAnnotation]; ok && operations != "" {
			ops := strings.Split(operations, ",")
			v.Spec.ProtectOperations = make([]v1beta1.PubOperation, 0, len(ops))
			for _, op := range ops {
				op = strings.TrimSpace(op)
				if op != "" {
					v.Spec.ProtectOperations = append(v.Spec.ProtectOperations, v1beta1.PubOperation(op))
				}
			}
		}

		// Convert annotation pub.kruise.io/protect-total-replicas to protectTotalReplicas field
		if totalReplicas, ok := pub.Annotations[PubProtectTotalReplicasAnnotation]; ok && totalReplicas != "" {
			if replicas, err := strconv.ParseInt(totalReplicas, 10, 32); err == nil {
				replicasInt32 := int32(replicas)
				v.Spec.ProtectTotalReplicas = &replicasInt32
			}
		}

		// Convert Status
		v.Status = v1beta1.PodUnavailableBudgetStatus{
			ObservedGeneration: pub.Status.ObservedGeneration,
			DisruptedPods:      pub.Status.DisruptedPods,
			UnavailablePods:    pub.Status.UnavailablePods,
			UnavailableAllowed: pub.Status.UnavailableAllowed,
			CurrentAvailable:   pub.Status.CurrentAvailable,
			DesiredAvailable:   pub.Status.DesiredAvailable,
			TotalReplicas:      pub.Status.TotalReplicas,
		}

		return nil
	default:
		return fmt.Errorf("unsupported type %T", t)
	}
}

func (pub *PodUnavailableBudget) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.PodUnavailableBudget:
		v := src.(*v1beta1.PodUnavailableBudget)
		pub.ObjectMeta = v.ObjectMeta

		// Convert Spec
		pub.Spec = PodUnavailableBudgetSpec{
			Selector:        v.Spec.Selector,
			TargetReference: convertTargetRefToV1Alpha1(v.Spec.TargetReference),
			MaxUnavailable:  v.Spec.MaxUnavailable,
			MinAvailable:    v.Spec.MinAvailable,
		}

		// Convert protectOperations field to annotation kruise.io/pub-protect-operations
		if len(v.Spec.ProtectOperations) > 0 {
			if pub.Annotations == nil {
				pub.Annotations = make(map[string]string)
			}
			ops := make([]string, 0, len(v.Spec.ProtectOperations))
			for _, op := range v.Spec.ProtectOperations {
				ops = append(ops, string(op))
			}
			pub.Annotations[PubProtectOperationAnnotation] = strings.Join(ops, ",")
		}

		// Convert protectTotalReplicas field to annotation pub.kruise.io/protect-total-replicas
		if v.Spec.ProtectTotalReplicas != nil {
			if pub.Annotations == nil {
				pub.Annotations = make(map[string]string)
			}
			pub.Annotations[PubProtectTotalReplicasAnnotation] = strconv.FormatInt(int64(*v.Spec.ProtectTotalReplicas), 10)
		}

		// Convert Status
		pub.Status = PodUnavailableBudgetStatus{
			ObservedGeneration: v.Status.ObservedGeneration,
			DisruptedPods:      v.Status.DisruptedPods,
			UnavailablePods:    v.Status.UnavailablePods,
			UnavailableAllowed: v.Status.UnavailableAllowed,
			CurrentAvailable:   v.Status.CurrentAvailable,
			DesiredAvailable:   v.Status.DesiredAvailable,
			TotalReplicas:      v.Status.TotalReplicas,
		}

		return nil
	default:
		return fmt.Errorf("unsupported type %T", t)
	}
}

func convertTargetRefToV1Beta1(in *TargetReference) *v1beta1.TargetReference {
	if in == nil {
		return nil
	}
	return &v1beta1.TargetReference{
		APIVersion: in.APIVersion,
		Kind:       in.Kind,
		Name:       in.Name,
	}
}

func convertTargetRefToV1Alpha1(in *v1beta1.TargetReference) *TargetReference {
	if in == nil {
		return nil
	}
	return &TargetReference{
		APIVersion: in.APIVersion,
		Kind:       in.Kind,
		Name:       in.Name,
	}
}
