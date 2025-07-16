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
	"testing"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetOrdinal(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Valid stateful pod name
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-statefulset-3",
		},
	}
	ordinal := GetOrdinal(pod)
	g.Expect(ordinal).To(gomega.Equal(int32(3)))

	// Test case 2: Pod name with zero ordinal
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-statefulset-0",
		},
	}
	ordinal = GetOrdinal(pod)
	g.Expect(ordinal).To(gomega.Equal(int32(0)))

	// Test case 3: Pod name with large ordinal
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-statefulset-999",
		},
	}
	ordinal = GetOrdinal(pod)
	g.Expect(ordinal).To(gomega.Equal(int32(999)))

	// Test case 4: Invalid pod name (no hyphen and number)
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pod",
		},
	}
	ordinal = GetOrdinal(pod)
	g.Expect(ordinal).To(gomega.Equal(int32(-1)))

	// Test case 5: Invalid pod name (no number after hyphen)
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pod-",
		},
	}
	ordinal = GetOrdinal(pod)
	g.Expect(ordinal).To(gomega.Equal(int32(-1)))

	// Test case 6: Invalid pod name (non-numeric after hyphen)
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pod-abc",
		},
	}
	ordinal = GetOrdinal(pod)
	g.Expect(ordinal).To(gomega.Equal(int32(-1)))

	// Test case 7: Complex stateful pod name with multiple hyphens
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-complex-statefulset-name-5",
		},
	}
	ordinal = GetOrdinal(pod)
	g.Expect(ordinal).To(gomega.Equal(int32(5)))

	// Test case 8: Empty pod name
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "",
		},
	}
	ordinal = GetOrdinal(pod)
	g.Expect(ordinal).To(gomega.Equal(int32(-1)))
}

func TestGetParentNameAndOrdinal(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Valid stateful pod name
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-statefulset-3",
		},
	}
	parent, ordinal := getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal("my-statefulset"))
	g.Expect(ordinal).To(gomega.Equal(int32(3)))

	// Test case 2: Complex parent name with hyphens
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-complex-statefulset-name-5",
		},
	}
	parent, ordinal = getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal("my-complex-statefulset-name"))
	g.Expect(ordinal).To(gomega.Equal(int32(5)))

	// Test case 3: Zero ordinal
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "statefulset-0",
		},
	}
	parent, ordinal = getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal("statefulset"))
	g.Expect(ordinal).To(gomega.Equal(int32(0)))

	// Test case 4: Invalid pod name format
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "regular-pod",
		},
	}
	parent, ordinal = getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal(""))
	g.Expect(ordinal).To(gomega.Equal(int32(-1)))

	// Test case 5: Pod name ending with non-numeric
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pod-abc",
		},
	}
	parent, ordinal = getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal(""))
	g.Expect(ordinal).To(gomega.Equal(int32(-1)))

	// Test case 6: Pod name with negative number (invalid for ordinal)
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pod--1",
		},
	}
	parent, ordinal = getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal("my-pod-"))
	g.Expect(ordinal).To(gomega.Equal(int32(1)))

	// Test case 7: Single character parent name
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "a-7",
		},
	}
	parent, ordinal = getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal("a"))
	g.Expect(ordinal).To(gomega.Equal(int32(7)))

	// Test case 8: Large ordinal number
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-2147483647", // max int32
		},
	}
	parent, ordinal = getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal("test"))
	g.Expect(ordinal).To(gomega.Equal(int32(2147483647)))

	// Test case 9: Very large number that exceeds int32
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-9999999999999999999",
		},
	}
	parent, ordinal = getParentNameAndOrdinal(pod)
	g.Expect(parent).To(gomega.Equal("test"))
	g.Expect(ordinal).To(gomega.Equal(int32(-1)))
}
