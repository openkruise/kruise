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
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testingclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/podadapter"
	utilpodreadiness "github.com/openkruise/kruise/pkg/util/podreadiness"
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

func TestHasPreRecreateGraceCondition(t *testing.T) {
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
					Type:               appsv1beta1.ContainerRecreateRequestPreRecreateGraceType,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "PreRecreateGrace",
				},
			},
			want: true,
		},
		{
			name: "condition present but false",
			conditions: []metav1.Condition{
				{
					Type:               appsv1beta1.ContainerRecreateRequestPreRecreateGraceType,
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
			got := hasPreRecreateGraceCondition(crr)
			if got != tt.want {
				t.Errorf("hasPreRecreateGraceCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSyncContainerStatuses_CollectsRunningContainers drives the real
// syncContainerStatuses() through a fake client and asserts the persisted
// status.containerStatusSnapshot, rather than re-implementing the loop. This
// covers the snapshot-promotion path (the v1alpha1 sync-container-statuses
// annotation is now a typed status field) including the StartedAt history
// filter and the Status().Patch write.
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

	cli := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(crr).
		WithStatusSubresource(crr).
		Build()
	r := &ReconcileContainerRecreateRequest{Client: cli}

	if err := r.syncContainerStatuses(crr, pod); err != nil {
		t.Fatalf("syncContainerStatuses() error: %v", err)
	}

	got := &appsv1beta1.ContainerRecreateRequest{}
	if err := cli.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "crr-0"}, got); err != nil {
		t.Fatalf("get crr error: %v", err)
	}
	if !snapshotEqual(got.Status.ContainerStatusSnapshot, expected) {
		t.Errorf("snapshot mismatch: got %v, want %v", got.Status.ContainerStatusSnapshot, expected)
	}
}

// TestWaitRecreateGracePeriod_PreservesConditionTimestamp guards the idempotency fix:
// repeated waitRecreateGracePeriod calls must keep the original PreRecreateGrace
// LastTransitionTime, because the kruise-daemon derives the unreadyGracePeriod
// drain deadline from it. If the timestamp were rewritten on every reconcile the
// grace period would never elapse.
func TestWaitRecreateGracePeriod_PreservesConditionTimestamp(t *testing.T) {
	crr := &appsv1beta1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "crr-0", Namespace: "default"},
		Spec: appsv1beta1.ContainerRecreateRequestSpec{
			PodName: "pod-0",
			Strategy: &appsv1beta1.ContainerRecreateRequestStrategy{
				UnreadyGracePeriodSeconds: func() *int64 { v := int64(30); return &v }(),
			},
		},
	}
	// Pod without the Kruise readiness gate → waitRecreateGracePeriod skips the
	// readiness-gate/finalizer path and only writes the condition.
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-0", Namespace: "default"}}

	cli := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(crr).
		WithStatusSubresource(crr).
		Build()
	fakeClock := testingclock.NewFakeClock(time.Now())
	r := &ReconcileContainerRecreateRequest{
		Client:              cli,
		clock:               fakeClock,
		podReadinessControl: utilpodreadiness.NewForAdapter(&podadapter.AdapterRuntimeClient{Client: cli}),
	}

	if err := r.waitRecreateGracePeriod(crr, pod); err != nil {
		t.Fatalf("first waitRecreateGracePeriod() error: %v", err)
	}
	first := &appsv1beta1.ContainerRecreateRequest{}
	if err := cli.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "crr-0"}, first); err != nil {
		t.Fatalf("get crr error: %v", err)
	}
	cond := meta.FindStatusCondition(first.Status.Conditions, appsv1beta1.ContainerRecreateRequestPreRecreateGraceType)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected PreRecreateGrace=True after first acquire, got %+v", first.Status.Conditions)
	}
	firstTime := cond.LastTransitionTime

	// Advance the clock and acquire again. The condition already exists with the
	// same status, so the timestamp must be preserved (and no write performed).
	fakeClock.Step(15 * time.Second)
	if err := r.waitRecreateGracePeriod(first.DeepCopy(), pod); err != nil {
		t.Fatalf("second waitRecreateGracePeriod() error: %v", err)
	}
	second := &appsv1beta1.ContainerRecreateRequest{}
	if err := cli.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "crr-0"}, second); err != nil {
		t.Fatalf("get crr error: %v", err)
	}
	secondCond := meta.FindStatusCondition(second.Status.Conditions, appsv1beta1.ContainerRecreateRequestPreRecreateGraceType)
	if secondCond == nil {
		t.Fatalf("PreRecreateGrace condition disappeared after second acquire")
	}
	if !secondCond.LastTransitionTime.Equal(&firstTime) {
		t.Errorf("LastTransitionTime was rewritten: first=%v second=%v (should be preserved)", firstTime, secondCond.LastTransitionTime)
	}
}
