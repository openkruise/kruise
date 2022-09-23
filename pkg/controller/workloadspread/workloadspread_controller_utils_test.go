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

package workloadspread

import (
	"fmt"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	testScheduleRequiredKey  = "apps.kruise.io/schedule-required"
	testSchedulePreferredKey = "apps.kruise.io/schedule-preferred"
	testNodeTestLabelKey     = "apps.kruise.io/is-test"
	testNodeUnitTestLabelKey = "apps.kruise.io/is-unit-test"
	testNodeTolerationKey    = "apps.kruise.io/toleration"
	testSubsetPatchLabelKey  = "apps.kruise.io/patch-label"
	testSubsetPatchAnnoKey   = "apps.kruise.io/patch-annotation"
)

var (
	matchSubsetDemo = appsv1alpha1.WorkloadSpreadSubset{
		Name:        "subset-a",
		MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
		Tolerations: []corev1.Toleration{
			{
				Key:      testNodeTolerationKey,
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
			},
		},
		RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      testScheduleRequiredKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"true"},
				},
			},
		},
		PreferredNodeSelectorTerms: []corev1.PreferredSchedulingTerm{
			{
				Weight: 100,
				Preference: corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      testSchedulePreferredKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"true"},
						},
					},
				},
			},
		},
		Patch: runtime.RawExtension{
			Raw: []byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"true"},"annotations":{"%s":"true"}}}`, testSubsetPatchLabelKey, testSubsetPatchAnnoKey)),
		},
	}

	matchPodDemo = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				testSubsetPatchAnnoKey: "true",
			},
			Labels: map[string]string{
				testSubsetPatchLabelKey: "true",
			},
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      testNodeTestLabelKey,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
									{
										Key:      testNodeUnitTestLabelKey,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	matchNodeDemo = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				testNodeTestLabelKey:     "true",
				testNodeUnitTestLabelKey: "true",
				testScheduleRequiredKey:  "true",
				testSchedulePreferredKey: "true",
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{
					Key:    testNodeTolerationKey,
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
	}
)

func TestMatchSubset(t *testing.T) {
	cases := []struct {
		name      string
		isMatch   bool
		score     int64
		getSubset func() *appsv1alpha1.WorkloadSpreadSubset
		getNode   func() *corev1.Node
		getPod    func() *corev1.Pod
	}{
		{
			name:    "match=true, Score=10010",
			isMatch: true,
			score:   10010,
			getSubset: func() *appsv1alpha1.WorkloadSpreadSubset {
				return matchSubsetDemo.DeepCopy()
			},
			getNode: func() *corev1.Node {
				return matchNodeDemo.DeepCopy()
			},
			getPod: func() *corev1.Pod {
				return matchPodDemo.DeepCopy()
			},
		},
		{
			name:    "match=true, Score=10000",
			isMatch: true,
			score:   10000,
			getSubset: func() *appsv1alpha1.WorkloadSpreadSubset {
				return matchSubsetDemo.DeepCopy()
			},
			getNode: func() *corev1.Node {
				return matchNodeDemo.DeepCopy()
			},
			getPod: func() *corev1.Pod {
				pod := matchPodDemo.DeepCopy()
				pod.Labels[testSubsetPatchLabelKey] = "false"
				pod.Annotations[testSubsetPatchAnnoKey] = "false"
				return pod
			},
		},
		{
			name:    "match=true, Score=10, preferred key not match",
			isMatch: true,
			score:   10,
			getSubset: func() *appsv1alpha1.WorkloadSpreadSubset {
				return matchSubsetDemo.DeepCopy()
			},
			getNode: func() *corev1.Node {
				node := matchNodeDemo.DeepCopy()
				node.Labels[testSchedulePreferredKey] = "false"
				return node
			},
			getPod: func() *corev1.Pod {
				return matchPodDemo.DeepCopy()
			},
		},
		{
			name:    "match=false, Score=-1, required key not match",
			isMatch: false,
			score:   -1,
			getSubset: func() *appsv1alpha1.WorkloadSpreadSubset {
				return matchSubsetDemo.DeepCopy()
			},
			getNode: func() *corev1.Node {
				node := matchNodeDemo.DeepCopy()
				node.Labels[testScheduleRequiredKey] = "false"
				return node
			},
			getPod: func() *corev1.Pod {
				return matchPodDemo.DeepCopy()
			},
		},
		{
			name:    "match=false, Score=-1, toleration key not match",
			isMatch: false,
			score:   -1,
			getSubset: func() *appsv1alpha1.WorkloadSpreadSubset {
				subset := matchSubsetDemo.DeepCopy()
				subset.Tolerations[0].Value = "false"
				return subsetDemo.DeepCopy()
			},
			getNode: func() *corev1.Node {
				return matchNodeDemo.DeepCopy()
			},
			getPod: func() *corev1.Pod {
				return matchPodDemo.DeepCopy()
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			subset := cs.getSubset()
			node := cs.getNode()
			pod := cs.getPod()
			isMatch, score, err := matchesSubset(pod, node, subset, 0)
			if err != nil {
				t.Fatal("unexpected err occurred")
			}
			if isMatch != cs.isMatch || score != cs.score {
				t.Fatalf("expect: %v %v, but got %v %v ", cs.isMatch, cs.score, isMatch, score)
			}
		})
	}
}
