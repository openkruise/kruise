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

package adapter

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestGetCurrentPartitionForStrategyOnDelete(t *testing.T) {
	currentPods := buildPodList([]int{0, 1, 2}, []string{"v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 1 {
		t.Fatalf("expected partition 1, got %d", *partition)
	}

	currentPods = buildPodList([]int{0, 1, 2}, []string{"v1", "v1", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 2 {
		t.Fatalf("expected partition 2, got %d", *partition)
	}

	currentPods = buildPodList([]int{0, 1, 2, 3}, []string{"v2", "v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 1 {
		t.Fatalf("expected partition 1, got %d", *partition)
	}

	currentPods = buildPodList([]int{1, 2, 3}, []string{"v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 1 {
		t.Fatalf("expected partition 1, got %d", *partition)
	}

	currentPods = buildPodList([]int{0, 1, 3}, []string{"v2", "v1", "v2"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 1 {
		t.Fatalf("expected partition 1, got %d", *partition)
	}

	currentPods = buildPodList([]int{0, 1, 2}, []string{"v1", "v1", "v1"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 3 {
		t.Fatalf("expected partition 3, got %d", *partition)
	}

	currentPods = buildPodList([]int{0, 1, 2, 4}, []string{"v1", "", "v2", "v3"}, t)
	if partition := getCurrentPartition(currentPods, "v2"); *partition != 3 {
		t.Fatalf("expected partition 3, got %d", *partition)
	}
}

func buildPodList(ordinals []int, revisions []string, t *testing.T) []*corev1.Pod {
	if len(ordinals) != len(revisions) {
		t.Fatalf("ordinals count should equals to revision count")
	}
	pods := []*corev1.Pod{}
	for i, ordinal := range ordinals {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      fmt.Sprintf("pod-%d", ordinal),
			},
		}
		if revisions[i] != "" {
			pod.Labels = map[string]string{
				appsv1alpha1.ControllerRevisionHashLabelKey: revisions[i],
			}
		}
		pods = append(pods, pod)
	}

	return pods
}

// mockPodList 创建一个模拟的Pod列表
func mockPodList(updatedRevision string, replicas int32, readyReplicas int32) []*corev1.Pod {
	pods := make([]*corev1.Pod, replicas)
	for i := int32(0); i < replicas; i++ {
		var status corev1.ConditionStatus
		if i < readyReplicas {
			status = corev1.ConditionTrue
		} else {
			status = corev1.ConditionFalse
		}
		pods[i] = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					appsv1alpha1.ControllerRevisionHashLabelKey: updatedRevision,
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: status,
					},
				},
			},
		}
	}
	return pods
}

func TestCalculateUpdatedReplicas(t *testing.T) {
	updatedRevision := "updated-revision"
	replicas := int32(5)
	readyReplicas := int32(3)

	podList := mockPodList(updatedRevision, replicas, readyReplicas)
	podList = append(podList, mockPodList("old-revision", 2, 3)...)

	updated, updatedReady := CalculateUpdatedReplicas(podList, updatedRevision)
	if updated != replicas {
		t.Errorf("Expected %d updated replicas, got %d", replicas, updated)
	}
	if updatedReady != readyReplicas {
		t.Errorf("Expected %d updated ready replicas, got %d", readyReplicas, updatedReady)
	}
}
