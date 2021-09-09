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
	utilpodreadiness "github.com/openkruise/kruise/pkg/util/podreadiness"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
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
	fakeClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(pod0, pod1).Build()
	reconciler := &ReconcilePodReadiness{Client: fakeClient}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pod0.Namespace, Name: pod0.Name}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pod1.Namespace, Name: pod1.Name}})
	if err != nil {
		t.Fatal(err)
	}

	newPod0 := &v1.Pod{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: pod0.Namespace, Name: pod0.Name}, newPod0); err != nil {
		t.Fatal(err)
	}
	condition := utilpodreadiness.GetReadinessCondition(newPod0)
	if condition == nil || condition.Status != v1.ConditionTrue {
		t.Fatalf("expect pod0 ready, got %v", condition)
	}

	newPod1 := &v1.Pod{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: pod1.Namespace, Name: pod1.Name}, newPod1); err != nil {
		t.Fatal(err)
	}
	condition = utilpodreadiness.GetReadinessCondition(newPod1)
	if condition != nil {
		t.Fatalf("expect pod1 no ready, got %v", condition)
	}
}
