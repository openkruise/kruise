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
	"fmt"
	"reflect"
	"unsafe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/kubernetes/pkg/util/slice"
)

// IsSelectorOverlapping indicates whether selector overlaps, the criteria:
// if exist one same key has different value and not overlap, then it is judged non-overlap, for examples:
//   - a=b and a=c
//   - a in [b,c] and a not in [b,c...]
//   - a not in [b] and a not exist
//   - a=b,c=d,e=f and a=x,c=d,e=f
//
// then others is overlap：
//   - a=b and c=d
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

// ValidatedLabelSelectorAsSelector is faster than native `metav1.LabelSelectorAsSelector` for the newRequirement function
// performs no validation. MAKE SURE the `ps` param is validated with `metav1.LabelSelectorAsSelector` before.
func ValidatedLabelSelectorAsSelector(ps *metav1.LabelSelector) (labels.Selector, error) {
	if ps == nil {
		return labels.Nothing(), nil
	}
	if len(ps.MatchLabels)+len(ps.MatchExpressions) == 0 {
		return labels.Everything(), nil
	}

	selector := labels.NewSelector()
	for k, v := range ps.MatchLabels {
		r, err := newRequirement(k, selection.Equals, []string{v})
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*r)
	}
	for _, expr := range ps.MatchExpressions {
		var op selection.Operator
		switch expr.Operator {
		case metav1.LabelSelectorOpIn:
			op = selection.In
		case metav1.LabelSelectorOpNotIn:
			op = selection.NotIn
		case metav1.LabelSelectorOpExists:
			op = selection.Exists
		case metav1.LabelSelectorOpDoesNotExist:
			op = selection.DoesNotExist
		default:
			return nil, fmt.Errorf("%q is not a valid pod selector operator", expr.Operator)
		}
		r, err := newRequirement(expr.Key, op, append([]string(nil), expr.Values...))
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*r)
	}
	return selector, nil
}

func newRequirement(key string, op selection.Operator, vals []string) (*labels.Requirement, error) {
	sel := &labels.Requirement{}
	selVal := reflect.ValueOf(sel)
	val := reflect.Indirect(selVal)

	keyField := val.FieldByName("key")
	keyFieldPtr := (*string)(unsafe.Pointer(keyField.UnsafeAddr()))
	*keyFieldPtr = key

	opField := val.FieldByName("operator")
	opFieldPtr := (*selection.Operator)(unsafe.Pointer(opField.UnsafeAddr()))
	*opFieldPtr = op

	if len(vals) > 0 {
		valuesField := val.FieldByName("strValues")
		valuesFieldPtr := (*[]string)(unsafe.Pointer(valuesField.UnsafeAddr()))
		*valuesFieldPtr = vals
	}

	return sel, nil
}

// IsSelectorLooseOverlap indicates whether selectors overlap (indicates that selector1, selector2 have same key, and there is an certain intersection）
// 1. when selector1、selector2 don't have same key, it is considered non-overlap, e.g. selector1(a=b) and selector2(c=d)
// 2. when selector1、selector2 have same key, and matchLabels & matchExps are intersection, it is considered overlap.
// For examples:
//
//	a In [b,c]    And a Exist
//	                  a In [b,...] [c,...] [Include any b,c,...]
//	                  a NotIn [a,...] [b,....] [c,....] [All other cases are allowed except for the inclusion of both b,c...] [b,c,e]
//	a Exist       And a Exist
//	                  a In [x,y,Any,...]
//	                  a NotIn [a,b,Any...]
//	a NotIn [b,c] And a Exist
//	                  a NotExist
//	                  a NotIn [a,b,Any...]
//	                  a In [a,b] [a,c] [e,f] [Any,...] other than [b],[c],[b,c]
//	a NotExist    And a NotExist
//	                  a NotIn [Any,...]
//	When selector1 and selector2 contain the same key, except for the above case, they are considered non-overlap
func IsSelectorLooseOverlap(selector1, selector2 *metav1.LabelSelector) bool {
	matchExp1 := convertSelectorToMatchExpressions(selector1)
	matchExp2 := convertSelectorToMatchExpressions(selector2)

	for k, exp1 := range matchExp1 {
		exp2, ok := matchExp2[k]
		if !ok {
			return false
		}

		if !isMatchExpOverlap(exp1, exp2) {
			return false
		}
	}

	for k, exp2 := range matchExp2 {
		exp1, ok := matchExp1[k]
		if !ok {
			return false
		}

		if !isMatchExpOverlap(exp2, exp1) {
			return false
		}
	}

	return true
}

func isMatchExpOverlap(matchExp1, matchExp2 metav1.LabelSelectorRequirement) bool {
	switch matchExp1.Operator {
	case metav1.LabelSelectorOpIn:
		if matchExp2.Operator == metav1.LabelSelectorOpExists {
			return true
		} else if matchExp2.Operator == metav1.LabelSelectorOpIn && sliceOverlaps(matchExp2.Values, matchExp1.Values) {
			return true
		} else if matchExp2.Operator == metav1.LabelSelectorOpNotIn && !sliceContains(matchExp2.Values, matchExp1.Values) {
			return true
		}
	case metav1.LabelSelectorOpExists:
		if matchExp2.Operator == metav1.LabelSelectorOpIn || matchExp2.Operator == metav1.LabelSelectorOpNotIn ||
			matchExp2.Operator == metav1.LabelSelectorOpExists {
			return true
		}
	case metav1.LabelSelectorOpNotIn:
		if matchExp2.Operator == metav1.LabelSelectorOpExists || matchExp2.Operator == metav1.LabelSelectorOpDoesNotExist ||
			matchExp2.Operator == metav1.LabelSelectorOpNotIn {
			return true
		} else if matchExp2.Operator == metav1.LabelSelectorOpIn && !sliceContains(matchExp1.Values, matchExp2.Values) {
			return true
		}
	case metav1.LabelSelectorOpDoesNotExist:
		if matchExp2.Operator == metav1.LabelSelectorOpDoesNotExist || matchExp2.Operator == metav1.LabelSelectorOpNotIn {
			return true
		}
	}

	return false
}

func convertSelectorToMatchExpressions(selector *metav1.LabelSelector) map[string]metav1.LabelSelectorRequirement {
	matchExps := map[string]metav1.LabelSelectorRequirement{}
	for _, exp := range selector.MatchExpressions {
		matchExps[exp.Key] = exp
	}

	for k, v := range selector.MatchLabels {
		matchExps[k] = metav1.LabelSelectorRequirement{
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{v},
		}
	}

	return matchExps
}
