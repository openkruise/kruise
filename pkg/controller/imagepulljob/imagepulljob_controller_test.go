package imagepulljob

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util"
)

func TestReconcileImagePullJob_calculateStatus(t *testing.T) {

	tests := []struct {
		name              string
		job               *appsv1beta1.ImagePullJob
		nodeImages        []*appsv1beta1.NodeImage
		secrets           []appsv1beta1.ReferenceObject
		expectedStatus    *appsv1beta1.ImagePullJobStatus
		expectedNotSynced []string
		expectError       bool
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
			secrets: []appsv1beta1.ReferenceObject{},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   1,
				Active:      0,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job has completed",
			},
			expectedNotSynced: []string{},
			expectError:       false,
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
			secrets: []appsv1beta1.ReferenceObject{},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     3,
				Succeeded:   1,
				Active:      1,
				Failed:      1,
				FailedNodes: []string{"node3"},
				Message:     "job is running, progress 66.7%",
			},
			expectedNotSynced: []string{},
			expectError:       false,
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
			secrets: []appsv1beta1.ReferenceObject{},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      1,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectedNotSynced: []string{},
			expectError:       false,
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
			secrets: []appsv1beta1.ReferenceObject{},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      1,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectedNotSynced: []string{},
			expectError:       false,
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
			secrets: []appsv1beta1.ReferenceObject{},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      1,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectedNotSynced: []string{},
			expectError:       false,
		},
		{
			name: "nodes not synced due to missing secrets",
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
			secrets: []appsv1beta1.ReferenceObject{
				{Namespace: "default", Name: "test-secret"},
			},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      0,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectedNotSynced: []string{"node1"},
			expectError:       false,
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
			secrets: []appsv1beta1.ReferenceObject{},
			expectedStatus: &appsv1beta1.ImagePullJobStatus{
				Desired:     1,
				Succeeded:   0,
				Active:      0,
				Failed:      0,
				FailedNodes: []string{},
				Message:     "job is running, progress 0.0%",
			},
			expectedNotSynced: []string{"node1"},
			expectError:       false,
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
			nodeImages:        []*appsv1beta1.NodeImage{},
			secrets:           []appsv1beta1.ReferenceObject{},
			expectedStatus:    nil,
			expectedNotSynced: nil,
			expectError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create reconciler with fake clock
			reconciler := &ReconcileImagePullJob{
				clock: k8stesting.NewFakeClock(time.Now()),
			}

			// Call calculateStatus
			status, notSynced, err := reconciler.calculateStatus(tt.job, tt.nodeImages, tt.secrets)

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

			// Check not synced nodes
			assert.ElementsMatch(t, tt.expectedNotSynced, notSynced)

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
		secrets        []appsv1beta1.ReferenceObject
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
			secrets: []appsv1beta1.ReferenceObject{},
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
			secrets: []appsv1beta1.ReferenceObject{},
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

			status, _, err := reconciler.calculateStatus(tt.job, tt.nodeImages, tt.secrets)

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
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/test-job",
						},
					},
				},
				// Secret that should be in deleteMap (not referenced by job)
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "delete-secret-1",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-2",
							SecretAnnotationReferenceJobs:   "default/test-job",
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
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/test-job",
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
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1/other",
							SecretAnnotationReferenceJobs:   "default/other-job",
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
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job",
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
			targetMap, deleteMap, err := r.classifyPullSecretsForJob(tt.job)

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
		targetMap       []*v1.Secret
		job             *appsv1beta1.ImagePullJob
		existingObjects []client.Object
		expectError     bool
		expectedUpdates int
	}{
		{
			name:      "Empty targetMap should return nil",
			targetMap: []*v1.Secret{},
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
			},
		},
		{
			name: "Nil secret in targetMap should be skipped",
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
							SecretAnnotationReferenceJobs: "default/test-job",
						},
					},
				},
			},
			expectError:     false,
			expectedUpdates: 1,
		},
		{
			name: "Secret not referenced by current job should not be updated",
			targetMap: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs: "default/other-job",
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
							SecretAnnotationReferenceJobs: "default/other-job",
						},
					},
				},
			},
			expectError:     false,
			expectedUpdates: 0,
		},
		{
			name: "Secret referenced by current job but also by others should only be updated",
			targetMap: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs: "default/test-job,default/other-job",
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
							SecretAnnotationReferenceJobs: "default/test-job,default/other-job",
						},
					},
				},
			},
			expectError:     false,
			expectedUpdates: 1, // Updated to remove reference
		},
		{
			name: "Secret only referenced by current job should be deleted",
			targetMap: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-1",
						Namespace: "kruise-daemon-config",
						UID:       types.UID("secret-1-uid"),
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs: "default/test-job",
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
							SecretAnnotationReferenceJobs: "default/test-job",
						},
					},
				},
			},
			expectError:     false,
			expectedUpdates: 1,
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
			err := r.releaseImagePullJobSecrets(tt.targetMap, tt.job)

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSyncJobPullSecrets(t *testing.T) {
	kruiseDaemonConfigNs := util.GetKruiseDaemonConfigNamespace()
	now := metav1.Now()
	tests := []struct {
		name               string
		job                *appsv1beta1.ImagePullJob
		existingObjects    []runtime.Object
		expectedSecrets    []v1.Secret
		generateRandomFunc func() string
		expectError        bool
		errorContains      string
	}{
		{
			name: "job in kruise-daemon-config namespace",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: kruiseDaemonConfigNs,
					Name:      "test-job",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"secret1", "secret2"},
					},
				},
			},
			existingObjects: []runtime.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1",
						Namespace: kruiseDaemonConfigNs,
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2",
						Namespace: kruiseDaemonConfigNs,
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1",
						Namespace: kruiseDaemonConfigNs,
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2",
						Namespace: kruiseDaemonConfigNs,
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
			},
			expectError:        false,
			generateRandomFunc: defaultGenerateRandomString,
		},
		{
			name: "job in other namespace without existing secrets",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-job",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"source-secret"},
					},
				},
			},
			existingObjects: []runtime.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: kruiseDaemonConfigNs,
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source-secret-123456",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs:   "default/test-job",
							SecretAnnotationSourceSecretKey: "default/source-secret",
							SecretAnnotationMode:            appsv1beta1.ReferenceObjectModeBatch,
						},
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
			},
			expectError: false,
			generateRandomFunc: func() string {
				return "123456"
			},
		},
		{
			name: "job in other namespace with existing synced secret",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-job",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"source-secret"},
					},
				},
			},
			existingObjects: []runtime.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source-secret",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret",
							SecretAnnotationReferenceJobs:   "default/test1-job",
						},
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: kruiseDaemonConfigNs,
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source-secret",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret",
							SecretAnnotationReferenceJobs: "default/test-job,default/test1" +
								"-job",
						},
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
			},
			generateRandomFunc: defaultGenerateRandomString,
			expectError:        false,
		},
		{
			name: "source secret not found",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-job",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"non-existent-secret"},
					},
				},
			},
			existingObjects:    []runtime.Object{},
			expectedSecrets:    []v1.Secret{},
			generateRandomFunc: defaultGenerateRandomString,
			expectError:        false, // Should handle missing secret gracefully
		},
		{
			name: "job in completed",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-job",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"source-secret"},
					},
				},
				Status: appsv1beta1.ImagePullJobStatus{
					CompletionTime: &now,
				},
			},
			existingObjects: []runtime.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source-secret-generated",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret",
							SecretAnnotationReferenceJobs:   "default/test-job",
						},
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source2-secret-generated",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source2-secret",
							SecretAnnotationReferenceJobs:   "default/test2-job",
						},
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
				&v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: kruiseDaemonConfigNs,
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "source2-secret-generated",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source2-secret",
							SecretAnnotationReferenceJobs:   "default/test2-job",
						},
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				},
			},
			generateRandomFunc: defaultGenerateRandomString,
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.existingObjects...).Build()
			r := &ReconcileImagePullJob{
				Client:                   fakeClient,
				scheme:                   scheme,
				generateRandomStringFunc: tt.generateRandomFunc,
			}

			_, err := r.syncJobPullSecrets(tt.job)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			}

			for _, secret := range tt.expectedSecrets {
				obj := &v1.Secret{}
				err = fakeClient.Get(context.TODO(), client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}, obj)
				if err != nil {
					t.Fatalf("get secret failed: %s", err.Error())
				}
				secret.ResourceVersion = ""
				obj.ResourceVersion = ""
				cur := util.DumpJSON(obj)
				expected := util.DumpJSON(secret)
				if cur != expected {
					t.Fatalf("expect(%s), but get(%s)", expected, cur)
				}
			}
		})
	}
}

