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

package v1alpha1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestAdvancedCronJob_ConvertTo(t *testing.T) {
	tests := []struct {
		name     string
		acj      *AdvancedCronJob
		expected *v1beta1.AdvancedCronJob
	}{
		{
			name: "convert with all fields populated",
			acj: &AdvancedCronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-acj",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: AdvancedCronJobSpec{
					Schedule:                   "0 0 * * *",
					TimeZone:                   stringPtr("UTC"),
					StartingDeadlineSeconds:    int64Ptr(300),
					ConcurrencyPolicy:          AllowConcurrent,
					Paused:                     boolPtr(false),
					SuccessfulJobsHistoryLimit: int32Ptr(3),
					FailedJobsHistoryLimit:     int32Ptr(1),
					Template: CronJobTemplate{
						BroadcastJobTemplate: &BroadcastJobTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-broadcast-job",
							},
							Spec: BroadcastJobSpec{
								Parallelism: intstrPtr("50%"),
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{
											{
												Name:  "test-container",
												Image: "nginx:latest",
											},
										},
									},
								},
								CompletionPolicy: CompletionPolicy{
									Type:                    Always,
									ActiveDeadlineSeconds:   int64Ptr(3600),
									TTLSecondsAfterFinished: int32Ptr(300),
								},
								Paused: false,
								FailurePolicy: FailurePolicy{
									Type:         FailurePolicyTypeFailFast,
									RestartLimit: 3,
								},
							},
						},
					},
				},
				Status: AdvancedCronJobStatus{
					Type: BroadcastJobTemplate,
					Active: []corev1.ObjectReference{
						{
							Name:      "test-job",
							Namespace: "default",
						},
					},
					LastScheduleTime: &metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
			},
			expected: &v1beta1.AdvancedCronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-acj",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.AdvancedCronJobSpec{
					Schedule:                   "0 0 * * *",
					TimeZone:                   stringPtr("UTC"),
					StartingDeadlineSeconds:    int64Ptr(300),
					ConcurrencyPolicy:          v1beta1.AllowConcurrent,
					Paused:                     boolPtr(false),
					SuccessfulJobsHistoryLimit: int32Ptr(3),
					FailedJobsHistoryLimit:     int32Ptr(1),
					Template: v1beta1.CronJobTemplate{
						BroadcastJobTemplate: &v1beta1.BroadcastJobTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-broadcast-job",
							},
							Spec: v1beta1.BroadcastJobSpec{
								Parallelism: intstrPtr("50%"),
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{
											{
												Name:  "test-container",
												Image: "nginx:latest",
											},
										},
									},
								},
								CompletionPolicy: v1beta1.CompletionPolicy{
									Type:                    v1beta1.Always,
									ActiveDeadlineSeconds:   int64Ptr(3600),
									TTLSecondsAfterFinished: int32Ptr(300),
								},
								Paused: false,
								FailurePolicy: v1beta1.FailurePolicy{
									Type:         v1beta1.FailurePolicyTypeFailFast,
									RestartLimit: 3,
								},
							},
						},
					},
				},
				Status: v1beta1.AdvancedCronJobStatus{
					Type: v1beta1.BroadcastJobTemplate,
					Active: []corev1.ObjectReference{
						{
							Name:      "test-job",
							Namespace: "default",
						},
					},
					LastScheduleTime: &metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
			},
		},
		{
			name: "convert with minimal fields",
			acj: &AdvancedCronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-acj",
					Namespace: "default",
				},
				Spec: AdvancedCronJobSpec{
					Schedule: "0 0 * * *",
					Template: CronJobTemplate{},
				},
				Status: AdvancedCronJobStatus{
					Type: JobTemplate,
				},
			},
			expected: &v1beta1.AdvancedCronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-acj",
					Namespace: "default",
				},
				Spec: v1beta1.AdvancedCronJobSpec{
					Schedule: "0 0 * * *",
					Template: v1beta1.CronJobTemplate{},
				},
				Status: v1beta1.AdvancedCronJobStatus{
					Type: v1beta1.JobTemplate,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &v1beta1.AdvancedCronJob{}
			err := tt.acj.ConvertTo(dst)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, dst)
		})
	}
}

