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
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

// fakeQueue implements workqueue.TypedRateLimitingInterface for testing
type fakeQueue struct {
	workqueue.TypedRateLimitingInterface[reconcile.Request]
	items []reconcile.Request
}

func (q *fakeQueue) Add(item reconcile.Request) {
	q.items = append(q.items, item)
}

func TestPodEventHandler_Update(t *testing.T) {
	podUID := "pod-uid-123"
	namespace := "default"

	activeCRR := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crr-active",
			Namespace: namespace,
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: podUID,
			},
		},
	}

	completedCRR := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crr-completed",
			Namespace: namespace,
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: podUID,
			},
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			CompletionTime: &metav1.Time{Time: metav1.Now().Time},
		},
	}

	otherCRR := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crr-other",
			Namespace: namespace,
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "other-uid",
			},
		},
	}

	// scheme is defined in crr_controller_test.go which is in the same package
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(activeCRR, completedCRR, otherCRR).Build()
	handler := &podEventHandler{Reader: fakeClient}

	tests := []struct {
		name          string
		oldPod        *v1.Pod
		newPod        *v1.Pod
		expectCount   int
		expectRequest *reconcile.Request
	}{
		{
			name: "Update: Container status unchanged -> No enqueue",
			oldPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: namespace, UID: types.UID(podUID)},
				Status:     v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{{Name: "c1", Ready: true}}},
			},
			newPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: namespace, UID: types.UID(podUID)},
				Status:     v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{{Name: "c1", Ready: true}}},
			},
			expectCount: 0,
		},
		{
			name: "Update: Container status changed -> Enqueue active CRR",
			oldPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: namespace, UID: types.UID(podUID)},
				Status:     v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{{Name: "c1", Ready: true}}},
			},
			newPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: namespace, UID: types.UID(podUID)},
				Status:     v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{{Name: "c1", Ready: false}}},
			},
			expectCount:   1,
			expectRequest: &reconcile.Request{NamespacedName: types.NamespacedName{Name: activeCRR.Name, Namespace: namespace}},
		},
		{
			name: "Update: DeletionTimestamp set -> Enqueue active CRR",
			oldPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: namespace, UID: types.UID(podUID)},
			},
			newPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: namespace, UID: types.UID(podUID), DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time}},
			},
			expectCount:   1,
			expectRequest: &reconcile.Request{NamespacedName: types.NamespacedName{Name: activeCRR.Name, Namespace: namespace}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := &fakeQueue{}
			evt := event.TypedUpdateEvent[*v1.Pod]{
				ObjectOld: tt.oldPod,
				ObjectNew: tt.newPod,
			}
			handler.Update(context.TODO(), evt, queue)

			if len(queue.items) != tt.expectCount {
				t.Errorf("Expected %d items, got %d", tt.expectCount, len(queue.items))
			}
			if tt.expectRequest != nil && len(queue.items) > 0 {
				if !reflect.DeepEqual(queue.items[0], *tt.expectRequest) {
					t.Errorf("Expected request %v, got %v", *tt.expectRequest, queue.items[0])
				}
			}
		})
	}
}
