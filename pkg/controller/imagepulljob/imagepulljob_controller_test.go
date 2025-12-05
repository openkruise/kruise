package imagepulljob

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stesting "k8s.io/utils/clock/testing"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
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
		expectError     bool
		expectedUpdates int
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
			expectError:     false,
			expectedUpdates: 1,
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
			expectError:     false,
			expectedUpdates: 0,
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
			expectError:     false,
			expectedUpdates: 1, // Updated to remove reference
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
			err := r.releaseTargetSecrets(tt.targetMap, tt.job)

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
