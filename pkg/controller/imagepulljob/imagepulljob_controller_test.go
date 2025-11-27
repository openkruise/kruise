package imagepulljob

import (
	"context"
	"github.com/openkruise/kruise/pkg/util/expectations"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stesting "k8s.io/utils/clock/testing"

	"github.com/openkruise/kruise/apis/apps/defaults"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestReconcileImagePullJob_calculateStatus(t *testing.T) {

	tests := []struct {
		name           string
		job            *appsv1beta1.ImagePullJob
		nodeImages     []*appsv1beta1.NodeImage
		expectedStatus *appsv1beta1.ImagePullJobStatus
		expectError    bool
	}{
		{
			name: "all nodes succeeded",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-1",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.20",
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag:     "1.20",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-1"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:     "1.20",
										Version: 1,
										Phase:   appsv1beta1.ImagePhaseSucceeded,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   1,
				Active:      0,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job has completed",
			},
			expectError: false,
		},
		{
			name: "nodes in different states",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-2",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.20",
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				// Succeeded node
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag:     "1.20",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-2"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:     "1.20",
										Version: 1,
										Phase:   appsv1beta1.ImagePhaseSucceeded,
									},
								},
							},
						},
					},
				},
				// Pulling node
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node2"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag:     "1.20",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-2"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:     "1.20",
										Version: 1,
										Phase:   appsv1beta1.ImagePhasePulling,
									},
								},
							},
						},
					},
				},
				// Failed node
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node3"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag:     "1.20",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-2"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:     "1.20",
										Version: 1,
										Phase:   appsv1beta1.ImagePhaseFailed,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     3,
				Succeeded:   1,
				Active:      1,
				Failed:      1,
				FailedNodes: []string{"node3"},
				Message:     "job is running, progress 66.7%",
			},
			expectError: false,
		},
		{
			name: "node with image in different tags",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-2",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.21",
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				// Succeeded node
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag: "1.20",
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-0"},
										},
									},
									{
										Tag:     "1.21",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-2"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:   "1.20",
										Phase: appsv1beta1.ImagePhaseSucceeded,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      1,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectError: false,
		},
		{
			name: "node with image in different versions",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-2",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.21",
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				// Succeeded node
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag: "1.20",
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-0"},
										},
									},
									{
										Tag:     "1.21",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-2"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:   "1.21",
										Phase: appsv1beta1.ImagePhaseSucceeded,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      1,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectError: false,
		},
		{
			name: "node with image just patched",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-2",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.21",
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				// Succeeded node
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag: "1.20",
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-0"},
										},
									},
									{
										Tag:     "1.21",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-2"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      1,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectError: false,
		},
		{
			name: "nodes still counted as active even if missing secrets (shall be created)",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-3",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.20",
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{
									{Namespace: "default", Name: "missing-secret"},
								},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag:     "1.20",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-3"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      1,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectError: false,
		},
		{
			name: "nodes not synced due to missing owner reference",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-4",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.20",
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag:     "1.20",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "different-uid"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      0,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectError: false,
		},
		{
			name: "invalid image reference",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-5",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "invalid image reference!!!",
				},
			},
			nodeImages:     []*appsv1beta1.NodeImage{},
			expectedStatus: nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create reconciler with fake clock
			reconciler := &ReconcileImagePullJob{
				clock: k8stesting.NewFakeClock(time.Now()),
			}

			// Call calculateStatus
			status, err := reconciler.calculateStatus(tt.job, tt.nodeImages)

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			// No error expected
			assert.NoError(t, err)

			// Check status
			assert.Equal(t, tt.expectedStatus.Desired, status.Desired)
			assert.Equal(t, tt.expectedStatus.Succeeded, status.Succeeded)
			assert.Equal(t, tt.expectedStatus.Active, status.Active)
			assert.Equal(t, tt.expectedStatus.Failed, status.Failed)
			assert.Equal(t, tt.expectedStatus.Message, status.Message)
			assert.ElementsMatch(t, tt.expectedStatus.FailedNodes, status.FailedNodes)

			// Check start time is set
			assert.NotNil(t, status.StartTime)
		})
	}
}

