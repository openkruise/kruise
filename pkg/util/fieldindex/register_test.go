package fieldindex

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

type fakeIndexCache struct {
	indexFieldFunc func(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error
	calls          int
	fields         []string
	objects        []client.Object
}

func (f *fakeIndexCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return nil
}

func (f *fakeIndexCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (f *fakeIndexCache) GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}

func (f *fakeIndexCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}

func (f *fakeIndexCache) RemoveInformer(ctx context.Context, obj client.Object) error {
	return nil
}

func (f *fakeIndexCache) Start(ctx context.Context) error {
	return nil
}

func (f *fakeIndexCache) WaitForCacheSync(ctx context.Context) bool {
	return true
}

func (f *fakeIndexCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	f.calls++
	f.objects = append(f.objects, obj)
	f.fields = append(f.fields, field)
	if f.indexFieldFunc != nil {
		return f.indexFieldFunc(ctx, obj, field, extractValue)
	}
	return nil
}

func newNoMatchError() error {
	return &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "apps.kruise.io", Kind: "ImagePullJob"},
		SearchedVersions: []string{"v1alpha1"},
	}
}

func TestRegisterFieldIndexFailsFastOnNoMatchError(t *testing.T) {
	noMatchErr := newNoMatchError()
	fakeCache := &fakeIndexCache{
		indexFieldFunc: func(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
			return noMatchErr
		},
	}

	err := registerFieldIndex(fakeCache, &corev1.Pod{}, IndexNameForOwnerRefUID, ownerIndexFunc)

	assert.ErrorIs(t, err, noMatchErr)
	assert.True(t, meta.IsNoMatchError(err))
	assert.Equal(t, 1, fakeCache.calls)
}

func TestRegisterCRDFieldIndexRetriesNoMatchError(t *testing.T) {
	noMatchErr := newNoMatchError()
	fakeCache := &fakeIndexCache{}
	fakeCache.indexFieldFunc = func(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
		if fakeCache.calls == 1 {
			return noMatchErr
		}
		return nil
	}

	err := registerCRDFieldIndexWithRetry(fakeCache, &appsv1alpha1.ImagePullJob{}, IndexNameForOwnerRefUID, ownerIndexFunc, time.Millisecond, 100*time.Millisecond)

	assert.NoError(t, err)
	assert.Equal(t, 2, fakeCache.calls)
	assert.Equal(t, []string{IndexNameForOwnerRefUID, IndexNameForOwnerRefUID}, fakeCache.fields)
}

func TestRegisterCRDFieldIndexTimesOutOnNoMatchError(t *testing.T) {
	noMatchErr := newNoMatchError()
	fakeCache := &fakeIndexCache{
		indexFieldFunc: func(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
			return noMatchErr
		},
	}

	err := registerCRDFieldIndexWithRetry(fakeCache, &appsv1alpha1.ImagePullJob{}, IndexNameForOwnerRefUID, ownerIndexFunc, time.Millisecond, 5*time.Millisecond)

	assert.Error(t, err)
	assert.True(t, meta.IsNoMatchError(err))
	assert.Contains(t, err.Error(), "timed out")
	assert.GreaterOrEqual(t, fakeCache.calls, 2)
}

func TestRegisterCRDFieldIndexDoesNotRetryNonNoMatchError(t *testing.T) {
	expectedErr := errors.New("index failed")
	fakeCache := &fakeIndexCache{
		indexFieldFunc: func(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
			return expectedErr
		},
	}

	err := registerCRDFieldIndexWithRetry(fakeCache, &appsv1alpha1.ImagePullJob{}, IndexNameForOwnerRefUID, ownerIndexFunc, time.Millisecond, 100*time.Millisecond)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 1, fakeCache.calls)
}

func TestRegisterFieldIndexesRegistersBuiltInsBeforeCRDs(t *testing.T) {
	fakeCache := &fakeIndexCache{}

	err := registerFieldIndexes(fakeCache)

	assert.NoError(t, err)
	actualTypes := make([]reflect.Type, 0, len(fakeCache.objects))
	for _, obj := range fakeCache.objects {
		actualTypes = append(actualTypes, reflect.TypeOf(obj))
	}
	assert.Equal(t, []reflect.Type{
		reflect.TypeOf(&corev1.Pod{}),
		reflect.TypeOf(&corev1.PersistentVolumeClaim{}),
		reflect.TypeOf(&corev1.Pod{}),
		reflect.TypeOf(&batchv1.Job{}),
		reflect.TypeOf(&appsv1alpha1.ImagePullJob{}),
		reflect.TypeOf(&appsv1beta1.ImagePullJob{}),
		reflect.TypeOf(&appsv1alpha1.BroadcastJob{}),
		reflect.TypeOf(&appsv1beta1.BroadcastJob{}),
		reflect.TypeOf(&appsv1alpha1.ImagePullJob{}),
		reflect.TypeOf(&appsv1beta1.ImagePullJob{}),
		reflect.TypeOf(&appsv1beta1.ImageListPullJob{}),
		reflect.TypeOf(&appsv1alpha1.SidecarSet{}),
		reflect.TypeOf(&appsv1beta1.SidecarSet{}),
	}, actualTypes)
}

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
