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

package podreadiness

import (
	"context"
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPodReadiness(t *testing.T) {
	pod0 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "pod0"},
		Spec: v1.PodSpec{
			ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.KruisePodReadyConditionType}},
		},
	}
	pod1 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "pod1"},
		Spec: v1.PodSpec{
			ReadinessGates: []v1.PodReadinessGate{},
		},
	}
	fakeClient := fake.NewFakeClientWithScheme(clientgoscheme.Scheme, pod0, pod1)

	msg0 := Message{UserAgent: "ua1", Key: "foo"}
	msg1 := Message{UserAgent: "ua1", Key: "bar"}

	if err := AddNotReadyKey(fakeClient, pod0, msg0); err != nil {
		t.Fatal(err)
	}
	if err := AddNotReadyKey(fakeClient, pod0, msg1); err != nil {
		t.Fatal(err)
	}
	if err := AddNotReadyKey(fakeClient, pod1, msg0); err != nil {
		t.Fatal(err)
	}
	if err := AddNotReadyKey(fakeClient, pod1, msg1); err != nil {
		t.Fatal(err)
	}

	newPod0 := &v1.Pod{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: pod0.Namespace, Name: pod0.Name}, newPod0); err != nil {
		t.Fatal(err)
	}
	if !alreadyHasKey(newPod0, msg0) || !alreadyHasKey(newPod0, msg1) {
		t.Fatalf("expect already has key, but not")
	}
	condition := GetReadinessCondition(newPod0)
	if condition.Status != v1.ConditionFalse {
		t.Fatalf("expect condition false, but not")
	}

	newPod1 := &v1.Pod{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: pod1.Namespace, Name: pod1.Name}, newPod1); err != nil {
		t.Fatal(err)
	}
	if alreadyHasKey(newPod1, msg0) || alreadyHasKey(newPod1, msg1) {
		t.Fatalf("expect not have key, but it does")
	}
	if condition = GetReadinessCondition(newPod1); condition != nil {
		t.Fatalf("expect condition nil, but exists: %v", condition)
	}

	if err := RemoveNotReadyKey(fakeClient, newPod0, msg0); err != nil {
		t.Fatal(err)
	}
	newPod0 = &v1.Pod{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: pod0.Namespace, Name: pod0.Name}, newPod0); err != nil {
		t.Fatal(err)
	}
	if !alreadyHasKey(newPod0, msg1) {
		t.Fatalf("expect already has key, but not")
	}
	if alreadyHasKey(newPod0, msg0) {
		t.Fatalf("expect not have key, but it does")
	}
	condition = GetReadinessCondition(newPod0)
	if condition.Status != v1.ConditionFalse {
		t.Fatalf("expect condition false, but not")
	}

	if err := RemoveNotReadyKey(fakeClient, newPod0, msg1); err != nil {
		t.Fatal(err)
	}
	newPod0 = &v1.Pod{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: pod0.Namespace, Name: pod0.Name}, newPod0); err != nil {
		t.Fatal(err)
	}
	if alreadyHasKey(newPod0, msg0) || alreadyHasKey(newPod0, msg1) {
		t.Fatalf("expect not have key, but it does")
	}
	condition = GetReadinessCondition(newPod0)
	if condition.Status != v1.ConditionTrue {
		t.Fatalf("expect condition true, but not")
	}
}
