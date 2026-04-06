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
	"slices"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
)

func FuzzParsePubProtectOperations(f *testing.F) {
	for _, seed := range []string{
		"",
		"DELETE",
		"DELETE,EVICT",
		"DELETE, UPDATE ,EVICT",
		"RESIZE,DELETE",
		",,DELETE,,",
		"BOGUS",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		operations, err := parsePubProtectOperations(value)
		if err != nil {
			return
		}

		roundTrip, err := parsePubProtectOperations(joinPubProtectOperations(operations))
		if err != nil {
			t.Fatalf("round-trip parse failed for %q: %v", value, err)
		}
		if !slices.Equal(operations, roundTrip) {
			t.Fatalf("round-trip operations mismatch: got=%v roundTrip=%v", operations, roundTrip)
		}
	})
}

func FuzzPodUnavailableBudgetConversion(f *testing.F) {
	f.Add([]byte("DELETE,EVICT"))
	f.Add([]byte("DELETE,BOGUS"))
	f.Add([]byte("15"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		annotations := map[string]string{}
		if operationValue, err := cf.GetString(); err == nil && operationValue != "" {
			annotations[PubProtectOperationAnnotation] = operationValue
		}
		if replicaValue, err := cf.GetString(); err == nil && replicaValue != "" {
			annotations[PubProtectTotalReplicasAnnotation] = replicaValue
		}

		src := &PodUnavailableBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "fuzz-pub",
				Namespace:   "default",
				Annotations: annotations,
			},
			Spec: PodUnavailableBudgetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "demo"},
				},
				MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
			},
			Status: PodUnavailableBudgetStatus{
				DisruptedPods:   map[string]metav1.Time{},
				UnavailablePods: map[string]metav1.Time{},
			},
		}

		if useTargetRef, err := cf.GetBool(); err == nil && useTargetRef {
			src.Spec.Selector = nil
			src.Spec.TargetReference = &TargetReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "demo",
			}
		}

		dst := &policyv1beta1.PodUnavailableBudget{}
		if err := convertPodUnavailableBudgetToV1beta1(src, dst); err != nil {
			return
		}

		roundTrip := &PodUnavailableBudget{}
		if err := convertPodUnavailableBudgetFromV1beta1(dst, roundTrip); err != nil {
			t.Fatalf("beta->alpha conversion failed: %v", err)
		}

		if dst.Spec.ProtectTotalReplicas != nil {
			expected := fmt.Sprintf("%d", *dst.Spec.ProtectTotalReplicas)
			if got := roundTrip.Annotations[PubProtectTotalReplicasAnnotation]; got != expected {
				t.Fatalf("protectTotalReplicas annotation mismatch: got=%q want=%q", got, expected)
			}
		}

		if len(dst.Spec.ProtectOperations) > 0 {
			reparsed, err := parsePubProtectOperations(roundTrip.Annotations[PubProtectOperationAnnotation])
			if err != nil {
				t.Fatalf("round-trip protectOperations parse failed: %v", err)
			}
			if !slices.Equal(dst.Spec.ProtectOperations, reparsed) {
				t.Fatalf("protectOperations mismatch: got=%v want=%v", reparsed, dst.Spec.ProtectOperations)
			}
		}
	})
}