func TestSyncJobPullSecrets_ReleaseSecrets(t *testing.T) {
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-job",
			UID:       "test-uid",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"new-secret"},
			},
		},
	}

	existingObjects := []runtime.Object{
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: util.GetKruiseDaemonConfigNamespace(),
			},
		},
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"key": []byte("value"),
			},
		},
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"key": []byte("value"),
			},
		},
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-secret",
				Namespace: util.GetKruiseDaemonConfigNamespace(),
				Annotations: map[string]string{
					SecretAnnotationSourceSecretKey: "default/old-secret",
					SecretAnnotationReferenceJobs:   "default/test-job,default/other-job",
				},
			},
			Data: map[string][]byte{
				"key": []byte("value"),
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existingObjects...).Build()

	r := &ReconcileImagePullJob{
		Client:                   fakeClient,
		scheme:                   scheme,
		generateRandomStringFunc: defaultGenerateRandomString,
	}

	result, err := r.syncJobPullSecrets(job)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	updatedSecret := &v1.Secret{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      "old-secret",
		Namespace: util.GetKruiseDaemonConfigNamespace(),
	}, updatedSecret)
	assert.NoError(t, err)

	assert.Contains(t, updatedSecret.Annotations[SecretAnnotationReferenceJobs], "other-job")
	assert.NotContains(t, updatedSecret.Annotations[SecretAnnotationReferenceJobs], "test-job")
}

