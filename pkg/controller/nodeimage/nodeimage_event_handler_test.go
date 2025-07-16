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

package nodeimage

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

func TestNodeHandler_Create(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*v1.Node]{
		Object: node,
	}

	handler.Create(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-node"))
}

func TestNodeHandler_CreateVirtualKubelet(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"type": VirtualKubelet,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*v1.Node]{
		Object: node,
	}

	handler.Create(context.TODO(), evt, queue)

	// Virtual kubelet nodes should be ignored
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestNodeHandler_CreateWithDeletion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	deletionTime := metav1.Now()
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-node",
			DeletionTimestamp: &deletionTime,
		},
	}

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(nodeImage).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*v1.Node]{
		Object: node,
	}

	handler.Create(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-node"))
}

func TestNodeHandler_CreateWithNotReadyNode(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionFalse,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*v1.Node]{
		Object: node,
	}

	handler.Create(context.TODO(), evt, queue)

	// Not ready nodes should not be enqueued
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestNodeHandler_Update(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	oldNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
	}

	newNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
				"kubernetes.io/os":   "linux",
			},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(nodeImage).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedUpdateEvent[*v1.Node]{
		ObjectOld: oldNode,
		ObjectNew: newNode,
	}

	handler.Update(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-node"))
}

func TestNodeHandler_UpdateWithDeletion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	oldNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	deletionTime := metav1.Now()
	newNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-node",
			DeletionTimestamp: &deletionTime,
		},
	}

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(nodeImage).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedUpdateEvent[*v1.Node]{
		ObjectOld: oldNode,
		ObjectNew: newNode,
	}

	handler.Update(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-node"))
}

func TestNodeHandler_UpdateSameLabels(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	oldNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
	}

	newNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
	}

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"kubernetes.io/arch": "amd64",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(nodeImage).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedUpdateEvent[*v1.Node]{
		ObjectOld: oldNode,
		ObjectNew: newNode,
	}

	handler.Update(context.TODO(), evt, queue)

	// No change in labels, should not enqueue
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestNodeHandler_Delete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	nodeImage := &appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(nodeImage).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedDeleteEvent[*v1.Node]{
		Object: node,
	}

	handler.Delete(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(1))

	item, _ := queue.Get()
	request := item
	g.Expect(request.Name).To(gomega.Equal("test-node"))
}

func TestNodeHandler_DeleteVirtualKubelet(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"type": VirtualKubelet,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedDeleteEvent[*v1.Node]{
		Object: node,
	}

	handler.Delete(context.TODO(), evt, queue)

	// Virtual kubelet nodes should be ignored
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestNodeHandler_DeleteWithoutNodeImage(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedDeleteEvent[*v1.Node]{
		Object: node,
	}

	handler.Delete(context.TODO(), evt, queue)

	// No NodeImage exists, should not enqueue
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestNodeHandler_Generic(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &nodeHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedGenericEvent[*v1.Node]{
		Object: node,
	}

	handler.Generic(context.TODO(), evt, queue)

	// Generic events should not trigger reconciliation
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestImagePullJobHandler_Delete(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	// Cache some node images for this job
	// Cache is not directly exposed, skip this test functionality

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &imagePullJobHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedDeleteEvent[*appsv1alpha1.ImagePullJob]{
		Object: job,
	}

	handler.Delete(context.TODO(), evt, queue)

	g.Expect(queue.Len()).To(gomega.Equal(2))

	// Check that both nodes are enqueued
	requests := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		item, _ := queue.Get()
		request := item
		requests = append(requests, request.Name)
	}
	g.Expect(requests).To(gomega.ContainElement("node1"))
	g.Expect(requests).To(gomega.ContainElement("node2"))
}

func TestImagePullJobHandler_Create(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &imagePullJobHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedCreateEvent[*appsv1alpha1.ImagePullJob]{
		Object: job,
	}

	handler.Create(context.TODO(), evt, queue)

	// Create events should not trigger reconciliation
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestImagePullJobHandler_Update(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	oldJob := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:1.0",
		},
	}

	newJob := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &imagePullJobHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedUpdateEvent[*appsv1alpha1.ImagePullJob]{
		ObjectOld: oldJob,
		ObjectNew: newJob,
	}

	handler.Update(context.TODO(), evt, queue)

	// Update events should not trigger reconciliation
	g.Expect(queue.Len()).To(gomega.Equal(0))
}

func TestImagePullJobHandler_Generic(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

	handler := &imagePullJobHandler{Reader: fakeClient}
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	evt := event.TypedGenericEvent[*appsv1alpha1.ImagePullJob]{
		Object: job,
	}

	handler.Generic(context.TODO(), evt, queue)

	// Generic events should not trigger reconciliation
	g.Expect(queue.Len()).To(gomega.Equal(0))
}
