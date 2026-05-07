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

package validating

import (
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
)

var pubValidationFuzzScheme = runtime.NewScheme()

func init() {
	_ = policyv1beta1.AddToScheme(pubValidationFuzzScheme)
}

func FuzzValidatePodUnavailableBudgetSpec(f *testing.F) {
	f.Add([]byte("protect-ops"))
	f.Add([]byte("ignored-selector"))
	f.Add([]byte("target-ref"))

	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		pub := fuzzPodUnavailableBudgetV1Beta1(cf, "fuzz-pub")
		old := fuzzPodUnavailableBudgetV1Beta1(cf, "fuzz-pub")
		other := fuzzPodUnavailableBudgetV1Beta1(cf, "other-pub")

		builder := fake.NewClientBuilder().WithScheme(pubValidationFuzzScheme)
		builder = builder.WithObjects(other)

		handler := &PodUnavailableBudgetCreateUpdateHandler{
			Client: builder.Build(),
		}

		_ = validatePodUnavailableBudgetSpecV1beta1(pub, field.NewPath("spec"))
		_ = handler.validatingPodUnavailableBudgetFnV1beta1(pub, nil)
		_ = handler.validatingPodUnavailableBudgetFnV1beta1(pub, old)
	})
}

func fuzzPodUnavailableBudgetV1Beta1(cf *fuzz.ConsumeFuzzer, name string) *policyv1beta1.PodUnavailableBudget {
	pub := &policyv1beta1.PodUnavailableBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Annotations: map[string]string{},
		},
	}
	_ = cf.GenerateStruct(pub)
	pub.Name = name
	pub.Namespace = "default"
	if pub.Annotations == nil {
		pub.Annotations = map[string]string{}
	}

	if value, err := cf.GetString(); err == nil && value != "" {
		pub.Annotations[policyv1beta1.PubProtectOperationAnnotation] = value
	}
	if value, err := cf.GetString(); err == nil && value != "" {
		pub.Annotations[policyv1beta1.PubProtectTotalReplicasAnnotation] = value
	}

	if useSelector, err := cf.GetBool(); err == nil && useSelector {
		pub.Spec.Selector = fuzzLabelSelector(cf)
		pub.Spec.TargetReference = nil
	} else {
		pub.Spec.TargetReference = fuzzTargetReference(cf)
		pub.Spec.Selector = nil
	}

	if useMaxUnavailable, err := cf.GetBool(); err == nil && useMaxUnavailable {
		pub.Spec.MaxUnavailable = fuzzIntOrString(cf)
		pub.Spec.MinAvailable = nil
	} else {
		pub.Spec.MinAvailable = fuzzIntOrString(cf)
		pub.Spec.MaxUnavailable = nil
	}

	if includeProtectOps, err := cf.GetBool(); err == nil && includeProtectOps {
		pub.Spec.ProtectOperations = fuzzPubOperations(cf)
	}
	if includeReplicas, err := cf.GetBool(); err == nil && includeReplicas {
		if value, err := cf.GetInt(); err == nil {
			replicas := int32(value)
			pub.Spec.ProtectTotalReplicas = &replicas
		}
	}
	if includeIgnoredSelector, err := cf.GetBool(); err == nil && includeIgnoredSelector {
		pub.Spec.IgnoredPodSelector = fuzzLabelSelector(cf)
	}

	return pub
}

func fuzzLabelSelector(cf *fuzz.ConsumeFuzzer) *metav1.LabelSelector {
	selector := &metav1.LabelSelector{}
	_ = cf.GenerateStruct(selector)
	return selector
}

func fuzzTargetReference(cf *fuzz.ConsumeFuzzer) *policyv1beta1.TargetReference {
	ref := &policyv1beta1.TargetReference{}
	_ = cf.GenerateStruct(ref)
	return ref
}

func fuzzIntOrString(cf *fuzz.ConsumeFuzzer) *intstr.IntOrString {
	useString, err := cf.GetBool()
	if err != nil {
		return nil
	}
	if useString {
		value, err := cf.GetString()
		if err != nil {
			return nil
		}
		return &intstr.IntOrString{Type: intstr.String, StrVal: value}
	}

	value, err := cf.GetInt()
	if err != nil {
		return nil
	}
	return &intstr.IntOrString{Type: intstr.Int, IntVal: int32(value)}
}

func fuzzPubOperations(cf *fuzz.ConsumeFuzzer) []policyv1beta1.PubOperation {
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
