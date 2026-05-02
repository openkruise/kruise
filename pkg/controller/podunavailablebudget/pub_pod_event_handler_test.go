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

package podunavailablebudget

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
)

// fakeMgrForPUBTest is a minimal manager.Manager stub that only exposes the
// client — enough to satisfy SetEnqueueRequestForPUB in unit tests.
type fakeMgrForPUBTest struct {
	manager.Manager
	client client.Client
}

func (f *fakeMgrForPUBTest) GetClient() client.Client { return f.client }
func (f *fakeMgrForPUBTest) GetScheme() *runtime.Scheme { return scheme }

func TestPodEventHandler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := newEnqueueRequestForPod(fakeClient)

	err := fakeClient.Create(context.TODO(), pubDemo.DeepCopy())
	if nil != err {
		t.Fatalf("unexpected create pub %s failed: %v", pubDemo.Name, err)
	}

	// create
	createQ := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
	createEvt := event.TypedCreateEvent[*corev1.Pod]{
		Object: podDemo.DeepCopy(),
	}
	createEvt.Object.SetAnnotations(map[string]string{pubcontrol.PodRelatedPubAnnotation: pubDemo.Name})
	handler.Create(context.TODO(), createEvt, createQ)
	if createQ.Len() != 1 {
		t.Errorf("unexpected create event handle queue size, expected 1 actual %d", createQ.Len())
	}

	// update with pod status changed and reconcile
	newPod := podDemo.DeepCopy()
	newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
	readyCondition := podutil.GetPodReadyCondition(newPod.Status)
	readyCondition.Status = corev1.ConditionFalse
	updateQ := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
	updateEvent := event.TypedUpdateEvent[*corev1.Pod]{
		ObjectOld: podDemo,
		ObjectNew: newPod,
	}
	updateEvent.ObjectOld.SetAnnotations(map[string]string{pubcontrol.PodRelatedPubAnnotation: pubDemo.Name})
	updateEvent.ObjectNew.SetAnnotations(map[string]string{pubcontrol.PodRelatedPubAnnotation: pubDemo.Name})
	handler.Update(context.TODO(), updateEvent, updateQ)
	if updateQ.Len() != 1 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

	// update with pod spec changed and no reconcile
	newPod = podDemo.DeepCopy()
	newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
	newPod.Spec.Containers[0].Image = "nginx:latest"
	updateQ = workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
	updateEvent = event.TypedUpdateEvent[*corev1.Pod]{
		ObjectOld: podDemo,
		ObjectNew: newPod,
	}
	updateEvent.ObjectOld.SetAnnotations(map[string]string{pubcontrol.PodRelatedPubAnnotation: pubDemo.Name})
	updateEvent.ObjectNew.SetAnnotations(map[string]string{pubcontrol.PodRelatedPubAnnotation: pubDemo.Name})
	handler.Update(context.TODO(), updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 0 actual %d", updateQ.Len())
	}
}

// TestSetEnqueueRequestForPUB_NoMatchShouldNotEnqueue verifies that when a
// workload scale event fires but no PodUnavailableBudget matches that workload,
// addSetRequest must NOT enqueue anything. Before the fix, the function always
// called q.Add regardless of whether a match was found, causing spurious
// reconciles with an empty NamespacedName on every scale event.
func TestSetEnqueueRequestForPUB_NoMatchShouldNotEnqueue(t *testing.T) {
	// Build a fake client with no PUBs at all — nothing should match.
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	handler := &SetEnqueueRequestForPUB{
		mgr: &fakeMgrForPUBTest{client: fakeClient},
	}

	q := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)

	dep := deploymentDemo.DeepCopy()
	handler.Update(context.TODO(), event.UpdateEvent{
		ObjectOld: dep,
		ObjectNew: dep,
	}, q)

	if q.Len() != 0 {
		t.Errorf("expected queue to be empty when no PUB matches the workload, but got %d item(s)", q.Len())
	}
}

// TestSetEnqueueRequestForPUB_MatchShouldEnqueue verifies that when a workload
// scale event fires and a matching PodUnavailableBudget exists, exactly one
// reconcile request is enqueued with the correct NamespacedName.
func TestSetEnqueueRequestForPUB_MatchShouldEnqueue(t *testing.T) {
	pub := pubDemo.DeepCopy()
	// Give it a targetRef pointing at our demo Deployment so it matches.
	pub.Spec.Selector = nil
	pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       deploymentDemo.Name,
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pub).Build()
	handler := &SetEnqueueRequestForPUB{
		mgr: &fakeMgrForPUBTest{client: fakeClient},
	}

	q := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)

	dep := deploymentDemo.DeepCopy()
	handler.Update(context.TODO(), event.UpdateEvent{
		ObjectOld: dep,
		ObjectNew: dep,
	}, q)

	if q.Len() != 1 {
		t.Errorf("expected exactly 1 item enqueued when a matching PUB exists, but got %d", q.Len())
	}
	item, _ := q.Get()
	if item.Name != pub.Name || item.Namespace != pub.Namespace {
		t.Errorf("enqueued wrong NamespacedName: got %v, want %v/%v", item, pub.Namespace, pub.Name)
	}
}
