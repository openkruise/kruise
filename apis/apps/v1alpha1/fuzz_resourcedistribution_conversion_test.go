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
	"reflect"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func FuzzResourceDistributionConversionRoundTrip(f *testing.F) {
	f.Add([]byte("configmap"))
	f.Add([]byte("secret"))

	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		src := &ResourceDistribution{
			TypeMeta: metav1.TypeMeta{APIVersion: GroupVersion.String(), Kind: "ResourceDistribution"},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "fuzz-rd",
				Annotations: map[string]string{},
				Labels:      map[string]string{},
			},
			Spec: ResourceDistributionSpec{
				Resource: runtime.RawExtension{
					Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"fuzz"},"data":{"k":"v"}}`),
				},
			},
		}
		_ = cf.GenerateStruct(&src.ObjectMeta)
		_ = cf.GenerateStruct(&src.Spec.Targets)
		_ = cf.GenerateStruct(&src.Status)
		src.APIVersion = GroupVersion.String()
		src.Kind = "ResourceDistribution"

		mid := &appsv1beta1.ResourceDistribution{}
		if err := src.ConvertTo(mid); err != nil {
			t.Fatalf("alpha to beta conversion failed: %v", err)
		}

		roundTrip := &ResourceDistribution{}
		if err := roundTrip.ConvertFrom(mid); err != nil {
			t.Fatalf("beta to alpha conversion failed: %v", err)
		}

		if roundTrip.APIVersion != GroupVersion.String() {
			t.Fatalf("unexpected alpha apiVersion %q", roundTrip.APIVersion)
		}
		if mid.APIVersion != appsv1beta1.GroupVersion.String() {
			t.Fatalf("unexpected beta apiVersion %q", mid.APIVersion)
		}
		if src.ObjectMeta.Name != roundTrip.ObjectMeta.Name {
			t.Fatalf("name changed after round trip: got=%q want=%q", roundTrip.ObjectMeta.Name, src.ObjectMeta.Name)
		}
		if src.ObjectMeta.Namespace != roundTrip.ObjectMeta.Namespace {
			t.Fatalf("namespace changed after round trip: got=%q want=%q", roundTrip.ObjectMeta.Namespace, src.ObjectMeta.Namespace)
		}
		if !reflect.DeepEqual(src.ObjectMeta.Labels, roundTrip.ObjectMeta.Labels) {
			t.Fatalf("labels changed after round trip")
		}
		if !reflect.DeepEqual(src.ObjectMeta.Annotations, roundTrip.ObjectMeta.Annotations) {
			t.Fatalf("annotations changed after round trip")
		}
		if !reflect.DeepEqual(normalizeTargets(src.Spec.Targets), normalizeTargets(roundTrip.Spec.Targets)) {
			t.Fatalf("targets changed after round trip")
		}
		if !reflect.DeepEqual(src.Spec.Resource.Raw, roundTrip.Spec.Resource.Raw) {
			t.Fatalf("resource raw bytes changed after round trip")
		}
		if src.Status.Desired != roundTrip.Status.Desired ||
			src.Status.Succeeded != roundTrip.Status.Succeeded ||
			src.Status.Failed != roundTrip.Status.Failed ||
			src.Status.ObservedGeneration != roundTrip.Status.ObservedGeneration {
			t.Fatalf("status counters changed after round trip")
		}
		if len(src.Status.Conditions) != len(roundTrip.Status.Conditions) {
			t.Fatalf("condition count changed after round trip: got=%d want=%d", len(roundTrip.Status.Conditions), len(src.Status.Conditions))
		}
		for i := range src.Status.Conditions {
			if !reflect.DeepEqual(src.Status.Conditions[i], roundTrip.Status.Conditions[i]) {
				t.Fatalf("condition %d changed after round trip", i)
			}
			if mid.Status.Conditions[i].Type != appsv1beta1.ResourceDistributionConditionType(src.Status.Conditions[i].Type) {
				t.Fatalf("beta condition %d type changed during alpha->beta conversion", i)
			}
			if mid.Status.Conditions[i].Status != appsv1beta1.ResourceDistributionConditionStatus(src.Status.Conditions[i].Status) {
				t.Fatalf("beta condition %d status changed during alpha->beta conversion", i)
			}
		}
	})
}

func normalizeTargets(targets ResourceDistributionTargets) ResourceDistributionTargets {
	targets.IncludedNamespaces.List = append([]ResourceDistributionNamespace(nil), targets.IncludedNamespaces.List...)
	targets.ExcludedNamespaces.List = append([]ResourceDistributionNamespace(nil), targets.ExcludedNamespaces.List...)
	if len(targets.IncludedNamespaces.List) == 0 {
		targets.IncludedNamespaces.List = nil
	}
	if len(targets.ExcludedNamespaces.List) == 0 {
		targets.ExcludedNamespaces.List = nil
	}
	return targets
}
