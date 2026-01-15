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

package v1alpha1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openkruise/kruise/apis/policy/v1beta1"
)

func TestPodUnavailableBudget_ConvertTo(t *testing.T) {
	tests := []struct {
		name     string
		pub      *PodUnavailableBudget
		expected *v1beta1.PodUnavailableBudget
	}{
		{
			name: "convert with all fields populated",
			pub: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					TargetReference: &TargetReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "test-deployment",
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					MinAvailable:   &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
				},
				Status: PodUnavailableBudgetStatus{
					ObservedGeneration: 1,
					DisruptedPods: map[string]metav1.Time{
						"pod-1": {Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
					UnavailablePods: map[string]metav1.Time{
						"pod-2": {Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
					UnavailableAllowed: 1,
					CurrentAvailable:   4,
					DesiredAvailable:   3,
					TotalReplicas:      5,
				},
			},
			expected: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					TargetReference: &v1beta1.TargetReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "test-deployment",
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					MinAvailable:   &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{
					ObservedGeneration: 1,
					DisruptedPods: map[string]metav1.Time{
						"pod-1": {Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
					UnavailablePods: map[string]metav1.Time{
						"pod-2": {Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
					UnavailableAllowed: 1,
					CurrentAvailable:   4,
					DesiredAvailable:   3,
					TotalReplicas:      5,
				},
			},
		},
		{
			name: "convert with annotations to fields - protectOperations",
			pub: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-operations",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectOperationAnnotation: "DELETE,UPDATE,EVICT",
					},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
				Status: PodUnavailableBudgetStatus{},
			},
			expected: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-operations",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectOperationAnnotation: "DELETE,UPDATE,EVICT",
					},
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable:    &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					ProtectOperations: []v1beta1.PubOperation{v1beta1.PubDeleteOperation, v1beta1.PubUpdateOperation, v1beta1.PubEvictOperation},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert with annotations to fields - protectTotalReplicas",
			pub: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-replicas",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectTotalReplicasAnnotation: "10",
					},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
				Status: PodUnavailableBudgetStatus{},
			},
			expected: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-replicas",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectTotalReplicasAnnotation: "10",
					},
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					ProtectTotalReplicas: int32Ptr(10),
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert with both annotations",
			pub: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-both",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectOperationAnnotation:     "EVICT, RESIZE",
						PubProtectTotalReplicasAnnotation: "15",
						"other-annotation":                "keep-this",
					},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MinAvailable: &intstr.IntOrString{Type: intstr.String, StrVal: "80%"},
				},
				Status: PodUnavailableBudgetStatus{},
			},
			expected: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-both",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectOperationAnnotation:     "EVICT, RESIZE",
						PubProtectTotalReplicasAnnotation: "15",
						"other-annotation":                "keep-this",
					},
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MinAvailable:         &intstr.IntOrString{Type: intstr.String, StrVal: "80%"},
					ProtectOperations:    []v1beta1.PubOperation{v1beta1.PubEvictOperation, v1beta1.PubResizeOperation},
					ProtectTotalReplicas: int32Ptr(15),
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert with minimal fields",
			pub: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-pub",
					Namespace: "default",
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: PodUnavailableBudgetStatus{},
			},
			expected: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-pub",
					Namespace: "default",
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert with targetRef",
			pub: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-targetref",
					Namespace: "default",
				},
				Spec: PodUnavailableBudgetSpec{
					TargetReference: &TargetReference{
						APIVersion: "apps.kruise.io/v1alpha1",
						Kind:       "CloneSet",
						Name:       "test-cloneset",
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				},
				Status: PodUnavailableBudgetStatus{},
			},
			expected: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-targetref",
					Namespace: "default",
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					TargetReference: &v1beta1.TargetReference{
						APIVersion: "apps.kruise.io/v1alpha1",
						Kind:       "CloneSet",
						Name:       "test-cloneset",
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &v1beta1.PodUnavailableBudget{}
			err := tt.pub.ConvertTo(dst)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, dst)
		})
	}
}

