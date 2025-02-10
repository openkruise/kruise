/*
Copyright 2024 The Kruise Authors.

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

package mutating

import (
	"context"
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestPodProbeMarkerMutatingPod(t *testing.T) {
	cases := []struct {
		name               string
		getPod             func() *v1.Pod
		getPodProbeMarkers func() []*appsv1alpha1.PodProbeMarker
		expected           map[string]string
	}{
		{
			name: "podprobemarker, selector matched, but no conditionType",
			getPod: func() *v1.Pod {
				al := v1.ContainerRestartPolicyAlways
				return &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"version": "test",
						},
						Labels: map[string]string{
							"app": "web",
						},
						Namespace: "test",
						Name:      "pod-1",
					},
					Spec: v1.PodSpec{
						InitContainers: []v1.Container{
							{
								Name: "init-1",
							},
							{
								Name:          "init-2",
								RestartPolicy: &al,
							},
						},
						Containers: []v1.Container{
							{
								Name: "main",
							},
							{
								Name: "envoy",
							},
						},
					},
				}
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				obj1 := &appsv1alpha1.PodProbeMarker{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "healthy",
						Namespace: "test",
					},
					Spec: appsv1alpha1.PodProbeMarkerSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "web",
							},
						},
						Probes: []appsv1alpha1.PodContainerProbe{
							{
								ContainerName: "main",
								Name:          "healthy",
							},
							{
								ContainerName:    "invalid",
								Name:             "healthy",
								PodConditionType: "game.kruise.io/healthy",
							},
						},
					},
				}

				return []*appsv1alpha1.PodProbeMarker{obj1}
			},
			expected: map[string]string{
				"version": "test",
			},
		},
		{
			name: "podprobemarker, selector matched",
			getPod: func() *v1.Pod {
				al := v1.ContainerRestartPolicyAlways
				return &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"version": "test",
						},
						Labels: map[string]string{
							"app": "web",
						},
						Namespace: "test",
						Name:      "pod-1",
					},
					Spec: v1.PodSpec{
						InitContainers: []v1.Container{
							{
								Name: "init-1",
							},
							{
								Name:          "init-2",
								RestartPolicy: &al,
							},
						},
						Containers: []v1.Container{
							{
								Name: "main",
							},
							{
								Name: "envoy",
							},
						},
					},
				}
			},
			getPodProbeMarkers: func() []*appsv1alpha1.PodProbeMarker {
				obj1 := &appsv1alpha1.PodProbeMarker{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "healthy",
						Namespace: "test",
					},
					Spec: appsv1alpha1.PodProbeMarkerSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "web",
							},
						},
						Probes: []appsv1alpha1.PodContainerProbe{
							{
								ContainerName:    "main",
								Name:             "healthy",
								PodConditionType: "game.kruise.io/healthy",
							},
							{
								ContainerName:    "envoy",
								Name:             "healthy",
								PodConditionType: "game.kruise.io/healthy",
							},
						},
					},
				}
				obj2 := &appsv1alpha1.PodProbeMarker{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "init",
						Namespace: "test",
					},
					Spec: appsv1alpha1.PodProbeMarkerSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "web",
							},
						},
						Probes: []appsv1alpha1.PodContainerProbe{
							{
								ContainerName:    "init-1",
								Name:             "init",
								PodConditionType: "game.kruise.io/init",
							},
							{
								ContainerName:    "init-2",
								Name:             "init",
								PodConditionType: "game.kruise.io/init",
							},
						},
					},
				}
				return []*appsv1alpha1.PodProbeMarker{obj1, obj2}
			},
			expected: map[string]string{
				"version":                                    "test",
				appsv1alpha1.PodProbeMarkerAnnotationKey:     `[{"name":"healthy","containerName":"main","probe":{},"podConditionType":"game.kruise.io/healthy"},{"name":"init","containerName":"init-2","probe":{},"podConditionType":"game.kruise.io/init"}]`,
				appsv1alpha1.PodProbeMarkerListAnnotationKey: "healthy,init",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme.Scheme)
			builder := fake.NewClientBuilder()
			for i := range c.getPodProbeMarkers() {
				obj := c.getPodProbeMarkers()[i]
				builder.WithObjects(obj)
			}
			testClient := builder.Build()
			podHandler := &PodCreateHandler{Decoder: decoder, Client: testClient}
			req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
			pod := c.getPod()
			if _, err := podHandler.podProbeMarkerMutatingPod(context.Background(), req, pod); err != nil {
				t.Fatalf("failed to mutating pod, err: %v", err)
			}
			if !reflect.DeepEqual(c.expected, pod.Annotations) {
				t.Fatalf("expected: %s, got: %s", util.DumpJSON(c.expected), util.DumpJSON(pod.Annotations))
			}
		})
	}
}
