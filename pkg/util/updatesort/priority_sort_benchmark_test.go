/*
Copyright 2026 The Kruise Authors.

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
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
)

func BenchmarkPrioritySort(b *testing.B) {
	for _, size := range []int{10, 1000, 10000} {
		pods := make([]*v1.Pod, size)
		for i := range pods {
			pods[i] = &v1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"group": strconv.Itoa(i % 10),
				"rank":  "pod-" + strconv.Itoa(i),
			}}}
		}
		for _, mode := range []string{"None", "Weight1", "Weight10", "Weight10NoMatch", "Weight10Sorted", "Order"} {
			b.Run(fmt.Sprintf("%s/Pods%d", mode, size), func(b *testing.B) {
				strategy := &appspub.UpdatePriorityStrategy{}
				switch mode {
				case "Order":
					strategy.OrderPriority = []appspub.UpdatePriorityOrderTerm{{OrderedKey: "rank"}}
				case "Weight1", "Weight10", "Weight10NoMatch", "Weight10Sorted":
					terms := 10
					if mode == "Weight1" {
						terms = 1
					}
					for i := 0; i < terms; i++ {
						value := strconv.Itoa(i)
						if mode == "Weight10NoMatch" {
							value = "missing-" + value
						}
						strategy.WeightPriority = append(strategy.WeightPriority, appspub.UpdatePriorityWeightTerm{
							Weight:        int32(i + 1),
							MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"group": value}},
						})
					}
				}
				// Reset to the same permutation each time instead of repeatedly sorting sorted input.
				input := rand.New(rand.NewSource(1)).Perm(size)
				if mode == "Weight10Sorted" {
					NewPrioritySorter(strategy).Sort(pods, input)
				}
				indexes := make([]int, size)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					copy(indexes, input)
					NewPrioritySorter(strategy).Sort(pods, indexes)
				}
			})
		}
	}
}
