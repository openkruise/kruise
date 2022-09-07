/*
Copyright 2022 The Kruise Authors.

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

	v1 "k8s.io/api/core/v1"

	batchv1 "k8s.io/api/batch/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
)

func TestValidateCronJobSpec(t *testing.T) {
	validPodTemplateSpec := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{Name: "foo", Image: "foo:latest", TerminationMessagePolicy: v1.TerminationMessageReadFile, ImagePullPolicy: v1.PullIfNotPresent},
			},
			RestartPolicy: v1.RestartPolicyAlways,
			DNSPolicy:     v1.DNSDefault,
		},
	}

	type testCase struct {
		acj       *appsv1alpha1.AdvancedCronJobSpec
		expectErr bool
	}

	cases := map[string]testCase{
		"no validation because timeZone is nil": {
			acj: &appsv1alpha1.AdvancedCronJobSpec{
				Schedule:          "0 * * * *",
				TimeZone:          nil,
				ConcurrencyPolicy: appsv1alpha1.AllowConcurrent,
				Template: appsv1alpha1.CronJobTemplate{
					JobTemplate: &batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Template: validPodTemplateSpec,
						},
					},
				},
			},
		},
		"check timeZone is valid": {
			acj: &appsv1alpha1.AdvancedCronJobSpec{
				Schedule:          "0 * * * *",
				TimeZone:          pointer.String("America/New_York"),
				ConcurrencyPolicy: appsv1alpha1.AllowConcurrent,
				Template: appsv1alpha1.CronJobTemplate{
					JobTemplate: &batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Template: validPodTemplateSpec,
						},
					},
				},
			},
		},
		"check timeZone is invalid": {
			acj: &appsv1alpha1.AdvancedCronJobSpec{
				Schedule:          "0 * * * *",
				TimeZone:          pointer.String("broken"),
				ConcurrencyPolicy: appsv1alpha1.AllowConcurrent,
				Template: appsv1alpha1.CronJobTemplate{
					JobTemplate: &batchv1.JobTemplateSpec{
						Spec: batchv1.JobSpec{
							Template: validPodTemplateSpec,
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for k, v := range cases {
		errs := validateAdvancedCronJobSpec(v.acj, field.NewPath("spec"))
		if len(errs) > 0 && !v.expectErr {
			t.Errorf("unexpected error for %s: %v", k, errs)
		} else if len(errs) == 0 && v.expectErr {
			t.Errorf("expected error for %s but got nil", k)
		}
	}
}
