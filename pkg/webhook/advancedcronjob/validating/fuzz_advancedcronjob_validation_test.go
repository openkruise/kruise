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
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// FuzzValidateAdvancedCronJob tests AdvancedCronJob validation
func FuzzValidateAdvancedCronJob(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		// Generate v1beta1 AdvancedCronJob
		acj := &appsv1beta1.AdvancedCronJob{}
		if err := fuzzutils.GenerateAdvancedCronJobV1Beta1(cf, acj); err != nil {
			return
		}

		// Validate AdvancedCronJob
		handler := &AdvancedCronJobCreateUpdateHandler{}
		allErrs := handler.validateAdvancedCronJob(acj)
		if len(allErrs) != 0 {
			t.Logf("Validation errors: %v", allErrs)
		}
	})
}

// FuzzValidateAdvancedCronJobSpec tests AdvancedCronJob Spec validation
func FuzzValidateAdvancedCronJobSpec(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		// Generate v1beta1 AdvancedCronJob
		acj := &appsv1beta1.AdvancedCronJob{}
		if err := fuzzutils.GenerateAdvancedCronJobV1Beta1(cf, acj); err != nil {
			return
		}

		// Validate AdvancedCronJob Spec
		allErrs := validateAdvancedCronJobSpec(&acj.Spec, field.NewPath("spec"))
		if len(allErrs) != 0 {
			t.Logf("Spec validation errors: %v", allErrs)
		}
	})
}

// FuzzValidateAdvancedCronJobSpecSchedule tests AdvancedCronJob Schedule validation
func FuzzValidateAdvancedCronJobSpecSchedule(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		// Generate v1beta1 AdvancedCronJob
		acj := &appsv1beta1.AdvancedCronJob{}
		if err := fuzzutils.GenerateAdvancedCronJobV1Beta1(cf, acj); err != nil {
			return
		}

		// Validate Schedule
		allErrs := validateAdvancedCronJobSpecSchedule(&acj.Spec, field.NewPath("spec"))
		if len(allErrs) != 0 {
			t.Logf("Schedule validation errors: %v", allErrs)
		}
	})
}

// FuzzValidateAdvancedCronJobSpecTemplate tests AdvancedCronJob Template validation
func FuzzValidateAdvancedCronJobSpecTemplate(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		// Generate v1beta1 AdvancedCronJob
		acj := &appsv1beta1.AdvancedCronJob{}
		if err := fuzzutils.GenerateAdvancedCronJobV1Beta1(cf, acj); err != nil {
			return
		}

		// Validate Template
		allErrs := validateAdvancedCronJobSpecTemplate(&acj.Spec, field.NewPath("spec"))
		if len(allErrs) != 0 {
			t.Logf("Template validation errors: %v", allErrs)
		}
	})
}
