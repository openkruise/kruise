package configmapset

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func init() {
	_ = appsv1alpha1.AddToScheme(scheme.Scheme)
}

func TestCalculateHash(t *testing.T) {
	data1 := map[string]string{"key1": "val1", "key2": "val2"}
	data2 := map[string]string{"key2": "val2", "key1": "val1"} // Same data, different order

	hash1, err := CalculateHash(data1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := CalculateHash(data2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("expected hashes to be identical for same data, got %s and %s", hash1, hash2)
	}

	data3 := map[string]string{"key1": "val3"}
	hash3, err := CalculateHash(data3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash3 {
		t.Errorf("expected hashes to be different for different data, got %s", hash1)
	}
}

func TestGetDistributionByPartition(t *testing.T) {
	cases := []struct {
		name                string
		replicas            int
		partition           *intstr.IntOrString
		expectedNewReplicas int32
		expectedOldReplicas int32
	}{
		{
			name:                "nil partition",
			replicas:            10,
			partition:           nil,
			expectedNewReplicas: 10,
			expectedOldReplicas: 0,
		},
		{
			name:                "int partition 0",
			replicas:            10,
			partition:           &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
			expectedNewReplicas: 10,
			expectedOldReplicas: 0,
		},
		{
			name:                "int partition 3",
			replicas:            10,
			partition:           &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
			expectedNewReplicas: 7,
			expectedOldReplicas: 3,
		},
		{
			name:                "int partition greater than replicas",
			replicas:            10,
			partition:           &intstr.IntOrString{Type: intstr.Int, IntVal: 15},
			expectedNewReplicas: 0,
			expectedOldReplicas: 10,
		},
		{
			name:                "string percent partition 30%",
			replicas:            10,
			partition:           &intstr.IntOrString{Type: intstr.String, StrVal: "30%"},
			expectedNewReplicas: 7,
			expectedOldReplicas: 3,
		},
		{
			name:                "string percent partition 33% ceil",
			replicas:            10,
			partition:           &intstr.IntOrString{Type: intstr.String, StrVal: "33%"}, // 3.3 -> ceil(3.3) = 4 old replicas -> 6 new
			expectedNewReplicas: 6,
			expectedOldReplicas: 4,
		},
		{
			name:                "string int partition 3",
			replicas:            10,
			partition:           &intstr.IntOrString{Type: intstr.String, StrVal: "3"},
			expectedNewReplicas: 7,
			expectedOldReplicas: 3,
		},
		{
			name:                "string percent partition 100%",
			replicas:            10,
			partition:           &intstr.IntOrString{Type: intstr.String, StrVal: "100%"},
			expectedNewReplicas: 0,
			expectedOldReplicas: 10,
		},
		{
			name:                "string percent partition 0%",
			replicas:            10,
			partition:           &intstr.IntOrString{Type: intstr.String, StrVal: "0%"},
			expectedNewReplicas: 10,
			expectedOldReplicas: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cms := &appsv1alpha1.ConfigMapSet{
				Spec: appsv1alpha1.ConfigMapSetSpec{
					Data: map[string]string{"key": "value"},
					UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
						Partition: tc.partition,
					},
				},
				Status: appsv1alpha1.ConfigMapSetStatus{
					CurrentRevision: "old-rev",
				},
			}

			pods := make([]*corev1.Pod, tc.replicas)
			for i := 0; i < tc.replicas; i++ {
				pods[i] = &corev1.Pod{}
			}

			distributions, err := getDistributionByPartition(cms, pods)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(distributions) != 2 {
				t.Fatalf("expected 2 distributions, got %d", len(distributions))
			}

			if distributions[0].Reserved != tc.expectedNewReplicas {
				t.Errorf("expected new replicas %d, got %d", tc.expectedNewReplicas, distributions[0].Reserved)
			}

			if distributions[1].Reserved != tc.expectedOldReplicas {
				t.Errorf("expected old replicas %d, got %d", tc.expectedOldReplicas, distributions[1].Reserved)
			}
		})
	}
}

