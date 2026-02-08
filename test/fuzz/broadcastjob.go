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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func generateValidLabels(cf *fuzz.ConsumeFuzzer) map[string]string {
	labels := make(map[string]string)
	numLabels := r.Intn(3) + 1
	for i := 0; i < numLabels; i++ {
		key := GenerateValidKey()
		value := GenerateValidValue()
		labels[key] = value
	}
	return labels
}

func generateValidContainers(cf *fuzz.ConsumeFuzzer) []corev1.Container {
	containers := make([]corev1.Container, r.Intn(2)+1)
	for i := range containers {
		container := corev1.Container{
			Name:  GenerateValidValue(),
			Image: "nginx:latest",
		}
		containers[i] = container
	}
	return containers
}

func GenerateBroadcastJobV1Beta1(cf *fuzz.ConsumeFuzzer, bj *appsv1beta1.BroadcastJob) error {
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
	parallelism := intstr.FromInt(r.Intn(10) + 1)
	bj.Spec = appsv1beta1.BroadcastJobSpec{
		Parallelism: &parallelism,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: generateValidLabels(cf),
			},
			Spec: corev1.PodSpec{
				Containers: generateValidContainers(cf),
			},
		},
		Paused: r.Intn(2) == 0,
	}

	// Generate CompletionPolicy
	validTypes := []appsv1beta1.CompletionPolicyType{
		appsv1beta1.Always,
		appsv1beta1.Never,
	}
	choice := r.Intn(len(validTypes))
	policyType := validTypes[choice]

	policy := appsv1beta1.CompletionPolicy{
		Type: policyType,
	}

	if policyType == appsv1beta1.Always {
		if r.Intn(2) == 0 {
			ttl := int32(r.Intn(3600))
			policy.TTLSecondsAfterFinished = &ttl
		}
		if r.Intn(2) == 0 {
			deadline := int64(r.Intn(3600))
			policy.ActiveDeadlineSeconds = &deadline
		}
	}

	bj.Spec.CompletionPolicy = policy

	// Generate FailurePolicy
	validFailureTypes := []appsv1beta1.FailurePolicyType{
		appsv1beta1.FailurePolicyTypeFailFast,
		appsv1beta1.FailurePolicyTypeContinue,
		appsv1beta1.FailurePolicyTypePause,
	}
	choice = r.Intn(len(validFailureTypes))
	failureType := validFailureTypes[choice]

	failurePolicy := appsv1beta1.FailurePolicy{
		Type: failureType,
	}

	if r.Intn(2) == 0 {
		limit := int32(r.Intn(10))
		failurePolicy.RestartLimit = limit
	}

	bj.Spec.FailurePolicy = failurePolicy

	// Generate Status
	validPhases := []appsv1beta1.BroadcastJobPhase{
		appsv1beta1.PhaseRunning,
		appsv1beta1.PhaseCompleted,
		appsv1beta1.PhasePaused,
		appsv1beta1.PhaseFailed,
	}
	choice = r.Intn(len(validPhases))
	phase := validPhases[choice]

	bj.Status = appsv1beta1.BroadcastJobStatus{
		Phase:     phase,
		Active:    int32(r.Intn(100)),
		Succeeded: int32(r.Intn(100)),
		Failed:    int32(r.Intn(100)),
		Desired:   int32(r.Intn(100)),
	}

	return nil
}

func GenerateBroadcastJobTemplateSpecV1Beta1(cf *fuzz.ConsumeFuzzer, template *appsv1beta1.BroadcastJobTemplateSpec) error {
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

	// Generate BroadcastJobSpec
	bj := &appsv1beta1.BroadcastJob{}
	if err := GenerateBroadcastJobV1Beta1(cf, bj); err != nil {
		return err
	}

	template.Spec = bj.Spec
	return nil
}
