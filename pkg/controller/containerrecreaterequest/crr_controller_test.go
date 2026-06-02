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

package containerrecreaterequest

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestCRRDeadlineMarksContainersFailed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)

	creationTime := metav1.NewTime(time.Now().Add(-30 * time.Second))
	deadlineSeconds := int64(10)

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-crr",
			Namespace:         "default",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName:               "test-pod",
			ActiveDeadlineSeconds: &deadlineSeconds,
			Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
			ContainerRecreateStates: []appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
				{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestRecreating},
				{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestPending},
			},
		},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(crr, pod).
		WithStatusSubresource(crr).
		Build()

	r := &ReconcileContainerRecreateRequest{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	_, err := r.Reconcile(context.TODO(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-crr", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	updatedCRR := &appsv1alpha1.ContainerRecreateRequest{}
	_ = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "test-crr", Namespace: "default"}, updatedCRR)

	if updatedCRR.Status.Phase != appsv1alpha1.ContainerRecreateRequestCompleted {
		t.Errorf("Expected CRR phase Completed, got %s", updatedCRR.Status.Phase)
	}

	for _, state := range updatedCRR.Status.ContainerRecreateStates {
		if state.Phase != appsv1alpha1.ContainerRecreateRequestFailed {
			t.Errorf("Expected container %s phase Failed, got %s", state.Name, state.Phase)
		}
	}

	t.Log("All containers correctly marked as Failed on deadline expiry")
}
