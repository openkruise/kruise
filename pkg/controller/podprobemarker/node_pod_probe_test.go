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

package podprobemarker

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncNodePodProbe(t *testing.T) {
	cases := []struct {
		name               string
		req                ctrl.Request
		getPods            func() []*corev1.Pod
		getPodProbeMarkers func() []*appsv1alpha1.PodProbeMarker
		getNodePodProbes   func() []*appsv1alpha1.NodePodProbe
		getNode            func() []*corev1.Node
		expectPods         func() []*corev1.Pod
	}{
		{
			name: "test1, probe success",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: fmt.Sprintf("%s#%s", ReconNodePodProbe, demoNodePodProbe.Name),
				},
			},
			getNode: func() []*corev1.Node {
				nodes := []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
					},
				}
				return nodes
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							Labels: map[string]string{
								"app": "test",
							},
							UID: types.UID("pod-1-uid"),
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
							Labels: map[string]string{
								"app": "test",
							},
							UID: types.UID("pod-2-uid"),
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				ppms := []*appsv1alpha1.PodProbeMarker{
					demoPodProbeMarker.DeepCopy(),
				}
				return ppms
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				podProbe2 := appsv1alpha1.PodProbe{
					Name: "pod-2",
					Uid:  "pod-2-uid",
					Probes: []appsv1alpha1.ContainerProbe{
						{
							Name:          "healthy",
							ContainerName: "main",
							Probe: appsv1alpha1.ContainerProbeSpec{
								Handler: corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/healthy.sh"},
									},
								},
							},
							PodProbeMarkerName: "ppm-1",
						},
					},
				}
				demo.Spec.PodProbes = append(demo.Spec.PodProbes, podProbe2)
				demo.Status = appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-1",
							Uid:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "healthy",
									State: appsv1alpha1.ProbeTrue,
								},
							},
						},
						{
							Name: "pod-2",
							Uid:  "pod-2-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "healthy",
									State: appsv1alpha1.ProbeFalse,
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
			expectPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							Labels: map[string]string{
								"app":            "test",
								"server-healthy": "true",
							},
							UID: types.UID("pod-1-uid"),
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "10",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodConditionType("healthy"),
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
							Labels: map[string]string{
								"app":            "test",
								"server-healthy": "false",
							},
							UID: types.UID("pod-2-uid"),
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "-10",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodConditionType("healthy"),
									Status: corev1.ConditionFalse,
								},
							},
						},
					},
				}
				return pods
			},
		},
		{
			name: "test2, probe failed",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: fmt.Sprintf("%s#%s", ReconNodePodProbe, demoNodePodProbe.Name),
				},
			},
			getNode: func() []*corev1.Node {
				nodes := []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
					},
				}
				return nodes
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app":            "test",
								"server-healthy": "true",
							},
							Annotations: map[string]string{
								"controller.kubernetes.io/pod-deletion-cost": "10",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodConditionType("healthy"),
									Status: corev1.ConditionTrue,
								},
								{
									Type:   corev1.PodConditionType("other"),
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				demo := demoPodProbeMarker.DeepCopy()
				demo.Spec.Probes[0].MarkerPolicy = demo.Spec.Probes[0].MarkerPolicy[:1]
				return []*appsv1alpha1.PodProbeMarker{demo}
			},
			getNodePodProbes: func() []*appsv1alpha1.NodePodProbe {
				demo := demoNodePodProbe.DeepCopy()
				demo.Status = appsv1alpha1.NodePodProbeStatus{
					PodProbeStatuses: []appsv1alpha1.PodProbeStatus{
						{
							Name: "pod-1",
							Uid:  "pod-1-uid",
							ProbeStates: []appsv1alpha1.ContainerProbeState{
								{
									Name:  "healthy",
									State: appsv1alpha1.ProbeFalse,
								},
							},
						},
					},
				}
				return []*appsv1alpha1.NodePodProbe{demo}
			},
			expectPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid"),
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{
								{
									Type:   corev1.PodConditionType("healthy"),
									Status: corev1.ConditionFalse,
								},
								{
									Type:   corev1.PodConditionType("other"),
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				}
				return pods
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			for _, obj := range cs.getPods() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create Pod failed: %s", err.Error())
				}
			}
			for _, obj := range cs.getPodProbeMarkers() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create PodProbeMarker failed: %s", err.Error())
				}
			}
			for _, obj := range cs.getNodePodProbes() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create NodePodProbes failed: %s", err.Error())
				}
			}
			for _, obj := range cs.getNode() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create Node failed: %s", err.Error())
				}
			}
			controllerfinder.Finder = &controllerfinder.ControllerFinder{Client: fakeClient}
			recon := ReconcilePodProbeMarker{Client: fakeClient}
			_, err := recon.Reconcile(context.TODO(), cs.req)
			if err != nil {
				t.Fatalf("Reconcile failed: %s", err.Error())
			}
			if !checkPodMarkerEqual(fakeClient, t, cs.expectPods()) {
				t.Fatalf("Reconcile failed")
			}
		})
	}
}

