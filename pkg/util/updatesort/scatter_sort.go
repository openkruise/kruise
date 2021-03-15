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
	"math"
	"sort"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

type scatterSort struct {
	strategy appsv1alpha1.UpdateScatterStrategy
}

func NewScatterSorter(s appsv1alpha1.UpdateScatterStrategy) Sorter {
	return &scatterSort{strategy: s}
}

// Sort helps scatter the indexes of pods by ScatterStrategy.
func (ss *scatterSort) Sort(pods []*v1.Pod, indexes []int) []int {
	if len(ss.strategy) == 0 || len(indexes) <= 1 {
		return indexes
	}

	terms := ss.getScatterTerms(pods, indexes)
	for _, term := range terms {
		indexes = ss.scatterPodsByRule(term, pods, indexes)
	}
	return indexes
}

// getScatterTerms returns all scatter terms in current sorting. It will sort all terms by sum of pods matched.
func (ss *scatterSort) getScatterTerms(pods []*v1.Pod, indexes []int) []appsv1alpha1.UpdateScatterTerm {
	if len(ss.strategy) == 1 {
		return ss.strategy
	}

	var termSlice []appsv1alpha1.UpdateScatterTerm
	ruleCounter := map[string]int{}

	termID := func(term appsv1alpha1.UpdateScatterTerm) string {
		return term.Key + ":" + term.Value
	}

	for _, term := range ss.strategy {
		for _, idx := range indexes {
			if val, ok := pods[idx].Labels[term.Key]; ok && val == term.Value {
				newTerm := appsv1alpha1.UpdateScatterTerm{Key: term.Key, Value: val}
				id := termID(newTerm)
				if count, ok := ruleCounter[id]; !ok {
					termSlice = append(termSlice, newTerm)
					ruleCounter[id] = 1
				} else {
					ruleCounter[id] = count + 1
				}
			}
		}
	}

	sort.SliceStable(termSlice, func(i, j int) bool {
		cI := ruleCounter[termID(termSlice[i])]
		cJ := ruleCounter[termID(termSlice[j])]
		if cI != cJ {
			return cI > cJ
		}
		return i <= j
	})

	return termSlice
}

// scatterPodsByRule scatters pods by given rule term.
func (ss *scatterSort) scatterPodsByRule(term appsv1alpha1.UpdateScatterTerm, pods []*v1.Pod, indexes []int) (ret []int) {

	// 1. counts the total number of matched and unmatched pods; find matched and unmatched pods in indexes waiting to update
	var matchedIndexes, unmatchedIndexes []int
	var totalMatched, totalUnmatched int

	for _, i := range indexes {
		if pods[i].Labels[term.Key] == term.Value {
			matchedIndexes = append(matchedIndexes, i)
		} else {
			unmatchedIndexes = append(unmatchedIndexes, i)
		}
	}

	if len(matchedIndexes) <= 1 || len(unmatchedIndexes) <= 1 {
		return indexes
	}

	for i := 0; i < len(pods); i++ {
		if pods[i] == nil {
			continue
		}
		if pods[i].Labels[term.Key] == term.Value {
			totalMatched++
		} else {
			totalUnmatched++
		}
	}

	// 2. keep the last matched one and append to the indexes returned
	lastMatchedIndex := matchedIndexes[len(matchedIndexes)-1]
	defer func() {
		ret = append(ret, lastMatchedIndex)
	}()
	matchedIndexes = matchedIndexes[:len(matchedIndexes)-1]
	totalMatched--

	// 3. calculate group number and size that pods to update should be combined
	group := calculateGroupByDensity(totalMatched, totalUnmatched, len(matchedIndexes), len(unmatchedIndexes))
	newIndexes := make([]int, 0, len(indexes))

	if group.unmatchedRemainder > 0 {
		newIndexes = append(newIndexes, unmatchedIndexes[:group.unmatchedRemainder]...)
		unmatchedIndexes = unmatchedIndexes[group.unmatchedRemainder:]
	}

	for i := 0; i < group.groupNum; i++ {
		matchedIndexes, newIndexes = migrateItems(matchedIndexes, newIndexes, group.matchedGroupSize)
		unmatchedIndexes, newIndexes = migrateItems(unmatchedIndexes, newIndexes, group.unmatchedGroupSize)
	}

	if len(matchedIndexes) > 0 {
		newIndexes = append(newIndexes, matchedIndexes...)
	}
	if len(unmatchedIndexes) > 0 {
		newIndexes = append(newIndexes, unmatchedIndexes...)
	}

	return newIndexes
}

func migrateItems(src, dst []int, size int) ([]int, []int) {
	if len(src) == 0 {
		return src, dst
	} else if len(src) < size {
		return []int{}, append(dst, src...)
	}
	dst = append(dst, src[:size]...)
	return src[size:], dst
}

type scatterGroup struct {
	groupNum           int
	matchedGroupSize   int
	unmatchedGroupSize int
	unmatchedRemainder int
}

func newScatterGroup(matched, unmatched int) scatterGroup {
	sg := scatterGroup{}
	if matched < unmatched {
		sg.groupNum = matched
	} else {
		sg.groupNum = unmatched
	}

	sg.matchedGroupSize = int(math.Round(float64(matched) / float64(sg.groupNum)))
	sg.unmatchedGroupSize = int(math.Round(float64(unmatched) / float64(sg.groupNum)))
	return sg
}

func calculateGroupByDensity(totalMatched, totalUnmatched, updateMatched, updateUnmatched int) scatterGroup {
	totalGroup := newScatterGroup(totalMatched, totalUnmatched)
	updateGroup := newScatterGroup(updateMatched, updateUnmatched)

	if float32(totalUnmatched)/float32(totalMatched) >= float32(updateUnmatched)/float32(updateMatched) {
		return updateGroup
	}

	newGroup := scatterGroup{matchedGroupSize: totalGroup.matchedGroupSize, unmatchedGroupSize: totalGroup.unmatchedGroupSize}
	if updateMatched/newGroup.matchedGroupSize < updateUnmatched/newGroup.unmatchedGroupSize {
		newGroup.groupNum = updateMatched / newGroup.matchedGroupSize
	} else {
		newGroup.groupNum = updateUnmatched / newGroup.unmatchedGroupSize
	}
	newGroup.unmatchedRemainder = updateUnmatched - newGroup.groupNum*newGroup.unmatchedGroupSize
	return newGroup
}
