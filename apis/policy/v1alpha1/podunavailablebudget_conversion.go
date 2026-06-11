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
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
)

func (pub *PodUnavailableBudget) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *policyv1beta1.PodUnavailableBudget:
		return convertPodUnavailableBudgetToV1beta1(pub, t)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func (pub *PodUnavailableBudget) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *policyv1beta1.PodUnavailableBudget:
		return convertPodUnavailableBudgetFromV1beta1(t, pub)
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

// convertPodUnavailableBudgetToV1beta1 converts the v1alpha1 PUB object into the v1beta1 hub shape.
// This is where deprecated alpha annotations are promoted into typed beta spec fields.
func convertPodUnavailableBudgetToV1beta1(src *PodUnavailableBudget, dst *policyv1beta1.PodUnavailableBudget) error {
	dst.TypeMeta = metav1.TypeMeta{
		APIVersion: policyv1beta1.GroupVersion.String(),
		Kind:       "PodUnavailableBudget",
	}
	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = policyv1beta1.PodUnavailableBudgetSpec{
		Selector:        src.Spec.Selector,
		TargetReference: convertTargetReferenceToV1beta1(src.Spec.TargetReference),
		MaxUnavailable:  src.Spec.MaxUnavailable,
		MinAvailable:    src.Spec.MinAvailable,
	}
	dst.Status = convertPodUnavailableBudgetStatusToV1beta1(src.Status)

	operations, err := parsePubProtectOperations(src.Annotations[PubProtectOperationAnnotation])
	if err != nil {
		return err
	}
	if len(operations) > 0 {
		dst.Spec.ProtectOperations = operations
	}

	if value := src.Annotations[PubProtectTotalReplicasAnnotation]; value != "" {
		replicas, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", PubProtectTotalReplicasAnnotation, err)
		}
		dst.Spec.ProtectTotalReplicas = ptrTo(int32(replicas))
	}

	return nil
}

// convertPodUnavailableBudgetFromV1beta1 converts the v1beta1 hub object back into the v1alpha1 shape.
// This is where beta typed fields are backfilled into deprecated alpha annotations for compatibility.
func convertPodUnavailableBudgetFromV1beta1(src *policyv1beta1.PodUnavailableBudget, dst *PodUnavailableBudget) error {
	dst.TypeMeta = metav1.TypeMeta{
		APIVersion: GroupVersion.String(),
		Kind:       "PodUnavailableBudget",
	}
	dst.ObjectMeta = src.ObjectMeta
	if src.Annotations != nil {
		dst.Annotations = make(map[string]string, len(src.Annotations))
		for key, value := range src.Annotations {
			dst.Annotations[key] = value
		}
	}
	dst.Spec = PodUnavailableBudgetSpec{
		Selector:        src.Spec.Selector,
		TargetReference: convertTargetReferenceFromV1beta1(src.Spec.TargetReference),
		MaxUnavailable:  src.Spec.MaxUnavailable,
		MinAvailable:    src.Spec.MinAvailable,
	}
	dst.Status = convertPodUnavailableBudgetStatusFromV1beta1(src.Status)

	if dst.Annotations == nil {
		dst.Annotations = map[string]string{}
	}

	if len(src.Spec.ProtectOperations) > 0 {
		dst.Annotations[PubProtectOperationAnnotation] = joinPubProtectOperations(src.Spec.ProtectOperations)
	} else {
		delete(dst.Annotations, PubProtectOperationAnnotation)
	}

	if src.Spec.ProtectTotalReplicas != nil {
		dst.Annotations[PubProtectTotalReplicasAnnotation] = strconv.FormatInt(int64(*src.Spec.ProtectTotalReplicas), 10)
	}
	if src.Spec.ProtectTotalReplicas == nil {
		delete(dst.Annotations, PubProtectTotalReplicasAnnotation)
	}

	return nil
}

