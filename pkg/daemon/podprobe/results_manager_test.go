/*
Copyright 2025 The Kruise Authors.

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
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

func TestResultManagerSet(t *testing.T) {
	cases := []struct {
		name      string
		getPrev   func() *Update
		getCur    func() Update
		expectLen int
		result    func() []Update
	}{
		{
			name: "first probe",
			getPrev: func() *Update {
				return nil
			},
			getCur: func() Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Now(),
				}
				return obj
			},
			expectLen: 1,
			result: func() []Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Now(),
				}
				return []Update{obj}
			},
		},
		{
			name: "second probe, no changed",
			getPrev: func() *Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Time{Time: metav1.Now().Add(-time.Second * 10)},
				}
				return &obj
			},
			getCur: func() Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Now(),
				}
				return obj
			},
			expectLen: 0,
			result: func() []Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Now(),
				}
				return []Update{obj}
			},
		},
		{
			name: "second probe, state changed",
			getPrev: func() *Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Time{Time: metav1.Now().Add(-time.Second * 10)},
				}
				return &obj
			},
			getCur: func() Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeFailed,
					Msg:           "failed, version1",
					LastProbeTime: metav1.Now(),
				}
				return obj
			},
			expectLen: 1,
			result: func() []Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeFailed,
					Msg:           "failed, version1",
					LastProbeTime: metav1.Now(),
				}
				return []Update{obj}
			},
		},
		{
			name: "second probe, msg changed",
			getPrev: func() *Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Time{Time: metav1.Now().Add(-time.Second * 10)},
				}
				return &obj
			},
			getCur: func() Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version2",
					LastProbeTime: metav1.Now(),
				}
				return obj
			},
			expectLen: 1,
			result: func() []Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version2",
					LastProbeTime: metav1.Now(),
				}
				return []Update{obj}
			},
		},
		{
			name: "100 probe, no changed, but over ten minutes",
			getPrev: func() *Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Time{Time: metav1.Now().Add(-time.Second * 610)},
				}
				return &obj
			},
			getCur: func() Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Now(),
				}
				return obj
			},
			expectLen: 1,
			result: func() []Update {
				obj := Update{
					ContainerID: "cc4c81e9128bf1489fa0975f8cd2ca949423acd840a909c31dfa753fa6cc61f6",
					Key: probeKey{
						podNs:         "default",
						podName:       "test-pod",
						podUID:        "fe2ad1bb-f32a-407d-9f43-884b5dff267e",
						containerName: "main",
						probeName:     "healthy",
					},
					State:         appsalphav1.ProbeSucceeded,
					Msg:           "success, version1",
					LastProbeTime: metav1.Now(),
				}
				return []Update{obj}
			},
		},
	}

	randInt, _ := rand.Int(rand.Reader, big.NewInt(5000))
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			updateQueue := workqueue.NewNamedRateLimitingQueue(
				// Backoff duration from 500ms to 50~55s
				workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 50*time.Second+time.Millisecond*time.Duration(randInt.Int64())),
				"update_node_pod_probe_status",
			)
			r := newResultManager(updateQueue)
			prev := cs.getPrev()
			if prev != nil {
				r.cache.Store(prev.ContainerID, Update{prev.ContainerID, prev.Key, prev.State, prev.Msg, prev.LastProbeTime})
			}
			cur := cs.getCur()
			r.set(cur.ContainerID, cur.Key, cur.State, cur.Msg)
			if r.queue.Len() != cs.expectLen {
				t.Fatalf("expect(%v), but get(%v)", cs.expectLen, r.queue.Len())
			}
			objs := r.listResults()
			for _, result := range cs.result() {
				found := false
				for _, obj := range objs {
					if result.Key == obj.Key {
						found = true
						if result.Msg != obj.Msg || result.State != obj.State {
							t.Fatalf("expect(%v), but get(%v)", util.DumpJSON(result), util.DumpJSON(obj))
						}
					}
				}
				if !found {
					t.Fatalf("Not found  result %s", result.Key)
				}
			}
		})
	}
}
