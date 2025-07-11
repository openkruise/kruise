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
