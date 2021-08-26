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

package validating

import (
	"context"
	"encoding/json"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	testScheme *runtime.Scheme
)

func podMarkerDemo() *appsv1alpha1.PodMarker {
	replicas := intstr.FromString("10%")
	return &appsv1alpha1.PodMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marker",
			Namespace: "default",
		},
		Spec: appsv1alpha1.PodMarkerSpec{
			Strategy: appsv1alpha1.PodMarkerStrategy{
				Replicas:       &replicas,
				ConflictPolicy: appsv1alpha1.PodMarkerConflictOverwrite,
			},
			MatchRequirements: appsv1alpha1.PodMarkerRequirements{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
				},
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"arch": "x86"},
				},
			},
			MatchPreferences: []appsv1alpha1.PodMarkerPreference{
				{
					PodReady: pointer.BoolPtr(true),
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"pod.phase": "running"},
					},
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"region": "usa"},
					},
				},
			},
			MarkItems: appsv1alpha1.PodMarkerMarkItems{
				Labels:      map[string]string{"upgradeStrategy": "inPlace"},
				Annotations: map[string]string{"upgradeStrategyMarkedByPodMarker": "marker"},
			},
		},
	}
}

func init() {
	testScheme = runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(testScheme)
}

func TestValidatePodMarkerCreate(t *testing.T) {
	decoder, _ := admission.NewDecoder(testScheme)
	markerHandler := &PodMarkerCreateUpdateHandler{Decoder: decoder}

	cases := []struct {
		name                 string
		getPodMarker         func() *appsv1alpha1.PodMarker
		isExpectedSuccessful func() bool
	}{
		{
			name: `normal case, in case of matchPodSelector={"a"="b"}, markItems={"c"="d"}`,
			getPodMarker: func() *appsv1alpha1.PodMarker {
				successfulMarker := podMarkerDemo()
				return successfulMarker
			},
			isExpectedSuccessful: func() bool {
				return true
			},
		},
		{
			name: `in case of matchPodSelector={"a"="b"}, markItems={"a"="b"}`,
			getPodMarker: func() *appsv1alpha1.PodMarker {
				successfulMarker := podMarkerDemo()
				successfulMarker.Spec.MatchRequirements.PodSelector.MatchLabels["app"] = "nginx"
				successfulMarker.Spec.MarkItems.Labels["app"] = "nginx"
				return successfulMarker
			},
			isExpectedSuccessful: func() bool {
				return true
			},
		},
		{
			name: `in case of matchPodSelector={"a" exist}, markItems={"a"="b"}`,
			getPodMarker: func() *appsv1alpha1.PodMarker {
				successfulMarker := podMarkerDemo()
				successfulMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "upgradeStrategy",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				}
				return successfulMarker
			},
			isExpectedSuccessful: func() bool {
				return true
			},
		},
		{
			name: "in case of replicas = -1",
			getPodMarker: func() *appsv1alpha1.PodMarker {
				failedPodMarker := podMarkerDemo()
				replicas := intstr.FromInt(-1)
				failedPodMarker.Spec.Strategy.Replicas = &replicas
				return failedPodMarker
			},
			isExpectedSuccessful: func() bool {
				return false
			},
		},
		{
			name: "in case of replicas > 100%",
			getPodMarker: func() *appsv1alpha1.PodMarker {
				failedPodMarker := podMarkerDemo()
				replicas := intstr.FromString("101%")
				failedPodMarker.Spec.Strategy.Replicas = &replicas
				return failedPodMarker
			},
			isExpectedSuccessful: func() bool {
				return false
			},
		},
		{
			name: "in case of wrong replicas",
			getPodMarker: func() *appsv1alpha1.PodMarker {
				failedPodMarker := podMarkerDemo()
				replicas := intstr.FromString("!@$@#%$%")
				failedPodMarker.Spec.Strategy.Replicas = &replicas
				return failedPodMarker
			},
			isExpectedSuccessful: func() bool {
				return false
			},
		},
		{
			name: "in case of empty matchSelector, NodeSelector=nil, PodSelector is empty labelSelector",
			getPodMarker: func() *appsv1alpha1.PodMarker {
				failedPodMarker := podMarkerDemo()
				failedPodMarker.Spec.Strategy.Replicas = nil
				failedPodMarker.Spec.MatchRequirements.NodeSelector = nil
				failedPodMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{}
				return failedPodMarker
			},
			isExpectedSuccessful: func() bool {
				return true
			},
		},
		{
			name: `in case of matchPodSelector={"a"="b"}, markItems={"a"="c"}`,
			getPodMarker: func() *appsv1alpha1.PodMarker {
				failedPodMarker := podMarkerDemo()
				failedPodMarker.Spec.MarkItems.Labels["app"] = "django"
				return failedPodMarker
			},
			isExpectedSuccessful: func() bool {
				return false
			},
		},
		{
			name: `in case of matchPodSelector={"a" not in ["b", "c"]}, markItems={"a"="b"}`,
			getPodMarker: func() *appsv1alpha1.PodMarker {
				failedPodMarker := podMarkerDemo()
				failedPodMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "upgradeStrategy",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"inPlace", "InPlaceIfPossible"},
						},
					},
				}
				return failedPodMarker
			},
			isExpectedSuccessful: func() bool {
				return false
			},
		},
		{
			name: `in case of matchPodSelector={"a" not exist}, markItems={"a"="b"}`,
			getPodMarker: func() *appsv1alpha1.PodMarker {
				failedPodMarker := podMarkerDemo()
				failedPodMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "upgradeStrategy",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				}
				return failedPodMarker
			},
			isExpectedSuccessful: func() bool {
				return false
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			marker := testCase.getPodMarker()
			req := newAdmission(admissionv1.Create, marker, nil, "")
			response := markerHandler.Handle(context.TODO(), req)
			if testCase.isExpectedSuccessful() && len(response.Result.Message) != 0 {
				t.Fatalf("validate failed when podMarker create, unexpected err: %v", response.Result.Message)
			} else if !testCase.isExpectedSuccessful() && len(response.Result.Message) == 0 {
				t.Fatalf("validate failed when podMarker create, expected error does not occur")
			}
		})
	}
}

