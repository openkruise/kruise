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
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Ranker interface {
	GetRank(*v1.Pod) float64
}

const (
	// PodDeletionCost can be used to set to an int32 that represent the cost of deleting
	// a pod compared to other pods belonging to the same ReplicaSet. Pods with lower
	// deletion cost are preferred to be deleted before pods with higher deletion cost.
	PodDeletionCost = "controller.kubernetes.io/pod-deletion-cost"
)

// ActivePodsWithRanks type allows custom sorting of pods so a controller can pick the best ones to delete.
type ActivePodsWithRanks struct {
	Pods          []*v1.Pod
	Ranker        Ranker
	AvailableFunc func(*v1.Pod) bool
}

func (s ActivePodsWithRanks) Len() int      { return len(s.Pods) }
func (s ActivePodsWithRanks) Swap(i, j int) { s.Pods[i], s.Pods[j] = s.Pods[j], s.Pods[i] }

func (s ActivePodsWithRanks) Less(i, j int) bool {
	// 1. Unassigned < assigned
	// If only one of the pods is unassigned, the unassigned one is smaller
	if s.Pods[i].Spec.NodeName != s.Pods[j].Spec.NodeName && (len(s.Pods[i].Spec.NodeName) == 0 || len(s.Pods[j].Spec.NodeName) == 0) {
		return len(s.Pods[i].Spec.NodeName) == 0
	}
	// 2. PodPending < PodUnknown < PodRunning
	podPhaseToOrdinal := map[v1.PodPhase]int{v1.PodPending: 0, v1.PodUnknown: 1, v1.PodRunning: 2}
	if podPhaseToOrdinal[s.Pods[i].Status.Phase] != podPhaseToOrdinal[s.Pods[j].Status.Phase] {
		return podPhaseToOrdinal[s.Pods[i].Status.Phase] < podPhaseToOrdinal[s.Pods[j].Status.Phase]
	}
	// 3. Not available < available; Not ready < ready
	// If only one of the pods is not ready, the not ready one is smaller
	if s.AvailableFunc != nil {
		if s.AvailableFunc(s.Pods[i]) != s.AvailableFunc(s.Pods[j]) {
			return !s.AvailableFunc(s.Pods[i])
		}
	}
	if podutil.IsPodReady(s.Pods[i]) != podutil.IsPodReady(s.Pods[j]) {
		return !podutil.IsPodReady(s.Pods[i])
	}

	// 4. Lower pod-deletion cost < higher pod-deletion-cost
	pi, _ := getDeletionCostFromPodAnnotations(s.Pods[i].Annotations)
	pj, _ := getDeletionCostFromPodAnnotations(s.Pods[j].Annotations)
	if pi != pj {
		return pi < pj
	}

	// 5. Higher ranks < lower ranks
	var rankI, rankJ float64
	if s.Ranker != nil {
		rankI = s.Ranker.GetRank(s.Pods[i])
		rankJ = s.Ranker.GetRank(s.Pods[j])
	}
	if rankI != rankJ {
		return rankI > rankJ
	}

	// TODO: take availability into account when we push minReadySeconds information from deployment into pods,
	//       see https://github.com/kubernetes/kubernetes/issues/22065
	// 6. Been ready for empty time < less time < more time
	// If both pods are ready, the latest ready one is smaller
	if podutil.IsPodReady(s.Pods[i]) && podutil.IsPodReady(s.Pods[j]) {
		readyTime1 := podReadyTime(s.Pods[i])
		readyTime2 := podReadyTime(s.Pods[j])
		if !readyTime1.Equal(readyTime2) {
			return afterOrZero(readyTime1, readyTime2)
		}
	}
	// 7. Pods with containers with higher restart counts < lower restart counts
	if maxContainerRestarts(s.Pods[i]) != maxContainerRestarts(s.Pods[j]) {
		return maxContainerRestarts(s.Pods[i]) > maxContainerRestarts(s.Pods[j])
	}
	// 8. Empty creation time pods < newer pods < older pods
	if !s.Pods[i].CreationTimestamp.Equal(&s.Pods[j].CreationTimestamp) {
		return afterOrZero(&s.Pods[i].CreationTimestamp, &s.Pods[j].CreationTimestamp)
	}
	return false
}

// afterOrZero checks if time t1 is after time t2; if one of them
// is zero, the zero time is seen as after non-zero time.
func afterOrZero(t1, t2 *metav1.Time) bool {
	if t1.Time.IsZero() || t2.Time.IsZero() {
		return t1.Time.IsZero()
	}
	return t1.After(t2.Time)
}

func podReadyTime(pod *v1.Pod) *metav1.Time {
	if podutil.IsPodReady(pod) {
		for _, c := range pod.Status.Conditions {
			// we only care about pod ready conditions
			if c.Type == v1.PodReady && c.Status == v1.ConditionTrue {
				return &c.LastTransitionTime
			}
		}
	}
	return &metav1.Time{}
}