func TestAdvancedCronJob_ConvertFrom(t *testing.T) {
	tests := []struct {
		name     string
		src      *v1beta1.AdvancedCronJob
		expected *AdvancedCronJob
	}{
		{
			name: "convert from v1beta1 with all fields populated",
			src: &v1beta1.AdvancedCronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-acj",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.AdvancedCronJobSpec{
					Schedule:                   "0 0 * * *",
					TimeZone:                   stringPtr("UTC"),
					StartingDeadlineSeconds:    int64Ptr(300),
					ConcurrencyPolicy:          v1beta1.AllowConcurrent,
					Paused:                     boolPtr(false),
					SuccessfulJobsHistoryLimit: int32Ptr(3),
					FailedJobsHistoryLimit:     int32Ptr(1),
					Template: v1beta1.CronJobTemplate{
						BroadcastJobTemplate: &v1beta1.BroadcastJobTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-broadcast-job",
							},
							Spec: v1beta1.BroadcastJobSpec{
								Parallelism: intstrPtr("50%"),
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{
											{
												Name:  "test-container",
												Image: "nginx:latest",
											},
										},
									},
								},
								CompletionPolicy: v1beta1.CompletionPolicy{
									Type:                    v1beta1.Always,
									ActiveDeadlineSeconds:   int64Ptr(3600),
									TTLSecondsAfterFinished: int32Ptr(300),
								},
								Paused: false,
								FailurePolicy: v1beta1.FailurePolicy{
									Type:         v1beta1.FailurePolicyTypeFailFast,
									RestartLimit: 3,
								},
							},
						},
					},
				},
				Status: v1beta1.AdvancedCronJobStatus{
					Type: v1beta1.BroadcastJobTemplate,
					Active: []corev1.ObjectReference{
						{
							Name:      "test-job",
							Namespace: "default",
						},
					},
					LastScheduleTime: &metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
			},
			expected: &AdvancedCronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-acj",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: AdvancedCronJobSpec{
					Schedule:                   "0 0 * * *",
					TimeZone:                   stringPtr("UTC"),
					StartingDeadlineSeconds:    int64Ptr(300),
					ConcurrencyPolicy:          AllowConcurrent,
					Paused:                     boolPtr(false),
					SuccessfulJobsHistoryLimit: int32Ptr(3),
					FailedJobsHistoryLimit:     int32Ptr(1),
					Template: CronJobTemplate{
						BroadcastJobTemplate: &BroadcastJobTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-broadcast-job",
							},
							Spec: BroadcastJobSpec{
								Parallelism: intstrPtr("50%"),
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{
											{
												Name:  "test-container",
												Image: "nginx:latest",
											},
										},
									},
								},
								CompletionPolicy: CompletionPolicy{
									Type:                    Always,
									ActiveDeadlineSeconds:   int64Ptr(3600),
									TTLSecondsAfterFinished: int32Ptr(300),
								},
								Paused: false,
								FailurePolicy: FailurePolicy{
									Type:         FailurePolicyTypeFailFast,
									RestartLimit: 3,
								},
							},
						},
					},
				},
				Status: AdvancedCronJobStatus{
					Type: BroadcastJobTemplate,
					Active: []corev1.ObjectReference{
						{
							Name:      "test-job",
							Namespace: "default",
						},
					},
					LastScheduleTime: &metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
			},
		},
		{
			name: "convert from v1beta1 with minimal fields",
			src: &v1beta1.AdvancedCronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-acj",
					Namespace: "default",
				},
				Spec: v1beta1.AdvancedCronJobSpec{
					Schedule: "0 0 * * *",
					Template: v1beta1.CronJobTemplate{},
				},
				Status: v1beta1.AdvancedCronJobStatus{
					Type: v1beta1.JobTemplate,
				},
			},
			expected: &AdvancedCronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-acj",
					Namespace: "default",
				},
				Spec: AdvancedCronJobSpec{
					Schedule: "0 0 * * *",
					Template: CronJobTemplate{},
				},
				Status: AdvancedCronJobStatus{
					Type: JobTemplate,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acj := &AdvancedCronJob{}
			err := acj.ConvertFrom(tt.src)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, acj)
		})
	}
}

