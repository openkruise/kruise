/*
Copyright 2019 The Kruise Authors.

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

package statefulset

import (
	"reflect"
	"testing"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

func TestSortPodsToUpdate(t *testing.T) {
	cases := []struct {
		strategy       *appsv1beta1.RollingUpdateStatefulSetStrategy
		updateRevision string
		totalReplicas  int32
		replicas       []*v1.Pod
		expected       []int
	}{
		{
			strategy:       nil,
			updateRevision: "r1",
			totalReplicas:  3,
			replicas: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
			},
			expected: []int{2, 1, 0},
		},
		{
			strategy: &appsv1beta1.RollingUpdateStatefulSetStrategy{
				UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{PriorityStrategy: &appspub.UpdatePriorityStrategy{
					WeightPriority: []appspub.UpdatePriorityWeightTerm{
						{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"k": "v1"}}},
						{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"k": "v2"}}},
					},
				}},
				Partition: func() *int32 { var i int32 = 6; return &i }(),
			},
			updateRevision: "r1",
			totalReplicas:  10,
			replicas: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
			},
			expected: []int{7, 6, 9, 8},
		},
		{
			strategy:       nil,
			updateRevision: "r1",
			totalReplicas:  3,
			replicas: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
			},
			expected: []int{2, 1, 0},
		},
		{
			// change the case because we define partition as number of pods with non-updated revision
			strategy:       &appsv1beta1.RollingUpdateStatefulSetStrategy{Partition: func() *int32 { var i int32 = 2; return &i }()},
			updateRevision: "r1",
			totalReplicas:  3,
			replicas: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				nil,
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				nil,
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
			},
			expected: []int{4},
		},
		{
			strategy: &appsv1beta1.RollingUpdateStatefulSetStrategy{
				UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{PriorityStrategy: &appspub.UpdatePriorityStrategy{
					WeightPriority: []appspub.UpdatePriorityWeightTerm{
						{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"k": "v1"}}},
						{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"k": "v2"}}},
					},
				}},
				Partition: func() *int32 { var i int32 = 6; return &i }(),
			},
			updateRevision: "r1",
			totalReplicas:  10,
			replicas: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				nil,
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				nil,
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
			},
			expected: []int{8, 7, 11, 9},
		},
		{
			strategy: &appsv1beta1.RollingUpdateStatefulSetStrategy{
				UnorderedUpdate: &appsv1beta1.UnorderedUpdateStrategy{PriorityStrategy: &appspub.UpdatePriorityStrategy{
					WeightPriority: []appspub.UpdatePriorityWeightTerm{
						{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"k": "v1"}}},
						{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"k": "v2"}}},
					},
				}},
				Partition: func() *int32 { var i int32 = 6; return &i }(),
			},
			updateRevision: "r1",
			totalReplicas:  10,
			replicas: []*v1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0", appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingUpdate)}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0", appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingUpdate)}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				nil,
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r1"}}},
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
				nil,
				{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.ControllerRevisionHashLabelKey: "r0"}}},
			},
			expected: []int{8, 7, 1, 0},
		},
	}

	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.PreparingUpdateAsUpdate, true)()

	for i, tc := range cases {
		res := sortPodsToUpdate(tc.strategy, tc.updateRevision, tc.totalReplicas, tc.replicas)
		if !reflect.DeepEqual(res, tc.expected) {
			t.Fatalf("case #%d failed, expected %v, got %v", i, tc.expected, res)
		}
	}
}