func TestGetUpdatePodsByDistributions(t *testing.T) {
	revisionKeys := &RevisionKeys{
		CurrentRevisionKey: "current-rev",
	}

	now := metav1.Now()
	later := metav1.Time{Time: now.Add(time.Hour)}

	// Revisions
	updateRev := "rev-update"
	currentRev := "rev-current"
	olderRev := "rev-older"
	oldestRev := "rev-oldest"
	unknownRev := "rev-unknown"

	hubRevisions := []RevisionEntry{
		{Hash: oldestRev},
		{Hash: olderRev},
		{Hash: currentRev},
		{Hash: updateRev},
	}

	// 1. Revisions in RMC: oldestRev, olderRev, currentRev, updateRev
	// 2. Revisions not in RMC: unknownRev

	// Construct Pods with different states
	createPod := func(name, rev string, creationTime metav1.Time, ready bool) *corev1.Pod {
		status := corev1.ConditionFalse
		if ready {
			status = corev1.ConditionTrue
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Annotations: map[string]string{
					GetConfigMapSetEnabledKey():     "true",
					revisionKeys.CurrentRevisionKey: rev,
				},
				CreationTimestamp: creationTime,
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: status},
				},
			},
		}
	}

	// pods list
	pCurrent := createPod("p-current", currentRev, now, true)
	pOldest := createPod("p-oldest", oldestRev, now, true)
	pOlder := createPod("p-older", olderRev, now, true)
	pUnknown := createPod("p-unknown", unknownRev, now, true)
	pUpdate := createPod("p-update", updateRev, now, true)

	pOlderLater := createPod("p-older-later", olderRev, later, true)
	pOlderAlpha1 := createPod("a-older", olderRev, now, true)
	pOlderAlpha2 := createPod("b-older", olderRev, now, true)

	pCurrentUnready := createPod("p-current-unready", currentRev, now, false)

	cases := []struct {
		name                 string
		pods                 []*corev1.Pod
		distributions        []Distribution
		maxUnavailable       *intstr.IntOrString
		expectedUpdateCount  int
		expectedFirstPodName string
	}{
		{
			name: "Priority Rule 1: Non-currentRevision > currentRevision",
			pods: []*corev1.Pod{pCurrent, pOlder},
			distributions: []Distribution{
				{Revision: updateRev, Reserved: 2}, // Want to update all
				{Revision: currentRev, Reserved: 0},
			},
			expectedUpdateCount:  2,
			expectedFirstPodName: "p-older", // pOlder is not current, pCurrent is
		},
		{
			name: "Priority Rule 2: Older Revision > Newer Revision",
			pods: []*corev1.Pod{pOlder, pOldest},
			distributions: []Distribution{
				{Revision: updateRev, Reserved: 1}, // Want to update 1
				{Revision: currentRev, Reserved: 1},
			},
			expectedUpdateCount:  1,
			expectedFirstPodName: "p-oldest", // pOldest is older than pOlder in RMC
		},
		{
			name: "Priority Rule 3: Unknown Revision > Known Revision",
			pods: []*corev1.Pod{pOldest, pUnknown},
			distributions: []Distribution{
				{Revision: updateRev, Reserved: 1}, // Want to update 1
				{Revision: currentRev, Reserved: 1},
			},
			expectedUpdateCount:  1,
			expectedFirstPodName: "p-unknown", // pUnknown is not in RMC, pOldest is
		},
		{
			name: "Priority Rule 4: Older CreationTimestamp > Newer CreationTimestamp",
			pods: []*corev1.Pod{pOlderLater, pOlder},
			distributions: []Distribution{
				{Revision: updateRev, Reserved: 1},
				{Revision: currentRev, Reserved: 1},
			},
			expectedUpdateCount:  1,
			expectedFirstPodName: "p-older", // pOlder created before pOlderLater
		},
		{
			name: "Priority Rule 5: Alphabetical Name",
			pods: []*corev1.Pod{pOlderAlpha2, pOlderAlpha1},
			distributions: []Distribution{
				{Revision: updateRev, Reserved: 1},
				{Revision: currentRev, Reserved: 1},
			},
			expectedUpdateCount:  1,
			expectedFirstPodName: "a-older", // a-older < b-older
		},
		{
			name: "MaxUnavailable limit reached",
			pods: []*corev1.Pod{
				createPod("p1", currentRev, now, false), // 1 unavailable
				createPod("p2", currentRev, now, true),
			},
			distributions: []Distribution{
				{Revision: updateRev, Reserved: 2},
				{Revision: currentRev, Reserved: 0},
			},
			maxUnavailable:      &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
			expectedUpdateCount: 0, // 1 unavailable >= maxUnavailable(1), cannot update more
		},
		{
			name: "Skip already updated pods",
			pods: []*corev1.Pod{pUpdate, pCurrent},
			distributions: []Distribution{
				{Revision: updateRev, Reserved: 2},
				{Revision: currentRev, Reserved: 0},
			},
			expectedUpdateCount:  1,
			expectedFirstPodName: "p-current", // pUpdate is already updated
		},
		{
			name: "MaxUnavailable limit reached exactly",
			pods: []*corev1.Pod{
				pCurrentUnready,
				pCurrent,
			},
			distributions: []Distribution{
				{Revision: updateRev, Reserved: 2},
				{Revision: currentRev, Reserved: 0},
			},
			maxUnavailable:      &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
			expectedUpdateCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cms := &appsv1alpha1.ConfigMapSet{
				Spec: appsv1alpha1.ConfigMapSetSpec{
					UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
						MaxUnavailable: tc.maxUnavailable,
					},
				},
			}

			podsToUpdate, _, _ := getUpdatePodsByDistributions(cms, tc.distributions, tc.pods, revisionKeys, hubRevisions)

			if len(podsToUpdate) != tc.expectedUpdateCount {
				t.Fatalf("expected %d pods to update, got %d", tc.expectedUpdateCount, len(podsToUpdate))
			}

			if tc.expectedUpdateCount > 0 && podsToUpdate[0].Name != tc.expectedFirstPodName {
				t.Errorf("expected first pod to be %s, got %s", tc.expectedFirstPodName, podsToUpdate[0].Name)
			}
		})
	}
}

