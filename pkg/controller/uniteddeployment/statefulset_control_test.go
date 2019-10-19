package uniteddeployment

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

func TestGetCurrentPartition(t *testing.T) {
	currentPods := buildPodList([]int{0, 1, 2}, []string{"v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(3, currentPods, "v2"); partition != 1 {
		t.Fatalf("expected partition 1, got %d", partition)
	}

	currentPods = buildPodList([]int{0, 1, 2}, []string{"v1", "v1", "v2"}, t)
	if partition := getCurrentPartition(3, currentPods, "v2"); partition != 2 {
		t.Fatalf("expected partition 2, got %d", partition)
	}

	currentPods = buildPodList([]int{0, 1, 2, 3}, []string{"v2", "v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(4, currentPods, "v2"); partition != 2 {
		t.Fatalf("expected partition 2, got %d", partition)
	}

	currentPods = buildPodList([]int{1, 2, 3}, []string{"v1", "v2", "v2"}, t)
	if partition := getCurrentPartition(4, currentPods, "v2"); partition != 2 {
		t.Fatalf("expected partition 2, got %d", partition)
	}

	currentPods = buildPodList([]int{0, 1, 3}, []string{"v2", "v1", "v2"}, t)
	if partition := getCurrentPartition(4, currentPods, "v2"); partition != 3 {
		t.Fatalf("expected partition 3, got %d", partition)
	}

	currentPods = buildPodList([]int{0, 1, 2}, []string{"v1", "v1", "v1"}, t)
	if partition := getCurrentPartition(4, currentPods, "v2"); partition != 4 {
		t.Fatalf("expected partition 4, got %d", partition)
	}
}

func buildPodList(ordinals []int, revisions []string, t *testing.T) []*corev1.Pod {
	if len(ordinals) != len(revisions) {
		t.Fatalf("ordinals count should equals to revision count")
	}
	pods := []*corev1.Pod{}
	for i, ordinal := range ordinals {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      fmt.Sprintf("pod-%d", ordinal),
				Labels: map[string]string{
					appsv1alpha1.ControllerRevisionHashLabelKey: revisions[i],
				},
			},
		})
	}

	return pods
}
