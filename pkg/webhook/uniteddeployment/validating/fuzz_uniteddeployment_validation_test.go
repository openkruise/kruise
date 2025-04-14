/*
Copyright 2021 The Kruise Authors.

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
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func FuzzValidateUnitedDeploymentSpec(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)
		ud := &appsv1alpha1.UnitedDeployment{}
		if err := cf.GenerateStruct(ud); err != nil {
			return
		}

		if err := fuzzutils.GenerateUnitedDeploymentReplicas(cf, ud); err != nil {
			return
		}

		if err := fuzzutils.GenerateUnitedDeploymentSelector(cf, ud); err != nil {
			return
		}

		if err := fuzzutils.GenerateUnitedDeploymentTemplate(cf, ud); err != nil {
			return
		}

		if err := fuzzutils.GenerateUnitedDeploymentSubset(cf, ud); err != nil {
			return
		}

		if err := fuzzutils.GenerateUnitedDeploymentUpdateStrategy(cf, ud); err != nil {
			return
		}

		_ = validateUnitedDeploymentSpec(&ud.Spec, field.NewPath("spec"))
	})
}
