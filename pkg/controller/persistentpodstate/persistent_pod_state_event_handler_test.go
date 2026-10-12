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

package persistentpodstate

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/webhook/pod/mutating"
)

type handlerCtxKey struct{}

// ctxCheckingReader fails the test when a read does not get the context the event handler was called with.
type ctxCheckingReader struct {
	client.Reader
	t     *testing.T
	calls int
}

func (r *ctxCheckingReader) check(ctx context.Context) {
	r.t.Helper()
	r.calls++
	if ctx.Value(handlerCtxKey{}) != "event-context" {
		r.t.Errorf("the read did not receive the context of the event handler")
	}
}

func (r *ctxCheckingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	r.check(ctx)
	return r.Reader.Get(ctx, key, obj, opts...)
}

func (r *ctxCheckingReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	r.check(ctx)
	return r.Reader.List(ctx, list, opts...)
}

func newHandlerTestPPS(apiVersion, kind, name string) *appsv1beta1.PersistentPodState {
	return &appsv1beta1.PersistentPodState{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-test", Name: "pps-" + name},
		Spec: appsv1beta1.PersistentPodStateSpec{
			TargetReference: appsv1beta1.TargetReference{APIVersion: apiVersion, Kind: kind, Name: name},
		},
	}
}

func newHandlerTestQueue() workqueue.TypedRateLimitingInterface[reconcile.Request] {
	return workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
}

func expectEnqueued(t *testing.T, q workqueue.TypedRateLimitingInterface[reconcile.Request], want types.NamespacedName) {
	t.Helper()
	if q.Len() != 1 {
		t.Fatalf("expected 1 queued request, got %d", q.Len())
	}
	got, _ := q.Get()
	if got.NamespacedName != want {
		t.Fatalf("expected request %v, got %v", want, got.NamespacedName)
	}
}

func TestEnqueueRequestForPod_PropagatesContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), handlerCtxKey{}, "event-context")
	owner := metav1.OwnerReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "sts", Controller: ptr.To(true)}
	newPod := func(annotations map[string]string, resourceVersion string, ready bool) *corev1.Pod {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace:       "ns-test",
			Name:            "pod-0",
			ResourceVersion: resourceVersion,
			Annotations:     annotations,
			OwnerReferences: []metav1.OwnerReference{owner},
		}}
		if ready {
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		}
		return pod
	}
	wantRequest := types.NamespacedName{Namespace: "ns-test", Name: "pps-sts"}

	tests := []struct {
		name string
		run  func(p *enqueueRequestForPod, q workqueue.TypedRateLimitingInterface[reconcile.Request])
	}{
		{
			name: "delete reads the injected PersistentPodState",
			run: func(p *enqueueRequestForPod, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				pod := newPod(map[string]string{mutating.InjectedPersistentPodStateKey: "pps-sts"}, "1", false)
				p.Delete(ctx, event.TypedDeleteEvent[*corev1.Pod]{Object: pod}, q)
			},
		},
		{
			name: "delete selects the PersistentPodState of the owner",
			run: func(p *enqueueRequestForPod, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				p.Delete(ctx, event.TypedDeleteEvent[*corev1.Pod]{Object: newPod(nil, "1", false)}, q)
			},
		},
		{
			name: "update of a pod that becomes ready",
			run: func(p *enqueueRequestForPod, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				p.Update(ctx, event.TypedUpdateEvent[*corev1.Pod]{
					ObjectOld: newPod(nil, "1", false),
					ObjectNew: newPod(nil, "2", true),
				}, q)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(newHandlerTestPPS("apps/v1", "StatefulSet", "sts")).Build()
			reader := &ctxCheckingReader{Reader: c, t: t}
			q := newHandlerTestQueue()

			tt.run(&enqueueRequestForPod{reader: reader, client: c}, q)

			expectEnqueued(t, q, wantRequest)
			if reader.calls == 0 {
				t.Fatalf("expected the handler to read through the reader")
			}
		})
	}

	t.Run("update without a relevant change does not read", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		reader := &ctxCheckingReader{Reader: c, t: t}
		q := newHandlerTestQueue()

		p := &enqueueRequestForPod{reader: reader, client: c}
		p.Update(ctx, event.TypedUpdateEvent[*corev1.Pod]{ObjectOld: newPod(nil, "1", false), ObjectNew: newPod(nil, "1", false)}, q)

		if q.Len() != 0 || reader.calls != 0 {
			t.Fatalf("expected nothing to be read or queued, got %d queued and %d reads", q.Len(), reader.calls)
		}
	})
}

