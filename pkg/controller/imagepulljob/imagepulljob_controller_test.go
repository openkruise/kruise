package imagepulljob

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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
				Active:      1, // Even when secret not synced, if tag is synced, node is counted as pulling/active
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
		name                  string
		job                   *appsv1beta1.ImagePullJob
		existingSecrets       []v1.Secret
		expectedTargetSecrets []string
		expectedDeleteSecrets []string
		expectError           bool
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
			expectedTargetSecrets: []string{"target-secret-1"},
			expectedDeleteSecrets: []string{"delete-secret-1"},
			expectError:           false,
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
			existingSecrets:       []v1.Secret{},
			expectedTargetSecrets: []string{},
			expectedDeleteSecrets: []string{},
			expectError:           false,
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
			expectedTargetSecrets: []string{},
			expectedDeleteSecrets: []string{},
			expectError:           false,
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
			expectedTargetSecrets: []string{},
			expectedDeleteSecrets: []string{},
			expectError:           false,
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
			expectedTargetSecrets: []string{},
			expectedDeleteSecrets: []string{},
			expectError:           false,
		},
		{
			name: "multiple matched source secrets",
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-3",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-2",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-1",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job",
						},
					},
				},
			},
			expectedTargetSecrets: []string{"target-secret-1"},
			expectedDeleteSecrets: []string{},
			expectError:           false,
		},
		{
			name: "multiple matched source secrets, and multiple matched refer secrets",
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
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-3",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-2",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-1",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-6",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job,default/test-job",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-4",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job,default/test-job",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-secret-5",
						Namespace: "kruise-daemon-config",
						Annotations: map[string]string{
							SecretAnnotationSourceSecretKey: "default/source-secret-1",
							SecretAnnotationReferenceJobs:   "default/other-job,default/test-job",
						},
					},
				},
			},
			expectedTargetSecrets: []string{"target-secret-6"},
			expectedDeleteSecrets: []string{},
			expectError:           false,
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name != "multiple matched source secrets, and multiple matched refer secrets" {
				return
			}

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
				assert.Len(t, targetMap, len(tt.expectedTargetSecrets))
				assert.Len(t, deleteMap, len(tt.expectedDeleteSecrets))
				targetSecrets := sets.NewString(tt.expectedTargetSecrets...)
				for _, obj := range targetMap {
					if !targetSecrets.Has(obj.Name) {
						t.Fatalf("expect(%v), but get(%s)", targetSecrets.List(), obj.Name)
					}
				}
				deleteSecrets := sets.NewString(tt.expectedDeleteSecrets...)
				for _, obj := range deleteMap {
					if !deleteSecrets.Has(obj.Name) {
						t.Fatalf("expect(%v), but get(%s)", deleteSecrets.List(), obj.Name)
					}
				}
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

// ==================== Tests for 100% coverage ====================

func TestSyncJobPullSecrets_ClassifyError(t *testing.T) {
	// Cover: syncJobPullSecrets line 312 - classifyPullSecretsForJob returns error
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-job",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"secret1"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(
			&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: util.GetKruiseDaemonConfigNamespace()}},
		).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*v1.SecretList); ok {
					return fmt.Errorf("list secrets failed")
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()

	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	_, err := r.syncJobPullSecrets(job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list secrets failed")
}

func TestSyncJobPullSecrets_ReleaseError(t *testing.T) {
	// Cover: syncJobPullSecrets line 315 - releaseImagePullJobSecrets returns error
	kruiseDaemonConfigNs := util.GetKruiseDaemonConfigNamespace()
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-job",
		},
		Status: appsv1beta1.ImagePullJobStatus{
			CompletionTime: func() *metav1.Time { t := metav1.Now(); return &t }(),
		},
	}

	// This secret will need to be released, and we inject a delete error
	secretToRelease := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "synced-secret",
			Namespace: kruiseDaemonConfigNs,
			Annotations: map[string]string{
				SecretAnnotationSourceSecretKey: "default/source-secret",
				SecretAnnotationReferenceJobs:   "default/test-job",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(
			&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: kruiseDaemonConfigNs}},
			secretToRelease,
		).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				return fmt.Errorf("delete failed")
			},
		}).Build()

	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	_, err := r.syncJobPullSecrets(job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

func TestSyncJobPullSecrets_NamespaceNotActive(t *testing.T) {
	// Cover: syncJobPullSecrets line 308 - namespaceIsActive error (namespace not found)
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-job",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"secret1"},
			},
		},
	}

	// No namespace object → namespaceIsActive will fail
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	_, err := r.syncJobPullSecrets(job)
	assert.Error(t, err)
}

