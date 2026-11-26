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

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

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

func TestGetPubForPodRolloutReplicaSet(t *testing.T) {
	rolloutUID := types.UID("rollout-uid")
	rsUID := types.UID("rollout-rs-uid")

	rolloutObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Rollout",
			"metadata": map[string]interface{}{
				"namespace": "default",
				"name":      "demo-rollout",
				"uid":       string(rolloutUID),
			},
		},
	}
	rs := &apps.ReplicaSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSet"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "demo-rollout-rs",
			UID:       rsUID,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "argoproj.io/v1alpha1",
				Kind:       "Rollout",
				Name:       "demo-rollout",
				UID:        rolloutUID,
				Controller: ptr.To(true),
			}},
		},
		Spec: apps.ReplicaSetSpec{Replicas: ptr.To[int32](2)},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "demo-rollout-pod-0",
			Labels:    map[string]string{"app": "demo"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       "demo-rollout-rs",
				UID:        rsUID,
				Controller: ptr.To(true),
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	pub := &policyv1beta1.PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{APIVersion: policyv1beta1.GroupVersion.String(), Kind: "PodUnavailableBudget"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "demo-pub",
		},
		Spec: policyv1beta1.PodUnavailableBudgetSpec{
			TargetReference: &policyv1beta1.TargetReference{
				APIVersion: "argoproj.io/v1alpha1",
				Kind:       "Rollout",
				Name:       "demo-rollout",
			},
			MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(rolloutObj, rs, pub).
		WithIndex(&corev1.Pod{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
			var owners []string
			for _, ref := range obj.GetOwnerReferences() {
				owners = append(owners, string(ref.UID))
			}
			return owners
		}).
		Build()

	origFinder := controllerfinder.Finder
	origPubControl := pubcontrol.PubControl
	t.Cleanup(func() {
		controllerfinder.Finder = origFinder
		pubcontrol.PubControl = origPubControl
	})

	finder := &controllerfinder.ControllerFinder{Client: fakeClient}
	controllerfinder.Finder = finder
	pubcontrol.InitPubControl(fakeClient, finder, record.NewFakeRecorder(10))

	gotPub, err := GetPubForPod(fakeClient, pod)
	if err != nil {
		t.Fatalf("GetPubForPod returned error: %v", err)
	}
	if gotPub == nil || gotPub.Name != pub.Name {
		t.Fatalf("expected PUB %q, got %#v", pub.Name, gotPub)
	}

	// Simulate mutating webhook stamping kruise.io/related-pub and verify the
	// Pod event handler enqueues the Rollout's PUB on add.
	annotatedPod := pod.DeepCopy()
	annotatedPod.Annotations = pubcontrol.SetPodRelatedPubAnnotation(annotatedPod.Annotations, gotPub.Name)

	handler := newEnqueueRequestForPod(fakeClient)
	addQ := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
	handler.Create(context.TODO(), event.TypedCreateEvent[*corev1.Pod]{Object: annotatedPod}, addQ)
	if addQ.Len() != 1 {
		t.Fatalf("expected 1 queued reconcile request on pod create, got %d", addQ.Len())
	}
}