func TestBroadcastJob_ConvertTo(t *testing.T) {
	tests := []struct {
		name     string
		bj       *BroadcastJob
		expected *v1beta1.BroadcastJob
	}{
		{
			name: "convert with all fields populated",
			bj: &BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bj",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: BroadcastJobSpec{
					Parallelism: intstrPtr("50%"),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: CompletionPolicy{
						Type:                    Always,
						ActiveDeadlineSeconds:   int64Ptr(3600),
						TTLSecondsAfterFinished: int32Ptr(300),
					},
					Paused: false,
					FailurePolicy: FailurePolicy{
						Type:         FailurePolicyTypeFailFast,
						RestartLimit: 3,
					},
				},
				Status: BroadcastJobStatus{
					Conditions: []JobCondition{
						{
							Type:               JobComplete,
							Status:             corev1.ConditionTrue,
							LastProbeTime:      metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
							LastTransitionTime: metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
							Reason:             "JobCompleted",
							Message:            "Job completed successfully",
						},
					},
					StartTime:      &metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2023, 1, 1, 1, 0, 0, 0, time.UTC)},
					Active:         0,
					Succeeded:      5,
					Failed:         0,
					Desired:        5,
					Phase:          PhaseCompleted,
				},
			},
			expected: &v1beta1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bj",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.BroadcastJobSpec{
					Parallelism: intstrPtr("50%"),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: v1beta1.CompletionPolicy{
						Type:                    v1beta1.Always,
						ActiveDeadlineSeconds:   int64Ptr(3600),
						TTLSecondsAfterFinished: int32Ptr(300),
					},
					Paused: false,
					FailurePolicy: v1beta1.FailurePolicy{
						Type:         v1beta1.FailurePolicyTypeFailFast,
						RestartLimit: 3,
					},
				},
				Status: v1beta1.BroadcastJobStatus{
					Conditions: []v1beta1.JobCondition{
						{
							Type:               v1beta1.JobComplete,
							Status:             corev1.ConditionTrue,
							LastProbeTime:      metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
							LastTransitionTime: metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
							Reason:             "JobCompleted",
							Message:            "Job completed successfully",
						},
					},
					StartTime:      &metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2023, 1, 1, 1, 0, 0, 0, time.UTC)},
					Active:         0,
					Succeeded:      5,
					Failed:         0,
					Desired:        5,
					Phase:          v1beta1.PhaseCompleted,
				},
			},
		},
		{
			name: "convert with minimal fields",
			bj: &BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-bj",
					Namespace: "default",
				},
				Spec: BroadcastJobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: CompletionPolicy{
						Type: Always,
					},
					FailurePolicy: FailurePolicy{
						Type: FailurePolicyTypeFailFast,
					},
				},
				Status: BroadcastJobStatus{
					Phase: PhaseRunning,
				},
			},
			expected: &v1beta1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-bj",
					Namespace: "default",
				},
				Spec: v1beta1.BroadcastJobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: v1beta1.CompletionPolicy{
						Type: v1beta1.Always,
					},
					FailurePolicy: v1beta1.FailurePolicy{
						Type: v1beta1.FailurePolicyTypeFailFast,
					},
				},
				Status: v1beta1.BroadcastJobStatus{
					Phase: v1beta1.PhaseRunning,
				},
			},
		},
		{
			name: "convert with nil conditions",
			bj: &BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nil-conditions",
				},
				Spec: BroadcastJobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: CompletionPolicy{
						Type: Always,
					},
					FailurePolicy: FailurePolicy{
						Type: FailurePolicyTypeFailFast,
					},
				},
				Status: BroadcastJobStatus{
					Conditions: nil,
					Phase:      PhaseFailed,
				},
			},
			expected: &v1beta1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nil-conditions",
				},
				Spec: v1beta1.BroadcastJobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: v1beta1.CompletionPolicy{
						Type: v1beta1.Always,
					},
					FailurePolicy: v1beta1.FailurePolicy{
						Type: v1beta1.FailurePolicyTypeFailFast,
					},
				},
				Status: v1beta1.BroadcastJobStatus{
					Conditions: nil,
					Phase:      v1beta1.PhaseFailed,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &v1beta1.BroadcastJob{}
			err := tt.bj.ConvertTo(dst)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, dst)
		})
	}
}