func TestSyncJobPullSecrets_NamespaceDeleting(t *testing.T) {
	// Cover: syncJobPullSecrets + namespaceIsActive - namespace is being deleted
	now := metav1.Now()
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-job",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"secret1"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(
			&v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              util.GetKruiseDaemonConfigNamespace(),
					DeletionTimestamp: &now,
					Finalizers:        []string{"test"},
				},
			},
		).Build()
	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	_, err := r.syncJobPullSecrets(job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is being deleted")
}

func TestSyncJobPullSecrets_NoPullSecrets(t *testing.T) {
	// Cover: syncJobPullSecrets - job with no pullSecrets in kruise-daemon-config namespace
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.GetKruiseDaemonConfigNamespace(),
			Name:      "test-job",
		},
		Spec: appsv1beta1.ImagePullJobSpec{},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	result, err := r.syncJobPullSecrets(job)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestClassifyPullSecretsForJob_ListError(t *testing.T) {
	// Cover: classifyPullSecretsForJob line 446-449 - List returns error
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"secret1"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				return fmt.Errorf("list error")
			},
		}).Build()

	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	pullSecrets, releasedSecrets, err := r.classifyPullSecretsForJob(job)
	assert.Error(t, err)
	assert.Nil(t, pullSecrets)
	assert.Nil(t, releasedSecrets)
}

func TestClassifyPullSecretsForJob_SecretPriorityWithJobRef(t *testing.T) {
	// Cover: classifyPullSecretsForJob line 493 - the "else if referJobs.Has(jobKey)" branch
	// where obj != nil (already assigned a secret) and the second secret also has jobKey
	kruiseDaemonConfigNs := util.GetKruiseDaemonConfigNamespace()
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"source-secret"},
			},
		},
	}

	// Two secrets with the same source, both mapping to the same pullSecret
	// First secret (alphabetically) will be picked first (obj == nil → assigned)
	// Second secret has jobKey in referJobs → triggers else if branch
	existingSecrets := []client.Object{
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aaa-secret",
				Namespace: kruiseDaemonConfigNs,
				Annotations: map[string]string{
					SecretAnnotationSourceSecretKey: "default/source-secret",
					SecretAnnotationReferenceJobs:   "default/other-job",
				},
			},
		},
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bbb-secret",
				Namespace: kruiseDaemonConfigNs,
				Annotations: map[string]string{
					SecretAnnotationSourceSecretKey: "default/source-secret",
					SecretAnnotationReferenceJobs:   "default/test-job",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingSecrets...).Build()
	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}

	pullSecrets, releasedSecrets, err := r.classifyPullSecretsForJob(job)
	assert.NoError(t, err)
	assert.Len(t, pullSecrets, 1)
	assert.Empty(t, releasedSecrets)

	// The secret with jobKey (bbb-secret) should be preferred
	ref := appsv1beta1.ReferenceObject{Namespace: "default", Name: "source-secret"}
	assert.Equal(t, "bbb-secret", pullSecrets[ref].Name)
}

