package nodeimage

import (
	"regexp"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

// TestDoUpdateNodeImage tests the doUpdateNodeImage function
func TestDoUpdateNodeImage(t *testing.T) {
	now := metav1.Now()

	// Common setup for test cases
	baseNodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodeimage",
			Namespace: "default",
			Labels: map[string]string{
				"key1": "value1",
			},
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {
					Tags: []appsv1beta1.ImageTagSpec{
						{
							Tag:       "latest",
							Version:   1,
							CreatedAt: &now,
							OwnerReferences: []v1.ObjectReference{
								{
									Name:       "job1",
									Namespace:  "default",
									UID:        "uid1",
									Kind:       "ImagePullJob",
									APIVersion: "apps.kruise.io/v1alpha1",
								},
							},
						},
					},
				},
			},
		},
	}

	baseNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"key1": "value1",
			},
		},
	}

	type args struct {
		nodeImage *appsv1beta1.NodeImage
		node      *v1.Node
	}

	tests := []struct {
		name           string
		args           args
		wantModified   bool
		wantMessages   []string
		wantWaitNotNil bool
		wantTagCounts  map[string]int  // New field to check tag counts
		objs           []client.Object // For mocking k8s objects
	}{
		{
			name: "normal case without modification",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					ni.Labels = map[string]string{"key1": "value1"}
					ni.Spec.Images["nginx"].Tags[0].OwnerReferences = append(
						ni.Spec.Images["nginx"].Tags[0].OwnerReferences,
						v1.ObjectReference{
							Name:      "job1",
							Namespace: "default",
							UID:       "uid1",
						})
					return ni
				}(),
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"key1": "value1",
						},
					},
				},
			},
			wantModified:  false,
			wantMessages:  nil,
			wantTagCounts: map[string]int{"nginx": 1},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job1",
						Namespace: "default",
						UID:       "uid1",
					},
					Status: appsv1beta1.ImagePullJobStatus{},
				},
			},
		},
		{
			name: "node labels changed",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					ni.Labels = map[string]string{"key1": "value1"}
					ni.Spec.Images["nginx"].Tags[0].OwnerReferences = append(
						ni.Spec.Images["nginx"].Tags[0].OwnerReferences,
						v1.ObjectReference{
							Name:      "job1",
							Namespace: "default",
							UID:       "uid1",
						})
					return ni
				}(),
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"key1": "value2",
						},
					},
				},
			},
			wantModified:  true,
			wantMessages:  []string{"node labels changed"},
			wantTagCounts: map[string]int{"nginx": 1},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job1",
						Namespace: "default",
						UID:       "uid1",
					},
					Status: appsv1beta1.ImagePullJobStatus{},
				},
			},
		},
		{
			name: "missing createdAt field",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					ni.Labels = map[string]string{"key1": "value1"}
					ni.Spec.Images["nginx"].Tags[0].CreatedAt = nil
					ni.Spec.Images["nginx"].Tags[0].OwnerReferences = append(
						ni.Spec.Images["nginx"].Tags[0].OwnerReferences,
						v1.ObjectReference{
							Name:      "job1",
							Namespace: "default",
							UID:       "uid1",
						})
					return ni
				}(),
				node: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
						Labels: map[string]string{
							"key1": "value1",
						},
					},
				},
			},
			wantModified: true,
			wantMessages: []string{
				"image nginx:latest has no createAt field",
				"no longer has nginx image spec",
			},
			wantTagCounts: map[string]int{}, // Tag should be removed
		},
		{
			name: "owner reference with non ImagePullJob kind should be kept",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					ni.Spec.Images["nginx"].Tags[0].OwnerReferences[0] =
						v1.ObjectReference{
							Name:       "deployment1",
							Namespace:  "default",
							UID:        "dep-uid1",
							Kind:       "Deployment",
							APIVersion: "apps/v1",
						}
					return ni
				}(),
				node: baseNode.DeepCopy(),
			},
			wantModified:  false,
			wantTagCounts: map[string]int{"nginx": 1},
		},
		{
			name: "ImagePullJob not found should remove owner reference",
			args: args{
				nodeImage: baseNodeImage.DeepCopy(),
				node:      baseNode.DeepCopy(),
			},
			wantModified: true,
			wantMessages: []string{
				"image nginx:latest owners is cleared",
				"no longer has nginx image spec",
			},
			wantTagCounts: map[string]int{},  // Tag should be removed
			objs:          []client.Object{}, // No jobs exist
		},
		{
			name: "ImagePullJob uid mismatch should remove owner reference",
			args: args{
				nodeImage: baseNodeImage.DeepCopy(),
				node:      baseNode.DeepCopy(),
			},
			wantModified: true,
			wantMessages: []string{
				"image nginx:latest owners is cleared",
				"no longer has nginx image spec",
			},
			wantTagCounts: map[string]int{}, // Tag should be removed
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job1",
						Namespace: "default",
						UID:       "different-uid", // Different UID
					},
				},
			},
		},
		{
			name: "completed ImagePullJob should keep owner reference",
			args: args{
				nodeImage: baseNodeImage.DeepCopy(),
				node:      baseNode.DeepCopy(),
			},
			wantModified:  false,
			wantMessages:  []string{},
			wantTagCounts: map[string]int{"nginx": 1},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job1",
						Namespace: "default",
						UID:       "uid1",
					},
					Status: appsv1beta1.ImagePullJobStatus{
						CompletionTime: &metav1.Time{Time: time.Now()},
					},
				},
			},
		},
		{
			name: "running ImagePullJob should keep owner reference",
			args: args{
				nodeImage: baseNodeImage.DeepCopy(),
				node:      baseNode.DeepCopy(),
			},
			wantModified:  false,
			wantTagCounts: map[string]int{"nginx": 1},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
			},
		},
		{
			name: "partial ownerrference removed should keep the image tag",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					// Get the current tags slice
					imageSpec := ni.Spec.Images["nginx"]

					ni.Spec.Images["nginx"].Tags[0].OwnerReferences = append(
						ni.Spec.Images["nginx"].Tags[0].OwnerReferences,
						v1.ObjectReference{
							Name:       "job2",
							Namespace:  "default",
							UID:        "uid2",
							Kind:       "ImagePullJob",
							APIVersion: "apps.kruise.io/v1alpha1",
						})
					ni.Spec.Images["nginx"] = imageSpec
					return ni
				}(),
				node: baseNode.DeepCopy(),
			},
			wantModified: true,
			wantMessages: []string{
				"image nginx:latest owners changed from",
			},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job2",
						Namespace: "default",
						UID:       "uid2",
					},
				},
			},
			wantTagCounts: map[string]int{"nginx": 1},
		},
		{
			name: "partial tags removed should keep the image spec",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					// Get the current tags slice
					imageSpec := ni.Spec.Images["nginx"]
					// Add another tag that will also be removed to trigger deletion of the whole image spec
					imageSpec.Tags = append(imageSpec.Tags, appsv1beta1.ImageTagSpec{
						Tag:       "1.20",
						Version:   1,
						CreatedAt: &now,
						OwnerReferences: []v1.ObjectReference{
							{
								Name:       "job2",
								Namespace:  "default",
								UID:        "another-uid",
								Kind:       "ImagePullJob",
								APIVersion: "apps.kruise.io/v1alpha1",
							},
						},
					})
					ni.Spec.Images["nginx"] = imageSpec
					return ni
				}(),
				node: baseNode.DeepCopy(),
			},
			wantModified: true,
			wantMessages: []string{
				"image nginx:latest owners is cleared",
			},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job2",
						Namespace: "default",
						UID:       "another-uid", // Different UID
					},
				},
			},
			wantTagCounts: map[string]int{"nginx": 1},
		},
		{
			name: "TTL exceeded should remove tag",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					pastTime := metav1.NewTime(time.Now().Add(-61 * time.Second)) // 61 seconds ago
					ni.Spec.Images["nginx"].Tags[0].PullPolicy = &appsv1beta1.ImageTagPullPolicy{
						TTLSecondsAfterFinished: int32Ptr(60),
					}
					ni.Status.ImageStatuses = map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{
									Tag:            "latest",
									Version:        1,
									CompletionTime: &pastTime,
								},
							},
						},
					}
					return ni
				}(),
				node: baseNode.DeepCopy(),
			},
			wantModified: true,
			wantMessages: []string{
				"image nginx:latest has exceeded TTL \\(60\\)s since .* completed",
				"no longer has nginx image spec",
			},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
			},
			wantTagCounts: map[string]int{}, // Tag removed due to TTL expiration
		},
		{
			name: "TTL not exceeded should set wait duration",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					pastTime := metav1.NewTime(time.Now().Add(-30 * time.Second)) // 30 seconds ago
					ni.Spec.Images["nginx"].Tags[0].PullPolicy = &appsv1beta1.ImageTagPullPolicy{
						TTLSecondsAfterFinished: int32Ptr(60),
					}
					ni.Status.ImageStatuses = map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{
									Tag:            "latest",
									Version:        1,
									CompletionTime: &pastTime,
								},
							},
						},
					}
					return ni
				}(),
				node: baseNode.DeepCopy(),
			},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
			},
			wantModified:   false,
			wantWaitNotNil: true,
			wantTagCounts:  map[string]int{"nginx": 1}, // Tag preserved as TTL not exceeded
		},
		{
			name: "multiple images scenario",
			args: args{
				nodeImage: func() *appsv1beta1.NodeImage {
					ni := baseNodeImage.DeepCopy()
					// Add another tag
					imageSpec := ni.Spec.Images["nginx"]
					imageSpec.Tags = append(imageSpec.Tags,
						appsv1beta1.ImageTagSpec{
							Tag:       "1.20",
							Version:   1,
							CreatedAt: &now,
							OwnerReferences: []v1.ObjectReference{
								{
									Name:       "job2",
									Namespace:  "default",
									UID:        "uid2",
									Kind:       "ImagePullJob",
									APIVersion: "apps.kruise.io/v1alpha1",
								},
							},
						})
					ni.Spec.Images["nginx"] = imageSpec
					// Add a second image
					ni.Spec.Images["redis"] = appsv1beta1.ImageSpec{
						Tags: []appsv1beta1.ImageTagSpec{
							{
								Tag:       "alpine",
								Version:   1,
								CreatedAt: &now,
								OwnerReferences: []v1.ObjectReference{
									{
										Name:       "job3",
										Namespace:  "default",
										UID:        "uid3",
										Kind:       "ImagePullJob",
										APIVersion: "apps.kruise.io/v1alpha1",
									},
								},
							},
						},
					}
					return ni
				}(),

				node: baseNode.DeepCopy(),
			},
			objs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job1",
						Namespace: "default",
						UID:       "uid1",
					},
				},
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job2",
						Namespace: "default",
						UID:       "uid2",
					},
				},
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "job3",
						Namespace: "default",
						UID:       "uid3",
					},
				},
			},
			wantModified:  false,
			wantTagCounts: map[string]int{"nginx": 2, "redis": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client with initial objects
			scheme := runtime.NewScheme()
			_ = appsv1beta1.AddToScheme(scheme)
			_ = v1.AddToScheme(scheme)

			fakeclient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objs...).
				WithStatusSubresource(tt.objs...).
				Build()

			r := &ReconcileNodeImage{
				Client: fakeclient,
			}

			gotModified, gotMessages, gotWait := r.doUpdateNodeImage(tt.args.nodeImage, tt.args.node)

			if gotModified != tt.wantModified {
				t.Errorf("doUpdateNodeImage() gotModified = %v, want %v", gotModified, tt.wantModified)
			}

			// Check messages match expected patterns
			if len(tt.wantMessages) > 0 {
				if len(gotMessages) != len(tt.wantMessages) {
					t.Fatalf("message count mismatch, got %d, want %d", len(gotMessages), len(tt.wantMessages))
				}

				for i, pattern := range tt.wantMessages {
					matched, err := regexp.MatchString(pattern, gotMessages[i])
					if err != nil {
						t.Fatalf("invalid regex pattern: %v", err)
					}
					if !matched {
						t.Errorf("message[%d] = %q, want pattern matching %q", i, gotMessages[i], pattern)
					}
				}
			} else if len(gotMessages) > 0 {
				t.Errorf("unexpected messages: %v", gotMessages)
			}

			if tt.wantWaitNotNil && gotWait == nil {
				t.Errorf("expected wait duration to be set, but not")
			}

			// Check tag counts
			if tt.wantTagCounts != nil {
				actualTagCounts := make(map[string]int)
				for imageName, imageSpec := range tt.args.nodeImage.Spec.Images {
					actualTagCounts[imageName] = len(imageSpec.Tags)
				}

				// Compare expected vs actual tag counts
				for imageName, expectedCount := range tt.wantTagCounts {
					if actualCount, exists := actualTagCounts[imageName]; !exists {
						t.Errorf("expected image %s to exist with %d tags, but it doesn't exist", imageName, expectedCount)
					} else if actualCount != expectedCount {
						t.Errorf("image %s tag count mismatch: got %d, want %d", imageName, actualCount, expectedCount)
					}
				}

				// Check for images that shouldn't exist
				for imageName, actualCount := range actualTagCounts {
					if _, expected := tt.wantTagCounts[imageName]; !expected && actualCount > 0 {
						t.Errorf("unexpected image %s with %d tags exists", imageName, actualCount)
					}
				}
			}
		})
	}
}

// Helper functions
func int32Ptr(i int32) *int32 { return &i }