func TestSyncJobPullSecrets_CreateNewSecret(t *testing.T) {
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-job",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"source-secret"},
			},
		},
	}

	existingObjects := []runtime.Object{
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: util.GetKruiseDaemonConfigNamespace(),
			},
		},
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"username": []byte("test-user"),
				"password": []byte("test-pass"),
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existingObjects...).Build()
	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: scheme,
		generateRandomStringFunc: func() string {
			return "123456"
		},
	}

	result, err := r.syncJobPullSecrets(job)
	assert.NoError(t, err)

	createdSecret := &v1.Secret{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      "source-secret-123456",
		Namespace: util.GetKruiseDaemonConfigNamespace(),
	}, createdSecret)
	assert.NoError(t, err)

	assert.Equal(t, "test-user", string(createdSecret.Data["username"]))
	assert.Equal(t, "test-pass", string(createdSecret.Data["password"]))
	assert.Equal(t, "default/source-secret", createdSecret.Annotations[SecretAnnotationSourceSecretKey])
	assert.Equal(t, "default/test-job", createdSecret.Annotations[SecretAnnotationReferenceJobs])

	assert.Len(t, result, 1)
	if len(result) > 0 {
		assert.Equal(t, util.GetKruiseDaemonConfigNamespace(), result[0].Namespace)
		assert.Equal(t, "source-secret-123456"+
			"", result[0].Name)
		assert.Equal(t, appsv1beta1.ReferenceObjectModeBatch, result[0].Mode)
	}
}