func TestGetContainerName(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"container-name-from-annotation": "annot-container",
			},
			Labels: map[string]string{
				"container-name-from-label": "label-container",
			},
		},
	}

	cases := []struct {
		name          string
		containerSpec appsv1alpha1.ConfigMapSetContainer
		expectedName  string
	}{
		{
			name: "direct name",
			containerSpec: appsv1alpha1.ConfigMapSetContainer{
				Name: "direct-container",
			},
			expectedName: "direct-container",
		},
		{
			name: "name from annotation",
			containerSpec: appsv1alpha1.ConfigMapSetContainer{
				NameFrom: &appsv1alpha1.SourceContainerNameSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.annotations['container-name-from-annotation']",
					},
				},
			},
			expectedName: "annot-container",
		},
		{
			name: "name from label",
			containerSpec: appsv1alpha1.ConfigMapSetContainer{
				NameFrom: &appsv1alpha1.SourceContainerNameSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.labels['container-name-from-label']",
					},
				},
			},
			expectedName: "label-container",
		},
		{
			name: "name from missing label",
			containerSpec: appsv1alpha1.ConfigMapSetContainer{
				NameFrom: &appsv1alpha1.SourceContainerNameSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.labels['missing-label']",
					},
				},
			},
			expectedName: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := GetContainerName(pod, tc.containerSpec)
			if name != tc.expectedName {
				t.Errorf("expected container name %s, got %s", tc.expectedName, name)
			}
		})
	}
}
