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

package utils

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSortingActivePods(t *testing.T) {
	now := metav1.Now()
	then := metav1.Time{Time: now.AddDate(0, -1, 0)}
	zeroTime := metav1.Time{}
	pod := func(podName, nodeName string, phase v1.PodPhase, ready bool, restarts int32, readySince metav1.Time, created metav1.Time, annotations map[string]string) *v1.Pod {
		var conditions []v1.PodCondition
		var containerStatuses []v1.ContainerStatus
		if ready {
			conditions = []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue, LastTransitionTime: readySince}}
			containerStatuses = []v1.ContainerStatus{{RestartCount: restarts}}
		}
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: created,
				Name:              podName,
				Annotations:       annotations,
			},
			Spec: v1.PodSpec{NodeName: nodeName},
			Status: v1.PodStatus{
				Conditions:        conditions,
				ContainerStatuses: containerStatuses,
				Phase:             phase,
			},
		}
	}
	var (
		unscheduledPod                      = pod("unscheduled", "", v1.PodPending, false, 0, zeroTime, zeroTime, nil)
		scheduledPendingPod                 = pod("pending", "node", v1.PodPending, false, 0, zeroTime, zeroTime, nil)
		unknownPhasePod                     = pod("unknown-phase", "node", v1.PodUnknown, false, 0, zeroTime, zeroTime, nil)
		runningNotReadyPod                  = pod("not-ready", "node", v1.PodRunning, false, 0, zeroTime, zeroTime, nil)
		runningReadyNoLastTransitionTimePod = pod("ready-no-last-transition-time", "node", v1.PodRunning, true, 0, zeroTime, zeroTime, nil)
		runningReadyNow                     = pod("ready-now", "node", v1.PodRunning, true, 0, now, now, nil)
		runningReadyThen                    = pod("ready-then", "node", v1.PodRunning, true, 0, then, then, nil)
		runningReadyNowHighRestarts         = pod("ready-high-restarts", "node", v1.PodRunning, true, 9001, now, now, nil)
		runningReadyNowCreatedThen          = pod("ready-now-created-then", "node", v1.PodRunning, true, 0, now, then, nil)
		lowPodDeletionCost                  = pod("low-deletion-cost", "node", v1.PodRunning, true, 0, now, then, map[string]string{PodDeletionCost: "10"})
		highPodDeletionCost                 = pod("high-deletion-cost", "node", v1.PodRunning, true, 0, now, then, map[string]string{PodDeletionCost: "100"})
	)
	equalityTests := []*v1.Pod{
		unscheduledPod,
		scheduledPendingPod,
		unknownPhasePod,
		runningNotReadyPod,
		runningReadyNowCreatedThen,
		runningReadyNow,
		runningReadyThen,
		runningReadyNowHighRestarts,
		runningReadyNowCreatedThen,
	}
	for _, pod := range equalityTests {
		podsWithDeletionCost := ActivePodsWithRanks{Pods: []*v1.Pod{pod, pod}}
		if podsWithDeletionCost.Less(0, 1) || podsWithDeletionCost.Less(1, 0) {
			t.Errorf("expected pod %q not to be less than than itself", pod.Name)
		}
	}
	type podWithDeletionCost *v1.Pod
	inequalityTests := []struct {
		lesser, greater podWithDeletionCost
	}{
		{lesser: podWithDeletionCost(unscheduledPod), greater: podWithDeletionCost(scheduledPendingPod)},
		{lesser: podWithDeletionCost(unscheduledPod), greater: podWithDeletionCost(scheduledPendingPod)},
		{lesser: podWithDeletionCost(scheduledPendingPod), greater: podWithDeletionCost(unknownPhasePod)},
		{lesser: podWithDeletionCost(unknownPhasePod), greater: podWithDeletionCost(runningNotReadyPod)},
		{lesser: podWithDeletionCost(runningNotReadyPod), greater: podWithDeletionCost(runningReadyNoLastTransitionTimePod)},
		{lesser: podWithDeletionCost(runningReadyNoLastTransitionTimePod), greater: podWithDeletionCost(runningReadyNow)},
		{lesser: podWithDeletionCost(runningReadyNow), greater: podWithDeletionCost(runningReadyThen)},
		{lesser: podWithDeletionCost(runningReadyNowHighRestarts), greater: podWithDeletionCost(runningReadyNow)},
		{lesser: podWithDeletionCost(runningReadyNow), greater: podWithDeletionCost(runningReadyNowCreatedThen)},
		{lesser: podWithDeletionCost(lowPodDeletionCost), greater: podWithDeletionCost(highPodDeletionCost)},
	}
	for i, test := range inequalityTests {
		t.Run(fmt.Sprintf("test%d", i), func(t *testing.T) {

			podsWithDeletionCost := ActivePodsWithRanks{Pods: []*v1.Pod{test.lesser, test.greater}}
			if !podsWithDeletionCost.Less(0, 1) {
				t.Errorf("expected pod %q to be less than %q", podsWithDeletionCost.Pods[0].Name, podsWithDeletionCost.Pods[1].Name)
			}
			if podsWithDeletionCost.Less(1, 0) {
				t.Errorf("expected pod %q not to be less than %v", podsWithDeletionCost.Pods[1].Name, podsWithDeletionCost.Pods[0].Name)
			}
		})
	}
}