func TestBroadcastJob_ConvertFrom(t *testing.T) {
	tests := []struct {
		name     string
		src      *v1beta1.BroadcastJob
		expected *BroadcastJob
	}{
		{
			name: "convert from v1beta1 with all fields populated",
			src: &v1beta1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bj",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.BroadcastJobSpec{
					Parallelism: intstrPtr("50%"),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: v1beta1.CompletionPolicy{
						Type:                    v1beta1.Always,
						ActiveDeadlineSeconds:   int64Ptr(3600),
						TTLSecondsAfterFinished: int32Ptr(300),
					},
					Paused: false,
					FailurePolicy: v1beta1.FailurePolicy{
						Type:         v1beta1.FailurePolicyTypeFailFast,
						RestartLimit: 3,
					},
				},
				Status: v1beta1.BroadcastJobStatus{
					Conditions: []v1beta1.JobCondition{
						{
							Type:               v1beta1.JobComplete,
							Status:             corev1.ConditionTrue,
							LastProbeTime:      metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
							LastTransitionTime: metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
							Reason:             "JobCompleted",
							Message:            "Job completed successfully",
						},
					},
					StartTime:      &metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2023, 1, 1, 1, 0, 0, 0, time.UTC)},
					Active:         0,
					Succeeded:      5,
					Failed:         0,
					Desired:        5,
					Phase:          v1beta1.PhaseCompleted,
				},
			},
			expected: &BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bj",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: BroadcastJobSpec{
					Parallelism: intstrPtr("50%"),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: CompletionPolicy{
						Type:                    Always,
						ActiveDeadlineSeconds:   int64Ptr(3600),
						TTLSecondsAfterFinished: int32Ptr(300),
					},
					Paused: false,
					FailurePolicy: FailurePolicy{
						Type:         FailurePolicyTypeFailFast,
						RestartLimit: 3,
					},
				},
				Status: BroadcastJobStatus{
					Conditions: []JobCondition{
						{
							Type:               JobComplete,
							Status:             corev1.ConditionTrue,
							LastProbeTime:      metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
							LastTransitionTime: metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
							Reason:             "JobCompleted",
							Message:            "Job completed successfully",
						},
					},
					StartTime:      &metav1.Time{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					CompletionTime: &metav1.Time{Time: time.Date(2023, 1, 1, 1, 0, 0, 0, time.UTC)},
					Active:         0,
					Succeeded:      5,
					Failed:         0,
					Desired:        5,
					Phase:          PhaseCompleted,
				},
			},
		},
		{
			name: "convert from v1beta1 with minimal fields",
			src: &v1beta1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-bj",
					Namespace: "default",
				},
				Spec: v1beta1.BroadcastJobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: v1beta1.CompletionPolicy{
						Type: v1beta1.Always,
					},
					FailurePolicy: v1beta1.FailurePolicy{
						Type: v1beta1.FailurePolicyTypeFailFast,
					},
				},
				Status: v1beta1.BroadcastJobStatus{
					Phase: v1beta1.PhaseRunning,
				},
			},
			expected: &BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-bj",
					Namespace: "default",
				},
				Spec: BroadcastJobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "nginx:latest",
								},
							},
						},
					},
					CompletionPolicy: CompletionPolicy{
						Type: Always,
					},
					FailurePolicy: FailurePolicy{
						Type: FailurePolicyTypeFailFast,
					},
				},
				Status: BroadcastJobStatus{
					Phase: PhaseRunning,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bj := &BroadcastJob{}
			err := bj.ConvertFrom(tt.src)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, bj)
		})
	}
}

// Helper functions for creating pointers
func stringPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func intstrPtr(s string) *intstr.IntOrString {
	return &intstr.IntOrString{Type: intstr.String, StrVal: s}
}

func intstrIntPtr(i int32) *intstr.IntOrString {
	return &intstr.IntOrString{Type: intstr.Int, IntVal: i}
}

