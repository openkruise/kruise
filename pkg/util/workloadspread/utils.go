/*
Copyright 2024 The Kruise Authors.

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
	"errors"
	"fmt"
	"strconv"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"k8s.io/apimachinery/pkg/labels"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
)

func hasPercentSubset(ws *appsv1alpha1.WorkloadSpread) (has bool) {
	if ws == nil {
		return false
	}
	for _, subset := range ws.Spec.Subsets {
		if subset.MaxReplicas != nil && subset.MaxReplicas.Type == intstrutil.String &&
			strings.HasSuffix(subset.MaxReplicas.StrVal, "%") {
			return true
		}
	}
	return false
}

func NestedField[T any](obj any, paths ...string) (T, bool, error) {
	if len(paths) == 0 {
		val, ok := obj.(T)
		if !ok {
			return *new(T), false, errors.New("object type error")
		}
		return val, true, nil
	}
	if o, ok := obj.(map[string]any); ok {
		return nestedMap[T](o, paths...)
	}
	if o, ok := obj.([]any); ok {
		return nestedSlice[T](o, paths...)
	}
	return *new(T), false, errors.New("object is not deep enough")
}

func nestedSlice[T any](obj []any, paths ...string) (T, bool, error) {
	idx, err := strconv.Atoi(paths[0])
	if err != nil {
		return *new(T), false, err
	}
	if len(obj) < idx+1 {
		return *new(T), false, fmt.Errorf("index %d out of range", idx)
	}
	return NestedField[T](obj[idx], paths[1:]...)
}

func nestedMap[T any](obj map[string]any, paths ...string) (T, bool, error) {
	if val, ok := obj[paths[0]]; ok {
		return NestedField[T](val, paths[1:]...)
	} else {
		return *new(T), false, fmt.Errorf("path \"%s\" not exists", paths[0])
	}
}

func IsPodSelected(filter *appsv1alpha1.TargetFilter, podLabels map[string]string) (bool, error) {
	if filter == nil {
		return true, nil
	}
	selector, err := util.ValidatedLabelSelectorAsSelector(filter.Selector)
	if err != nil {
		return false, err
	}
	return selector.Matches(labels.Set(podLabels)), nil
}