func TestReconcileImagePullJob_calculateStatus_CompletionPolicy(t *testing.T) {
	fakeClock := k8stesting.NewFakeClock(time.Now())
	now := metav1.NewTime(fakeClock.Now())

	tests := []struct {
		name           string
		job            *appsv1beta1.ImagePullJob
		nodeImages     []*appsv1beta1.NodeImage
		timeOffset     time.Duration // offset from now for testing timeout
		expectedStatus *appsv1beta1.ImagePullJobStatus
	}{
		{
			name: "job completed with completion policy",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-1",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.20",
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						CompletionPolicy: appsv1beta1.CompletionPolicy{
							Type: appsv1beta1.Always,
						},
					},
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag:     "1.20",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-1"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:     "1.20",
										Version: 1,
										Phase:   appsv1beta1.ImagePhaseSucceeded,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   1,
				Active:      0,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job has completed",
			},
		},
		{
			name: "job timeout with active deadline seconds",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid-2",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:1.20",
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						CompletionPolicy: appsv1beta1.CompletionPolicy{
							Type:                  appsv1beta1.Always,
							ActiveDeadlineSeconds: func() *int64 { i := int64(300); return &i }(),
						},
					},
				},
				Status: appsv1beta1.ImagePullJobStatus{
					StartTime: &metav1.Time{Time: now.Add(-10 * time.Minute)}, // 10 minutes ago
				},
			},
			nodeImages: []*appsv1beta1.NodeImage{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								PullSecrets: []appsv1beta1.ReferenceObject{},
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag:     "1.20",
										Version: 1,
										OwnerReferences: []v1.ObjectReference{
											{UID: "job-uid-2"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:     "1.20",
										Version: 1,
										Phase:   appsv1beta1.ImagePhasePulling,
									},
								},
							},
						},
					},
				},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      0,
				Failed:      1,
				FailedNodes: []string{"node1"},
				Message:     "job exceeds activeDeadlineSeconds",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &ReconcileImagePullJob{
				clock: fakeClock,
			}

			status, err := reconciler.calculateStatus(tt.job, tt.nodeImages)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus.Desired, status.Desired)
			assert.Equal(t, tt.expectedStatus.Succeeded, status.Succeeded)
			assert.Equal(t, tt.expectedStatus.Active, status.Active)
			assert.Equal(t, tt.expectedStatus.Failed, status.Failed)
			assert.Equal(t, tt.expectedStatus.Message, status.Message)
			assert.ElementsMatch(t, tt.expectedStatus.FailedNodes, status.FailedNodes)

			// Check if completion time is set when job is completed or timed out
			if tt.job.Spec.CompletionPolicy.Type == appsv1beta1.Always {
				if tt.job.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil &&
					time.Duration(*tt.job.Spec.CompletionPolicy.ActiveDeadlineSeconds)*time.Second <=
						fakeClock.Now().Sub(tt.job.Status.StartTime.Time) {
					// Timeout case - should have completion time
					assert.NotNil(t, status.CompletionTime)
				} else if (status.Desired - status.Succeeded - status.Failed) == 0 {
					// All nodes completed - should have completion time
					assert.NotNil(t, status.CompletionTime)
				}
			}
		})
	}
}

