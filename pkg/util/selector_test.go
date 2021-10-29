/*
Copyright 2020 The Kruise Authors.

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

package util

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestCase struct {
	Input  [2]metav1.LabelSelector
	Output bool
}

func TestSelectorConflict(t *testing.T) {
	testCases := []TestCase{
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"a": "i"},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"b": "i"},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						"a": "h",
						"b": "i",
						"c": "j",
					},
				},
				{
					MatchLabels: map[string]string{
						"a": "h",
						"b": "x",
						"c": "j",
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"h", "i", "j"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"i", "j"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"h", "i"},
						},
					},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: false,
		},
	}

	for i, testCase := range testCases {
		output := !IsDefinitelyNeverOverlapped(&testCase.Input[0], &testCase.Input[1])
		if output != testCase.Output {
			t.Errorf("%v: expect %v but got %v", i, testCase.Output, output)
		}
	}
}

func TestSelectorLooseOverlap(t *testing.T) {
	testCases := []TestCase{
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "e", "f"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c", "f"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c"},
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"b", "c", "e"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"b", "c", "e"},
						},
					},
				},
			},
			Output: true,
		},
	}

	for i, testCase := range testCases {
		output := IsSelectorLooseOverlap(&testCase.Input[0], &testCase.Input[1])
		if output != testCase.Output {
			t.Errorf("%v: expect %v but got %v", i, testCase.Output, output)
		}
	}
}

func TestIsMatchExpOverlap(t *testing.T) {
	cases := []struct {
		name      string
		matchExp1 func() metav1.LabelSelectorRequirement
		matchExp2 func() metav1.LabelSelectorRequirement
		except    bool
	}{
		{
			name: "LabelSelectorOpIn and LabelSelectorOpExists, and overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpExists,
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpIn, and overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"a", "c"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpIn and SelectorOpExists, and not overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"a", "d"},
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpNotIn, and not overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c", "e"},
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpIn and LabelSelectorOpNotIn, and overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "e"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpExists and LabelSelectorOpDoesNotExist, and not overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpExists,
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpDoesNotExist,
				}
			},
			except: false,
		},
		{
			name: "LabelSelectorOpNotIn and LabelSelectorOpIn, and overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"a", "c"},
				}
			},
			except: true,
		},
		{
			name: "LabelSelectorOpNotIn and LabelSelectorOpIn, and not overlap",
			matchExp1: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"b", "c"},
				}
			},
			matchExp2: func() metav1.LabelSelectorRequirement {
				return metav1.LabelSelectorRequirement{
					Key:      "a",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"c"},
				}
			},
			except: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			overlap := isMatchExpOverlap(cs.matchExp1(), cs.matchExp2())
			if cs.except != overlap {
				t.Fatalf("isMatchExpOverlap(%s) except(%v) but get(%v)", cs.name, cs.except, overlap)
			}
		})
	}
}
