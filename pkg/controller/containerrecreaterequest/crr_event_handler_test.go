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

package containerrecreaterequest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
)

func TestPodEventHandler_Create(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = policyv1alpha1.AddToScheme(scheme)
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
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crr).Build()
	handler := &podEventHandler{Reader: client}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*v1.Pod]{
		Object: pod,
	}

	handler.Create(context.Background(), evt, queue)

	// Verify that the CRR was queued
	assert.Equal(t, 1, queue.Len())
	item, _ := queue.Get()
	req := item
	assert.Equal(t, "test-crr", req.Name)
	assert.Equal(t, "default", req.Namespace)
}

func TestPodEventHandler_Update(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = policyv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name          string
		oldPod        *v1.Pod
		newPod        *v1.Pod
		shouldEnqueue bool
	}{
		{
			name: "pod deletion timestamp changed",
			oldPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
				},
			},
			newPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-pod",
					Namespace:         "default",
					UID:               "test-pod-uid",
					DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				},
			},
			shouldEnqueue: true,
		},
		{
			name: "container statuses changed",
			oldPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:  "container1",
							Ready: false,
						},
					},
				},
			},
			newPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:  "container1",
							Ready: true,
						},
					},
				},
			},
			shouldEnqueue: true,
		},
		{
			name: "no relevant changes",
			oldPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
					Labels: map[string]string{
						"app": "test",
					},
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:  "container1",
							Ready: true,
						},
					},
				},
			},
			newPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
					Labels: map[string]string{
						"app": "test-updated",
					},
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:  "container1",
							Ready: true,
						},
					},
				},
			},
			shouldEnqueue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crr).Build()
			handler := &podEventHandler{Reader: client}
			queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

			evt := event.TypedUpdateEvent[*v1.Pod]{
				ObjectOld: tt.oldPod,
				ObjectNew: tt.newPod,
			}

			handler.Update(context.Background(), evt, queue)

			if tt.shouldEnqueue {
				assert.Equal(t, 1, queue.Len())
			} else {
				assert.Equal(t, 0, queue.Len())
			}
		})
	}
}

func TestPodEventHandler_Delete(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = policyv1alpha1.AddToScheme(scheme)
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
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crr).Build()
	handler := &podEventHandler{Reader: client}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedDeleteEvent[*v1.Pod]{
		Object: pod,
	}

	handler.Delete(context.Background(), evt, queue)

	assert.Equal(t, 1, queue.Len())
	item, _ := queue.Get()
	req := item
	assert.Equal(t, "test-crr", req.Name)
	assert.Equal(t, "default", req.Namespace)
}

func TestPodEventHandler_Handle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = policyv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name          string
		pod           *v1.Pod
		crrs          []appsv1alpha1.ContainerRecreateRequest
		expectedQueue int
	}{
		{
			name: "single active CRR",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
				},
			},
			crrs: []appsv1alpha1.ContainerRecreateRequest{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "crr-1",
						Namespace: "default",
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
					},
				},
			},
			expectedQueue: 1,
		},
		{
			name: "multiple active CRRs",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
				},
			},
			crrs: []appsv1alpha1.ContainerRecreateRequest{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "crr-1",
						Namespace: "default",
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "crr-2",
						Namespace: "default",
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
					},
				},
			},
			expectedQueue: 2,
		},
		{
			name: "CRR with deletion timestamp should be ignored",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
				},
			},
			crrs: []appsv1alpha1.ContainerRecreateRequest{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "crr-1",
						Namespace: "default",
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
						Finalizers:        []string{"test-finalizer"},
						DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
					},
				},
			},
			expectedQueue: 0,
		},
		{
			name: "completed CRR should be ignored",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
				},
			},
			crrs: []appsv1alpha1.ContainerRecreateRequest{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "crr-1",
						Namespace: "default",
						Labels: map[string]string{
							appsv1alpha1.ContainerRecreateRequestPodUIDKey: "test-pod-uid",
						},
					},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: "test-pod",
					},
					Status: appsv1alpha1.ContainerRecreateRequestStatus{
						CompletionTime: &metav1.Time{Time: metav1.Now().Time},
					},
				},
			},
			expectedQueue: 0,
		},
		{
			name: "no matching CRRs",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					UID:       "test-pod-uid",
				},
			},
			crrs:          []appsv1alpha1.ContainerRecreateRequest{},
			expectedQueue: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := make([]client.Object, len(tt.crrs))
			for i := range tt.crrs {
				objects[i] = &tt.crrs[i]
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			handler := &podEventHandler{Reader: client}
			queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

			handler.handle(tt.pod, queue)

			assert.Equal(t, tt.expectedQueue, queue.Len())
		})
	}
}

func TestPodEventHandler_Generic(t *testing.T) {
	// Generic events should not enqueue anything
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)
	_ = policyv1alpha1.AddToScheme(scheme)
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-pod-uid",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := &podEventHandler{Reader: client}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedGenericEvent[*v1.Pod]{
		Object: pod,
	}

	handler.Generic(context.Background(), evt, queue)

	assert.Equal(t, 0, queue.Len())
}

func TestGetReadinessMessage(t *testing.T) {
	crr := &appsv1alpha1.ContainerRecreateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-crr",
			Namespace: "default",
		},
	}

	msg := getReadinessMessage(crr)

	assert.Equal(t, "ContainerRecreateRequest", msg.UserAgent)
	assert.Equal(t, "default/test-crr", msg.Key)
}
