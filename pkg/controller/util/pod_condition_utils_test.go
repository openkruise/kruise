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
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetMessageKvFromCondition(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test case 1: Nil condition
	kv, err := GetMessageKvFromCondition(nil)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(kv).To(gomega.Equal(make(map[string]interface{})))

	// Test case 2: Empty message
	condition := &v1.PodCondition{
		Type:    v1.PodReady,
		Status:  v1.ConditionTrue,
		Message: "",
	}
	kv, err = GetMessageKvFromCondition(condition)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(kv).To(gomega.Equal(make(map[string]interface{})))

	// Test case 3: Valid JSON message
	condition = &v1.PodCondition{
		Type:    v1.PodReady,
		Status:  v1.ConditionTrue,
		Message: `{"key1":"value1","key2":123,"key3":true}`,
	}
	kv, err = GetMessageKvFromCondition(condition)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(kv["key1"]).To(gomega.Equal("value1"))
	g.Expect(kv["key2"]).To(gomega.Equal(float64(123))) // JSON numbers are parsed as float64
	g.Expect(kv["key3"]).To(gomega.Equal(true))

	// Test case 4: Invalid JSON message
	condition = &v1.PodCondition{
		Type:    v1.PodReady,
		Status:  v1.ConditionTrue,
		Message: `invalid json`,
	}
	kv, err = GetMessageKvFromCondition(condition)
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(kv).To(gomega.BeNil())
}

func TestUpdateMessageKvCondition(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	condition := &v1.PodCondition{
		Type:   v1.PodReady,
		Status: v1.ConditionTrue,
	}

	// Test case 1: Simple key-value pairs
	kv := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	UpdateMessageKvCondition(kv, condition)
	g.Expect(condition.Message).NotTo(gomega.BeEmpty())

	// Verify the message can be unmarshaled back
	resultKv, err := GetMessageKvFromCondition(condition)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(resultKv["key1"]).To(gomega.Equal("value1"))
	g.Expect(resultKv["key2"]).To(gomega.Equal(float64(123)))
	g.Expect(resultKv["key3"]).To(gomega.Equal(true))

	// Test case 2: Empty map
	emptyKv := make(map[string]interface{})
	UpdateMessageKvCondition(emptyKv, condition)
	g.Expect(condition.Message).To(gomega.Equal("{}"))
}

func TestGetTimeBeforePendingTimeout(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	timeout := 5 * time.Minute
	now := time.Now()

	// Test case 1: Pod is not pending
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Minute)),
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}
	timeouted, nextCheckAfter := GetTimeBeforePendingTimeout(pod, timeout)
	g.Expect(timeouted).To(gomega.BeFalse())
	g.Expect(nextCheckAfter).To(gomega.Equal(time.Duration(-1)))

	// Test case 2: Pod has deletion timestamp
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Minute)),
			DeletionTimestamp: &metav1.Time{Time: now},
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}
	timeouted, nextCheckAfter = GetTimeBeforePendingTimeout(pod, timeout)
	g.Expect(timeouted).To(gomega.BeFalse())
	g.Expect(nextCheckAfter).To(gomega.Equal(time.Duration(-1)))

	// Test case 3: Pod has been scheduled
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Minute)),
		},
		Spec: v1.PodSpec{
			NodeName: "test-node",
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}
	timeouted, nextCheckAfter = GetTimeBeforePendingTimeout(pod, timeout)
	g.Expect(timeouted).To(gomega.BeFalse())
	g.Expect(nextCheckAfter).To(gomega.Equal(time.Duration(-1)))

	// Test case 4: Pod is pending but no unschedulable condition
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Minute)),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}
	timeouted, nextCheckAfter = GetTimeBeforePendingTimeout(pod, timeout)
	g.Expect(timeouted).To(gomega.BeFalse())
	g.Expect(nextCheckAfter).To(gomega.Equal(time.Duration(-1)))

	// Test case 5: Pod is pending and unschedulable but not timed out
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Minute)),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodScheduled,
					Status: v1.ConditionFalse,
					Reason: v1.PodReasonUnschedulable,
				},
			},
		},
	}
	timeouted, nextCheckAfter = GetTimeBeforePendingTimeout(pod, timeout)
	g.Expect(timeouted).To(gomega.BeFalse())
	g.Expect(nextCheckAfter).To(gomega.BeNumerically(">", 0))
	g.Expect(nextCheckAfter).To(gomega.BeNumerically("<=", 2*time.Minute))

	// Test case 6: Pod is pending and timed out
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			CreationTimestamp: metav1.NewTime(now.Add(-10 * time.Minute)),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodScheduled,
					Status: v1.ConditionFalse,
					Reason: v1.PodReasonUnschedulable,
				},
			},
		},
	}
	timeouted, nextCheckAfter = GetTimeBeforePendingTimeout(pod, timeout)
	g.Expect(timeouted).To(gomega.BeTrue())
	g.Expect(nextCheckAfter).To(gomega.Equal(time.Duration(-1)))

	// Test case 7: Pod is pending but scheduled condition is not unschedulable
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Minute)),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodScheduled,
					Status: v1.ConditionFalse,
					Reason: "SomeOtherReason",
				},
			},
		},
	}
	timeouted, nextCheckAfter = GetTimeBeforePendingTimeout(pod, timeout)
	g.Expect(timeouted).To(gomega.BeFalse())
	g.Expect(nextCheckAfter).To(gomega.Equal(time.Duration(-1)))
}
