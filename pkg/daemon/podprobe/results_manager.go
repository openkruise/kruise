/*
Copyright 2022 The Kruise Authors.

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

package podprobe

import (
	"sync"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
)

const maxSyncProbeTime = 600

// Update is an enum of the types of updates sent over the Updates channel.
type Update struct {
	ContainerID   string
	Key           probeKey
	State         appsv1alpha1.ProbeState
	Msg           string
	LastProbeTime metav1.Time
}

// resultManager implementation, store container probe result
type resultManager struct {
	// map of container ID -> probe Result
	cache *sync.Map
	queue workqueue.RateLimitingInterface
}

// newResultManager creates and returns an empty results resultManager.
func newResultManager(queue workqueue.RateLimitingInterface) *resultManager {
	return &resultManager{
		cache: &sync.Map{},
		queue: queue,
	}
}

func (m *resultManager) listResults() []Update {
	var results []Update
	listFunc := func(key, value any) bool {
		results = append(results, value.(Update))
		return true
	}
	m.cache.Range(listFunc)
	return results
}

func (m *resultManager) set(id string, key probeKey, result appsv1alpha1.ProbeState, msg string) {
	currentTime := metav1.Now()
	prev, exists := m.cache.Load(id)
	if !exists || prev.(Update).State != result || currentTime.Sub(prev.(Update).LastProbeTime.Time) >= maxSyncProbeTime {
		m.cache.Store(id, Update{id, key, result, msg, currentTime})
		m.queue.Add("updateStatus")
	}
}

func (m *resultManager) remove(id string) {
	m.cache.Delete(id)
}
