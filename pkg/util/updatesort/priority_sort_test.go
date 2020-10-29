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

package updatesort

import (
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		strategy *appspub.UpdatePriorityStrategy
		podI     map[string]string
		podJ     map[string]string
		expected bool
	}{
		{
			strategy: &appspub.UpdatePriorityStrategy{
				WeightPriority: []appspub.UpdatePriorityWeightTerm{
					{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "foo"}}},
					{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "bar"}}},
				},
			},
			podI:     map[string]string{},
			podJ:     map[string]string{},
			expected: true,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				WeightPriority: []appspub.UpdatePriorityWeightTerm{
					{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "foo"}}},
					{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "bar"}}},
				},
			},
			podI:     map[string]string{"key": "foo"},
			podJ:     map[string]string{"key": "foo"},
			expected: true,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				WeightPriority: []appspub.UpdatePriorityWeightTerm{
					{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "foo"}}},
					{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "bar"}}},
				},
			},
			podI:     map[string]string{"key": "bar"},
			podJ:     map[string]string{"key": "foo"},
			expected: false,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				WeightPriority: []appspub.UpdatePriorityWeightTerm{
					{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "foo"}}},
					{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "bar"}}},
				},
			},
			podI:     map[string]string{},
			podJ:     map[string]string{"key": "bar"},
			expected: false,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				WeightPriority: []appspub.UpdatePriorityWeightTerm{
					{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "foo"}}},
					{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "bar"}}},
				},
			},
			podI:     map[string]string{},
			podJ:     map[string]string{"key": "foo"},
			expected: false,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				WeightPriority: []appspub.UpdatePriorityWeightTerm{
					{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key1": "foo"}}},
					{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key2": "bar"}}},
					{Weight: 15, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key3": "baz"}}},
				},
			},
			podI:     map[string]string{"key1": "foo"},
			podJ:     map[string]string{"key2": "bar", "key3": "baz"},
			expected: false,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				WeightPriority: []appspub.UpdatePriorityWeightTerm{
					{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key1": "foo"}}},
					{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key2": "bar"}}},
					{Weight: 5, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key3": "baz"}}},
				},
			},
			podI:     map[string]string{"key2": "bar", "key3": "baz"},
			podJ:     map[string]string{"key1": "foo"},
			expected: false,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				OrderPriority: []appspub.UpdatePriorityOrderTerm{
					{OrderedKey: "key1"},
					{OrderedKey: "key2"},
				},
			},
			podI:     map[string]string{},
			podJ:     map[string]string{},
			expected: true,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				OrderPriority: []appspub.UpdatePriorityOrderTerm{
					{OrderedKey: "key1"},
					{OrderedKey: "key2"},
				},
			},
			podI:     map[string]string{"key1": "o-10"},
			podJ:     map[string]string{"key1": "o-5"},
			expected: true,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				OrderPriority: []appspub.UpdatePriorityOrderTerm{
					{OrderedKey: "key1"},
					{OrderedKey: "key2"},
				},
			},
			podI:     map[string]string{"key1": "o-10"},
			podJ:     map[string]string{"key1": "o-10"},
			expected: true,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				OrderPriority: []appspub.UpdatePriorityOrderTerm{
					{OrderedKey: "key1"},
					{OrderedKey: "key2"},
				},
			},
			podI:     map[string]string{"key1": "o-10"},
			podJ:     map[string]string{"key1": "o-20"},
			expected: false,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				OrderPriority: []appspub.UpdatePriorityOrderTerm{
					{OrderedKey: "key1"},
					{OrderedKey: "key2"},
				},
			},
			podI:     map[string]string{"key2": "o-20"},
			podJ:     map[string]string{"key1": "o-10"},
			expected: false,
		},
		{
			strategy: &appspub.UpdatePriorityStrategy{
				OrderPriority: []appspub.UpdatePriorityOrderTerm{
					{OrderedKey: "key1"},
					{OrderedKey: "key2"},
				},
			},
			podI:     map[string]string{"key2": "o-20"},
			podJ:     map[string]string{"key2": "o-10"},
			expected: true,
		},
	}

	for i, tc := range cases {
		ps := prioritySort{strategy: tc.strategy}
		got := ps.compare(tc.podI, tc.podJ, true)
		if got != tc.expected {
			t.Fatalf("case #%d expected %v, got %v", i, tc.expected, got)
		}
	}
}