func TestClaimImagePullJobSecrets(t *testing.T) {
	kruiseDaemonConfigNs := util.GetKruiseDaemonConfigNamespace()
	tests := []struct {
		name               string
		job                *appsv1beta1.ImagePullJob
		sourceSecret       []v1.Secret
		existingSecret     []v1.Secret
		expectedError      bool
		expectedSecrets    []v1.Secret
		generateRandomFunc func() string
	}{
		{
			name: "create new secret successfully",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"my-secret"},
					},
				},
			},
			sourceSecret: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("password"),
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret-generated",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/my-secret",
							SecretAnnotationReferenceJobs:   "default/test-job",
							SecretAnnotationMode:            appsv1beta1.ReferenceObjectModeBatch,
						},
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("password"),
					},
				},
			},
			generateRandomFunc: func() string {
				return "generated"
			},
		},
		{
			name: "update existing secret successfully",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"my-secret"},
					},
				},
			},
			sourceSecret: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("password"),
					},
				},
			},
			existingSecret: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret-generated",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs:   "",
							SecretAnnotationSourceSecretKey: "default/my-secret",
						},
					},
					Data: map[string][]byte{
						"username": []byte("old-admin"),
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret-generated",
						Namespace: kruiseDaemonConfigNs,
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs:   "default/test-job",
							SecretAnnotationSourceSecretKey: "default/my-secret",
						},
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("password"),
					},
				},
			},
			generateRandomFunc: defaultGenerateRandomString,
		},
		{
			name: "skip update when no changes",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"my-secret"},
					},
				},
			},
			sourceSecret: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("password"),
					},
				},
			},
			existingSecret: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret-generated",
						Namespace: util.GetKruiseDaemonConfigNamespace(),
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs: "default/test-job",
						},
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("password"),
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-secret-generated",
						Namespace: util.GetKruiseDaemonConfigNamespace(),
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs: "default/test-job",
						},
					},
					Data: map[string][]byte{
						"username": []byte("admin"),
						"password": []byte("password"),
					},
				},
			},
			generateRandomFunc: defaultGenerateRandomString,
		},
		{
			name: "handle multiple secrets",
			job: &appsv1beta1.ImagePullJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: appsv1beta1.ImagePullJobSpec{
					ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
						PullSecrets: []string{"secret1", "secret2"},
					},
				},
			},
			sourceSecret: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key": []byte("value1"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"key": []byte("value2"),
					},
				},
			},
			existingSecret: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2-generated",
						Namespace: util.GetKruiseDaemonConfigNamespace(),
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs:   "default/other-job",
							SecretAnnotationSourceSecretKey: "default/secret2",
						},
					},
					Data: map[string][]byte{
						"key": []byte("value2"),
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2-generated",
						Namespace: util.GetKruiseDaemonConfigNamespace(),
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs:   "default/other-job,default/test-job",
							SecretAnnotationSourceSecretKey: "default/secret2",
						},
					},
					Data: map[string][]byte{
						"key": []byte("value2"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1-generated",
						Namespace: util.GetKruiseDaemonConfigNamespace(),
						Annotations: map[string]string{
							SecretAnnotationReferenceJobs:   "default/test-job",
							SecretAnnotationSourceSecretKey: "default/secret1",
							SecretAnnotationMode:            appsv1beta1.ReferenceObjectModeBatch,
						},
					},
					Data: map[string][]byte{
						"key": []byte("value1"),
					},
				},
			},
			generateRandomFunc: func() string {
				return "generated"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{tt.job}
			for i := range tt.sourceSecret {
				obj := &tt.sourceSecret[i]
				objs = append(objs, obj)
			}
			for i := range tt.existingSecret {
				obj := &tt.existingSecret[i]
				objs = append(objs, obj)
			}

			fakeC := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
			r := &ReconcileImagePullJob{
				Client:                   fakeC,
				scheme:                   scheme,
				generateRandomStringFunc: func() string { return "generated" },
			}

			pullSecrets := make(map[appsv1beta1.ReferenceObject]*v1.Secret)
			for _, secretName := range tt.job.Spec.PullSecrets {
				ref := appsv1beta1.ReferenceObject{Namespace: tt.job.Namespace, Name: secretName}
				if tt.existingSecret != nil && tt.existingSecret[0].Name == secretName+"-generated" {
					pullSecrets[ref] = &tt.existingSecret[0]
				} else {
					pullSecrets[ref] = nil
				}
			}

			_, err := r.claimImagePullJobSecrets(tt.job, pullSecrets)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			for _, secret := range tt.expectedSecrets {
				obj := &v1.Secret{}
				err = fakeC.Get(context.TODO(), client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}, obj)
				if err != nil {
					t.Fatalf("get secret failed: %s", err.Error())
				}
				secret.ResourceVersion = ""
				obj.ResourceVersion = ""
				cur := util.DumpJSON(obj)
				expected := util.DumpJSON(secret)
				if cur != expected {
					t.Fatalf("expect(%s), but get(%s)", expected, cur)
				}
			}
		})
	}
}

func TestClaimImagePullJobSecrets_SourceSecretNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)

	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"nonexistent-secret"},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(job).Build()

	r := &ReconcileImagePullJob{
		Client:                   client,
		scheme:                   scheme,
		generateRandomStringFunc: func() string { return "generated" },
	}

	pullSecrets := map[appsv1beta1.ReferenceObject]*v1.Secret{
		{Namespace: "default", Name: "nonexistent-secret"}: nil,
	}

	result, err := r.claimImagePullJobSecrets(job, pullSecrets)

	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestClaimImagePullJobSecrets_CreateSecretAnnotations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)

	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"my-secret"},
			},
		},
	}

	sourceSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
			Annotations: map[string]string{
				"custom.annotation": "value",
			},
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("password"),
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(job, sourceSecret).Build()

	r := &ReconcileImagePullJob{
		Client:                   client,
		scheme:                   scheme,
		generateRandomStringFunc: func() string { return "test-generated" },
	}

	pullSecrets := map[appsv1beta1.ReferenceObject]*v1.Secret{
		{Namespace: "default", Name: "my-secret"}: nil,
	}

	result, err := r.claimImagePullJobSecrets(job, pullSecrets)
	assert.NoError(t, err)
	assert.Len(t, result, 1)

	createdSecret := &v1.Secret{}
	err = r.Get(context.TODO(), types.NamespacedName{
		Namespace: util.GetKruiseDaemonConfigNamespace(),
		Name:      result[0].Name,
	}, createdSecret)
	assert.NoError(t, err)

	assert.Equal(t, "default/test-job", createdSecret.Annotations[SecretAnnotationReferenceJobs])
	assert.Equal(t, "default/my-secret", createdSecret.Annotations[SecretAnnotationSourceSecretKey])
	assert.Equal(t, appsv1beta1.ReferenceObjectModeBatch, createdSecret.Annotations[SecretAnnotationMode])
	assert.Equal(t, "value", createdSecret.Annotations["custom.annotation"])

	assert.Equal(t, sourceSecret.Data, createdSecret.Data)
}

