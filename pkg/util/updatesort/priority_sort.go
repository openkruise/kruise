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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
)

type prioritySort struct {
	strategy *appspub.UpdatePriorityStrategy
}

func NewPrioritySorter(s *appspub.UpdatePriorityStrategy) Sorter {
	return &prioritySort{strategy: s}
}

// Sort helps sort the indexes of pods by UpdatePriorityStrategy.
func (ps *prioritySort) Sort(pods []*v1.Pod, indexes []int) []int {
	if ps.strategy == nil || (len(ps.strategy.WeightPriority) == 0 && len(ps.strategy.OrderPriority) == 0) || len(indexes) < 2 {
		return indexes
	}

	// Keep priorities local to this sort so later calls observe changed pod labels and rules.
	priorities := make([]struct {
		index int
		level int64
		order int64
	}, len(indexes))
	for i, index := range indexes {
		priorities[i].index = index
	}

	if len(ps.strategy.WeightPriority) > 0 {
		for _, term := range ps.strategy.WeightPriority {
			selector, err := util.ValidatedLabelSelectorAsSelector(&term.MatchSelector)
			if err != nil {
				continue
			}
			for i := range priorities {
				if selector.Matches(labels.Set(pods[priorities[i].index].Labels)) {
					priorities[i].order += int64(term.Weight)
				}
			}
		}
	} else {
		for i := range priorities {
			priorities[i].level, priorities[i].order = ps.getPodOrderPriority(pods[priorities[i].index].Labels)
		}
	}

	sort.SliceStable(priorities, func(i, j int) bool {
		if priorities[i].level != priorities[j].level {
			return priorities[i].level < priorities[j].level
		}
		return priorities[i].order > priorities[j].order
	})
	for i := range priorities {
		indexes[i] = priorities[i].index
	}
	return indexes
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
