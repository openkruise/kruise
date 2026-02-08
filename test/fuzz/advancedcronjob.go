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
	"math/rand"
	"time"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func GenerateJobTemplateSpec(cf *fuzz.ConsumeFuzzer, jobTemplate *batchv1.JobTemplateSpec) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		if err := cf.GenerateStruct(jobTemplate); err != nil {
			return err
		}
		return nil
	}

	// Generate ObjectMeta
	jobTemplate.ObjectMeta = metav1.ObjectMeta{
		Name:      GenerateValidValue(),
		Namespace: GenerateValidNamespaceName(),
		Labels:    generateValidLabels(cf),
	}

	// Generate JobSpec
	jobTemplate.Spec = batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: generateValidLabels(cf),
			},
			Spec: corev1.PodSpec{
				Containers:    generateValidContainers(cf),
				RestartPolicy: corev1.RestartPolicyOnFailure,
			},
		},
	}

	// Set parallelism
	if setParallelism, err := cf.GetBool(); err == nil && setParallelism {
		parallelism := int32(r.Intn(5) + 1)
		jobTemplate.Spec.Parallelism = &parallelism
	}

	// Set completions
	if setCompletions, err := cf.GetBool(); err == nil && setCompletions {
		completions := int32(r.Intn(5) + 1)
		jobTemplate.Spec.Completions = &completions
	}

	// Set active deadline
	if setActiveDeadline, err := cf.GetBool(); err == nil && setActiveDeadline {
		deadline := int64(r.Intn(3600))
		jobTemplate.Spec.ActiveDeadlineSeconds = &deadline
	}

	return nil
}

func GenerateAdvancedCronJobV1Beta1(cf *fuzz.ConsumeFuzzer, acj *appsv1beta1.AdvancedCronJob) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		if err := cf.GenerateStruct(acj); err != nil {
			return err
		}
		return nil
	}

	// Generate basic ObjectMeta
	acj.ObjectMeta = metav1.ObjectMeta{
		Name:      GenerateValidValue(),
		Namespace: GenerateValidNamespaceName(),
		Labels:    generateValidLabels(cf),
	}

	// Generate Spec
	acj.Spec = appsv1beta1.AdvancedCronJobSpec{
		Schedule: generateCronSchedule(cf),
		Paused:   &[]bool{r.Intn(2) == 0}[0],
	}

	// Generate timezone
	timezone := generateTimezone(cf)
	acj.Spec.TimeZone = &timezone

	// Generate starting deadline
	if r.Intn(2) == 0 {
		seconds := int64(r.Intn(3600))
		acj.Spec.StartingDeadlineSeconds = &seconds
	}

	// Generate concurrency policy
	validPolicies := []appsv1beta1.ConcurrencyPolicy{
		appsv1beta1.AllowConcurrent,
		appsv1beta1.ForbidConcurrent,
		appsv1beta1.ReplaceConcurrent,
	}
	choice := r.Intn(len(validPolicies))
	acj.Spec.ConcurrencyPolicy = validPolicies[choice]

	// Generate history limits
	if r.Intn(2) == 0 {
		limit := int32(r.Intn(10) + 1)
		acj.Spec.SuccessfulJobsHistoryLimit = &limit
	}
	if r.Intn(2) == 0 {
		limit := int32(r.Intn(10) + 1)
		acj.Spec.FailedJobsHistoryLimit = &limit
	}

	// Generate template
	template := appsv1beta1.CronJobTemplate{}

	switch r.Intn(3) {
	case 0:
		// Generate JobTemplate
		jobTemplate := &batchv1.JobTemplateSpec{}
		if err := GenerateJobTemplateSpec(cf, jobTemplate); err == nil {
			template.JobTemplate = jobTemplate
		}
	case 1:
		// Generate BroadcastJobTemplate
		broadcastJobTemplate := &appsv1beta1.BroadcastJobTemplateSpec{}
		if err := GenerateBroadcastJobTemplateSpecV1Beta1(cf, broadcastJobTemplate); err == nil {
			template.BroadcastJobTemplate = broadcastJobTemplate
		}
	case 2:
		// Generate ImageListPullJobTemplate
		imageListPullJobTemplate := &appsv1beta1.ImageListPullJobTemplateSpec{}
		if err := GenerateImageListPullJobTemplateSpecV1Beta1(cf, imageListPullJobTemplate); err == nil {
			template.ImageListPullJobTemplate = imageListPullJobTemplate
		}
	}

	acj.Spec.Template = template

	// Generate Status
	validTypes := []appsv1beta1.TemplateKind{
		appsv1beta1.JobTemplate,
		appsv1beta1.BroadcastJobTemplate,
		appsv1beta1.ImageListPullJobTemplate,
	}
	choice = r.Intn(len(validTypes))
	templateType := validTypes[choice]

	acj.Status = appsv1beta1.AdvancedCronJobStatus{
		Type: templateType,
	}

	// Generate active jobs list
	if r.Intn(2) == 0 {
		active := make([]corev1.ObjectReference, r.Intn(3)+1)
		for i := range active {
			active[i] = corev1.ObjectReference{
				Kind:      "Job",
				Name:      GenerateValidValue(),
				Namespace: GenerateValidNamespaceName(),
			}
		}
		acj.Status.Active = active
	}

	// Generate last schedule time
	if r.Intn(2) == 0 {
		acj.Status.LastScheduleTime = &metav1.Time{}
		if err := cf.GenerateStruct(acj.Status.LastScheduleTime); err == nil {
			// Ensure timestamp is reasonable
		}
	}

	return nil
}

// generateTimezone generates a random timezone string for fuzzing
func generateTimezone(cf *fuzz.ConsumeFuzzer) string {
	// Use framework to generate boolean value to decide generation strategy
	useStructured, err := cf.GetBool()
	if err != nil {
		useStructured = false
	}

	if useStructured {
		// Use framework to generate random string
		randomTimezone, err := cf.GetString()
		if err == nil && len(randomTimezone) > 0 {
			return randomTimezone
		}
	}

	timeZones := []string{
		"UTC", "Asia/Shanghai", "America/New_York", "Europe/London", "Asia/Tokyo", "",
		"GMT", "GMT+0", "GMT-0",
	}

	// Randomly select a timezone
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	choice := r.Intn(len(timeZones))
	return timeZones[choice]
}

// generateCronSchedule generates various cron schedule expressions for fuzzing
func generateCronSchedule(cf *fuzz.ConsumeFuzzer) string {
	// Use framework to generate boolean value to decide generation strategy
	useStructured, err := cf.GetBool()
	if err != nil {
		useStructured = false
	}

	if useStructured {
		// Use framework to generate random string
		randomSchedule, err := cf.GetString()
		if err == nil && len(randomSchedule) > 0 {
			return randomSchedule
		}
	}

	cronSchedules := []string{
		"0 0 * * *", "0 12 * * *", "30 9 * * *", "0 */6 * * *",
		"0 0 */2 * *", "0 0 * * 0", "0 0 1 * *", "0 0 1 1 *",

		"*/1 * * * *", "*/5 * * * *", "*/15 * * * *", "*/30 * * * *",
		"0,30 * * * *", "15,45 * * * *",

		"0 0 */1 * *", "0 0 */2 * *", "0 0 */3 * *", "0 0 */7 * *",
		"0 0 */15 * *", "0 0 */30 * *",

		"@yearly", "@annually", "@monthly", "@weekly", "@daily",
		"@midnight", "@hourly", "@reboot", "@invalid",
	}

	// Randomly select a cron expression
	choice := r.Intn(len(cronSchedules))
	return cronSchedules[choice]
}
