/*
Copyright 2025 The Kruise Authors.

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

	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
)

func TestPubMutatingPod(t *testing.T) {
	lTrue := true
	cases := []struct {
		name                string
		getPod              func() *v1.Pod
		getPub              func() *policyv1alpha1.PodUnavailableBudget
		getWorkload         func() *appsv1alpha1.CloneSet
		expectedAnnotations map[string]string
	}{
		{
			name: "pub, selector matched",
			getPod: func() *v1.Pod {
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
				}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				obj := &policyv1alpha1.PodUnavailableBudget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pub-1",
						Namespace: "test",
					},
					Spec: policyv1alpha1.PodUnavailableBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "web",
							},
						},
					},
				}

				return obj
			},
			expectedAnnotations: map[string]string{
				"version":                          "test",
				pubcontrol.PodRelatedPubAnnotation: "pub-1",
			},
		},
		{
			name: "pub, selector no matched",
			getPod: func() *v1.Pod {
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
				}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				obj := &policyv1alpha1.PodUnavailableBudget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pub-1",
						Namespace: "test",
					},
					Spec: policyv1alpha1.PodUnavailableBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "nginx",
							},
						},
					},
				}

				return obj
			},
			expectedAnnotations: map[string]string{
				"version": "test",
			},
		},
		{
			name: "pub, workload matched",
			getPod: func() *v1.Pod {
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
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "CloneSet",
								Name:       "cs02",
								UID:        "002",
								Controller: &lTrue,
							},
						},
					},
				}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				obj := &policyv1alpha1.PodUnavailableBudget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pub-1",
						Namespace: "test",
					},
					Spec: policyv1alpha1.PodUnavailableBudgetSpec{
						TargetReference: &policyv1alpha1.TargetReference{
							APIVersion: "apps.kruise.io/v1alpha1",
							Kind:       "CloneSet",
							Name:       "cs02",
						},
					},
				}

				return obj
			},
			getWorkload: func() *appsv1alpha1.CloneSet {
				var replicas int32 = 1
				obj := &appsv1alpha1.CloneSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "CloneSet",
						APIVersion: "apps.kruise.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs02",
						Namespace: "test",
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "web",
							},
						},
						Replicas: &replicas,
					},
				}
				return obj
			},
			expectedAnnotations: map[string]string{
				"version":                          "test",
				pubcontrol.PodRelatedPubAnnotation: "pub-1",
			},
		},
		{
			name: "pub, workload no matched",
			getPod: func() *v1.Pod {
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
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps.kruise.io/v1alpha1",
								Kind:       "CloneSet",
								Name:       "cs02",
								UID:        "002",
								Controller: &lTrue,
							},
						},
					},
				}
			},
			getPub: func() *policyv1alpha1.PodUnavailableBudget {
				obj := &policyv1alpha1.PodUnavailableBudget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pub-1",
						Namespace: "test",
					},
					Spec: policyv1alpha1.PodUnavailableBudgetSpec{
						TargetReference: &policyv1alpha1.TargetReference{
							APIVersion: "apps.kruise.io/v1alpha1",
							Kind:       "CloneSet",
							Name:       "cs01",
						},
					},
				}

				return obj
			},
			getWorkload: func() *appsv1alpha1.CloneSet {
				var replicas int32 = 1
				obj := &appsv1alpha1.CloneSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cs02",
						Namespace: "test",
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "web",
							},
						},
						Replicas: &replicas,
					},
				}
				return obj
			},
			expectedAnnotations: map[string]string{
				"version": "test",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme.Scheme)
			builder := fake.NewClientBuilder()
			pub := c.getPub()
			builder.WithObjects(pub)
			if c.getWorkload != nil {
				builder.WithObjects(c.getWorkload())
			}
			testClient := builder.Build()
			controllerfinder.Finder = &controllerfinder.ControllerFinder{Client: testClient}
			podHandler := &PodCreateHandler{Decoder: decoder, Client: testClient}
			req := newAdmission(admissionv1.Create, runtime.RawExtension{}, runtime.RawExtension{}, "")
			pod := c.getPod()
			_, err := podHandler.pubMutatingPod(context.Background(), req, pod)
			controllerfinder.Finder = nil
			if err != nil {
				t.Fatalf("failed to mutating pod, err: %v", err)
			}
			if !reflect.DeepEqual(c.expectedAnnotations, pod.Annotations) {
				t.Fatalf("expected: %s, got: %s", util.DumpJSON(c.expectedAnnotations), util.DumpJSON(pod.Annotations))
			}
		})
	}
}
