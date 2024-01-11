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

package containerlauchpriority

import (
	"context"
	"testing"
	"time"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	pod0 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "pod0"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name: "testContainer1",
				Env: []v1.EnvVar{{
					Name: appspub.ContainerLaunchBarrierEnvName,
					ValueFrom: &v1.EnvVarSource{
						ConfigMapKeyRef: &v1.ConfigMapKeySelector{
							Key: "p_100",
						},
					},
				}},
			}},
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{{
				Type:   v1.PodInitialized,
				Status: v1.ConditionTrue,
			}, {
				Type:   v1.ContainersReady,
				Status: v1.ConditionFalse,
			}},
			ContainerStatuses: []v1.ContainerStatus{{
				Name:  "testContainer1",
				Ready: false,
			}},
		},
	}
	pod1 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "pod1"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name: "testContainer1",
				Env: []v1.EnvVar{{
					Name: appspub.ContainerLaunchBarrierEnvName,
					ValueFrom: &v1.EnvVarSource{
						ConfigMapKeyRef: &v1.ConfigMapKeySelector{
							Key: "p_100",
						},
					},
				}},
			}, {
				Name: "testContainer2",
				Env: []v1.EnvVar{{
					Name: appspub.ContainerLaunchBarrierEnvName,
					ValueFrom: &v1.EnvVarSource{
						ConfigMapKeyRef: &v1.ConfigMapKeySelector{
							Key: "p_1000",
						},
					},
				}, {
					Name:  appspub.ContainerLaunchTimeOutEnvName,
					Value: "3",
				}},
			}},
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{{
				Type:   v1.PodInitialized,
				Status: v1.ConditionTrue,
			}, {
				Type:   v1.ContainersReady,
				Status: v1.ConditionFalse,
			}},
			ContainerStatuses: []v1.ContainerStatus{{
				Name:  "testContainer2",
				Ready: false,
			}, {
				Name:  "testContainer1",
				Ready: false,
			}},
		},
	}

	barrier0 := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "pod0-barrier",
			Annotations: map[string]string{
				appspub.ContainerLaunchPriorityUpdateTimeKey: time.Now().Format(time.RFC3339),
			},
		},
	}
	barrier1 := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "pod1-barrier",
			Annotations: map[string]string{
				appspub.ContainerLaunchPriorityUpdateTimeKey: time.Now().Format(time.RFC3339),
			},
		},
	}

	fakeClient := fake.NewFakeClientWithScheme(clientgoscheme.Scheme, pod0, pod1, barrier0, barrier1)
	recorder := record.NewFakeRecorder(100)
	reconciler := &ReconcileContainerLaunchPriority{Client: fakeClient, recorder: recorder}
	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pod0.Namespace, Name: pod0.Name}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pod1.Namespace, Name: pod1.Name}})
	if err != nil {
		t.Fatal(err)
	}

	newBarrier0 := &v1.ConfigMap{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: barrier0.Namespace, Name: barrier0.Name}, newBarrier0); err != nil {
		t.Fatal(err)
	}
	if v, ok := newBarrier0.Data["p_100"]; !ok {
		t.Fatalf("expect barrier0 env set, but not")
	} else if v != "true" {
		t.Fatalf("expect barrier0 p_100 to be true, but get %s", v)
	}

	newBarrier1 := &v1.ConfigMap{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: barrier1.Namespace, Name: barrier1.Name}, newBarrier1); err != nil {
		t.Fatal(err)
	}
	if _, ok := newBarrier1.Data["p_1000"]; !ok {
		t.Fatalf("expect barrier1 env set, but not")
	}

	if _, ok := newBarrier1.Data["p_100"]; ok {
		t.Fatalf("expect barrier1 p_100 not to be set, but get ")
	}
	time.Sleep(time.Second * 4)
	_, err = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pod1.Namespace, Name: pod1.Name}})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 1 {
		t.Fatal("expect a event")
	}
	pod1.Status.ContainerStatuses[0].Ready = true
	err = fakeClient.Update(context.Background(), pod1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: pod1.Namespace, Name: pod1.Name}})
	if err != nil {
		t.Fatal(err)
	}

	newBarrier1 = &v1.ConfigMap{}
	if err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: barrier1.Namespace, Name: barrier1.Name}, newBarrier1); err != nil {
		t.Fatal(err)
	}

	if _, ok := newBarrier1.Data["p_100"]; !ok {
		t.Fatalf("expect barrier1 p_100 to be set, but not ")
	}

}
func collectEvents(source <-chan string) []string {
	done := false
	events := make([]string, 0)
	for !done {
		select {
		case event := <-source:
			events = append(events, event)
		default:
			done = true
		}
	}
	return events
}
