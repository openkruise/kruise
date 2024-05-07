/*
Copyright 2021 The Kruise Authors.

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
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
	wsDemo.Spec.ScheduleStrategy = appsv1alpha1.WorkloadSpreadScheduleStrategy{
		Type: appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType,
		Adaptive: &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
			RescheduleCriticalSeconds: pointer.Int32Ptr(5),
		},
	}
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
			Conditions: []appsv1alpha1.WorkloadSpreadSubsetCondition{
				*NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionTrue, "", ""),
			},
		},
		{
			Name:            "subset-b",
			MissingReplicas: -1,
			CreatingPods:    map[string]metav1.Time{},
			DeletingPods:    map[string]metav1.Time{},
			Conditions: []appsv1alpha1.WorkloadSpreadSubsetCondition{
				*NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionTrue, "", ""),
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
			name: "close reschedule strategy, condition is null",
			getPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Spec.ScheduleStrategy.Type = appsv1alpha1.FixedWorkloadSpreadScheduleStrategyType
				ws.Status.SubsetStatuses[0].Conditions = nil
				ws.Status.SubsetStatuses[1].Conditions = nil
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
				ws.Status.SubsetStatuses[0].Conditions = nil
				ws.Status.SubsetStatuses[1].Conditions = nil
				return ws
			},
		},
		{
			name: "create no Pods, subset-a is scheduable",
			getPods: func() []*corev1.Pod {
				return []*corev1.Pod{}
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Status.SubsetStatuses[0].Conditions = nil
				ws.Status.SubsetStatuses[1].Conditions = nil
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
				ws.Status.SubsetStatuses[0].Conditions = []appsv1alpha1.WorkloadSpreadSubsetCondition{
					*NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionTrue, "", ""),
				}
				ws.Status.SubsetStatuses[1].Conditions = []appsv1alpha1.WorkloadSpreadSubsetCondition{
					*NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionTrue, "", ""),
				}
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
				ws.Status.SubsetStatuses[0].Conditions = []appsv1alpha1.WorkloadSpreadSubsetCondition{
					*NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionFalse, "", ""),
				}
				ws.Status.SubsetStatuses[1].Conditions = []appsv1alpha1.WorkloadSpreadSubsetCondition{
					*NewWorkloadSpreadSubsetCondition(appsv1alpha1.SubsetSchedulable, corev1.ConditionTrue, "", ""),
				}
				return ws
			},
		},
		{
			name: "subset-a was unscheduable and not reach up 5 minutes",
			getPods: func() []*corev1.Pod {
				pods := make([]*corev1.Pod, 0)
				return pods
			},
			getWorkloadSpread: func() *appsv1alpha1.WorkloadSpread {
				ws := wsDemo.DeepCopy()
				ws.Status.SubsetStatuses[0].Conditions = []appsv1alpha1.WorkloadSpreadSubsetCondition{
					{
						Type:               appsv1alpha1.SubsetSchedulable,
						Status:             corev1.ConditionFalse,
						LastTransitionTime: metav1.Time{Time: currentTime.Add(4 * m)},
					},
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
				ws.Status.SubsetStatuses[0].Conditions = []appsv1alpha1.WorkloadSpreadSubsetCondition{
					{
						Type:               appsv1alpha1.SubsetSchedulable,
						Status:             corev1.ConditionFalse,
						LastTransitionTime: metav1.Time{Time: currentTime.Add(4 * m)},
					},
				}
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
				ws.Status.SubsetStatuses[0].Conditions = []appsv1alpha1.WorkloadSpreadSubsetCondition{
					{
						Type:               appsv1alpha1.SubsetSchedulable,
						Status:             corev1.ConditionFalse,
						LastTransitionTime: metav1.Time{Time: currentTime.Add(10 * m)},
					},
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
				ws.Status.SubsetStatuses[0].Conditions = []appsv1alpha1.WorkloadSpreadSubsetCondition{
					{
						Type:               appsv1alpha1.SubsetSchedulable,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				}
				return ws
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			currentTime = time.Now()
			workloadSpread := cs.getWorkloadSpread()
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs.getCloneSet(), workloadSpread).
				WithIndex(&corev1.Pod{}, fieldindex.IndexNameForOwnerRefUID, func(obj client.Object) []string {
					var owners []string
					for _, ref := range obj.GetOwnerReferences() {
						owners = append(owners, string(ref.UID))
					}
					return owners
				}).WithStatusSubresource(&appsv1alpha1.WorkloadSpread{}).Build()
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
				controllerFinder: &controllerfinder.ControllerFinder{
					Client: fakeClient,
				},
			}

			err := reconciler.syncWorkloadSpread(workloadSpread)
			if err != nil {
				t.Fatalf("sync WorkloadSpread failed: %s", err.Error())
			}

			latestWorkloadSpread, err := getLatestWorkloadSpread(fakeClient, workloadSpread)
			if err != nil {
				t.Fatalf("getLatestWorkloadSpread failed: %s", err.Error())
			}

			latestPodList, _ := getLatestPods(fakeClient, workloadSpread)
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
				lc := GetWorkloadSpreadSubsetCondition(&latestStatus.SubsetStatuses[i], appsv1alpha1.SubsetSchedulable)
				ec := GetWorkloadSpreadSubsetCondition(&exceptStatus.SubsetStatuses[i], appsv1alpha1.SubsetSchedulable)

				if lc == nil && ec != nil {
					t.Fatalf("reschedule failed")
				}
				if lc != nil && ec == nil {
					t.Fatalf("reschedule failed")
				}
				if lc != nil && ec != nil && lc.Status != ec.Status {
					t.Fatalf("reschedule failed")
				}
			}
		})
	}
}
