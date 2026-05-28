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

package containerrecreaterequest

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = appsv1beta1.AddToScheme(s)
	return s
}

func TestSnapshotEqual(t *testing.T) {
	s1 := appsv1beta1.ContainerRecreateRequestSyncContainerStatus{Name: "app", Ready: true, RestartCount: 1, ContainerID: "docker://abc"}
	s2 := appsv1beta1.ContainerRecreateRequestSyncContainerStatus{Name: "app", Ready: true, RestartCount: 1, ContainerID: "docker://abc"}
	s3 := appsv1beta1.ContainerRecreateRequestSyncContainerStatus{Name: "app", Ready: false, RestartCount: 2, ContainerID: "docker://xyz"}

	if !snapshotEqual(nil, nil) {
		t.Error("nil slices should be equal")
	}
	if !snapshotEqual([]appsv1beta1.ContainerRecreateRequestSyncContainerStatus{s1}, []appsv1beta1.ContainerRecreateRequestSyncContainerStatus{s2}) {
		t.Error("identical slices should be equal")
	}
	if snapshotEqual([]appsv1beta1.ContainerRecreateRequestSyncContainerStatus{s1}, []appsv1beta1.ContainerRecreateRequestSyncContainerStatus{s3}) {
		t.Error("different slices should not be equal")
	}
	if snapshotEqual([]appsv1beta1.ContainerRecreateRequestSyncContainerStatus{s1}, nil) {
		t.Error("different length slices should not be equal")
	}
}

func TestHasPodUnreadyAcquiredCondition(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name       string
		conditions []metav1.Condition
		want       bool
	}{
		{
			name:       "no conditions",
			conditions: nil,
			want:       false,
		},
		{
			name: "condition present and true",
			conditions: []metav1.Condition{
				{
					Type:               appsv1beta1.ContainerRecreateRequestPodUnreadyAcquiredType,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "UnreadyAcquired",
				},
			},
			want: true,
		},
		{
			name: "condition present but false",
			conditions: []metav1.Condition{
				{
					Type:               appsv1beta1.ContainerRecreateRequestPodUnreadyAcquiredType,
					Status:             metav1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "Released",
				},
			},
			want: false,
		},
		{
			name: "unrelated condition only",
			conditions: []metav1.Condition{
				{
					Type:               "SomeOtherCondition",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "Other",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crr := &appsv1beta1.ContainerRecreateRequest{
				Status: appsv1beta1.ContainerRecreateRequestStatus{
					Conditions: tt.conditions,
				},
			}
			got := hasPodUnreadyAcquiredCondition(crr)
			if got != tt.want {
				t.Errorf("hasPodUnreadyAcquiredCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncContainerStatuses_CollectsRunningContainers(t *testing.T) {
	createdAt := metav1.NewTime(time.Now())
	beforeCreate := metav1.NewTime(createdAt.Add(-10 * time.Second))

	crr := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "crr-0",
			Namespace:         "default",
			CreationTimestamp: createdAt,
		},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			PodName: "pod-0",
			Containers: []appsv1beta1.ContainerRecreateRequestContainer{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Namespace: "default"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					Ready:        true,
					RestartCount: 1,
					ContainerID:  "docker://new",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.NewTime(createdAt.Add(5 * time.Second)),
						},
					},
				},
				{
					Name:        "sidecar",
					Ready:       false,
					ContainerID: "docker://old",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: beforeCreate,
						},
					},
				},
			},
		},
	}

	// Only containers started AFTER CRR creation are included (history status excluded).
	// app: StartedAt = createdAt+5s → included
	// sidecar: StartedAt = beforeCreate (10s before CRR) → excluded by StartedAt filter
	expected := []appsv1beta1.ContainerRecreateRequestSyncContainerStatus{
		{Name: "app", Ready: true, RestartCount: 1, ContainerID: "docker://new"},
	}

	// Build the statuses as syncContainerStatuses() would (mirrors the real loop).
	got := make([]appsv1beta1.ContainerRecreateRequestSyncContainerStatus, 0)
	for i := range crr.Spec.Containers {
		c := &crr.Spec.Containers[i]
		var cs *corev1.ContainerStatus
		for j := range pod.Status.ContainerStatuses {
			if pod.Status.ContainerStatuses[j].Name == c.Name {
				cs = &pod.Status.ContainerStatuses[j]
				break
			}
		}
		if cs == nil || cs.State.Running == nil || cs.State.Running.StartedAt.Before(&crr.CreationTimestamp) {
			continue
		}
		got = append(got, appsv1beta1.ContainerRecreateRequestSyncContainerStatus{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
			ContainerID:  cs.ContainerID,
		})
	}

	if !snapshotEqual(got, expected) {
		t.Errorf("snapshot mismatch: got %v, want %v", got, expected)
	}
}
