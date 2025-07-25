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

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestPodEventHandler_Create(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
	}

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-crr",
			Namespace: "default",
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: "test-pod",
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

	handler := &podEventHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*v1.Pod]{
		Object: pod,
	}

	handler.Create(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-crr"))
	g.Expect(request.Namespace).To(gomega.Equal("default"))
}

func TestPodEventHandler_Update(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	oldPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:         "container1",
					Ready:        false,
					RestartCount: 0,
				},
			},
		},
	}

	newPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:         "container1",
					Ready:        true,
					RestartCount: 1,
				},
			},
		},
	}

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-crr",
			Namespace: "default",
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: "test-pod",
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

	handler := &podEventHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedUpdateEvent[*v1.Pod]{
		ObjectOld: oldPod,
		ObjectNew: newPod,
	}

	handler.Update(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-crr"))
	g.Expect(request.Namespace).To(gomega.Equal("default"))
}

func TestPodEventHandler_UpdateWithDeletion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deletionTime := metav1.Now()
	oldPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
	}

	newPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			Namespace:         "default",
			UID:               "test-pod-uid",
			DeletionTimestamp: &deletionTime,
		},
	}

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-crr",
			Namespace: "default",
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: "test-pod",
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

	handler := &podEventHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedUpdateEvent[*v1.Pod]{
		ObjectOld: oldPod,
		ObjectNew: newPod,
	}

	handler.Update(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-crr"))
	g.Expect(request.Namespace).To(gomega.Equal("default"))
}

func TestPodEventHandler_Delete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
	}

	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-crr",
			Namespace: "default",
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: "test-pod",
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

	handler := &podEventHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedDeleteEvent[*v1.Pod]{
		Object: pod,
	}

	handler.Delete(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-crr"))
	g.Expect(request.Namespace).To(gomega.Equal("default"))
}

func TestPodEventHandler_IgnoreCompletedCRR(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
	}

	// CRR with completion time should be ignored
	completionTime := metav1.Now()
	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-crr",
			Namespace: "default",
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
			},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: "test-pod",
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			Phase:          appsv1alpha1.ContainerRecreateRequestCompleted,
			CompletionTime: &completionTime,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

	handler := &podEventHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*v1.Pod]{
		Object: pod,
	}

	handler.Create(context.TODO(), evt, queue)

	// Should not enqueue completed CRR
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestPodEventHandler_IgnoreDeletedCRR(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
	}

	// CRR with deletion timestamp should be ignored
	deletionTime := metav1.Now()
	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-crr",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			Labels: map[string]string{
				appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
			},
			Finalizers: []string{"sss"},
		},
		Spec: appsv1alpha1.ContainerRecreateRequestSpec{
			PodName: "test-pod",
		},
		Status: appsv1alpha1.ContainerRecreateRequestStatus{
			Phase: appsv1alpha1.ContainerRecreateRequestRecreating,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(crr).Build()

	handler := &podEventHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*v1.Pod]{
		Object: pod,
	}

	handler.Create(context.TODO(), evt, queue)

	// Should not enqueue deleted CRR
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestPodEventHandler_Generic(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &podEventHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedGenericEvent[*v1.Pod]{
		Object: pod,
	}

	handler.Generic(context.TODO(), evt, queue)

	// Generic events should not trigger reconciliation
	g.Expect(queue.Len()).To(gomega.Equal(0))
}
