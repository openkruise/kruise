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

package pubcontrol

import (
	"strconv"
	"strings"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var pubControlFuzzScheme = runtime.NewScheme()

func init() {
	_ = policyv1beta1.AddToScheme(pubControlFuzzScheme)
}

func FuzzPubProtectionCompatibility(f *testing.F) {
	f.Add([]byte("DELETE,UPDATE"))
	f.Add([]byte("15"))
	f.Add([]byte("true"))

	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		pub := &policyv1beta1.PodUnavailableBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "fuzz-pub",
				Namespace:   "default",
				Annotations: map[string]string{},
			},
		}

		if includeAnnotationOps, err := cf.GetBool(); err == nil && includeAnnotationOps {
			if value, err := cf.GetString(); err == nil {
				pub.Annotations[policyv1beta1.PubProtectOperationAnnotation] = value
			}
		}
		if includeTypedOps, err := cf.GetBool(); err == nil && includeTypedOps {
			pub.Spec.ProtectOperations = fuzzRuntimeOperations(cf)
		}

		if includeTypedReplicas, err := cf.GetBool(); err == nil && includeTypedReplicas {
			if value, err := cf.GetInt(); err == nil {
				replicas := int32(value)
				pub.Spec.ProtectTotalReplicas = &replicas
			}
		}

		operations := getPubProtectOperations(pub)
		if len(pub.Spec.ProtectOperations) > 0 {
			for _, operation := range pub.Spec.ProtectOperations {
				if operation == "" {
					continue
				}
				if !operations.Has(operation) {
					t.Fatalf("typed operation %q was not preserved in effective set %v", operation, operations.UnsortedList())
				}
			}
		}

		replicas := getPubProtectTotalReplicas(pub)
		if pub.Spec.ProtectTotalReplicas != nil {
			if replicas == nil || *replicas != *pub.Spec.ProtectTotalReplicas {
				t.Fatalf("protectTotalReplicas mismatch: got=%v want=%d", replicas, *pub.Spec.ProtectTotalReplicas)
			}
		} else if replicas != nil {
			t.Fatalf("expected nil protectTotalReplicas when spec unset, got=%v", replicas)
		}

		noProtectValue := ""
		if value, err := cf.GetString(); err == nil {
			noProtectValue = value
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					policyv1beta1.PodPubNoProtectionAnnotation: noProtectValue,
				},
			},
		}
		exempt := isPodNoProtection(pod)
		want := false
		if parsed, parseErr := strconv.ParseBool(noProtectValue); parseErr == nil {
			want = parsed
		}
		if exempt != want {
			t.Fatalf("isPodNoProtection mismatch: got=%t want=%t for value=%q", exempt, want, noProtectValue)
		}
	})
}

func FuzzIgnoredPubSelectorLookup(f *testing.F) {
	f.Add("kruise.io/force-deletable", "true", true, true)
	f.Add("app", "demo", false, true)
	f.Add("app", "demo", true, false)

	f.Fuzz(func(t *testing.T, labelKey, labelValue string, includeSelector, includeRelatedPub bool) {
		if strings.TrimSpace(labelKey) == "" {
			labelKey = "kruise.io/force-deletable"
		}
		if labelValue == "" {
			labelValue = "true"
		}

		pub := &policyv1beta1.PodUnavailableBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fuzz-pub",
				Namespace: "default",
			},
		}
		if includeSelector {
			pub.Spec.IgnoredPodSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{labelKey: labelValue},
			}
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fuzz-pod",
				Namespace: "default",
				Labels:    map[string]string{labelKey: labelValue},
			},
		}
		if includeRelatedPub {
			pod.Annotations = SetPodRelatedPubAnnotation(nil, pub.Name)
		}

		matched, err := isPodMatchedIgnoredPubSelector(pub, pod)
		if err != nil {
			t.Fatalf("ignored selector lookup returned unexpected error: %v", err)
		}

		want := includeSelector && includeRelatedPub
		if matched != want {
			t.Fatalf("ignored selector lookup mismatch: got=%t want=%t", matched, want)
		}
	})
}

func fuzzRuntimeOperations(cf *fuzz.ConsumeFuzzer) []policyv1beta1.PubOperation {
	count, err := cf.GetInt()
	if err != nil {
		return nil
	}
	if count < 0 {
		count = -count
	}
	count = count % 6

	operations := make([]policyv1beta1.PubOperation, 0, count)
	for i := 0; i < count; i++ {
		value, err := cf.GetString()
		if err != nil {
			break
		}
		operations = append(operations, policyv1beta1.PubOperation(value))
	}
	return operations
}