func TestEnqueueRequestForStatefulSets_PropagateContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), handlerCtxKey{}, "event-context")
	now := metav1.Now()

	t.Run("StatefulSet", func(t *testing.T) {
		pps := newHandlerTestPPS(KruiseKindSts.GroupVersion().String(), KruiseKindSts.Kind, "sts")
		ppsApps := newHandlerTestPPS(KindSts.GroupVersion().String(), KindSts.Kind, "sts-apps")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pps, ppsApps).Build()
		reader := &ctxCheckingReader{Reader: c, t: t}
		h := &enqueueRequestForStatefulSet{reader: reader}

		// The delete event selects the PersistentPodState referring to the workload.
		q := newHandlerTestQueue()
		h.Delete(ctx, event.TypedDeleteEvent[*appsv1.StatefulSet]{
			Object: &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-test", Name: "sts"}},
		}, q)
		expectEnqueued(t, q, types.NamespacedName{Namespace: "ns-test", Name: "pps-sts"})

		// The update event of a StatefulSet that is being deleted does the same.
		q = newHandlerTestQueue()
		old := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-test", Name: "sts-apps"}}
		deleting := old.DeepCopy()
		deleting.DeletionTimestamp = &now
		h.Update(ctx, event.TypedUpdateEvent[*appsv1.StatefulSet]{ObjectOld: old, ObjectNew: deleting}, q)
		expectEnqueued(t, q, types.NamespacedName{Namespace: "ns-test", Name: "pps-sts-apps"})
		if reader.calls != 2 {
			t.Fatalf("expected 2 reads, got %d", reader.calls)
		}
	})

	t.Run("Kruise StatefulSet", func(t *testing.T) {
		pps := newHandlerTestPPS(KruiseKindSts.GroupVersion().String(), KruiseKindSts.Kind, "sts")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pps).Build()
		reader := &ctxCheckingReader{Reader: c, t: t}
		h := &enqueueRequestForKruiseStatefulSet{reader: reader}

		q := newHandlerTestQueue()
		h.Delete(ctx, event.TypedDeleteEvent[*appsv1beta1.StatefulSet]{
			Object: &appsv1beta1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-test", Name: "sts"}},
		}, q)
		expectEnqueued(t, q, types.NamespacedName{Namespace: "ns-test", Name: "pps-sts"})

		q = newHandlerTestQueue()
		old := &appsv1beta1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-test", Name: "sts"}}
		deleting := old.DeepCopy()
		deleting.DeletionTimestamp = &now
		h.Update(ctx, event.TypedUpdateEvent[*appsv1beta1.StatefulSet]{ObjectOld: old, ObjectNew: deleting}, q)
		expectEnqueued(t, q, types.NamespacedName{Namespace: "ns-test", Name: "pps-sts"})
		if reader.calls != 2 {
			t.Fatalf("expected 2 reads, got %d", reader.calls)
		}
	})

	t.Run("StatefulSet like workload", func(t *testing.T) {
		pps := newHandlerTestPPS("example.io/v1", "CustomSet", "custom")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pps).Build()
		reader := &ctxCheckingReader{Reader: c, t: t}
		h := &enqueueRequestForStatefulSetLike{reader: reader}
		newWorkload := func() *unstructured.Unstructured {
			u := &unstructured.Unstructured{}
			u.SetAPIVersion("example.io/v1")
			u.SetKind("CustomSet")
			u.SetNamespace("ns-test")
			u.SetName("custom")
			return u
		}

		q := newHandlerTestQueue()
		h.Delete(ctx, event.DeleteEvent{Object: newWorkload()}, q)
		expectEnqueued(t, q, types.NamespacedName{Namespace: "ns-test", Name: "pps-custom"})

		q = newHandlerTestQueue()
		deleting := newWorkload()
		deleting.SetDeletionTimestamp(&now)
		h.Update(ctx, event.UpdateEvent{ObjectOld: newWorkload(), ObjectNew: deleting}, q)
		expectEnqueued(t, q, types.NamespacedName{Namespace: "ns-test", Name: "pps-custom"})
		if reader.calls != 2 {
			t.Fatalf("expected 2 reads, got %d", reader.calls)
		}
	})
}