func maxContainerRestarts(pod *v1.Pod) int {
	maxRestarts := 0
	for _, c := range pod.Status.ContainerStatuses {
		maxRestarts = integer.IntMax(maxRestarts, int(c.RestartCount))
	}
	return maxRestarts
}

// getDeletionCostFromPodAnnotations returns the integer value of pod-deletion-cost. Returns 0
// if not set or the value is invalid.
func getDeletionCostFromPodAnnotations(annotations map[string]string) (int32, error) {
	if value, exist := annotations[PodDeletionCost]; exist {
		// values that start with plus sign (e.g, "+10") or leading zeros (e.g., "008") are not valid.
		if !validFirstDigit(value) {
			return 0, fmt.Errorf("invalid value %q", value)
		}

		i, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			// make sure we default to 0 on error.
			return 0, err
		}
		return int32(i), nil
	}
	return 0, nil
}

func validFirstDigit(str string) bool {
	if len(str) == 0 {
		return false
	}
	return str[0] == '-' || (str[0] == '0' && str == "0") || (str[0] >= '1' && str[0] <= '9')
}

type sameNodeRanker struct {
	podRanks map[types.UID]float64
}

func NewSameNodeRanker(pods []*v1.Pod) Ranker {
	r := &sameNodeRanker{podRanks: make(map[types.UID]float64)}
	podsOnNode := make(map[string][]*v1.Pod)
	for _, pod := range pods {
		if pod.Spec.NodeName != "" {
			podsOnNode[pod.Spec.NodeName] = append(podsOnNode[pod.Spec.NodeName], pod)
		}
	}
	for _, podsOnSameNode := range podsOnNode {
		if len(podsOnSameNode) <= 1 {
			continue
		}
		// sort pods in same node by active
		sort.Sort(ActivePodsWithRanks{Pods: podsOnSameNode})
		podsOnSameNode = podsOnSameNode[:len(podsOnSameNode)-1]
		for i, pod := range podsOnSameNode {
			r.podRanks[pod.UID] = float64(len(podsOnSameNode) - i)
		}
	}
	return r
}

func (r *sameNodeRanker) GetRank(pod *v1.Pod) float64 {
	return r.podRanks[pod.UID]
}

type PodSpreadConstraint struct {
	TopologyKey   string   `json:"topologyKey"`
	LimitedValues []string `json:"limitedValues,omitempty"`
}

type spreadConstraintsRanker struct {
	podRanks map[types.UID]float64
}

func NewSpreadConstraintsRanker(pods []*v1.Pod, constraints []PodSpreadConstraint, reader client.Reader) Ranker {
	r := &spreadConstraintsRanker{podRanks: make(map[types.UID]float64)}
	var nodes []*v1.Node
	gotNodes := sets.NewString()
	podsOnNode := make(map[string][]*v1.Pod)
	for _, pod := range pods {
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			continue
		}
		if !gotNodes.Has(nodeName) {
			node := &v1.Node{}
			if err := reader.Get(context.TODO(), types.NamespacedName{Name: nodeName}, node); err != nil {
				continue
			}
			nodes = append(nodes, node)
			gotNodes.Insert(nodeName)
		}
		podsOnNode[nodeName] = append(podsOnNode[nodeName], pod)
	}

	smRanker := NewSameNodeRanker(pods)
	for _, constraint := range constraints {
		var limitedValues sets.String
		if len(constraint.LimitedValues) > 0 {
			limitedValues = sets.NewString(constraint.LimitedValues...)
		}

		// split pods into topologies
		podsInTopologies := make(map[string][]*v1.Pod)
		for _, node := range nodes {
			topologyValue, exists := node.Labels[constraint.TopologyKey]
			if !exists {
				continue
			}
			if limitedValues != nil && !limitedValues.Has(topologyValue) {
				continue
			}
			podsInTopologies[topologyValue] = append(podsInTopologies[topologyValue], podsOnNode[node.Name]...)
		}

		var useRanker Ranker
		if constraint.TopologyKey != v1.LabelHostname {
			useRanker = smRanker
		}
		topologyWeight := topologyNormalizingWeight(len(podsInTopologies))
		for _, podsInTopology := range podsInTopologies {
			sort.Sort(ActivePodsWithRanks{Pods: podsInTopology, Ranker: useRanker})
			for i, pod := range podsInTopology {
				rank := float64(len(podsInTopology) - i)
				r.podRanks[pod.UID] += rank * topologyWeight
			}
		}
	}
	return r
}

func (r *spreadConstraintsRanker) GetRank(pod *v1.Pod) float64 {
	return r.podRanks[pod.UID]
}

// topologyNormalizingWeight calculates the weight for the topology, based on
// the number of values that exist for a topology.
// Since <size> is at least 1 (all nodes that passed the Filters are in the
// same topology), and k8s supports 5k nodes, the result is in the interval
// <1.09, 8.52>.
//
// Note: <size> could also be zero when no nodes have the required topologies,
// however we don't care about topology weight in this case as we return a 0
// score for all nodes.
func topologyNormalizingWeight(size int) float64 {
	return math.Log(float64(size + 2))
}