func TestClassifyPullSecretsForJob_JobBeingDeleted(t *testing.T) {
	// Cover: classifyPullSecretsForJob - job.DeletionTimestamp != nil path (line 479)
	kruiseDaemonConfigNs := util.GetKruiseDaemonConfigNamespace()
	now := metav1.Now()
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-job",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{"test"},
		},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				PullSecrets: []string{"source-secret"},
			},
		},
	}

	existingSecrets := []client.Object{
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "synced-secret",
				Namespace: kruiseDaemonConfigNs,
				Annotations: map[string]string{
					SecretAnnotationSourceSecretKey: "default/source-secret",
					SecretAnnotationReferenceJobs:   "default/test-job",
				},
			},
		},
		// Secret not referenced by this job
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-secret",
				Namespace: kruiseDaemonConfigNs,
				Annotations: map[string]string{
					SecretAnnotationSourceSecretKey: "default/other-source",
					SecretAnnotationReferenceJobs:   "default/other-job",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingSecrets...).Build()
	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}

	pullSecrets, releasedSecrets, err := r.classifyPullSecretsForJob(job)
	assert.NoError(t, err)
	assert.Empty(t, pullSecrets)
	assert.Len(t, releasedSecrets, 1)
	assert.Equal(t, "synced-secret", releasedSecrets[0].Name)
}

func TestReleaseImagePullJobSecrets_SecretBeingDeleted(t *testing.T) {
	// Cover: releaseImagePullJobSecrets line 515 - secret.DeletionTimestamp is non-zero
	now := metav1.Now()
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
	}

	secrets := []*v1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "deleting-secret",
				Namespace:         "kruise-daemon-config",
				DeletionTimestamp: &now,
				Finalizers:        []string{"test"},
				Annotations: map[string]string{
					SecretAnnotationReferenceJobs: "default/test-job",
				},
			},
		},
	}

	r := &ReconcileImagePullJob{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	err := r.releaseImagePullJobSecrets(secrets, job)
	assert.NoError(t, err)
}

func TestReleaseImagePullJobSecrets_NilSecret(t *testing.T) {
	// Cover: releaseImagePullJobSecrets line 515 - secret == nil
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
	}

	secrets := []*v1.Secret{nil}

	r := &ReconcileImagePullJob{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	err := r.releaseImagePullJobSecrets(secrets, job)
	assert.NoError(t, err)
}

func TestReleaseImagePullJobSecrets_DeleteError(t *testing.T) {
	// Cover: releaseImagePullJobSecrets line 522-525 - Delete returns error
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-1",
			Namespace: "kruise-daemon-config",
			Annotations: map[string]string{
				SecretAnnotationReferenceJobs: "default/test-job",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(secret).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				return fmt.Errorf("delete error")
			},
		}).Build()

	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	err := r.releaseImagePullJobSecrets([]*v1.Secret{secret}, job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete error")
}

func TestReleaseImagePullJobSecrets_NilAnnotations(t *testing.T) {
	// Cover: releaseImagePullJobSecrets line 534-536 - clone.Annotations == nil
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
	}

	// Secret with nil annotations but referencing multiple jobs via annotations set on the raw map
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-1",
			Namespace: "kruise-daemon-config",
		},
	}

	// We need to construct a secret where Annotations is nil but still has referJobs
	// Since getReferencingJobsFromSecret reads from Annotations, if nil → empty set → Len==0 → Delete path
	// So to hit the nil Annotations path at line 534, we need referJobs.Len() > 0
	// This means Annotations must not be nil for getReferencingJobsFromSecret but nil after DeepCopy
	// Actually, looking at the code: it first calls getReferencingJobsFromSecret (which reads annotations),
	// then does DeepCopy. If annotations map has values, DeepCopy will copy them. So clone.Annotations won't be nil.
	// The nil Annotations path would only be hit if secret.Annotations is nil AND referJobs still has entries.
	// But if Annotations is nil, getReferencingJobsFromSecret returns empty set → Delete path.
	// So this branch is unreachable in normal flow... Let me re-check.
	// Actually wait - getReferencingJobsFromSecret checks secret.Annotations[SecretAnnotationReferenceJobs] == ""
	// If Annotations is nil, accessing a nil map returns "" (zero value), so it returns empty set → Len==0 → Delete path.
	// So the nil Annotations check at line 534 is indeed a defensive check that's hard to trigger naturally.
	// But it CAN be triggered if Annotations is nil AND we inject referJobs differently.
	// Actually no... we can't. The nil annotations path for line 534 requires referJobs.Len() > 0,
	// but if annotations is nil, referJobs will be empty. So this IS dead code / defensive code.
	// But wait, looking more carefully: the secret input to releaseImagePullJobSecrets might have been
	// constructed from the classifyPullSecretsForJob which returns pointers to local copy.
	// Actually these are pointers into syncedSecrets array. Let's skip this and move on.

	// Let's just verify the Delete path (referJobs empty) with nil annotations
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(secret.DeepCopy()).Build()
	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	err := r.releaseImagePullJobSecrets([]*v1.Secret{secret}, job)
	assert.NoError(t, err)
}

