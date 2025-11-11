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

package fuzz

import (
	fuzz "github.com/AdaLogics/go-fuzz-headers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func generateValidNames(cf *fuzz.ConsumeFuzzer) []string {
	var names []string
	numLabels := r.Intn(3) + 1
	for i := 0; i < numLabels; i++ {
		names = append(names, randomRFC1123())
	}
	return names
}

func GenerateImageListPullJobV1Beta1(cf *fuzz.ConsumeFuzzer, bj *appsv1beta1.ImageListPullJob) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		if err := cf.GenerateStruct(bj); err != nil {
			return err
		}
		return nil
	}

	// Generate basic ObjectMeta
	bj.ObjectMeta = metav1.ObjectMeta{
		Name:      GenerateValidValue(),
		Namespace: GenerateValidNamespaceName(),
		Labels:    generateValidLabels(cf),
	}

	// Generate Spec
	bj.Spec = appsv1beta1.ImageListPullJobSpec{
		Images: generateValidNames(cf),
		ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
			Selector: &appsv1beta1.ImagePullJobNodeSelector{
				Names: generateValidNames(cf),
			},
		},
	}

	policy := appsv1beta1.CompletionPolicy{
		Type: appsv1beta1.Always,
	}

	if r.Intn(2) == 0 {
		ttl := int32(r.Intn(3600))
		policy.TTLSecondsAfterFinished = &ttl
	}
	if r.Intn(2) == 0 {
		deadline := int64(r.Intn(3600))
		policy.ActiveDeadlineSeconds = &deadline
	}

	bj.Spec.CompletionPolicy = policy

	// Generate FailurePolicy
	validFailureTypes := []appsv1beta1.FailurePolicyType{
		appsv1beta1.FailurePolicyTypeFailFast,
		appsv1beta1.FailurePolicyTypeContinue,
		appsv1beta1.FailurePolicyTypePause,
	}
	choice := r.Intn(len(validFailureTypes))
	failureType := validFailureTypes[choice]

	failurePolicy := appsv1beta1.FailurePolicy{
		Type: failureType,
	}

	if r.Intn(2) == 0 {
		limit := int32(r.Intn(10))
		failurePolicy.RestartLimit = limit
	}

	// Generate Status
	bj.Status = appsv1beta1.ImageListPullJobStatus{
		Active:    int32(r.Intn(1)),
		Succeeded: int32(r.Intn(1)),
		Desired:   int32(r.Intn(3)),
		Completed: int32(r.Intn(1)),
	}

	return nil
}

func GenerateImageListPullJobTemplateSpecV1Beta1(cf *fuzz.ConsumeFuzzer, template *appsv1beta1.ImageListPullJobTemplateSpec) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		if err := cf.GenerateStruct(template); err != nil {
			return err
		}
		return nil
	}

	// Generate ObjectMeta
	template.ObjectMeta = metav1.ObjectMeta{
		Name:      GenerateValidValue(),
		Namespace: GenerateValidNamespaceName(),
		Labels:    generateValidLabels(cf),
	}

	// Generate ImageListPullJobSpec
	bj := &appsv1beta1.ImageListPullJob{}
	if err := GenerateImageListPullJobV1Beta1(cf, bj); err != nil {
		return err
	}

	template.Spec = bj.Spec
	return nil
}
