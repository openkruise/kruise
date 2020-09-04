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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// GetPodNames returns names of the given Pods array
func GetPodNames(pods []*v1.Pod) sets.String {
	set := sets.NewString()
	for _, pod := range pods {
		set.Insert(pod.Name)
	}
	return set
}

// MergePods merges two pods arrays
func MergePods(pods1, pods2 []*v1.Pod) []*v1.Pod {
	var ret []*v1.Pod
	names := sets.NewString()

	for _, pod := range pods1 {
		if !names.Has(pod.Name) {
			ret = append(ret, pod)
			names.Insert(pod.Name)
		}
	}
	for _, pod := range pods2 {
		if !names.Has(pod.Name) {
			ret = append(ret, pod)
			names.Insert(pod.Name)
		}
	}
	return ret
}
