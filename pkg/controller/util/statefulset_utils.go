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

package util

import (
	"regexp"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

var statefulPodRegex = regexp.MustCompile("(.*)-([0-9]+)$")

func GetOrdinal(pod *corev1.Pod) int32 {
	_, ordinal := getParentNameAndOrdinal(pod)
	return ordinal
}

func getParentNameAndOrdinal(pod *corev1.Pod) (string, int32) {
	parent := ""
	var ordinal int32 = -1
	subMatches := statefulPodRegex.FindStringSubmatch(pod.Name)
	if len(subMatches) < 3 {
		return parent, ordinal
	}
	parent = subMatches[1]
	if i, err := strconv.ParseInt(subMatches[2], 10, 32); err == nil {
		ordinal = int32(i)
	}
	return parent, ordinal
}
