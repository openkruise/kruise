/*
Copyright 2020 The Kruise Authors.

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

package sidecarset

import (
	"context"
	"fmt"
	"testing"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestPodEventHandler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := enqueueRequestForPod{reader: fakeClient}

	err := fakeClient.Create(context.TODO(), sidecarSetDemo.DeepCopy())
	if nil != err {
		t.Fatalf("unexpected create sidecarSet %s failed: %v", sidecarSetDemo.Name, err)
	}

	// create
	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	createEvt := event.TypedCreateEvent[*corev1.Pod]{
		Object: podDemo,
	}
	handler.Create(context.TODO(), createEvt, createQ)
	if createQ.Len() != 1 {
		t.Errorf("unexpected create event handle queue size, expected 1 actual %d", createQ.Len())
	}

	// update with pod status changed and reconcile
	newPod := podDemo.DeepCopy()
	newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
	readyCondition := podutil.GetPodReadyCondition(newPod.Status)
	readyCondition.Status = corev1.ConditionFalse
	updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	updateEvent := event.TypedUpdateEvent[*corev1.Pod]{
		ObjectOld: podDemo,
		ObjectNew: newPod,
	}
	handler.Update(context.TODO(), updateEvent, updateQ)
	if updateQ.Len() != 1 {
		t.Errorf("unexpected update event handle queue size, expected 1 actual %d", updateQ.Len())
	}

	// update with pod spec changed and no reconcile
	newPod = podDemo.DeepCopy()
	newPod.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
	newPod.Spec.Containers[0].Image = "nginx:latest"
	updateQ = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	updateEvent = event.TypedUpdateEvent[*corev1.Pod]{
		ObjectOld: podDemo,
		ObjectNew: newPod,
	}
	handler.Update(context.TODO(), updateEvent, updateQ)
	if updateQ.Len() != 0 {
		t.Errorf("unexpected update event handle queue size, expected 0 actual %d", updateQ.Len())
	}

	// delete
	deleteQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	deleteEvt := event.TypedDeleteEvent[*corev1.Pod]{
		Object: podDemo,
	}
	handler.Delete(context.TODO(), deleteEvt, deleteQ)
	if deleteQ.Len() != 0 {
		t.Errorf("unexpected delete event handle queue size, expected 1 actual %d", deleteQ.Len())
	}
}

func TestGetPodMatchedSidecarSets(t *testing.T) {
	cases := []struct {
		name                  string
		getPod                func() *corev1.Pod
		getSidecarSets        func() []*appsv1alpha1.SidecarSet
		exceptSidecarSetCount int
	}{
		{
			name: "pod matched single sidecarSet",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[sidecarcontrol.SidecarSetListAnnotation] = "test-sidecarset-2"
				return pod
			},
			getSidecarSets: func() []*appsv1alpha1.SidecarSet {
				sidecar1 := sidecarSetDemo.DeepCopy()
				sidecar1.Name = "test-sidecarset-1"
				sidecar2 := sidecarSetDemo.DeepCopy()
				sidecar2.Name = "test-sidecarset-2"
				sidecar3 := sidecarSetDemo.DeepCopy()
				sidecar3.Name = "test-sidecarset-3"
				return []*appsv1alpha1.SidecarSet{sidecar1, sidecar2, sidecar3}
			},
			exceptSidecarSetCount: 1,
		},
		{
			name: "pod matched two sidecarSets",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[sidecarcontrol.SidecarSetListAnnotation] = "test-sidecarset-1,test-sidecarset-3"
				return pod
			},
			getSidecarSets: func() []*appsv1alpha1.SidecarSet {
				sidecar1 := sidecarSetDemo.DeepCopy()
				sidecar1.Name = "test-sidecarset-1"
				sidecar2 := sidecarSetDemo.DeepCopy()
				sidecar2.Name = "test-sidecarset-2"
				sidecar3 := sidecarSetDemo.DeepCopy()
				sidecar3.Name = "test-sidecarset-3"
				return []*appsv1alpha1.SidecarSet{sidecar1, sidecar2, sidecar3}
			},
			exceptSidecarSetCount: 2,
		},
		{
			name: "pod matched no sidecarSets",
			getPod: func() *corev1.Pod {
				pod := podDemo.DeepCopy()
				pod.Annotations[sidecarcontrol.SidecarSetListAnnotation] = "test-sidecarset-4"
				return pod
			},
			getSidecarSets: func() []*appsv1alpha1.SidecarSet {
				sidecar1 := sidecarSetDemo.DeepCopy()
				sidecar1.Name = "test-sidecarset-1"
				sidecar2 := sidecarSetDemo.DeepCopy()
				sidecar2.Name = "test-sidecarset-2"
				sidecar3 := sidecarSetDemo.DeepCopy()
				sidecar3.Name = "test-sidecarset-3"
				return []*appsv1alpha1.SidecarSet{sidecar1, sidecar2, sidecar3}
			},
			exceptSidecarSetCount: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			pod := cs.getPod()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
			sidecarSets := cs.getSidecarSets()
			for _, sidecarSet := range sidecarSets {
				fakeClient.Create(context.TODO(), sidecarSet)
			}
			e := enqueueRequestForPod{fakeClient}
			matched, err := e.getPodMatchedSidecarSets(pod)
			if err != nil {
				t.Fatalf("getPodMatchedSidecarSets failed: %s", err.Error())
			}

			if len(matched) != cs.exceptSidecarSetCount {
				t.Fatalf("except matched sidecarSet(count=%d), but get count=%d", cs.exceptSidecarSetCount, len(matched))
			}
		})
	}
}