func TestSameNodeRanker(t *testing.T) {
	// node a: 0, 4, 10
	// node b: 3, 6
	// node c: 5
	// node d: 7, 8, 9
	pods := []*v1.Pod{
		{ObjectMeta: metav1.ObjectMeta{UID: "0"}, Spec: v1.PodSpec{NodeName: "a"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "1"}, Spec: v1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{UID: "2"}, Spec: v1.PodSpec{}},
		{ObjectMeta: metav1.ObjectMeta{UID: "3"}, Spec: v1.PodSpec{NodeName: "b"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "4"}, Spec: v1.PodSpec{NodeName: "a"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "5"}, Spec: v1.PodSpec{NodeName: "c"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "6"}, Spec: v1.PodSpec{NodeName: "b"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "7"}, Spec: v1.PodSpec{NodeName: "d"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "8"}, Spec: v1.PodSpec{NodeName: "d"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "9"}, Spec: v1.PodSpec{NodeName: "d"}},
		{ObjectMeta: metav1.ObjectMeta{UID: "10"}, Spec: v1.PodSpec{NodeName: "a"}},
	}

	expectedRanks := map[types.UID]float64{
		"0": 2,
		"4": 1,
		"3": 1,
		"7": 2,
		"8": 1,
	}

	ranker := NewSameNodeRanker(pods).(*sameNodeRanker)
	if !reflect.DeepEqual(ranker.podRanks, expectedRanks) {
		t.Fatalf("got unexpected ranks: %+v", ranker.podRanks)
	}
}

