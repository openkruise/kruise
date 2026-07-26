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

package ephemeraljob

import (
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func newTestEphemeralJob() *appsv1alpha1.EphemeralJob {
	return &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default", UID: "test-uid"},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: []v1.EphemeralContainer{
					{EphemeralContainerCommon: v1.EphemeralContainerCommon{Name: "debugger"}},
				},
			},
		},
	}
}

func podWithEphemeralContainerState(name string, state v1.ContainerState) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status: v1.PodStatus{
			EphemeralContainerStatuses: []v1.ContainerStatus{
				{Name: "debugger", State: state},
			},
		},
	}
}

func succeededPod(name string) *v1.Pod {
	return podWithEphemeralContainerState(name, v1.ContainerState{
		Terminated: &v1.ContainerStateTerminated{ExitCode: 0, FinishedAt: metav1.NewTime(time.Now())},
	})
}

func failedPod(name string) *v1.Pod {
	return podWithEphemeralContainerState(name, v1.ContainerState{
		Terminated: &v1.ContainerStateTerminated{ExitCode: 1, FinishedAt: metav1.NewTime(time.Now())},
	})
}

func runningPod(name string) *v1.Pod {
	return podWithEphemeralContainerState(name, v1.ContainerState{
		Running: &v1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Now())},
	})
}

func waitingPod(name string) *v1.Pod {
	return podWithEphemeralContainerState(name, v1.ContainerState{
		Waiting: &v1.ContainerStateWaiting{Reason: "ContainerCreating"},
	})
}

func TestCalculateStatus(t *testing.T) {
	cases := []struct {
		name               string
		replicas           *int32
		targetPods         []*v1.Pod
		wantPhase          appsv1alpha1.EphemeralJobPhase
		wantCompletionTime bool
	}{
		{
			name:               "no matched pods",
			targetPods:         nil,
			wantPhase:          appsv1alpha1.EphemeralJobWaiting,
			wantCompletionTime: false,
		},
		{
			name:               "all succeeded",
			targetPods:         []*v1.Pod{succeededPod("pod-1"), succeededPod("pod-2")},
			wantPhase:          appsv1alpha1.EphemeralJobSucceeded,
			wantCompletionTime: true,
		},
		{
			name:               "all failed",
			targetPods:         []*v1.Pod{failedPod("pod-1"), failedPod("pod-2")},
			wantPhase:          appsv1alpha1.EphemeralJobFailed,
			wantCompletionTime: true,
		},
		{
			name:               "mixed succeeded and failed reaches a terminal Failed phase",
			targetPods:         []*v1.Pod{succeededPod("pod-1"), failedPod("pod-2")},
			wantPhase:          appsv1alpha1.EphemeralJobFailed,
			wantCompletionTime: true,
		},
		{
			name:               "mixed succeeded and waiting stays Waiting, not Unknown",
			targetPods:         []*v1.Pod{succeededPod("pod-1"), waitingPod("pod-2")},
			wantPhase:          appsv1alpha1.EphemeralJobWaiting,
			wantCompletionTime: false,
		},
		{
			name:               "any running pod reports Running even with other pods finished",
			targetPods:         []*v1.Pod{succeededPod("pod-1"), runningPod("pod-2")},
			wantPhase:          appsv1alpha1.EphemeralJobRunning,
			wantCompletionTime: false,
		},
		{
			name:               "replicas larger than matched pods still completes",
			replicas:           ptr.To(int32(5)),
			targetPods:         []*v1.Pod{succeededPod("pod-1"), succeededPod("pod-2")},
			wantPhase:          appsv1alpha1.EphemeralJobSucceeded,
			wantCompletionTime: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := newTestEphemeralJob()
			job.Spec.Replicas = tc.replicas
			r := &ReconcileEphemeralJob{}

			if err := r.calculateStatus(job, tc.targetPods); err != nil {
				t.Fatalf("calculateStatus returned error: %v", err)
			}

			if job.Status.Phase != tc.wantPhase {
				t.Errorf("phase = %s, want %s (Matches=%d Succeeded=%d Failed=%d Running=%d Waiting=%d)",
					job.Status.Phase, tc.wantPhase, job.Status.Matches, job.Status.Succeeded,
					job.Status.Failed, job.Status.Running, job.Status.Waiting)
			}

			gotCompletionTime := job.Status.CompletionTime != nil
			if gotCompletionTime != tc.wantCompletionTime {
				t.Errorf("CompletionTime set = %v, want %v", gotCompletionTime, tc.wantCompletionTime)
			}
		})
	}
}

func TestCalculateStatusFailedCondition(t *testing.T) {
	job := newTestEphemeralJob()
	r := &ReconcileEphemeralJob{}

	if err := r.calculateStatus(job, []*v1.Pod{succeededPod("pod-1"), failedPod("pod-2")}); err != nil {
		t.Fatalf("calculateStatus returned error: %v", err)
	}

	var found *appsv1alpha1.EphemeralJobCondition
	for i := range job.Status.Conditions {
		if job.Status.Conditions[i].Type == appsv1alpha1.EJobFailed {
			found = &job.Status.Conditions[i]
		}
	}
	if found == nil {
		t.Fatalf("no %s condition found in %v", appsv1alpha1.EJobFailed, job.Status.Conditions)
	}
	if found.Reason != "JobFailed" {
		t.Errorf("reason = %s, want JobFailed", found.Reason)
	}
	if !strings.Contains(found.Message, "1/2 pods failed") {
		t.Errorf("message = %q, want it to report 1/2 pods failed", found.Message)
	}
}
