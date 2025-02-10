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
	"k8s.io/client-go/util/workqueue"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openkruise/kruise/pkg/control/pubcontrol"
)

func TestPodEventHandler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := newEnqueueRequestForPod(fakeClient)

	err := fakeClient.Create(context.TODO(), pubDemo.DeepCopy())
	if nil != err {
		t.Fatalf("unexpected create pub %s failed: %v", pubDemo.Name, err)
	}

	// create
	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
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
	updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
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
	updateQ = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
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
