/*
Copyright 2022 The Kruise Authors.

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

package nodepodprobe

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var (
	demoNode = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: corev1.NodeStatus{
			Phase: corev1.NodeRunning,
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
)

func TestNodeEventHandler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := enqueueRequestForNode{Reader: fakeClient}
	demo := demoNode.DeepCopy()
	_ = fakeClient.Create(context.TODO(), demo)
	// create
	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	createEvt := event.TypedCreateEvent[*corev1.Node]{
		Object: demo,
	}
	handler.Create(context.TODO(), createEvt, createQ)
	if createQ.Len() != 1 {
		t.Errorf("unexpected create event handle queue size, expected 1 actual %d", createQ.Len())
	}
}
