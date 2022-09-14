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

package updatesort

import (
	"sort"
	"strconv"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type prioritySort struct {
	strategy *appspub.UpdatePriorityStrategy
}

func NewPrioritySorter(s *appspub.UpdatePriorityStrategy) Sorter {
	return &prioritySort{strategy: s}
}

// Sort helps sort the indexes of pods by UpdatePriorityStrategy.
func (ps *prioritySort) Sort(pods []*v1.Pod, indexes []int) []int {
	if ps.strategy == nil || (len(ps.strategy.WeightPriority) == 0 && len(ps.strategy.OrderPriority) == 0) {
		return indexes
	}

	f := func(i, j int) bool {
		podI := pods[indexes[i]]
		podJ := pods[indexes[j]]
		return ps.compare(podI.Labels, podJ.Labels, indexes[i] > indexes[j])
	}

	sort.SliceStable(indexes, f)
	return indexes
}

func (ps *prioritySort) compare(podI, podJ map[string]string, defaultVal bool) bool {
	if len(ps.strategy.WeightPriority) > 0 {
		if wI, wJ := ps.getPodWeightPriority(podI), ps.getPodWeightPriority(podJ); wI != wJ {
			return wI > wJ
		}
	} else if len(ps.strategy.OrderPriority) > 0 {
		levelI, orderI := ps.getPodOrderPriority(podI)
		levelJ, orderJ := ps.getPodOrderPriority(podJ)
		if levelI != levelJ {
			return levelI < levelJ
		} else if orderI != orderJ {
			return orderI > orderJ
		}
	}
	return defaultVal
}

func (ps *prioritySort) getPodWeightPriority(podLabels map[string]string) int64 {
	var weight int64
	for _, p := range ps.strategy.WeightPriority {
		selector, err := util.ValidatedLabelSelectorAsSelector(&p.MatchSelector)
		if err != nil {
			continue
		}
		if selector.Matches(labels.Set(podLabels)) {
			weight += int64(p.Weight)
		}
	}
	return weight
}

func (ps *prioritySort) getPodOrderPriority(podLabels map[string]string) (int64, int64) {
	for i, p := range ps.strategy.OrderPriority {
		if value, ok := podLabels[p.OrderedKey]; ok {
			return int64(i), getIntFromStringSuffix(value)
		}
	}
	return -1, 0
}

func getIntFromStringSuffix(v string) int64 {
	startIdx := -1
	for i := len(v) - 1; i >= 0; i-- {
		if v[i] >= '0' && v[i] <= '9' {
			startIdx = i
		} else {
			break
		}
	}
	if startIdx < 0 {
		return 0
	}
	if order, err := strconv.ParseInt(v[startIdx:], 10, 64); err == nil {
		return order
	}
	return 0
}