// TestGetTargetSecretMap tests the getTargetSecretMap function using only fake client
func TestGetTargetSecretMap(t *testing.T) {
	now := metav1.NewTime(time.Now())

	// Define test cases
	tests := []struct {
		name              string
		job               *appsv1beta1.ImagePullJob
		existingSecrets   []v1.Secret
		expectedTargetLen int
		expectedDeleteLen int
		expectError       bool
	}{
		{
			name: "Normal case - successfully categorize secrets",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       types.UID("job-uid-1"),
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"source-secret-1"},
					},
				},
			},
			existingSecrets: []v1.Secret{
				// Secret that should be in targetMap (referenced by job)
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-1",
						Namespace: "kruise-daemon-config",
						Labels: map[string]string{
							SourceSecretUIDLabelKey: "source-uid-1",
						},
						Annotations: map[string]string{
							SourceSecretKeyAnno:       "default/source-secret-1",
							TargetOwnerReferencesAnno: "default/test-job",
						},
					},
				},
				// Secret that should be in deleteMap (not referenced by job)
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "delete-secret-1",
						Namespace: "kruise-daemon-config",
						Labels: map[string]string{
							SourceSecretUIDLabelKey: "source-uid-2",
						},
						Annotations: map[string]string{
							SourceSecretKeyAnno:       "default/source-secret-2",
							TargetOwnerReferencesAnno: "default/test-job",
						},
					},
				},
			},
			expectedTargetLen: 1,
			expectedDeleteLen: 1,
			expectError:       false,
		},
		{
			name: "Empty secrets list",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       types.UID("job-uid-1"),
				},
			},
			existingSecrets:   []v1.Secret{},
			expectedTargetLen: 0,
			expectedDeleteLen: 0,
			expectError:       false,
		},
		{
			name: "All secrets being deleted should be ignored",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       types.UID("job-uid-1"),
				},
			},
			existingSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "deleting-secret",
						Namespace:         "kruise-daemon-config",
						DeletionTimestamp: &now,
						Finalizers:        []string{"test-finalizer"},
						Labels: map[string]string{
							SourceSecretUIDLabelKey: "source-uid-1",
						},
						Annotations: map[string]string{
							SourceSecretKeyAnno:       "default/source-secret-1",
							TargetOwnerReferencesAnno: "default/test-job",
						},
					},
				},
			},
			expectedTargetLen: 0,
			expectedDeleteLen: 0,
			expectError:       false,
		},
		{
			name: "Secrets not associated with job should be ignored",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       types.UID("job-uid-1"),
				},
			},
			existingSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-secret",
						Namespace: "kruise-daemon-config",
						Labels: map[string]string{
							SourceSecretUIDLabelKey: "source-uid-1",
						},
						Annotations: map[string]string{
							SourceSecretKeyAnno:       "default/source-secret-1/other",
							TargetOwnerReferencesAnno: "default/other-job",
						},
					},
				},
			},
			expectedTargetLen: 0,
			expectedDeleteLen: 0,
			expectError:       false,
		},
		{
			name: "Secrets with invalid annotation should be ignored",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       types.UID("job-uid-1"),
				},
			},
			existingSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-secret",
						Namespace: "kruise-daemon-config",
						Labels: map[string]string{
							SourceSecretUIDLabelKey: "source-uid-1",
						},
						Annotations: map[string]string{
							SourceSecretKeyAnno:       "default/source-secret-1",
							TargetOwnerReferencesAnno: "default/other-job",
						},
					},
				},
			},
			expectedTargetLen: 0,
			expectedDeleteLen: 0,
			expectError:       false,
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing secrets
			objs := make([]client.Object, len(tt.existingSecrets))
			for i := range tt.existingSecrets {
				objs[i] = &tt.existingSecrets[i]
			}

			// Create a fake client
			cl := fake.NewClientBuilder().WithObjects(objs...).Build()

			// Create reconciler with fake client
			r := &ReconcileImagePullJob{
				Client: cl,
			}

			// Execute the function
			targetMap, deleteMap, err := r.getTargetSecretMap(tt.job)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, targetMap)
				assert.Nil(t, deleteMap)
			} else {
				assert.NoError(t, err)
				assert.Len(t, targetMap, tt.expectedTargetLen)
				assert.Len(t, deleteMap, tt.expectedDeleteLen)
			}
		})
	}
}

// TestReleaseTargetSecrets tests the releaseTargetSecrets function
func TestReleaseTargetSecrets(t *testing.T) {
	// Define test cases
	tests := []struct {
		name            string
		targetMap       map[string]*v1.Secret
		job             *appsv1beta1.ImagePullJob
		existingObjects []client.Object
		expectedDeleted []appsv1beta1.ReferenceObject
		expectError     bool
		expectedUpdates int
		expectedDeletes int
	}{
		{
			name:      "Empty targetMap should return nil",
			targetMap: map[string]*v1.Secret{},
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
			},
			expectedDeleted: nil,
			expectError:     false,
		},
		{
			name: "Nil secret in targetMap should be skipped",
			targetMap: map[string]*v1.Secret{
				"secret-1": nil,
				"secret-2": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-2",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-2-uid"),
						Annotations: map[string]string{
							TargetOwnerReferencesAnno: "default/test-job",
						},
					},
				},
			},
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
			},
			existingObjects: []client.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-2",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-2-uid"),
						Annotations: map[string]string{
							TargetOwnerReferencesAnno: "default/test-job",
						},
					},
				},
			},
			expectedDeleted: []appsv1beta1.ReferenceObject{
				{
					Name:      "secret-2",
					Namespace: "kruise-daemon-config",
				},
			},
			expectError:     false,
			expectedUpdates: 1,
			expectedDeletes: 1,
		},
		{
			name: "Secret not referenced by current job should not be updated",
			targetMap: map[string]*v1.Secret{
				"secret-1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							TargetOwnerReferencesAnno: "default/other-job",
						},
					},
				},
			},
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
			},
			existingObjects: []client.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							TargetOwnerReferencesAnno: "default/other-job",
						},
					},
				},
			},
			expectedDeleted: nil,
			expectError:     false,
			expectedUpdates: 0,
			expectedDeletes: 0,
		},
		{
			name: "Secret referenced by current job but also by others should only be updated",
			targetMap: map[string]*v1.Secret{
				"secret-1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							TargetOwnerReferencesAnno: "default/test-job,default/other-job",
						},
					},
				},
			},
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
			},
			existingObjects: []client.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							TargetOwnerReferencesAnno: "default/test-job,default/other-job",
						},
					},
				},
			},
			expectedDeleted: nil, // Not deleted because still referenced by other job
			expectError:     false,
			expectedUpdates: 1, // Updated to remove reference
			expectedDeletes: 0, // Not deleted
		},
		{
			name: "Secret only referenced by current job should be deleted",
			targetMap: map[string]*v1.Secret{
				"secret-1": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							TargetOwnerReferencesAnno: "default/test-job",
						},
					},
				},
			},
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
			},
			existingObjects: []client.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							TargetOwnerReferencesAnno: "default/test-job",
						},
					},
				},
			},
			expectedDeleted: []appsv1beta1.ReferenceObject{
				{
					Name:      "secret-1",
					Namespace: "kruise-daemon-config",
				},
			},
			expectError:     false,
			expectedUpdates: 1,
			expectedDeletes: 1,
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing objects
			clientBuilder := fake.NewClientBuilder()
			if tt.existingObjects != nil {
				clientBuilder.WithObjects(tt.existingObjects...)
			}

			// Create reconciler with fake client
			r := &ReconcileImagePullJob{
				Client: clientBuilder.Build(),
			}

			// Call function under test
			deleted, err := r.releaseTargetSecrets(tt.targetMap, tt.job)

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check deleted secrets
			assert.ElementsMatch(t, tt.expectedDeleted, deleted)
		})
	}
}