func TestDaemonSet_ConvertTo(t *testing.T) {
	tests := []struct {
		name     string
		ds       *DaemonSet
		expected *v1beta1.DaemonSet
	}{
		{
			name: "convert with all fields populated including Standard rolling update",
			ds: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.14.2",
								},
							},
						},
					},
					UpdateStrategy: DaemonSetUpdateStrategy{
						Type: RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &RollingUpdateDaemonSet{
							Type:           StandardRollingUpdateType,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
							MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"region": "us-west"},
							},
							Partition: intstrIntPtr(5),
							Paused:    boolPtr(false),
						},
					},
					MinReadySeconds:      30,
					BurstReplicas:        intstrPtr("50%"),
					RevisionHistoryLimit: int32Ptr(10),
				},
				Status: DaemonSetStatus{
					CurrentNumberScheduled: 5,
					NumberMisscheduled:     0,
					DesiredNumberScheduled: 5,
					NumberReady:            5,
					ObservedGeneration:     1,
					UpdatedNumberScheduled: 5,
					NumberAvailable:        5,
					NumberUnavailable:      0,
					CollisionCount:         int32Ptr(0),
					DaemonSetHash:          "abc123",
				},
			},
			expected: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.14.2",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
						Type: v1beta1.RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &v1beta1.RollingUpdateDaemonSet{
							Type:           v1beta1.StandardRollingUpdateType,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
							MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"region": "us-west"},
							},
							Partition: intstrIntPtr(5),
							Paused:    boolPtr(false),
						},
					},
					MinReadySeconds:      30,
					BurstReplicas:        intstrPtr("50%"),
					RevisionHistoryLimit: int32Ptr(10),
				},
				Status: v1beta1.DaemonSetStatus{
					CurrentNumberScheduled: 5,
					NumberMisscheduled:     0,
					DesiredNumberScheduled: 5,
					NumberReady:            5,
					ObservedGeneration:     1,
					UpdatedNumberScheduled: 5,
					NumberAvailable:        5,
					NumberUnavailable:      0,
					CollisionCount:         int32Ptr(0),
					UpdateRevision:         "abc123",
				},
			},
		},
		{
			name: "convert with deprecated Surging rolling update type",
			ds: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-surging",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: DaemonSetUpdateStrategy{
						Type: RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &RollingUpdateDaemonSet{
							Type:           DeprecatedSurgingRollingUpdateType, // Should be mapped to Standard
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
			expected: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-surging",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
						Type: v1beta1.RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &v1beta1.RollingUpdateDaemonSet{
							Type:           v1beta1.StandardRollingUpdateType, // Mapped from Surging
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
		},
		{
			name: "convert with InPlaceIfPossible rolling update type",
			ds: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-inplace",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: DaemonSetUpdateStrategy{
						Type: RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &RollingUpdateDaemonSet{
							Type:           InplaceRollingUpdateType,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
			expected: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-inplace",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
						Type: v1beta1.RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &v1beta1.RollingUpdateDaemonSet{
							Type:           v1beta1.InplaceRollingUpdateType,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
		},
		{
			name: "convert with progressive-create-pod annotation",
			ds: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-progressive",
					Namespace: "default",
					Annotations: map[string]string{
						ProgressiveCreatePodAnnotation: "true",
						"other-annotation":             "keep-this",
					},
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
			expected: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-progressive",
					Namespace: "default",
					Annotations: map[string]string{
						ProgressiveCreatePodAnnotation: "true",
						"other-annotation":             "keep-this",
					},
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: &v1beta1.DaemonSetScaleStrategy{
						PartitionedScaling: true,
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
		},
		{
			name: "convert without progressive-create-pod annotation",
			ds: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-no-progressive",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation": "keep-this",
					},
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
			expected: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-no-progressive",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation": "keep-this",
					},
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
		},
		{
			name: "convert with OnDelete update strategy",
			ds: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-ondelete",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: DaemonSetUpdateStrategy{
						Type: OnDeleteDaemonSetStrategyType,
					},
				},
				Status: DaemonSetStatus{},
			},
			expected: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-ondelete",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
						Type: v1beta1.OnDeleteDaemonSetStrategyType,
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
		},
		{
			name: "convert minimal fields",
			ds: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-ds",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
			expected: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-ds",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
		},
		{
			name: "convert with nil annotations and progressive annotation true",
			ds: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-nil-annotations",
					Namespace: "default",
					Annotations: map[string]string{
						ProgressiveCreatePodAnnotation: "true",
					},
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
			expected: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-nil-annotations",
					Namespace: "default",
					Annotations: map[string]string{
						ProgressiveCreatePodAnnotation: "true",
					},
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: &v1beta1.DaemonSetScaleStrategy{
						PartitionedScaling: true,
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &v1beta1.DaemonSet{}
			err := tt.ds.ConvertTo(dst)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, dst)
		})
	}
}

