/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOWorkloadSpread WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workloadspread

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
)

func newScheduledFailedPod() *corev1.Pod {
	pod := podDemo.DeepCopy()
	pod.CreationTimestamp = metav1.Time{Time: currentTime.Add(1 * m)}
	pod.Spec.NodeName = ""
	pod.Status.Phase = corev1.PodPending
	condition := corev1.PodCondition{
		Type:   corev1.PodScheduled,
		Status: corev1.ConditionFalse,
		Reason: corev1.PodReasonUnschedulable,
	}
	pod.Status.Conditions = append(pod.Status.Conditions, condition)
	return pod
}

func TestRescheduleSubset(t *testing.T) {
	currentTime = time.Now()

	wsDemo := workloadSpreadDemo.DeepCopy()
	wsDemo.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{
		{
			Name:        "subset-a",
			MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 5},
		},
		{
			Name: "subset-b",
		},
	}
	wsDemo.Status.SubsetStatuses = []appsv1alpha1.WorkloadSpreadSubsetStatus{
		{
			Name:            "subset-a",
			MissingReplicas: 5,
			CreatingPods:    map[string]metav1.Time{},
			DeletingPods:    map[string]metav1.Time{},
		},
		{
			Name:            "subset-b",
			MissingReplicas: -1,
			CreatingPods:    map[string]metav1.Time{},
			DeletingPods:    map[string]metav1.Time{},
			SubsetUnscheduledStatus: appsv1alpha1.SubsetUnscheduledStatus{
				Unschedulable:   false,
				FailedCount:     0,
				UnscheduledTime: metav1.Now(),
			},
		},
	}
	cases := []struct {
		name                 string
		getPods              func() []*corev1.Pod
		getWorkloadSpread    func() *appsv1alpha1.WorkloadSpread
		getCloneSet          func() *appsv1alpha1.CloneSet
		expectPods           func() []*corev1.Pod
		expectWorkloadSpread func() *appsv1alpha1.WorkloadSpread
	}{
		{
			name: "create no Pods, subset-a is scheduable",
			getPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				return ws
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Status.ObservedGeneration = 10
				ws.Status.SubsetStatuses[0].MissingReplicas = 5
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = false
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 0
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.UnscheduledTime = metav1.Now()
				return ws
			},
		},
		{
			name: "create two Pods, two pending, subset-a is unscheduable",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 2)
				for i := range pods {
					pods[i] = newScheduledFailedPod()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Spec.ScheduleStrategy.Type = appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType
				ws.Spec.ScheduleStrategy.Adaptive = &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
					RescheduleCriticalSeconds: pointer.Int32Ptr(20),
				}
				return ws
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Status.ObservedGeneration = 10
				ws.Status.SubsetStatuses[0].MissingReplicas = 3
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = true
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.UnscheduledTime = metav1.Now()
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 1
				return ws
			},
		},
		{
			name: "subset-a is unscheduable, subset-b is scheduable",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 2)
				for i := range pods {
					pods[i] = newScheduledFailedPod()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Spec.ScheduleStrategy.Type = appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType
				ws.Spec.ScheduleStrategy.Adaptive = &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
					RescheduleCriticalSeconds: pointer.Int32Ptr(20),
				}
				return ws
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Status.ObservedGeneration = 10
				ws.Status.SubsetStatuses[0].MissingReplicas = 3
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = true
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.UnscheduledTime = metav1.Now()
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 1
				return ws
			},
		},
		{
			name: "subset-a was unscheduable and not reach up 10 minutes",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 0)
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Spec.ScheduleStrategy.Type = appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType
				ws.Spec.ScheduleStrategy.Adaptive = &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
					RescheduleCriticalSeconds: pointer.Int32Ptr(20),
				}
				ws.Status.ObservedGeneration = 10
				ws.Status.SubsetStatuses[0].MissingReplicas = 3
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = true
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.UnscheduledTime = metav1.Time{Time: currentTime.Add(5 * m)}
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 1
				return ws
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Status.SubsetStatuses[0].MissingReplicas = 5
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = true
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.UnscheduledTime = metav1.Time{Time: currentTime.Add(5 * m)}
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 1
				return ws
			},
		},
		{
			name: "subset-a was recovered from unscheduable to scheduable",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 0)
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Spec.ScheduleStrategy.Type = appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType
				ws.Spec.ScheduleStrategy.Adaptive = &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
					RescheduleCriticalSeconds: pointer.Int32Ptr(20),
				}
				ws.Status.ObservedGeneration = 10
				ws.Status.SubsetStatuses[0].MissingReplicas = 3
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = true
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.UnscheduledTime = metav1.Time{Time: currentTime.Add(15 * m)}
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 1
				return ws
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Status.SubsetStatuses[0].MissingReplicas = 5
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = false
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 1
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.UnscheduledTime = metav1.Now()
				return ws
			},
		},
		{
			name: "subset-a was unscheduable again",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 2)
				for i := range pods {
					pods[i] = newScheduledFailedPod()
					pods[i].Name = fmt.Sprintf("test-pod-%d", i)
					pods[i].Annotations = map[string]string{
						wsutil.MatchedWorkloadSpreadSubsetAnnotations: `{"Name":"test-workloadSpread","Subset":"subset-a"}`,
					}
				}
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Spec.ScheduleStrategy.Type = appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType
				ws.Spec.ScheduleStrategy.Adaptive = &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
					RescheduleCriticalSeconds: pointer.Int32Ptr(20),
				}
				ws.Status.ObservedGeneration = 10
				ws.Status.SubsetStatuses[0].MissingReplicas = 5
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = false
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 1
				return ws
			},
			getCloneSet: func() *appsv1alpha1.CloneSet {
				return cloneSetDemo.DeepCopy()
			},
			expectPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			expectWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Status.ObservedGeneration = 10
				ws.Status.SubsetStatuses[0].MissingReplicas = 3
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.Unschedulable = true
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.UnscheduledTime = metav1.Now()
				ws.Status.SubsetStatuses[0].SubsetUnscheduledStatus.FailedCount = 2
				return ws
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			currentTime = time.Now()
			workloadSpread := cs.getWorkloadSpread()
			fakeClient := fake.NewFakeClientWithScheme(scheme, cs.getCloneSet(), workloadSpread)
			for _, pod := range cs.getPods() {
				podIn := pod.DeepCopy()
				err := fakeClient.Create(context.TODO(), podIn)
				if err != nil {
					t.Fatalf("create pod failed: %s", err.Error())
				}
			}

			reconciler := ReconcileWorkloadSpread{
				Client:   fakeClient,
				recorder: record.NewFakeRecorder(10),
			}

			err := reconciler.syncWorkloadSpread(workloadSpread)
			if err != nil {
				t.Fatalf("sync WorkloadSpread failed: %s", err.Error())
			}

			latestWorkloadSpread, err := getLatestWorkloadSpread(fakeClient, workloadSpread)
			if err != nil {
				t.Fatalf("getLatestWorkloadSpread failed: %s", err.Error())
			}

			latestPodList, err := getLatestPods(fakeClient, workloadSpread)
			expectPodList := cs.expectPods()
			if len(latestPodList) != len(expectPodList) {
				fmt.Println(len(latestPodList))
				fmt.Println(len(expectPodList))
				t.Fatalf("delete Pod failed")
			}

			latestStatus := latestWorkloadSpread.Status
			by, _ := json.Marshal(latestStatus)
			fmt.Println(string(by))

			exceptStatus := cs.expectWorkloadSpread().Status
			by, _ = json.Marshal(exceptStatus)
			fmt.Println(string(by))

			for i := 0; i < len(latestWorkloadSpread.Spec.Subsets); i++ {
				ls := latestStatus.SubsetStatuses[i].SubsetUnscheduledStatus
				es := exceptStatus.SubsetStatuses[i].SubsetUnscheduledStatus

				if ls.Unschedulable != es.Unschedulable || ls.FailedCount != es.FailedCount {
					t.Fatalf("rescheudle failed")
				}

				if ls.UnscheduledTime.Time.Sub(es.UnscheduledTime.Time) > 1*time.Second {
					t.Fatalf("rescheudle failed")
				}
			}
		})
	}
}
