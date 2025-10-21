package fieldindex

import (
	"fmt"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestIndexSidecarSet(t *testing.T) {
	type args struct {
		workload *appsv1alpha1.SidecarSet
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "nil obj",
			args: args{
				workload: nil,
			},
			want: nil,
		},
		{
			name: "namespace is specified in SidecarSet",
			args: args{
				workload: &appsv1alpha1.SidecarSet{
					Spec: appsv1alpha1.SidecarSetSpec{
						Namespace: "default",
					},
				},
			},
			want: []string{"default"},
		},
		{
			name: fmt.Sprintf("namespaceSelector is specified in SidecarSet and exists labels: %s", LabelMetadataName),
			args: args{
				workload: &appsv1alpha1.SidecarSet{
					Spec: appsv1alpha1.SidecarSetSpec{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								LabelMetadataName: "default",
							},
							MatchExpressions: nil,
						},
					},
				},
			},
			want: []string{"default"},
		},
		{
			name: fmt.Sprintf("namespaceSelector is specified in SidecarSet and exists labels: %s", LabelMetadataName),
			args: args{
				workload: &appsv1alpha1.SidecarSet{
					Spec: appsv1alpha1.SidecarSetSpec{
						NamespaceSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      LabelMetadataName,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										"default",
									},
								},
							},
						},
					},
				},
			},
			want: []string{"default"},
		},
		{
			name: "namespace and namespaceSelector not specified",
			args: args{
				workload: &appsv1alpha1.SidecarSet{
					Spec: appsv1alpha1.SidecarSetSpec{
						NamespaceSelector: &metav1.LabelSelector{},
					},
				},
			},
			want: []string{IndexValueSidecarSetClusterScope},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IndexSidecarSet(tt.args.workload)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IndexSidecarSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIndexImagePullJob tests the IndexImagePullJob function for correct indexing behavior
func TestIndexImagePullJob(t *testing.T) {
	// Case 1: Active job (no deletion timestamp, no completion time)
	activeJob := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{},
		Status: appsv1beta1.ImagePullJobStatus{
			CompletionTime: nil,
		},
	}
	assert.Equal(t, []string{"true"}, IndexImagePullJob(activeJob), "Expected active job to return 'true'")

	// Case 2: Inactive job due to completion time being set
	completedJob := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{},
		Status: appsv1beta1.ImagePullJobStatus{
			CompletionTime: &metav1.Time{Time: time.Now()},
		},
	}
	assert.Equal(t, []string{"false"}, IndexImagePullJob(completedJob), "Expected completed job to return 'false'")

	// Case 3: Inactive job due to deletion timestamp
	deletedJob := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Status: appsv1beta1.ImagePullJobStatus{
			CompletionTime: nil,
		},
	}
	assert.Equal(t, []string{"false"}, IndexImagePullJob(deletedJob), "Expected deleted job to return 'false'")
}