func TestPodUnavailableBudget_ConvertFrom(t *testing.T) {
	tests := []struct {
		name     string
		src      *v1beta1.PodUnavailableBudget
		expected *PodUnavailableBudget
	}{
		{
			name: "convert from v1beta1 with all fields populated",
			src: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					TargetReference: &v1beta1.TargetReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "test-deployment",
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					MinAvailable:   &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{
					ObservedGeneration: 1,
					DisruptedPods: map[string]metav1.Time{
						"pod-1": {Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
					UnavailablePods: map[string]metav1.Time{
						"pod-2": {Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
					UnavailableAllowed: 1,
					CurrentAvailable:   4,
					DesiredAvailable:   3,
					TotalReplicas:      5,
				},
			},
			expected: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub",
					Namespace: "default",
					Labels:    map[string]string{"app": "test"},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					TargetReference: &TargetReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "test-deployment",
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					MinAvailable:   &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
				},
				Status: PodUnavailableBudgetStatus{
					ObservedGeneration: 1,
					DisruptedPods: map[string]metav1.Time{
						"pod-1": {Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
					UnavailablePods: map[string]metav1.Time{
						"pod-2": {Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
					},
					UnavailableAllowed: 1,
					CurrentAvailable:   4,
					DesiredAvailable:   3,
					TotalReplicas:      5,
				},
			},
		},
		{
			name: "convert from v1beta1 with protectOperations field to annotation",
			src: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-operations",
					Namespace: "default",
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable:    &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					ProtectOperations: []v1beta1.PubOperation{v1beta1.PubDeleteOperation, v1beta1.PubUpdateOperation},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
			expected: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-operations",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectOperationAnnotation: "DELETE,UPDATE",
					},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
				Status: PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with protectTotalReplicas field to annotation",
			src: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-replicas",
					Namespace: "default",
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					ProtectTotalReplicas: int32Ptr(20),
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
			expected: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-replicas",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectTotalReplicasAnnotation: "20",
					},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
				Status: PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with both fields to annotations",
			src: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-both",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation": "keep-this",
					},
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MinAvailable:         &intstr.IntOrString{Type: intstr.String, StrVal: "70%"},
					ProtectOperations:    []v1beta1.PubOperation{v1beta1.PubEvictOperation, v1beta1.PubResizeOperation},
					ProtectTotalReplicas: int32Ptr(25),
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
			expected: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-both",
					Namespace: "default",
					Annotations: map[string]string{
						PubProtectOperationAnnotation:     "EVICT,RESIZE",
						PubProtectTotalReplicasAnnotation: "25",
						"other-annotation":                "keep-this",
					},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MinAvailable: &intstr.IntOrString{Type: intstr.String, StrVal: "70%"},
				},
				Status: PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with minimal fields",
			src: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-pub",
					Namespace: "default",
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
			expected: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-pub",
					Namespace: "default",
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
				Status: PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert from v1beta1 with targetRef",
			src: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-targetref",
					Namespace: "default",
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					TargetReference: &v1beta1.TargetReference{
						APIVersion: "apps.kruise.io/v1beta1",
						Kind:       "StatefulSet",
						Name:       "test-sts",
					},
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
			expected: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-targetref",
					Namespace: "default",
				},
				Spec: PodUnavailableBudgetSpec{
					TargetReference: &TargetReference{
						APIVersion: "apps.kruise.io/v1beta1",
						Kind:       "StatefulSet",
						Name:       "test-sts",
					},
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				},
				Status: PodUnavailableBudgetStatus{},
			},
		},
		{
			name: "convert from v1beta1 without protectOperations and protectTotalReplicas",
			src: &v1beta1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-nil-fields",
					Namespace: "default",
					Annotations: map[string]string{
						"existing-annotation": "value",
					},
				},
				Spec: v1beta1.PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
				Status: v1beta1.PodUnavailableBudgetStatus{},
			},
			expected: &PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pub-nil-fields",
					Namespace: "default",
					Annotations: map[string]string{
						"existing-annotation": "value",
					},
				},
				Spec: PodUnavailableBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
				Status: PodUnavailableBudgetStatus{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &PodUnavailableBudget{}
			err := pub.ConvertFrom(tt.src)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, pub)
		})
	}
}

// Helper function for creating int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}