func TestReleaseImagePullJobSecrets_UpdateError(t *testing.T) {
	// Cover: releaseImagePullJobSecrets line 544-547 - Update returns error
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-1",
			Namespace: "kruise-daemon-config",
			Annotations: map[string]string{
				SecretAnnotationReferenceJobs: "default/test-job,default/other-job",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(secret.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				return fmt.Errorf("update error")
			},
		}).Build()

	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme}
	err := r.releaseImagePullJobSecrets([]*v1.Secret{secret}, job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update error")
}

func TestClaimImagePullJobSecrets_GetError(t *testing.T) {
	// Cover: claimImagePullJobSecrets line 565 - Get returns non-NotFound error
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*v1.Secret); ok {
					return fmt.Errorf("internal server error")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()

	pullSecrets := map[appsv1beta1.ReferenceObject]*v1.Secret{
		{Namespace: "default", Name: "my-secret"}: nil,
	}

	r := &ReconcileImagePullJob{Client: fakeClient, scheme: scheme, generateRandomStringFunc: defaultGenerateRandomString}
	_, err := r.claimImagePullJobSecrets(job, pullSecrets)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "internal server error")
}

func TestClaimImagePullJobSecrets_CreateError(t *testing.T) {
	// Cover: claimImagePullJobSecrets line 571-575 - Create returns error
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
	}

	sourceSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{"key": []byte("value")},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(sourceSecret).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if secret, ok := obj.(*v1.Secret); ok && secret.Namespace == util.GetKruiseDaemonConfigNamespace() {
					return fmt.Errorf("create error")
				}
				return c.Create(ctx, obj, opts...)
			},
		}).Build()

	pullSecrets := map[appsv1beta1.ReferenceObject]*v1.Secret{
		{Namespace: "default", Name: "my-secret"}: nil,
	}

	r := &ReconcileImagePullJob{
		Client:                   fakeClient,
		scheme:                   scheme,
		generateRandomStringFunc: func() string { return "test" },
	}
	_, err := r.claimImagePullJobSecrets(job, pullSecrets)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "create error")
}

func TestClaimImagePullJobSecrets_UpdateError(t *testing.T) {
	// Cover: claimImagePullJobSecrets line 610-613 - Update returns error
	kruiseDaemonConfigNs := util.GetKruiseDaemonConfigNamespace()
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
	}

	sourceSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{"key": []byte("new-value")},
	}

	existingSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "synced-secret",
			Namespace: kruiseDaemonConfigNs,
			Annotations: map[string]string{
				SecretAnnotationReferenceJobs: "default/other-job",
			},
		},
		Data: map[string][]byte{"key": []byte("old-value")},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(sourceSecret, existingSecret).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				return fmt.Errorf("update error")
			},
		}).Build()

	pullSecrets := map[appsv1beta1.ReferenceObject]*v1.Secret{
		{Namespace: "default", Name: "my-secret"}: existingSecret,
	}

	r := &ReconcileImagePullJob{
		Client:                   fakeClient,
		scheme:                   scheme,
		generateRandomStringFunc: defaultGenerateRandomString,
	}
	_, err := r.claimImagePullJobSecrets(job, pullSecrets)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update error")
}
