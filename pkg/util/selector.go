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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/kubernetes/pkg/util/slice"
)

// whether selector overlaps, the criteria:
// if exist one same key has different value and not overlap, then it is judged non-overlap, for examples:
//   * a=b and a=c
//   * a in [b,c] and a not in [b,c...]
//   * a not in [b] and a not exist
//   * a=b,c=d,e=f and a=x,c=d,e=f
// then others is overlap：
//   * a=b and c=d
func IsSelectorOverlapping(selector1, selector2 *metav1.LabelSelector) bool {
	return !(isDisjoint(selector1, selector2) || isDisjoint(selector2, selector1))
}

func isDisjoint(selector1, selector2 *metav1.LabelSelector) bool {
	// label -> values
	// a=b convert to a -> [b]
	// a in [b,c] convert to a -> [b,c]
	// a exist convert to a -> [ALL]
	matchedLabels1 := make(map[string][]string)
	for key, value := range selector1.MatchLabels {
		matchedLabels1[key] = []string{value}
	}
	for _, req := range selector1.MatchExpressions {
		switch req.Operator {
		case metav1.LabelSelectorOpIn:
			for _, value := range req.Values {
				matchedLabels1[req.Key] = append(matchedLabels1[req.Key], value)
			}
		case metav1.LabelSelectorOpExists:
			matchedLabels1[req.Key] = []string{"ALL"}
		}
	}

	for key, value := range selector2.MatchLabels {
		values, ok := matchedLabels1[key]
		if ok {
			if !slice.ContainsString(values, "ALL", nil) && !slice.ContainsString(values, value, nil) {
				return true
			}
		}
	}
	for _, req := range selector2.MatchExpressions {
		values, ok := matchedLabels1[req.Key]

		switch req.Operator {
		case metav1.LabelSelectorOpIn:
			if ok && !slice.ContainsString(values, "ALL", nil) && !sliceOverlaps(values, req.Values) {
				return true
			}
		case metav1.LabelSelectorOpNotIn:
			if ok && sliceContains(req.Values, values) {
				return true
			}
		case metav1.LabelSelectorOpExists:
			if !ok {
				return true
			}
		case metav1.LabelSelectorOpDoesNotExist:
			if ok {
				return true
			}
		}
	}

	return false
}

func sliceOverlaps(a, b []string) bool {
	keyExist := make(map[string]bool, len(a))
	for _, key := range a {
		keyExist[key] = true
	}
	for _, key := range b {
		if keyExist[key] {
			return true
		}
	}
	return false
}

// a contains b
func sliceContains(a, b []string) bool {
	keyExist := make(map[string]bool, len(a))
	for _, key := range a {
		keyExist[key] = true
	}
	for _, key := range b {
		if !keyExist[key] {
			return false
		}
	}
	return true
}

func GetFastLabelSelector(ps *metav1.LabelSelector) (labels.Selector, error) {
	var selector labels.Selector
	if len(ps.MatchExpressions) == 0 && len(ps.MatchLabels) != 0 {
		selector = labels.SelectorFromValidatedSet(ps.MatchLabels)
		return selector, nil
	}

	return metav1.LabelSelectorAsSelector(ps)
}