func checkPodMarkerEqual(c client.WithWatch, t *testing.T, expect []*corev1.Pod) bool {
	for i := range expect {
		obj := expect[i]
		pod := &corev1.Pod{}
		err := c.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, pod)
		if err != nil {
			t.Fatalf("get NodePodProbe failed: %s", err.Error())
			return false
		}
		if !reflect.DeepEqual(obj.Labels, pod.Labels) || !reflect.DeepEqual(obj.Annotations, pod.Annotations) ||
			!reflect.DeepEqual(obj.Status.Conditions, pod.Status.Conditions) {
			return false
		}
	}
	return true
}

func TestSyncPodFromNodePodProbe(t *testing.T) {
	cases := []struct {
		name               string
		req                ctrl.Request
		getPods            func() []*corev1.Pod
		getNodePodProbe    func() *appsv1alpha1.NodePodProbe
		expectNodePodProbe func() *appsv1alpha1.NodePodProbe
		getNode            func() []*corev1.Node
	}{
		{
			name: "test1, pod no changed",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: fmt.Sprintf("%s#%s", ReconNodePodProbe, "node-1"),
				},
			},
			getNode: func() []*corev1.Node {
				nodes := []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
					},
				}
				return nodes
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
							Labels: map[string]string{
								"app": "test",
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
					},
				}
				return pods
			},
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := &appsv1alpha1.NodePodProbe{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: appsv1alpha1.NodePodProbeSpec{
						PodProbes: []appsv1alpha1.PodProbe{
							{
								Name: "pod-1",
							},
							{
								Name: "pod-2",
							},
						},
					},
				}
				return demo
			},
			expectNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := &appsv1alpha1.NodePodProbe{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: appsv1alpha1.NodePodProbeSpec{
						PodProbes: []appsv1alpha1.PodProbe{
							{
								Name: "pod-1",
							},
							{
								Name: "pod-2",
							},
						},
					},
				}
				return demo
			},
		},
		{
			name: "test2, pod not found",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: fmt.Sprintf("%s#%s", ReconNodePodProbe, "node-1"),
				},
			},
			getNode: func() []*corev1.Node {
				nodes := []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
					},
				}
				return nodes
			},
			getPods: func() []*corev1.Pod {
				now := metav1.Now()
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							Labels: map[string]string{
								"app": "test",
							},
							DeletionTimestamp: &now,
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
					},
				}
				return pods
			},
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := &appsv1alpha1.NodePodProbe{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: appsv1alpha1.NodePodProbeSpec{
						PodProbes: []appsv1alpha1.PodProbe{
							{
								Name: "pod-1",
							},
							{
								Name: "pod-2",
							},
						},
					},
				}
				return demo
			},
			expectNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := &appsv1alpha1.NodePodProbe{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: appsv1alpha1.NodePodProbeSpec{},
				}
				return demo
			},
		},
		{
			name: "test3, pod uid changed",
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: fmt.Sprintf("%s#%s", ReconNodePodProbe, "node-1"),
				},
			},
			getNode: func() []*corev1.Node {
				nodes := []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
					},
				}
				return nodes
			},
			getPods: func() []*corev1.Pod {
				pods := []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
							UID:  types.UID("pod-1-uid-02"),
							Labels: map[string]string{
								"app": "test",
							},
						},
					},
				}
				return pods
			},
			getNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := &appsv1alpha1.NodePodProbe{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: appsv1alpha1.NodePodProbeSpec{
						PodProbes: []appsv1alpha1.PodProbe{
							{
								Name: "pod-1",
								Uid:  "pod-1-uid-01",
							},
						},
					},
				}
				return demo
			},
			expectNodePodProbe: func() *appsv1alpha1.NodePodProbe {
				demo := &appsv1alpha1.NodePodProbe{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: appsv1alpha1.NodePodProbeSpec{},
				}
				return demo
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			for _, obj := range cs.getPods() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create Pod failed: %s", err.Error())
				}
			}
			for _, obj := range cs.getNode() {
				err := fakeClient.Create(context.TODO(), obj.DeepCopy())
				if err != nil {
					t.Fatalf("create Node failed: %s", err.Error())
				}
			}
			err := fakeClient.Create(context.TODO(), cs.getNodePodProbe())
			if err != nil {
				t.Fatalf("create NodePodProbes failed: %s", err.Error())
			}

			controllerfinder.Finder = &controllerfinder.ControllerFinder{Client: fakeClient}
			recon := ReconcilePodProbeMarker{Client: fakeClient}
			_, err = recon.Reconcile(context.TODO(), cs.req)
			if err != nil {
				t.Fatalf("Reconcile failed: %s", err.Error())
			}
			if !checkNodePodProbeEqual(fakeClient, t, []*appsv1alpha1.NodePodProbe{cs.expectNodePodProbe()}) {
				t.Fatalf("Reconcile failed")
			}
		})
	}
}
