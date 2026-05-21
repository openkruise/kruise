package configmapset

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func init() {
	_ = appsv1alpha1.AddToScheme(scheme.Scheme)
}

func TestGetMatchedPods(t *testing.T) {
	now := metav1.Now()

	cms := &appsv1alpha1.ConfigMapSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cms",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ConfigMapSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	podActive := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-active",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	podPending := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-pending",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	podInactive := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-inactive",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded, // Will be filtered out by IsPodActive
		},
	}

	podDeleting := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pod-deleting",
			Namespace:         "default",
			Labels:            map[string]string{"app": "test"},
			DeletionTimestamp: &now,
			Finalizers:        []string{"test-finalizer"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning, // Will be filtered out because of DeletionTimestamp != nil
		},
	}

	podNotMatch := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-not-match",
			Namespace: "default",
			Labels:    map[string]string{"app": "other"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(podActive, podPending, podInactive, podDeleting, podNotMatch).Build()

	matchedPods, err := GetMatchedPods(context.TODO(), fakeClient, cms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matchedPods) != 2 {
		t.Fatalf("expected 2 matched pod, got %d", len(matchedPods))
	}

	if matchedPods[0].Name != "pod-active" {
		t.Errorf("expected pod-active, got %s", matchedPods[0].Name)
	}
}

func TestGetMatchConfigMapSets(t *testing.T) {
	now := metav1.Now()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
	}

	cmsMatched := &appsv1alpha1.ConfigMapSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cms-matched",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ConfigMapSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	cmsMatched2 := &appsv1alpha1.ConfigMapSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cms-matched-2",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ConfigMapSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	cmsNotMatched := &appsv1alpha1.ConfigMapSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cms-not-matched",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ConfigMapSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "other"},
			},
		},
	}

	cmsDeleting := &appsv1alpha1.ConfigMapSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cms-deleting",
			Namespace:         "default",
			DeletionTimestamp: &now, // Should be ignored based on our new logic
			Finalizers:        []string{"test-finalizer"},
		},
		Spec: appsv1alpha1.ConfigMapSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(cmsMatched, cmsMatched2, cmsNotMatched, cmsDeleting).Build()

	res, err := GetMatchConfigMapSets(fakeClient, pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res) != 2 {
		t.Fatalf("expected 2 matched ConfigMapSet, got %d", len(res))
	}

	if res[0].Name != "cms-matched" {
		t.Errorf("expected cms-matched, got %s", res[0].Name)
	}
}

func TestIsPodReady(t *testing.T) {
	cases := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "pod ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			},
			expected: true,
		},
		{
			name: "pod not ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionFalse},
					},
				},
			},
			expected: false,
		},
		{
			name: "no ready condition",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsPodReady(tc.pod)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}
