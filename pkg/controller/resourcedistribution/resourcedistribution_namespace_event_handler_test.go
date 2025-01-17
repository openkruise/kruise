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

package resourcedistribution

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var (
	enqueueHandler = &enqueueRequestForNamespace{}
)

func init() {

}

func TestNamespaceEventHandler(t *testing.T) {
	distributor1 := buildResourceDistributionWithSecret()
	env := append(makeEnvironment(), distributor1)
	handlerClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(env...).Build()
	enqueueHandler.reader = handlerClient

	// case 1
	namespaceDemo1 := &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: "ns-6",
			Labels: map[string]string{
				"group": "one",
			},
		},
	}
	testEnqueueRequestForNamespaceCreate(namespaceDemo1, 1, t)

	// case 2
	namespaceDemo2 := namespaceDemo1.DeepCopy()
	namespaceDemo2.ObjectMeta.Labels["group"] = "seven"
	distributor2 := buildResourceDistributionWithSecret()
	distributor2.SetName("test-resource-distribution-2")
	distributor2.Spec.Targets.NamespaceLabelSelector.MatchLabels["group"] = "seven"
	if err := handlerClient.Create(context.TODO(), distributor2, &client.CreateOptions{}); err != nil {
		t.Fatalf("add distributor2 to fake client, err %v", err)
	}
	testEnqueueRequestForNamespaceUpdate(namespaceDemo1, namespaceDemo2, 2, t)

	// case 3
	namespaceDemo1.SetName("ns-1")
	namespaceDemo1.SetNamespace("ns-1")
	testEnqueueRequestForNamespaceDelete(namespaceDemo1, 2, t)
	namespaceDemo2.SetName("ns-8")
	namespaceDemo2.SetNamespace("ns-8")
	testEnqueueRequestForNamespaceDelete(namespaceDemo2, 0, t)
}

func testEnqueueRequestForNamespaceCreate(namespace *corev1.Namespace, expectedNumber int, t *testing.T) {
	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	createEvt := event.TypedCreateEvent[*corev1.Namespace]{
		Object: namespace,
	}
	enqueueHandler.Create(context.TODO(), createEvt, createQ)
	if createQ.Len() != expectedNumber {
		t.Errorf("unexpected create event handle queue size, expected %d actual %d", expectedNumber, createQ.Len())
	}
}

func testEnqueueRequestForNamespaceUpdate(namespaceOld, namespaceNew *corev1.Namespace, expectedNumber int, t *testing.T) {
	updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	updateEvt := event.TypedUpdateEvent[*corev1.Namespace]{
		ObjectOld: namespaceOld,
		ObjectNew: namespaceNew,
	}
	enqueueHandler.Update(context.TODO(), updateEvt, updateQ)
	if updateQ.Len() != expectedNumber {
		t.Errorf("unexpected update event handle queue size, expected %d actual %d", expectedNumber, updateQ.Len())
	}
}

func testEnqueueRequestForNamespaceDelete(namespace *corev1.Namespace, expectedNumber int, t *testing.T) {
	deleteQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	deleteEvt := event.TypedDeleteEvent[*corev1.Namespace]{
		Object: namespace,
	}
	enqueueHandler.Delete(context.TODO(), deleteEvt, deleteQ)
	if deleteQ.Len() != expectedNumber {
		t.Errorf("unexpected delete event handle queue size, expected %d actual %d", expectedNumber, deleteQ.Len())
	}
}