// TestSyncNodeImages_NoNodeImagesToSync tests the case when there are no node images to sync
func TestSyncNodeImages_NoNodeImagesToSync(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	// Create test data
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	status := &appsv1beta1.ImagePullJobStatus{
		Active: 0,
	}

	// Create a node image that is already synced
	nodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {
					Tags: []appsv1beta1.ImageTagSpec{
						{
							Tag: "latest",
						},
					},
				},
			},
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create reconciler
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	secrets := []appsv1beta1.ReferenceObject{}
	secretsDeleted := []appsv1beta1.ReferenceObject{}

	// Execute test
	err := r.syncNodeImages(job, status, []*appsv1beta1.NodeImage{nodeImage}, secrets, secretsDeleted)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestSyncNodeImages_ParallelismLimitExceeded tests the case when parallelism limit is exceeded
func TestSyncNodeImages_ParallelismLimitExceeded(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create reconciler
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	// Create test data
	parallelism := 2
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:latest",
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				Parallelism: &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: int32(parallelism),
				},
			},
		},
	}

	// Set active count equal to parallelism limit
	status := &appsv1beta1.ImagePullJobStatus{
		Active: int32(parallelism),
	}

	// Create a node image that needs syncing
	nodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {}, // Missing tag
			},
		},
	}

	secrets := []appsv1beta1.ReferenceObject{}
	secretsDeleted := []appsv1beta1.ReferenceObject{}

	// Execute test
	err := r.syncNodeImages(job, status, []*appsv1beta1.NodeImage{nodeImage}, secrets, secretsDeleted)

	// Verify results
	assert.NoError(t, err)
}

// TestSyncNodeImages_NormalSync tests the normal sync flow
func TestSyncNodeImages_NormalSync(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	// Create test data
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	status := &appsv1beta1.ImagePullJobStatus{
		Active: 0,
	}

	// Create a node image that needs syncing
	initialNodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {}, // Missing tag
			},
		},
	}

	secrets := []appsv1beta1.ReferenceObject{}
	secretsDeleted := []appsv1beta1.ReferenceObject{}

	// Create fake client with initial node image
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialNodeImage).
		Build()

	// Create reconciler
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	// Execute test
	err := r.syncNodeImages(job, status, []*appsv1beta1.NodeImage{initialNodeImage}, secrets, secretsDeleted)

	// Verify results
	assert.NoError(t, err)

	// Check that the node image was updated
	updatedNodeImage := &appsv1beta1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node-1"}, updatedNodeImage)
	assert.NoError(t, err)
	assert.NotNil(t, updatedNodeImage.Spec.Images["nginx"].Tags)
	assert.Len(t, updatedNodeImage.Spec.Images["nginx"].Tags, 1)
	assert.Equal(t, "latest", updatedNodeImage.Spec.Images["nginx"].Tags[0].Tag)
}