func TestDaemonSet_ConvertFrom(t *testing.T) {
	tests := []struct {
		name     string
		src      *v1beta1.DaemonSet
		expected *DaemonSet
	}{
		{
			name: "convert from v1beta1 with all fields populated",
			src: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.14.2",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
						Type: v1beta1.RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &v1beta1.RollingUpdateDaemonSet{
							Type:           v1beta1.StandardRollingUpdateType,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
							MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"region": "us-west"},
							},
							Partition: intstrIntPtr(5),
							Paused:    boolPtr(false),
						},
					},
					MinReadySeconds:      30,
					BurstReplicas:        intstrPtr("50%"),
					RevisionHistoryLimit: int32Ptr(10),
				},
				Status: v1beta1.DaemonSetStatus{
					CurrentNumberScheduled: 5,
					NumberMisscheduled:     0,
					DesiredNumberScheduled: 5,
					NumberReady:            5,
					ObservedGeneration:     1,
					UpdatedNumberScheduled: 5,
					NumberAvailable:        5,
					NumberUnavailable:      0,
					CollisionCount:         int32Ptr(0),
					UpdateRevision:         "abc123",
				},
			},
			expected: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.14.2",
								},
							},
						},
					},
					UpdateStrategy: DaemonSetUpdateStrategy{
						Type: RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &RollingUpdateDaemonSet{
							Type:           StandardRollingUpdateType,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
							MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"region": "us-west"},
							},
							Partition: intstrIntPtr(5),
							Paused:    boolPtr(false),
						},
					},
					MinReadySeconds:      30,
					BurstReplicas:        intstrPtr("50%"),
					RevisionHistoryLimit: int32Ptr(10),
				},
				Status: DaemonSetStatus{
					CurrentNumberScheduled: 5,
					NumberMisscheduled:     0,
					DesiredNumberScheduled: 5,
					NumberReady:            5,
					ObservedGeneration:     1,
					UpdatedNumberScheduled: 5,
					NumberAvailable:        5,
					NumberUnavailable:      0,
					CollisionCount:         int32Ptr(0),
					DaemonSetHash:          "abc123",
				},
			},
		},
		{
			name: "convert from v1beta1 with ScaleStrategy progressive true",
			src: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-progressive",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation": "keep-this",
					},
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: &v1beta1.DaemonSetScaleStrategy{
						PartitionedScaling: true,
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
			expected: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-progressive",
					Namespace: "default",
					Annotations: map[string]string{
						ProgressiveCreatePodAnnotation: "true",
						"other-annotation":             "keep-this",
					},
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with ScaleStrategy progressive false",
			src: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-no-progressive",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: &v1beta1.DaemonSetScaleStrategy{
						PartitionedScaling: false,
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
			expected: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-no-progressive",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 without ScaleStrategy",
			src: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-nil-scale-strategy",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
			expected: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-nil-scale-strategy",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with InPlaceIfPossible rolling update",
			src: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-inplace",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
						Type: v1beta1.RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &v1beta1.RollingUpdateDaemonSet{
							Type:           v1beta1.InplaceRollingUpdateType,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
			expected: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-inplace",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: DaemonSetUpdateStrategy{
						Type: RollingUpdateDaemonSetStrategyType,
						RollingUpdate: &RollingUpdateDaemonSet{
							Type:           InplaceRollingUpdateType,
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with OnDelete update strategy",
			src: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-ondelete",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
						Type: v1beta1.OnDeleteDaemonSetStrategyType,
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
			expected: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-ondelete",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: DaemonSetUpdateStrategy{
						Type: OnDeleteDaemonSetStrategyType,
					},
				},
				Status: DaemonSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 minimal fields",
			src: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-ds",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
			expected: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-ds",
					Namespace: "default",
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with nil annotations and progressive true",
			src: &v1beta1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-nil-annotations",
					Namespace: "default",
				},
				Spec: v1beta1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: &v1beta1.DaemonSetScaleStrategy{
						PartitionedScaling: true,
					},
				},
				Status: v1beta1.DaemonSetStatus{},
			},
			expected: &DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ds-nil-annotations",
					Namespace: "default",
					Annotations: map[string]string{
						ProgressiveCreatePodAnnotation: "true",
					},
				},
				Spec: DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: DaemonSetStatus{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &DaemonSet{}
			err := ds.ConvertFrom(tt.src)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, ds)
		})
	}
}

