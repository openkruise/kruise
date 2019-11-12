package scale

import (
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetOrGenAvailableIDs(t *testing.T) {
	pods := []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "a"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "b"},
			},
		},
	}

	pvcs := []*v1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "b"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{appsv1alpha1.CloneSetInstanceID: "c"},
			},
		},
	}

	gotIDs := getOrGenAvailableIDs(2, pods, pvcs)
	if gotIDs.Len() != 2 {
		t.Fatalf("expected got 2")
	}

	if !gotIDs.Has("c") {
		t.Fatalf("expected got c")
	}

	gotIDs.Delete("c")
	if id, _ := gotIDs.PopAny(); len(id) != 5 {
		t.Fatalf("expected got random id, but actually %v", id)
	}
}