// TestSyncNodeImages_SecretHandling tests secret addition and removal
func TestSyncNodeImages_SecretHandling(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	tests := []struct {
		name             string
		job              *appsv1beta1.ImagePullJob
		initialNodeImage *appsv1beta1.NodeImage
		secrets          []appsv1beta1.ReferenceObject
		secretsDeleted   []appsv1beta1.ReferenceObject
		expectedSecrets  []appsv1beta1.ReferenceObject
		expectError      bool
	}{
		{
			name: "add new secret",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"new-secret"},
					},
				},
			},
			initialNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{
									Tag: "latest",
									OwnerReferences: []v1.ObjectReference{
										{UID: "job-uid"},
									},
								},
							},
						},
					},
				},
			},
			secrets: []appsv1beta1.ReferenceObject{
				{
					Namespace: "default",
					Name:      "new-secret",
				},
			},
			secretsDeleted: []appsv1beta1.ReferenceObject{},
			expectedSecrets: []appsv1beta1.ReferenceObject{
				{
					Namespace: "default",
					Name:      "new-secret",
				},
			},
			expectError: false,
		},
		{
			name: "remove old secret",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
				},
			},
			initialNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							PullSecrets: []appsv1beta1.ReferenceObject{
								{
									Namespace: "default",
									Name:      "old-secret",
								},
							},
							Tags: []appsv1beta1.ImageTagSpec{
								{
									Tag: "latest",
									OwnerReferences: []v1.ObjectReference{
										{UID: "job-uid"},
									},
								},
							},
						},
					},
				},
			},
			secrets: []appsv1beta1.ReferenceObject{},
			secretsDeleted: []appsv1beta1.ReferenceObject{
				{
					Namespace: "default",
					Name:      "old-secret",
				},
			},
			expectedSecrets: []appsv1beta1.ReferenceObject{},
			expectError:     false,
		},
		{
			name: "add new secret and remove old secret",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"new-secret"},
					},
				},
			},
			initialNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							PullSecrets: []appsv1beta1.ReferenceObject{
								{
									Namespace: "default",
									Name:      "old-secret",
								},
							},
							Tags: []appsv1beta1.ImageTagSpec{
								{
									Tag: "latest",
									OwnerReferences: []v1.ObjectReference{
										{UID: "job-uid"},
									},
								},
							},
						},
					},
				},
			},
			secrets: []appsv1beta1.ReferenceObject{
				{
					Namespace: "default",
					Name:      "new-secret",
				},
			},
			secretsDeleted: []appsv1beta1.ReferenceObject{
				{
					Namespace: "default",
					Name:      "old-secret",
				},
			},
			expectedSecrets: []appsv1beta1.ReferenceObject{
				{
					Namespace: "default",
					Name:      "new-secret",
				},
			},
			expectError: false,
		},
		{
			name: "no secret changes",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "job-uid",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
				},
			},
			initialNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							PullSecrets: []appsv1beta1.ReferenceObject{
								{
									Namespace: "default",
									Name:      "existing-secret",
								},
							},
							Tags: []appsv1beta1.ImageTagSpec{
								{
									Tag: "latest",
									OwnerReferences: []v1.ObjectReference{
										{UID: "job-uid"},
									},
								},
							},
						},
					},
				},
			},
			secrets: []appsv1beta1.ReferenceObject{
				{
					Namespace: "default",
					Name:      "existing-secret",
				},
			},
			secretsDeleted: []appsv1beta1.ReferenceObject{},
			expectedSecrets: []appsv1beta1.ReferenceObject{
				{
					Namespace: "default",
					Name:      "existing-secret",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &appsv1beta1.ImagePullJobStatus{
				Active: 0,
			}

			// Create fake client with initial node image
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.initialNodeImage).
				Build()

			// Create reconciler
			r := &ReconcileImagePullJob{
				Client: fakeClient,
				clock:  clock.RealClock{},
			}

			// Execute test
			err := r.syncNodeImages(tt.job, status, []*appsv1beta1.NodeImage{tt.initialNodeImage}, tt.secrets, tt.secretsDeleted)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Check that the node image was updated correctly
				updatedNodeImage := &appsv1beta1.NodeImage{}
				err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node-1"}, updatedNodeImage)
				assert.NoError(t, err)

				// Check secrets
				imageSpec := updatedNodeImage.Spec.Images["nginx"]
				assert.Len(t, imageSpec.PullSecrets, len(tt.expectedSecrets))
				for _, expectedSecret := range tt.expectedSecrets {
					found := false
					for _, actualSecret := range imageSpec.PullSecrets {
						if actualSecret.Namespace == expectedSecret.Namespace && actualSecret.Name == expectedSecret.Name {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected secret %s/%s not found", expectedSecret.Namespace, expectedSecret.Name)
				}
			}
		})
	}
}

// TestSyncNodeImages_UpdateError tests error handling during node image update
func TestSyncNodeImages_UpdateError(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	// Create test data
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	status := &appsv1beta1.ImagePullJobStatus{
		Active: 0,
	}

	// Create a node image that needs syncing
	initialNodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {}, // Missing tag
			},
		},
	}

	secrets := []appsv1beta1.ReferenceObject{}
	secretsDeleted := []appsv1beta1.ReferenceObject{}

	// Create fake client that simulates an update error by deleting the object before update
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialNodeImage).
		Build()

	// Manually delete the object to simulate a conflict
	err := fakeClient.Delete(context.TODO(), initialNodeImage)
	assert.NoError(t, err)

	// Create reconciler
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	// Execute test
	err = r.syncNodeImages(job, status, []*appsv1beta1.NodeImage{initialNodeImage}, secrets, secretsDeleted)

	// Verify results - should get a not found error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update NodeImage node-1 error")
}

