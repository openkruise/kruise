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

package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"unsafe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// DumpJSON returns the JSON encoding
func DumpJSON(o interface{}) string {
	j, _ := json.Marshal(o)
	return string(j)
}

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

func GetNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); len(ns) > 0 {
		return ns
	}
	return "kruise-system"
}

// SplitMaybeSubscriptedPath checks whether the specified fieldPath is
// subscripted, and
//  - if yes, this function splits the fieldPath into path and subscript, and
//    returns (path, subscript, true).
//  - if no, this function returns (fieldPath, "", false).
//
// Example inputs and outputs:
//  - "metadata.annotations['myKey']" --> ("metadata.annotations", "myKey", true)
//  - "metadata.annotations['a[b]c']" --> ("metadata.annotations", "a[b]c", true)
//  - "metadata.labels['']"           --> ("metadata.labels", "", true)
//  - "metadata.labels"               --> ("metadata.labels", "", false)
func SplitMaybeSubscriptedPath(fieldPath string) (string, string, bool) {
	if !strings.HasSuffix(fieldPath, "']") {
		return fieldPath, "", false
	}
	s := strings.TrimSuffix(fieldPath, "']")
	parts := strings.SplitN(s, "['", 2)
	if len(parts) < 2 {
		return fieldPath, "", false
	}
	if len(parts[0]) == 0 {
		return fieldPath, "", false
	}
	return parts[0], parts[1], true
}