func TestCloneSet_ConvertTo(t *testing.T) {
	tests := []struct {
		name     string
		cs       *CloneSet
		expected *v1beta1.CloneSet
	}{
		{
			name: "convert with all fields populated",
			cs: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(5),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.14.2",
								},
							},
						},
					},
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "data",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							},
						},
					},
					ScaleStrategy: CloneSetScaleStrategy{
						PodsToDelete:           []string{"pod-1"},
						MaxUnavailable:         intstrIntPtr(1),
						DisablePVCReuse:        false,
						ExcludePreparingDelete: true,
					},
					UpdateStrategy: CloneSetUpdateStrategy{
						Type:           InPlaceIfPossibleCloneSetUpdateStrategyType,
						Partition:      intstrIntPtr(2),
						MaxUnavailable: intstrPtr("20%"),
						MaxSurge:       intstrIntPtr(1),
						Paused:         false,
					},
					RevisionHistoryLimit: int32Ptr(10),
					MinReadySeconds:      30,
				},
				Status: CloneSetStatus{
					ObservedGeneration:       1,
					Replicas:                 5,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          3,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 3,
					ExpectedUpdatedReplicas:  3,
					UpdateRevision:           "rev-123",
					CurrentRevision:          "rev-122",
					CollisionCount:           int32Ptr(0),
					LabelSelector:            "app=test",
				},
			},
			expected: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(5),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.14.2",
								},
							},
						},
					},
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "data",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							},
						},
					},
					ScaleStrategy: v1beta1.CloneSetScaleStrategy{
						PodsToDelete:           []string{"pod-1"},
						MaxUnavailable:         intstrIntPtr(1),
						DisablePVCReuse:        false,
						ExcludePreparingDelete: true,
					},
					UpdateStrategy: v1beta1.CloneSetUpdateStrategy{
						Type:           v1beta1.InPlaceIfPossibleCloneSetUpdateStrategyType,
						Partition:      intstrIntPtr(2),
						MaxUnavailable: intstrPtr("20%"),
						MaxSurge:       intstrIntPtr(1),
						Paused:         false,
					},
					RevisionHistoryLimit: int32Ptr(10),
					MinReadySeconds:      30,
				},
				Status: v1beta1.CloneSetStatus{
					ObservedGeneration:       1,
					Replicas:                 5,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          3,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 3,
					ExpectedUpdatedReplicas:  3,
					UpdateRevision:           "rev-123",
					CurrentRevision:          "rev-122",
					CollisionCount:           int32Ptr(0),
					LabelSelector:            "app=test",
				},
			},
		},
		{
			name: "convert with label-based excludePreparingDelete to spec field",
			cs: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-label",
					Namespace: "default",
					Labels: map[string]string{
						"app":                                    "test",
						CloneSetScalingExcludePreparingDeleteKey: "true",
					},
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: CloneSetScaleStrategy{
						DisablePVCReuse: true,
					},
				},
				Status: CloneSetStatus{},
			},
			expected: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-label",
					Namespace: "default",
					Labels: map[string]string{
						"app":                                    "test",
						CloneSetScalingExcludePreparingDeleteKey: "true",
					},
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: v1beta1.CloneSetScaleStrategy{
						DisablePVCReuse:        true,
						ExcludePreparingDelete: true, // Converted from label
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
		},
		{
			name: "convert with minimal fields",
			cs: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-cs",
					Namespace: "default",
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: CloneSetStatus{},
			},
			expected: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-cs",
					Namespace: "default",
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
		},
		{
			name: "convert with ReCreate update strategy",
			cs: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-recreate",
					Namespace: "default",
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: CloneSetUpdateStrategy{
						Type: RecreateCloneSetUpdateStrategyType,
					},
				},
				Status: CloneSetStatus{},
			},
			expected: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-recreate",
					Namespace: "default",
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.CloneSetUpdateStrategy{
						Type: v1beta1.RecreateCloneSetUpdateStrategyType,
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
		},
		{
			name: "convert with InPlaceOnly update strategy",
			cs: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-inplaceonly",
					Namespace: "default",
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: CloneSetUpdateStrategy{
						Type: InPlaceOnlyCloneSetUpdateStrategyType,
					},
				},
				Status: CloneSetStatus{},
			},
			expected: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-inplaceonly",
					Namespace: "default",
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.CloneSetUpdateStrategy{
						Type: v1beta1.InPlaceOnlyCloneSetUpdateStrategyType,
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &v1beta1.CloneSet{}
			err := tt.cs.ConvertTo(dst)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, dst)
		})
	}
}