// TestSyncNodeImages_ImageNotExists tests the case when the image doesn't exist in NodeImage before sync
func TestSyncNodeImages_ImageNotExists(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	// Create test data
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:1.20",
		},
	}

	status := &appsv1beta1.ImagePullJobStatus{
		Active: 0,
	}

	// Create a node image that doesn't have the image at all
	initialNodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"busybox": {
					Tags: []appsv1beta1.ImageTagSpec{
						{
							Tag: "latest",
						},
					},
				},
			},
		},
	}

	secrets := []appsv1beta1.ReferenceObject{}
	secretsDeleted := []appsv1beta1.ReferenceObject{}

	// Create fake client with initial node image
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialNodeImage).
		Build()

	// Create reconciler
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	// Execute test
	err := r.syncNodeImages(job, status, []*appsv1beta1.NodeImage{initialNodeImage}, secrets, secretsDeleted)

	// Verify results
	assert.NoError(t, err)

	// Check that the node image was updated with the new image
	updatedNodeImage := &appsv1beta1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node-1"}, updatedNodeImage)
	assert.NoError(t, err)

	// Check that nginx image was added
	nginxSpec, exists := updatedNodeImage.Spec.Images["nginx"]
	assert.True(t, exists, "nginx image should be added to NodeImage")
	assert.Len(t, nginxSpec.Tags, 1)
	assert.Equal(t, "1.20", nginxSpec.Tags[0].Tag)
	assert.Equal(t, int64(0), nginxSpec.Tags[0].Version)
	assert.Len(t, nginxSpec.Tags[0].OwnerReferences, 1)
	assert.Equal(t, "job-uid", string(nginxSpec.Tags[0].OwnerReferences[0].UID))

	// Check that existing images are still there
	_, busyboxExists := updatedNodeImage.Spec.Images["busybox"]
	assert.True(t, busyboxExists, "existing busybox image should still be present")
}

// TestSyncNodeImages_TagCreation tests creation of new image tag
func TestSyncNodeImages_TagCreation(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	// Create test data
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:1.20",
		},
	}

	status := &appsv1beta1.ImagePullJobStatus{
		Active: 0,
	}

	// Create a node image with image but no tags
	initialNodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {
					Tags: []appsv1beta1.ImageTagSpec{},
				},
				"busybox": {
					Tags: []appsv1beta1.ImageTagSpec{},
				},
			},
		},
		Status: appsv1beta1.NodeImageStatus{
			ImageStatuses: map[string]appsv1beta1.ImageStatus{
				"nginx": {
					Tags: []appsv1beta1.ImageTagStatus{
						{
							Tag:     "1.20",
							Version: 1,
						},
					},
				},
			},
		},
	}

	secrets := []appsv1beta1.ReferenceObject{}
	secretsDeleted := []appsv1beta1.ReferenceObject{}

	// Create fake client with initial node image
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialNodeImage).
		WithStatusSubresource(initialNodeImage).
		Build()

	// Create reconciler with fixed time
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	// Execute test
	err := r.syncNodeImages(job, status, []*appsv1beta1.NodeImage{initialNodeImage}, secrets, secretsDeleted)

	// Verify results
	assert.NoError(t, err)

	// Check that the node image was updated with new tag
	updatedNodeImage := &appsv1beta1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node-1"}, updatedNodeImage)
	assert.NoError(t, err)

	imageSpec := updatedNodeImage.Spec.Images["nginx"]
	assert.Len(t, imageSpec.Tags, 1)
	assert.Equal(t, "1.20", imageSpec.Tags[0].Tag)
	assert.Equal(t, int64(2), imageSpec.Tags[0].Version)
}