func TestClaimImagePullJobSecrets_UpdateExistingSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = appsv1beta1.AddToScheme(scheme)

	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"my-secret"},
			},
		},
	}

	sourceSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"updated-data": []byte("new-value"),
		},
	}

	existingSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret-generated",
			Namespace: util.GetKruiseDaemonConfigNamespace(),
			Annotations: map[string]string{
				SecretAnnotationReferenceJobs: "default/other-job",
			},
		},
		Data: map[string][]byte{
			"old-data": []byte("old-value"),
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(job, sourceSecret, existingSecret).Build()

	r := &ReconcileImagePullJob{
		Client:                   client,
		scheme:                   scheme,
		generateRandomStringFunc: func() string { return "generated" },
	}

	pullSecrets := map[appsv1beta1.ReferenceObject]*v1.Secret{
		{Namespace: "default", Name: "my-secret"}: existingSecret,
	}

	result, err := r.claimImagePullJobSecrets(job, pullSecrets)
	assert.NoError(t, err)
	assert.Len(t, result, 1)

	updatedSecret := &v1.Secret{}
	err = r.Get(context.TODO(), types.NamespacedName{
		Namespace: util.GetKruiseDaemonConfigNamespace(),
		Name:      "my-secret-generated",
	}, updatedSecret)
	assert.NoError(t, err)

	assert.Equal(t, map[string][]byte{"updated-data": []byte("new-value")}, updatedSecret.Data)

	assert.Contains(t, updatedSecret.Annotations[SecretAnnotationReferenceJobs], "default/test-job")
	assert.Contains(t, updatedSecret.Annotations[SecretAnnotationReferenceJobs], "default/other-job")
}

func TestReconcileImagePullJob_syncNodeImages_UpdateTimeout(t *testing.T) {
	// Create a job with timeout 600
	timeout := int32(600)
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
			UID:       types.UID("job-uid"),
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			Image: "nginx:latest",
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullPolicy: &appsv1beta1.PullPolicy{
					TimeoutSeconds: &timeout,
				},
			},
		},
	}

	// Create a NodeImage that already has the tag, OwnerReference pointing to the job,
	// BUT has a DIFFERENT timeout (e.g. 300) in its PullPolicy.
	// This simulates the state where the Job was updated but NodeImage is not yet.
	oldTimeout := int32(300)
	nodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-1",
			Namespace: "", // Cluster scoped
		},
		Spec: appsv1beta1.NodeImageSpec{
			Images: map[string]appsv1beta1.ImageSpec{
				"nginx": {
					Tags: []appsv1beta1.ImageTagSpec{
						{
							Tag: "latest",
							OwnerReferences: []v1.ObjectReference{
								{
									APIVersion: appsv1beta1.SchemeGroupVersion.String(),
									Kind:       "ImagePullJob",
									Name:       "test-job",
									UID:        "job-uid",
								},
							},
							ImagePullPolicy: appsv1beta1.ImagePullPolicy(appsv1beta1.PullIfNotPresent),
							PullPolicy: &appsv1beta1.ImageTagPullPolicy{
								TimeoutSeconds: &oldTimeout,
							},
						},
					},
				},
			},
		},
	}

	// scheme is globally defined in imagepulljob_event_handler_test.go which is in the same package
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, nodeImage).Build()

	r := &ReconcileImagePullJob{
		Client: fakeClient,
		scheme: scheme,
		clock:  k8stesting.NewFakeClock(time.Now()),
	}

	// Call syncNodeImages
	err := r.syncNodeImages(job, &appsv1beta1.ImagePullJobStatus{Active: 0}, []string{"node-1"}, nil)
	assert.NoError(t, err)

	// Fetch updated NodeImage
	updatedNodeImage := &appsv1beta1.NodeImage{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node-1"}, updatedNodeImage)
	assert.NoError(t, err)

	// Check if TimeoutSeconds is updated to 600
	imageSpec := updatedNodeImage.Spec.Images["nginx"]
	tagSpec := imageSpec.Tags[0]

	if tagSpec.PullPolicy == nil || tagSpec.PullPolicy.TimeoutSeconds == nil || *tagSpec.PullPolicy.TimeoutSeconds != 600 {
		t.Fatalf("Regression Test Failed: Expected timeout 600, got %v", tagSpec.PullPolicy.TimeoutSeconds)
	}
}
