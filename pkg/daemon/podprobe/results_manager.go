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
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

// Update is an enum of the types of updates sent over the Updates channel.
type Update struct {
	ContainerID string
	Key         probeKey
	State       appsv1alpha1.ProbeState
	Msg         string
}

// resultManager implementation, store container probe result
type resultManager struct {
	// guards the cache
	sync.RWMutex
	// map of container ID -> probe Result
	cache map[string]Update
	queue workqueue.RateLimitingInterface
}

// NewResultManager creates and returns an empty results resultManager.
func NewResultManager(queue workqueue.RateLimitingInterface) *resultManager {
	return &resultManager{
		cache: make(map[string]Update),
		queue: queue,
	}
}

func (m *resultManager) ListResults() []Update {
	m.RLock()
	defer m.RUnlock()

	var results []Update
	for _, result := range m.cache {
		results = append(results, result)
	}
	return results
}

func (m *resultManager) Set(id string, key probeKey, result appsv1alpha1.ProbeState, msg string) {
	m.Lock()
	defer m.Unlock()
	prev, exists := m.cache[id]
	if !exists || prev.State != result {
		klog.V(5).Infof("Pod(%s) do container(%s) probe(%s) result(%s)", key.podUID, key.containerName, key.probeName, result)
		m.cache[id] = Update{id, key, result, msg}
		m.queue.Add("update")
	}
}

func (m *resultManager) Remove(id string) {
	m.Lock()
	defer m.Unlock()
	delete(m.cache, id)
}