// convertTargetReferenceToV1beta1 converts a v1alpha1 targetRef into the v1beta1 targetRef type.
func convertTargetReferenceToV1beta1(src *TargetReference) *policyv1beta1.TargetReference {
	if src == nil {
		return nil
	}
	return &policyv1beta1.TargetReference{
		APIVersion: src.APIVersion,
		Kind:       src.Kind,
		Name:       src.Name,
	}
}

// convertTargetReferenceFromV1beta1 converts a v1beta1 targetRef into the v1alpha1 targetRef type.
func convertTargetReferenceFromV1beta1(src *policyv1beta1.TargetReference) *TargetReference {
	if src == nil {
		return nil
	}
	return &TargetReference{
		APIVersion: src.APIVersion,
		Kind:       src.Kind,
		Name:       src.Name,
	}
}

// convertPodUnavailableBudgetStatusToV1beta1 converts v1alpha1 status into the v1beta1 status type.
func convertPodUnavailableBudgetStatusToV1beta1(src PodUnavailableBudgetStatus) policyv1beta1.PodUnavailableBudgetStatus {
	return policyv1beta1.PodUnavailableBudgetStatus{
		ObservedGeneration: src.ObservedGeneration,
		DisruptedPods:      src.DisruptedPods,
		UnavailablePods:    src.UnavailablePods,
		UnavailableAllowed: src.UnavailableAllowed,
		CurrentAvailable:   src.CurrentAvailable,
		DesiredAvailable:   src.DesiredAvailable,
		TotalReplicas:      src.TotalReplicas,
	}
}

// convertPodUnavailableBudgetStatusFromV1beta1 converts v1beta1 status into the v1alpha1 status type.
func convertPodUnavailableBudgetStatusFromV1beta1(src policyv1beta1.PodUnavailableBudgetStatus) PodUnavailableBudgetStatus {
	return PodUnavailableBudgetStatus{
		ObservedGeneration: src.ObservedGeneration,
		DisruptedPods:      src.DisruptedPods,
		UnavailablePods:    src.UnavailablePods,
		UnavailableAllowed: src.UnavailableAllowed,
		CurrentAvailable:   src.CurrentAvailable,
		DesiredAvailable:   src.DesiredAvailable,
		TotalReplicas:      src.TotalReplicas,
	}
}

// parsePubProtectOperations converts the deprecated alpha annotation value
// "DELETE,EVICT" into the typed v1beta1 []PubOperation slice.
func parsePubProtectOperations(value string) ([]policyv1beta1.PubOperation, error) {
	if value == "" {
		return nil, nil
	}

	items := strings.Split(value, ",")
	operations := make([]policyv1beta1.PubOperation, 0, len(items))
	for _, item := range items {
		operation := policyv1beta1.PubOperation(strings.TrimSpace(item))
		if operation == "" {
			continue
		}
		if !isValidPubOperation(operation) {
			return nil, fmt.Errorf("invalid %s value %q", PubProtectOperationAnnotation, operation)
		}
		operations = append(operations, operation)
	}
	return operations, nil
}

// joinPubProtectOperations converts the typed v1beta1 []PubOperation slice
// back into the deprecated alpha annotation string form like "DELETE,EVICT".
func joinPubProtectOperations(operations []policyv1beta1.PubOperation) string {
	values := make([]string, 0, len(operations))
	for _, operation := range operations {
		if operation == "" {
			continue
		}
		values = append(values, string(operation))
	}
	return strings.Join(values, ",")
}

func isValidPubOperation(operation policyv1beta1.PubOperation) bool {
	switch operation {
	case policyv1beta1.PubDeleteOperation, policyv1beta1.PubUpdateOperation, policyv1beta1.PubEvictOperation, policyv1beta1.PubResizeOperation:
		return true
	default:
		return false
	}
}

// ptrTo converts a value into a pointer so scalar alpha annotation values can be promoted into beta pointer fields.
func ptrTo[T any](value T) *T {
	return &value
}
