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
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
)

func TestPrioritySortPairs(t *testing.T) {
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
		pods := []*v1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Labels: tc.podI}},
			{ObjectMeta: metav1.ObjectMeta{Labels: tc.podJ}},
		}
		got := ps.Sort(pods, []int{0, 1})[0] == 0
		if got != tc.expected {
			t.Fatalf("case #%d expected %v, got %v", i, tc.expected, got)
		}
	}
}

func Test_prioritySort_Sort(t *testing.T) {
	type fields struct {
		strategy *appspub.UpdatePriorityStrategy
	}
	type args struct {
		pods    []*v1.Pod
		indexes []int
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []int
	}{
		{
			name: "empty priority strategy",
			fields: fields{
				strategy: &appspub.UpdatePriorityStrategy{},
			},
			args: args{
				pods: []*v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-0",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
						},
					},
				},
				indexes: []int{0, 1, 2},
			},
			want: []int{0, 1, 2},
		},
		{
			name: "priority strategy, keep original order if not match any priority rule",
			fields: fields{
				strategy: &appspub.UpdatePriorityStrategy{
					WeightPriority: []appspub.UpdatePriorityWeightTerm{
						{
							Weight: 100,
							MatchSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"foo": "bar",
								},
							},
						},
						{
							Weight: 90,
							MatchSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"hello": "world",
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []*v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-0",
							Labels: map[string]string{
								"hello": "world",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3",
						},
					},
				},
				indexes: []int{0, 1, 2, 3},
			},
			want: []int{2, 0, 1, 3},
		},
		{
			name: "priority strategy, no Pods matched any priority rule, keep original order",
			fields: fields{
				strategy: &appspub.UpdatePriorityStrategy{
					WeightPriority: []appspub.UpdatePriorityWeightTerm{
						{
							Weight: 100,
							MatchSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"not": "match",
								},
							},
						},
						{
							Weight: 90,
							MatchSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"not1": "match1",
								},
							},
						},
					},
				},
			},
			args: args{
				pods: []*v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-0",
							Labels: map[string]string{
								"hello": "world",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-2",
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod-3",
						},
					},
				},
				indexes: []int{0, 1, 2, 3},
			},
			want: []int{0, 1, 2, 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &prioritySort{
				strategy: tt.fields.strategy,
			}
			if got := ps.Sort(tt.args.pods, tt.args.indexes); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("prioritySort.Sort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrioritySortCandidates(t *testing.T) {
	pods := []*v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"zone": "a", "rank": "pod-3"}}},
		{},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"zone": "b", "rank": "pod-10"}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"zone": "a", "rank": "invalid"}}},
		nil, // Unselected pods must not be inspected.
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"zone": "a", "backup": "pod-20"}}},
	}
	original := make([]*v1.Pod, len(pods))
	for i := range pods {
		original[i] = pods[i].DeepCopy()
	}
	weight := appspub.UpdatePriorityWeightTerm{
		Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"zone": "a"}},
	}
	invalid := appspub.UpdatePriorityWeightTerm{
		Weight: 100, MatchSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "zone", Operator: "invalid"},
		}},
	}
	for _, tc := range []struct {
		name     string
		strategy *appspub.UpdatePriorityStrategy
		want     []int
	}{
		{
			name: "nil strategy", want: []int{5, 2, 3, 0, 1, 5},
		},
		{
			name: "empty strategy", strategy: &appspub.UpdatePriorityStrategy{},
			want: []int{5, 2, 3, 0, 1, 5},
		},
		{
			name: "overlapping weights and stable ties",
			strategy: &appspub.UpdatePriorityStrategy{WeightPriority: []appspub.UpdatePriorityWeightTerm{
				weight,
				{Weight: 7, MatchSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "rank", Operator: metav1.LabelSelectorOpExists},
				}}},
			}},
			want: []int{3, 0, 5, 5, 2, 1},
		},
		{
			name: "invalid selectors are skipped",
			strategy: &appspub.UpdatePriorityStrategy{WeightPriority: []appspub.UpdatePriorityWeightTerm{
				invalid, weight,
			}},
			want: []int{5, 3, 0, 5, 2, 1},
		},
		{
			name:     "all invalid selectors preserve order",
			strategy: &appspub.UpdatePriorityStrategy{WeightPriority: []appspub.UpdatePriorityWeightTerm{invalid}},
			want:     []int{5, 2, 3, 0, 1, 5},
		},
		{
			name: "no matches preserve order",
			strategy: &appspub.UpdatePriorityStrategy{WeightPriority: []appspub.UpdatePriorityWeightTerm{
				{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"zone": "missing"}}},
			}},
			want: []int{5, 2, 3, 0, 1, 5},
		},
		{
			name: "ordered keys retain missing label and suffix behavior",
			strategy: &appspub.UpdatePriorityStrategy{OrderPriority: []appspub.UpdatePriorityOrderTerm{
				{OrderedKey: "rank"}, {OrderedKey: "backup"},
			}},
			want: []int{1, 2, 0, 3, 5, 5},
		},
		{
			name: "weights take precedence over ordered keys",
			strategy: &appspub.UpdatePriorityStrategy{
				WeightPriority: []appspub.UpdatePriorityWeightTerm{weight},
				OrderPriority:  []appspub.UpdatePriorityOrderTerm{{OrderedKey: "rank"}},
			},
			want: []int{5, 3, 0, 5, 2, 1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			indexes := []int{5, 2, 3, 0, 1, 5}
			got := NewPrioritySorter(tc.strategy).Sort(pods, indexes)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
			if &got[0] != &indexes[0] || !reflect.DeepEqual(indexes, tc.want) {
				t.Fatal("Sort must update and return the input slice")
			}
			if !reflect.DeepEqual(pods, original) {
				t.Fatal("Sort modified the pods")
			}
		})
	}
}

func TestPrioritySortReused(t *testing.T) {
	strategy := &appspub.UpdatePriorityStrategy{WeightPriority: []appspub.UpdatePriorityWeightTerm{
		{Weight: 10, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"zone": "a"}}},
	}}
	sorter := NewPrioritySorter(strategy)
	pods := []*v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"zone": "a"}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"zone": "b"}}},
	}
	check := func(want []int) {
		t.Helper()
		if got := sorter.Sort(pods, []int{0, 1}); !reflect.DeepEqual(got, want) {
			t.Fatalf("want %v, got %v", want, got)
		}
	}
	check([]int{0, 1})
	pods[0].Labels["zone"], pods[1].Labels["zone"] = "b", "a"
	check([]int{1, 0})
	strategy.WeightPriority[0].MatchSelector.MatchLabels["zone"] = "b"
	check([]int{0, 1})
	strategy.WeightPriority = nil
	strategy.OrderPriority = []appspub.UpdatePriorityOrderTerm{{OrderedKey: "rank"}}
	pods[0].Labels["rank"], pods[1].Labels["rank"] = "pod-1", "pod-2"
	check([]int{1, 0})
	pods[0].Labels["rank"] = "pod-3"
	check([]int{0, 1})
}

func TestPrioritySortEmptyCandidates(t *testing.T) {
	sorter := NewPrioritySorter(&appspub.UpdatePriorityStrategy{
		OrderPriority: []appspub.UpdatePriorityOrderTerm{{OrderedKey: "rank"}},
	})
	for _, indexes := range [][]int{nil, {}, {0}} {
		if got := sorter.Sort([]*v1.Pod{nil}, indexes); !reflect.DeepEqual(got, indexes) {
			t.Fatalf("want %v, got %v", indexes, got)
		}
	}
}
