package sidecarcontrol

import (
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSidecarSetHash(t *testing.T) {
	sidecarSet := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecar-set",
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "container1",
						Image: "test-image",
					},
				},
			},
		},
	}

	hash, err := SidecarSetHash(sidecarSet)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if hash == "" {
		t.Fatalf("Expected non-empty hash")
	}

	// Change sidecar set and expect different hash
	sidecarSet.Spec.Containers[0].Image = "new-image"
	newHash, err := SidecarSetHash(sidecarSet)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newHash == hash {
		t.Fatalf("Expected different hashes for different SidecarSets")
	}
}

func TestSidecarSetHashWithoutImage(t *testing.T) {
	sidecarSet := &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecar-set",
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "container1",
						Image: "test-image",
					},
				},
			},
		},
	}

	hash, err := SidecarSetHashWithoutImage(sidecarSet)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if hash == "" {
		t.Fatalf("Expected non-empty hash")
	}

	// Change sidecar set image and expect same hash
	sidecarSet.Spec.Containers[0].Image = "new-image"
	newHash, err := SidecarSetHashWithoutImage(sidecarSet)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newHash != hash {
		t.Fatalf("Expected same hashes for SidecarSets with different images")
	}
}
