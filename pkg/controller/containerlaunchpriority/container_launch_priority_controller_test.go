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
	"fmt"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	utilcontainerlaunchpriority "github.com/openkruise/kruise/pkg/util/containerlaunchpriority"
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
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "pod0-barrier"},
	}
	barrier1 := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "pod1-barrier"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithRuntimeObjects(pod0, pod1, barrier0, barrier1).Build()
	reconciler := &ReconcileContainerLaunchPriority{Client: fakeClient}

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
	if v, ok := newBarrier1.Data["p_1000"]; !ok {
		t.Fatalf("expect barrier1 env set, but not")
	} else if v != "true" {
		t.Fatalf("expect barrier1 p_1000 to be true, but get %s", v)
	}
}

func TestFindNextPriorities(t *testing.T) {
	podName := "fake"
	cases := []struct {
		pod      *v1.Pod
		expected []int
	}{
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(4, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: false},
					{Name: "b", Ready: true},
				}},
			},
			expected: []int{-1, 0, 3, 4},
		},
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: false},
					{Name: "b", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			expected: []int{-1, 0, 3},
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			got := findNextPriorities(tc.pod)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestHandle(t *testing.T) {
	namespace := "default"
	podName := "fake-pod"
	configMapName := "fake-pod-barrier"
	cases := []struct {
		pod                *v1.Pod
		existedPriorities  []int
		expectedPriorities []int
		expectedCompleted  bool
	}{
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{}},
			},
			existedPriorities:  []int{},
			expectedPriorities: []int{5},
			expectedCompleted:  false,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: false},
					{Name: "b", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			existedPriorities:  []int{5, 3},
			expectedPriorities: []int{5, 3},
			expectedCompleted:  false,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: true},
					{Name: "b", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			existedPriorities:  []int{5, 3},
			expectedPriorities: []int{5, 3, 0},
			expectedCompleted:  false,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: true},
					{Name: "b", Ready: true},
					{Name: "c", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			existedPriorities:  []int{5, 3, 0},
			expectedPriorities: []int{5, 3, 0, -1},
			expectedCompleted:  true,
		},
	}

	var err error
	for i, tc := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			barrier := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: configMapName},
				Data:       map[string]string{},
			}
			for _, priority := range tc.existedPriorities {
				barrier.Data[utilcontainerlaunchpriority.GetKey(priority)] = "true"
			}
			cli := fake.NewClientBuilder().WithObjects(tc.pod, barrier).Build()
			r := &ReconcileContainerLaunchPriority{Client: cli}

			// reconcile or handle multiple times
			for i := 0; i < 2; i++ {
				if err = r.handle(tc.pod, barrier); err != nil {
					t.Fatal(err)
				}

				if err = cli.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: configMapName}, barrier); err != nil {
					t.Fatal(err)
				}
				if len(barrier.Data) != len(tc.expectedPriorities) {
					t.Fatalf("expected %v, got %v", tc.expectedPriorities, barrier.Data)
				}
				for _, priority := range tc.expectedPriorities {
					if barrier.Data[utilcontainerlaunchpriority.GetKey(priority)] != "true" {
						t.Fatalf("expected %v, got %v", tc.expectedPriorities, barrier.Data)
					}
				}

				gotPod := &v1.Pod{}
				if err = cli.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: podName}, gotPod); err != nil {
					t.Fatal(err)
				}
				gotCompleted := gotPod.Annotations[appspub.ContainerLaunchPriorityCompletedKey] == "true"
				if gotCompleted != tc.expectedCompleted {
					t.Fatalf("expected completed %v, got %v", tc.expectedCompleted, gotCompleted)
				}
			}
		})
	}
}

func TestShouldEnqueue(t *testing.T) {
	namespace := "default"
	podName := "fake-pod"
	configMapName := "fake-pod-barrier"
	cases := []struct {
		pod               *v1.Pod
		existedPriorities []int
		expectedEnqueue   bool
	}{
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{}},
			},
			existedPriorities: []int{},
			expectedEnqueue:   true,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{}},
			},
			existedPriorities: []int{5},
			expectedEnqueue:   false,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: false},
					{Name: "b", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			existedPriorities: []int{5, 3},
			expectedEnqueue:   false,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: true},
					{Name: "b", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			existedPriorities: []int{5, 3},
			expectedEnqueue:   true,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: true},
					{Name: "b", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			existedPriorities: []int{5, 3, 0},
			expectedEnqueue:   false,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: true},
					{Name: "b", Ready: true},
					{Name: "c", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			existedPriorities: []int{5, 3, 0},
			expectedEnqueue:   true,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: true},
					{Name: "b", Ready: true},
					{Name: "c", Ready: true},
					{Name: "f", Ready: true},
				}},
			},
			existedPriorities: []int{5, 3, 0, -1},
			expectedEnqueue:   false,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName, Annotations: map[string]string{appspub.ContainerLaunchPriorityCompletedKey: "true"}},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "a", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
					{Name: "b", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(5, podName)}},
					{Name: "c", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, podName)}},
					{Name: "d"},
					{Name: "e", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, podName)}},
					{Name: "f", Env: []v1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(3, podName)}},
				}},
				Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{
					{Name: "a", Ready: true},
					{Name: "b", Ready: true},
					{Name: "c", Ready: true},
					{Name: "3", Ready: true},
					{Name: "f", Ready: false},
				}},
			},
			existedPriorities: []int{5, 3, 0, -1},
			expectedEnqueue:   false,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			barrier := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: configMapName},
				Data:       map[string]string{},
			}
			for _, priority := range tc.existedPriorities {
				barrier.Data[utilcontainerlaunchpriority.GetKey(priority)] = "true"
			}
			cli := fake.NewClientBuilder().WithObjects(tc.pod, barrier).Build()

			gotEnqueue := shouldEnqueue(tc.pod, cli)
			if gotEnqueue != tc.expectedEnqueue {
				t.Fatalf("expected enqueue %v, got %v", tc.expectedEnqueue, gotEnqueue)
			}
		})
	}
}
