package sidecarcontrol

import (
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var always = corev1.ContainerRestartPolicyAlways

func TestSidecarSetHash(t *testing.T) {
	cases := []struct {
		name          string
		getSidecarSet func() *appsv1alpha1.SidecarSet
		expectHash    string
	}{
		{
			name: "containers",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return &appsv1alpha1.SidecarSet{
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
			},
			expectHash: "w26c4x8fz245642fdv499b464248f974xddx4x55z5dw55bc6x66464fxz77dc78",
		},
		{
			name: "containers and initContainers",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return &appsv1alpha1.SidecarSet{
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
						InitContainers: []appsv1alpha1.SidecarContainer{
							{
								Container: corev1.Container{
									Name:  "container1",
									Image: "test-image",
								},
							},
						},
					},
				}
			},
			expectHash: "w26c4x8fz245642fdv499b464248f974xddx4x55z5dw55bc6x66464fxz77dc78",
		},
		{
			name: "containers and initContainers with restartPolicy=Always",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return &appsv1alpha1.SidecarSet{
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
						InitContainers: []appsv1alpha1.SidecarContainer{
							{
								Container: corev1.Container{
									Name:          "container1",
									Image:         "test-image",
									RestartPolicy: &always,
								},
							},
						},
					},
				}
			},
			expectHash: "4xwx4d4844vd4v9x79wb4xbxf4xb29475cc4446v8cz2c2f2f5c5bw448vd42z8w",
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			sidecarSet := cs.getSidecarSet()
			hash1, err := SidecarSetHash(sidecarSet)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			} else if hash1 == "" {
				t.Fatalf("Expected non-empty hash")
			}
			if cs.expectHash != hash1 {
				t.Fatalf("expect(%s), but get(%s)", cs.expectHash, hash1)
			}

			// Change sidecar set and expect different hash
			sidecarSet.Spec.Containers[0].Image = "new-image"
			newHash, err := SidecarSetHash(sidecarSet)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			} else if newHash == hash1 {
				t.Fatalf("Expected different hashes for different SidecarSets")
			}
		})
	}
}

func TestSidecarSetHashWithoutImage(t *testing.T) {
	cases := []struct {
		name          string
		getSidecarSet func() *appsv1alpha1.SidecarSet
		expectHash    string
	}{
		{
			name: "containers and initContainers",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return &appsv1alpha1.SidecarSet{
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
						InitContainers: []appsv1alpha1.SidecarContainer{
							{
								Container: corev1.Container{
									Name:  "container1",
									Image: "test-image",
								},
							},
						},
					},
				}
			},
			expectHash: "8wzddb4dvv9c6x8zdc77z4z75987424f457dfv6724ddw6zbdx467wz5x24fc759",
		},
		{
			name: "containers and initContainers with restartPolicy=Always",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return &appsv1alpha1.SidecarSet{
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
						InitContainers: []appsv1alpha1.SidecarContainer{
							{
								Container: corev1.Container{
									Name:          "container1",
									Image:         "test-image",
									RestartPolicy: &always,
								},
							},
						},
					},
				}
			},
			expectHash: "5725fw8bwbx249bw57v5892c847dzf48bww9zb7c86xb95264fdz26654847b2c8",
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			sidecarSet := cs.getSidecarSet()
			hash1, err := SidecarSetHashWithoutImage(sidecarSet)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			} else if hash1 == "" {
				t.Fatalf("Expected non-empty hash")
			}
			if cs.expectHash != hash1 {
				t.Fatalf("expect(%s), but get(%s)", cs.expectHash, hash1)
			}

			// Change sidecar set and expect different hash
			sidecarSet.Spec.Containers[0].Image = "new-image"
			sidecarSet.Spec.InitContainers[0].Image = "new-image"
			newHash, err := SidecarSetHashWithoutImage(sidecarSet)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			} else if newHash != hash1 {
				t.Fatalf("Expected same hashes for different SidecarSets")
			}
		})
	}
}
