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

package validating

import (
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"k8s.io/apimachinery/pkg/util/validation/field"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
)

// FuzzValidateBroadcastJob tests BroadcastJob validation
func FuzzValidateBroadcastJob(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		// Generate v1beta1 BroadcastJob
		bj := &appsv1beta1.BroadcastJob{}
		if err := fuzzutils.GenerateBroadcastJobV1Beta1(cf, bj); err != nil {
			return
		}

		// Validate BroadcastJob
		allErrs := validateBroadcastJob(bj)
		if len(allErrs) != 0 {
			t.Logf("Validation errors: %v", allErrs)
		}
	})
}

// FuzzBroadcastJobSpecValidation tests BroadcastJob Spec validation
func FuzzBroadcastJobSpecValidation(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		// Generate v1beta1 BroadcastJob
		bj := &appsv1beta1.BroadcastJob{}
		if err := fuzzutils.GenerateBroadcastJobV1Beta1(cf, bj); err != nil {
			return
		}

		// Validate BroadcastJob Spec
		allErrs := validateBroadcastJobSpec(&bj.Spec, field.NewPath("spec"))
		if len(allErrs) != 0 {
			t.Logf("Spec validation errors: %v", allErrs)
		}
	})
}