// TestSyncNodeImages_TagUpdate tests update of existing image tag
func TestSyncNodeImages_TagUpdate(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	// Create test data
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "job-uid",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:1.20",
		},
	}

	status := &appsv1beta1.ImagePullJobStatus{
		Active: 0,
	}

	// Create a node image with existing tag owned by this job
	initialNodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {
					Tags: []appsv1beta1.ImageTagSpec{
						{
							Tag:     "1.20",
							Version: 1,
							OwnerReferences: []v1.ObjectReference{
								{
									UID: "previous-job-uid",
								},
							},
						},
					},
				},
			},
		},
	}

	secrets := []appsv1beta1.ReferenceObject{}
	secretsDeleted := []appsv1beta1.ReferenceObject{}

	// Create fake client with initial node image
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialNodeImage).
		Build()

	// Create reconciler
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	// Execute test
	err := r.syncNodeImages(job, status, []*appsv1beta1.NodeImage{initialNodeImage}, secrets, secretsDeleted)

	// Verify results
	assert.NoError(t, err)

	// Check that the node image tag was updated (version incremented)
	updatedNodeImage := &appsv1beta1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node-1"}, updatedNodeImage)
	assert.NoError(t, err)

	imageSpec := updatedNodeImage.Spec.Images["nginx"]
	assert.Len(t, imageSpec.Tags, 1)
	assert.Equal(t, "1.20", imageSpec.Tags[0].Tag)
	assert.Equal(t, int64(2), imageSpec.Tags[0].Version) // Version incremented
}

// TestSyncNodeImages_JobDeletion tests the case when job has deletion timestamp and expects image tag to be deleted
func TestSyncNodeImages_JobDeletion(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	// Create test data
	now := metav1.Now()
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-job",
			Namespace:         "default",
			UID:               "job-uid",
			DeletionTimestamp: &now,
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:latest",
		},
	}

	status := &appsv1beta1.ImagePullJobStatus{
		Active: 0,
	}

	// Create a node image with existing tag owned by this job that should be deleted
	initialNodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {
					Tags: []appsv1beta1.ImageTagSpec{
						{
							Tag:     "latest",
							Version: 1,
							OwnerReferences: []v1.ObjectReference{
								{
									UID: "job-uid",
								},
							},
						},
					},
				},
			},
		},
	}

	secrets := []appsv1beta1.ReferenceObject{}
	secretsDeleted := []appsv1beta1.ReferenceObject{}

	// Create fake client with initial node image
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialNodeImage).
		Build()

	// Create reconciler
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		clock:  clock.RealClock{},
	}

	// Execute test
	err := r.syncNodeImages(job, status, []*appsv1beta1.NodeImage{initialNodeImage}, secrets, secretsDeleted)

	// Verify results
	assert.NoError(t, err)

	// Check that the node image was updated and tag was removed
	updatedNodeImage := &appsv1beta1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node-1"}, updatedNodeImage)
	assert.NoError(t, err)

	// Check that the tag no longer has owner reference to this job
	imageSpec := updatedNodeImage.Spec.Images["nginx"]
	assert.Len(t, imageSpec.Tags, 0)
}