func TestValidatePodMarkerUpdate(t *testing.T) {
	decoder, _ := admission.NewDecoder(testScheme)
	markerHandler := &PodMarkerCreateUpdateHandler{Decoder: decoder}

	cases := []struct {
		name                 string
		getOldPodMarker      func() *appsv1alpha1.PodMarker
		getNewPodMarker      func() *appsv1alpha1.PodMarker
		isExpectedSuccessful func() bool
	}{
		{
			name: "successful update case",
			getOldPodMarker: func() *appsv1alpha1.PodMarker {
				return podMarkerDemo()
			},
			getNewPodMarker: func() *appsv1alpha1.PodMarker {
				newMarker := podMarkerDemo()
				replicas := intstr.FromInt(7)
				newMarker.Spec.Strategy.Replicas = &replicas
				newMarker.Spec.Strategy.ConflictPolicy = appsv1alpha1.PodMarkerConflictIgnore
				return newMarker
			},
			isExpectedSuccessful: func() bool {
				return true
			},
		},
		{
			name: "failed update case",
			getOldPodMarker: func() *appsv1alpha1.PodMarker {
				return podMarkerDemo()
			},
			getNewPodMarker: func() *appsv1alpha1.PodMarker {
				newMarker := podMarkerDemo()
				newMarker.Spec.MarkItems.Annotations = nil
				return newMarker
			},
			isExpectedSuccessful: func() bool {
				return false
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			oldPodMarker := testCase.getOldPodMarker()
			newPodMarker := testCase.getNewPodMarker()
			req := newAdmission(admissionv1.Update, newPodMarker, oldPodMarker, "")
			response := markerHandler.Handle(context.TODO(), req)
			if testCase.isExpectedSuccessful() && len(response.Result.Message) != 0 {
				t.Fatalf("validate failed when podMarker update, unexpected err: %v", response.Result.Message)
			} else if !testCase.isExpectedSuccessful() && len(response.Result.Message) == 0 {
				t.Fatalf("validate failed when podMarker update, expected error does not occur")
			}
		})
	}
}

func toRaw(marker *appsv1alpha1.PodMarker) runtime.RawExtension {
	if marker == nil {
		return runtime.RawExtension{}
	}
	by, _ := json.Marshal(marker)
	return runtime.RawExtension{Raw: by}
}

func newAdmission(op admissionv1.Operation, marker, oldMarker *appsv1alpha1.PodMarker, subResource string) admission.Request {
	object, oldObject := toRaw(marker), toRaw(oldMarker)
	return admission.Request{
		AdmissionRequest: newAdmissionRequest(op, object, oldObject, subResource),
	}
}

func newAdmissionRequest(op admissionv1.Operation, object, oldObject runtime.RawExtension, subResource string) admissionv1.AdmissionRequest {
	return admissionv1.AdmissionRequest{
		Resource:    metav1.GroupVersionResource{Group: corev1.SchemeGroupVersion.Group, Version: corev1.SchemeGroupVersion.Version, Resource: "pods"},
		Operation:   op,
		Object:      object,
		OldObject:   oldObject,
		SubResource: subResource,
	}
}