func TestCloneSet_ConvertFrom(t *testing.T) {
	tests := []struct {
		name     string
		src      *v1beta1.CloneSet
		expected *CloneSet
	}{
		{
			name: "convert from v1beta1 with all fields populated",
			src: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(5),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.14.2",
								},
							},
						},
					},
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "data",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							},
						},
					},
					ScaleStrategy: v1beta1.CloneSetScaleStrategy{
						PodsToDelete:           []string{"pod-1"},
						MaxUnavailable:         intstrIntPtr(1),
						DisablePVCReuse:        true,
						ExcludePreparingDelete: true,
					},
					UpdateStrategy: v1beta1.CloneSetUpdateStrategy{
						Type:           v1beta1.InPlaceIfPossibleCloneSetUpdateStrategyType,
						Partition:      intstrIntPtr(2),
						MaxUnavailable: intstrPtr("20%"),
						MaxSurge:       intstrIntPtr(1),
						Paused:         false,
					},
					RevisionHistoryLimit: int32Ptr(10),
					MinReadySeconds:      30,
				},
				Status: v1beta1.CloneSetStatus{
					ObservedGeneration:       1,
					Replicas:                 5,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          3,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 3,
					ExpectedUpdatedReplicas:  3,
					UpdateRevision:           "rev-123",
					CurrentRevision:          "rev-122",
					CollisionCount:           int32Ptr(0),
					LabelSelector:            "app=test",
				},
			},
			expected: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(5),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.14.2",
								},
							},
						},
					},
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "data",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							},
						},
					},
					ScaleStrategy: CloneSetScaleStrategy{
						PodsToDelete:           []string{"pod-1"},
						MaxUnavailable:         intstrIntPtr(1),
						DisablePVCReuse:        true,
						ExcludePreparingDelete: true,
					},
					UpdateStrategy: CloneSetUpdateStrategy{
						Type:           InPlaceIfPossibleCloneSetUpdateStrategyType,
						Partition:      intstrIntPtr(2),
						MaxUnavailable: intstrPtr("20%"),
						MaxSurge:       intstrIntPtr(1),
						Paused:         false,
					},
					RevisionHistoryLimit: int32Ptr(10),
					MinReadySeconds:      30,
				},
				Status: CloneSetStatus{
					ObservedGeneration:       1,
					Replicas:                 5,
					ReadyReplicas:            5,
					AvailableReplicas:        5,
					UpdatedReplicas:          3,
					UpdatedReadyReplicas:     3,
					UpdatedAvailableReplicas: 3,
					ExpectedUpdatedReplicas:  3,
					UpdateRevision:           "rev-123",
					CurrentRevision:          "rev-122",
					CollisionCount:           int32Ptr(0),
					LabelSelector:            "app=test",
				},
			},
		},
		{
			name: "convert from v1beta1 with excludePreparingDelete true",
			src: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-exclude",
					Namespace: "default",
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: v1beta1.CloneSetScaleStrategy{
						ExcludePreparingDelete: true,
						DisablePVCReuse:        true,
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
			expected: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-exclude",
					Namespace: "default",
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: CloneSetScaleStrategy{
						ExcludePreparingDelete: true,
						DisablePVCReuse:        true,
					},
				},
				Status: CloneSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with minimal fields",
			src: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-cs",
					Namespace: "default",
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
			expected: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-cs",
					Namespace: "default",
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
				Status: CloneSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with disablePVCReuse false",
			src: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-pvc-reuse",
					Namespace: "default",
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: v1beta1.CloneSetScaleStrategy{
						DisablePVCReuse: false,
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
			expected: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-pvc-reuse",
					Namespace: "default",
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					ScaleStrategy: CloneSetScaleStrategy{
						DisablePVCReuse: false,
					},
				},
				Status: CloneSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with ReCreate update strategy",
			src: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-recreate",
					Namespace: "default",
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.CloneSetUpdateStrategy{
						Type: v1beta1.RecreateCloneSetUpdateStrategyType,
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
			expected: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-recreate",
					Namespace: "default",
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: CloneSetUpdateStrategy{
						Type: RecreateCloneSetUpdateStrategyType,
					},
				},
				Status: CloneSetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with InPlaceOnly update strategy",
			src: &v1beta1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-inplaceonly",
					Namespace: "default",
				},
				Spec: v1beta1.CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: v1beta1.CloneSetUpdateStrategy{
						Type: v1beta1.InPlaceOnlyCloneSetUpdateStrategyType,
					},
				},
				Status: v1beta1.CloneSetStatus{},
			},
			expected: &CloneSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cs-inplaceonly",
					Namespace: "default",
				},
				Spec: CloneSetSpec{
					Replicas: int32Ptr(3),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
					UpdateStrategy: CloneSetUpdateStrategy{
						Type: InPlaceOnlyCloneSetUpdateStrategyType,
					},
				},
				Status: CloneSetStatus{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &CloneSet{}
			err := cs.ConvertFrom(tt.src)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, cs)
		})
	}
}