// TestReconcileImagePullJob tests the main Reconcile function
func TestReconcileImagePullJob(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	appsv1beta1.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kruise-daemon-config",
		},
	}

	ttlseconds := int32(120)
	activeDeadlineSeconds := int64(300)

	tests := []struct {
		name             string
		job              *appsv1beta1.ImagePullJob
		nodeImages       []client.Object
		secrets          []client.Object
		expectError      bool
		expectResult     reconcile.Result
		expectJobDeleted bool
		validateJob      func(*testing.T, *appsv1beta1.ImagePullJob)
	}{
		{
			name: "job not found",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-existent-job",
					Namespace: "default",
				},
			},
			expectError:  false,
			expectResult: reconcile.Result{},
			validateJob: func(t *testing.T, job *appsv1beta1.ImagePullJob) {
				// Job should not exist, so nothing to validate
			},
		},
		{
			name: "successful reconcile with new job",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
					UID:       "test-job-uid",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
				},
			},
			nodeImages: []client.Object{
				&appsv1beta1.NodeImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag: "latest",
									},
								},
							},
						},
					},
				},
			},
			expectError:  false,
			expectResult: reconcile.Result{},
			validateJob: func(t *testing.T, job *appsv1beta1.ImagePullJob) {
				// Check that finalizer was added
				assert.Contains(t, job.Finalizers, defaults.ProtectionFinalizer)
				// Check that status was initialized
				assert.NotNil(t, job.Status.StartTime)
			},
		},
		{
			name: "job with deletion timestamp",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "deleting-job",
					Namespace:         "default",
					UID:               "deleting-job-uid",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{defaults.ProtectionFinalizer},
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
				},
			},
			nodeImages: []client.Object{
				&appsv1beta1.NodeImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
				},
			},
			expectError:      false,
			expectJobDeleted: true,
			expectResult:     reconcile.Result{},
			validateJob: func(t *testing.T, job *appsv1beta1.ImagePullJob) {
				// Check that finalizer was removed
				assert.NotContains(t, job.Finalizers, defaults.ProtectionFinalizer)
			},
		},
		{
			name: "job already exceed TTL",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "completed-job",
					Namespace:  "default",
					UID:        "completed-job-uid",
					Finalizers: []string{defaults.ProtectionFinalizer},
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						CompletionPolicy: appsv1beta1.CompletionPolicy{
							Type:                    appsv1beta1.Always,
							TTLSecondsAfterFinished: &ttlseconds,
						},
					},
				},
				Status: appsv1beta1.ImagePullJobStatus{
					CompletionTime: &metav1.Time{Time: time.Now().Add(-time.Hour)},
				},
			},
			expectError:  false,
			expectResult: reconcile.Result{},
			validateJob: func(t *testing.T, job *appsv1beta1.ImagePullJob) {
				// Job should be deleted in next reconcile cycle, so no specific validation needed here
				assert.True(t, job.DeletionTimestamp != nil)
			},
		},
		{
			name: "job with active deadline seconds and status changes",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "active-deadline-job",
					Namespace: "default",
					UID:       "active-deadline-job-uid",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					Image: "nginx:latest",
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						CompletionPolicy: appsv1beta1.CompletionPolicy{
							Type:                  appsv1beta1.Always,
							ActiveDeadlineSeconds: &activeDeadlineSeconds,
						},
					},
				},
				Status: appsv1beta1.ImagePullJobStatus{
					StartTime: &metav1.Time{Time: time.Now().Add(-time.Minute * 2)}, // Started 2 minutes ago
					Desired:   1,
					Succeeded: 0,
					Active:    1,
					Failed:    0,
				},
			},
			nodeImages: []client.Object{
				&appsv1beta1.NodeImage{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: appsv1beta1.NodeImageSpec{
						Images: map[string]appsv1beta1.ImageSpec{
							"nginx": {
								Tags: []appsv1beta1.ImageTagSpec{
									{
										Tag: "latest",
										OwnerReferences: []v1.ObjectReference{
											{UID: "active-deadline-job-uid"},
										},
									},
								},
							},
						},
					},
					Status: appsv1beta1.NodeImageStatus{
						ImageStatuses: map[string]appsv1beta1.ImageStatus{
							"nginx": {
								Tags: []appsv1beta1.ImageTagStatus{
									{
										Tag:   "latest",
										Phase: appsv1beta1.ImagePhasePulling,
									},
								},
							},
						},
					},
				},
			},
			expectError:  false,
			expectResult: reconcile.Result{RequeueAfter: time.Second}, // Should requeue with a delay
			validateJob: func(t *testing.T, job *appsv1beta1.ImagePullJob) {
				// Status should not have changed since job is still within active deadline
				assert.Equal(t, int32(1), job.Status.Desired)
				assert.Equal(t, int32(0), job.Status.Succeeded)
				assert.Equal(t, int32(1), job.Status.Active)
				assert.Equal(t, int32(0), job.Status.Failed)
				assert.Nil(t, job.Status.CompletionTime) // Not completed yet
				assert.Contains(t, job.Finalizers, defaults.ProtectionFinalizer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects slice
			objects := []client.Object{ns}
			if tt.job != nil && tt.job.Name != "non-existent-job" {
				objects = append(objects, tt.job)
			}
			objects = append(objects, tt.nodeImages...)
			if tt.secrets != nil {
				objects = append(objects, tt.secrets...)
			}

			// Create fake client
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithStatusSubresource(objects...).
				Build()

			// Create reconciler
			r := &ReconcileImagePullJob{
				Client: fakeClient,
				scheme: scheme,
				clock:  clock.RealClock{},
			}

			// Create request
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.job.Name,
					Namespace: tt.job.Namespace,
				},
			}

			// Register cleanup — runs after subtest finishes
			t.Cleanup(func() {
				resourceVersionExpectations = expectations.NewResourceVersionExpectation()
				scaleExpectations = expectations.NewScaleExpectations()
			})

			// Execute reconcile
			result, err := r.Reconcile(context.TODO(), req)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check result
			if tt.expectResult.RequeueAfter > 0 {
				// it is hard to compare time.Duration precisely
				assert.True(t, result.RequeueAfter > 0)
			} else {
				assert.True(t, result.RequeueAfter == 0)
			}

			// For job not found case, we expect empty result
			if tt.job.Name == "non-existent-job" {
				return
			}

			// Get updated job and validate
			updatedJob := &appsv1beta1.ImagePullJob{}
			getErr := fakeClient.Get(context.TODO(), types.NamespacedName{
				Name:      tt.job.Name,
				Namespace: tt.job.Namespace,
			}, updatedJob)

			if tt.expectJobDeleted {
				assert.True(t, errors.IsNotFound(getErr))
			} else {
				assert.NoError(t, getErr)
				if tt.validateJob != nil {
					tt.validateJob(t, updatedJob)
				}
			}
		})
	}
}
