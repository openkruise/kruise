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