func TestSpreadConstraintsRanker(t *testing.T) {
	nodes := []client.Object{
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "a", Labels: map[string]string{}}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "b", Labels: map[string]string{v1.LabelHostname: "b", v1.LabelZoneFailureDomain: "z1"}}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "c", Labels: map[string]string{v1.LabelHostname: "c", v1.LabelZoneFailureDomain: "z1"}}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "d", Labels: map[string]string{v1.LabelHostname: "d", v1.LabelZoneFailureDomain: "z1"}}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "e", Labels: map[string]string{v1.LabelHostname: "e", v1.LabelZoneFailureDomain: "z2"}}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "f", Labels: map[string]string{v1.LabelHostname: "f", v1.LabelZoneFailureDomain: "z2"}}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "g", Labels: map[string]string{v1.LabelHostname: "g", v1.LabelZoneFailureDomain: "z2"}}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "h", Labels: map[string]string{v1.LabelHostname: "h", v1.LabelZoneFailureDomain: "z3"}}},
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "i", Labels: map[string]string{v1.LabelHostname: "i", v1.LabelZoneFailureDomain: "z3"}}},
	}
	genPods := func(n int, node string) []*v1.Pod {
		pods := make([]*v1.Pod, n)
		for i := 0; i < n; i++ {
			pods[i] = &v1.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID(fmt.Sprintf("%v-%v", node, i))}, Spec: v1.PodSpec{NodeName: node}}
		}
		return pods
	}
	var pods []*v1.Pod
	pods = append(pods, genPods(2, "a")...)
	pods = append(pods, genPods(2, "b")...)
	pods = append(pods, genPods(3, "c")...)
	pods = append(pods, genPods(5, "d")...)
	pods = append(pods, genPods(5, "e")...)
	pods = append(pods, genPods(3, "f")...)
	pods = append(pods, genPods(5, "i")...)

	fakeClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(nodes...).Build()

	// z1: 10 pods (b: 2, c: 3, d: 5)
	// z2: 8  pods (e: 5, f: 3)
	// z3: 5  pods (i: 5)

	cases := []struct {
		name               string
		constraints        []PodSpreadConstraint
		expectedPodsSorted []types.UID
	}{
		{
			name: "zone{no limit}",
			constraints: []PodSpreadConstraint{
				{TopologyKey: v1.LabelZoneFailureDomain},
			},
			expectedPodsSorted: []types.UID{
				"d-0",
				"d-1",
				"c-0", "e-0",
				"d-2", "e-1",
				"b-0", "e-2",
				"c-1", "f-0", "i-0",
				"d-3", "e-3", "i-1",
				"b-1", "f-1", "i-2",
				"c-2", "e-4", "i-3",
				"d-4", "f-2", "i-4",
			},
		},
		{
			name: "zone{limited (z2, z3)}",
			constraints: []PodSpreadConstraint{
				{TopologyKey: v1.LabelZoneFailureDomain, LimitedValues: []string{"z2", "z3"}},
			},
			expectedPodsSorted: []types.UID{
				"e-0",
				"e-1",
				"e-2",
				"f-0", "i-0",
				"e-3", "i-1",
				"f-1", "i-2",
				"e-4", "i-3",
				"f-2", "i-4",
			},
		},
		{
			name: "zone{no limit}, host",
			constraints: []PodSpreadConstraint{
				{TopologyKey: v1.LabelZoneFailureDomain},
				{TopologyKey: v1.LabelHostname},
			},
			expectedPodsSorted: []types.UID{
				"d-0", "e-0",
				"d-1", "e-1",
				"c-0", "i-0",
				"d-2", "e-2",
				"i-1", "f-0",
				"b-0", "c-1",
				"i-2", "d-3",
				"e-3", "f-1",
				"i-3", "b-1",
				"c-2", "e-4",
				"d-4", "f-2", "i-4",
			},
		},
	}

	for _, tc := range cases {
		ranker := NewSpreadConstraintsRanker(pods, tc.constraints, fakeClient)

		var gotPodsSorted []types.UID
		for _, pod := range pods {
			if ranker.GetRank(pod) > 0 {
				gotPodsSorted = append(gotPodsSorted, pod.UID)
			}
		}
		sort.SliceStable(gotPodsSorted, func(i, j int) bool {
			return ranker.GetRank(&v1.Pod{ObjectMeta: metav1.ObjectMeta{UID: gotPodsSorted[i]}}) > ranker.GetRank(&v1.Pod{ObjectMeta: metav1.ObjectMeta{UID: gotPodsSorted[j]}})
		})

		if !reflect.DeepEqual(gotPodsSorted, tc.expectedPodsSorted) {
			t.Fatalf("%v\n  got pods sorted: %v\n  expected sorted: %v", tc.name, gotPodsSorted, tc.expectedPodsSorted)
		}
	}
}
