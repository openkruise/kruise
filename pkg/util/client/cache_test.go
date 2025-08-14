package client

import (
	"context"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNewCache(t *testing.T) {
	tests := []struct {
		name      string
		config    *rest.Config
		opts      ctrcache.Options
		wantErr   bool
		wantPanic bool
	}{
		{
			name:    "successful cache creation",
			config:  &rest.Config{},
			opts:    ctrcache.Options{Scheme: runtime.NewScheme()},
			wantErr: false,
		},
		{
			name:      "panic due to nil config",
			config:    nil,
			opts:      ctrcache.Options{Scheme: runtime.NewScheme()},
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.Panics(t, func() {
					NewCache(tt.config, tt.opts)
				})
			} else {
				got, err := NewCache(tt.config, tt.opts)
				if (err != nil) != tt.wantErr {
					t.Errorf("NewCache() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !tt.wantErr && got == nil {
					t.Errorf("NewCache() got = nil, want non-nil")
				}
			}
		})
	}
}

func Test_internalCache_List(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)

	mockC := &mockCache{}
	noDeepCopyLister := &noDeepCopyLister{
		cache:  mockC,
		scheme: scheme,
	}
	ic := &internalCache{
		Cache:            mockC,
		noDeepCopyLister: noDeepCopyLister,
	}

	ctx := context.Background()

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "PodList",
	})

	tests := []struct {
		name                  string
		disableNoDeepCopyFlag bool
		useDisableDeepCopyOpt bool
		wantErr               bool
	}{
		{
			name:                  "DisableDeepCopy option present, flag false",
			disableNoDeepCopyFlag: false,
			useDisableDeepCopyOpt: true,
			wantErr:               false,
		},
		{
			name:                  "DisableDeepCopy option absent, flag false",
			disableNoDeepCopyFlag: false,
			useDisableDeepCopyOpt: false,
			wantErr:               false,
		},
		{
			name:                  "DisableDeepCopy option present, flag true",
			disableNoDeepCopyFlag: true,
			useDisableDeepCopyOpt: true,
			wantErr:               false,
		},
		{
			name:                  "DisableDeepCopy option absent, flag true",
			disableNoDeepCopyFlag: true,
			useDisableDeepCopyOpt: false,
			wantErr:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disableNoDeepCopy = tt.disableNoDeepCopyFlag

			var listOpts []client.ListOption
			if tt.useDisableDeepCopyOpt {
				listOpts = []client.ListOption{DisableDeepCopy}
			} else {
				listOpts = []client.ListOption{}
			}

			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "PodList",
			})

			err := ic.List(ctx, list, listOpts...)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_isDisableDeepCopy(t *testing.T) {
	tests := []struct {
		name string
		opts []client.ListOption
		want bool
	}{
		{
			name: "with DisableDeepCopy option",
			opts: []client.ListOption{DisableDeepCopy},
			want: true,
		},
		{
			name: "without DisableDeepCopy option",
			opts: []client.ListOption{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDisableDeepCopy(tt.opts); got != tt.want {
				t.Errorf("isDisableDeepCopy() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockListerWatcher struct{}

func (m *mockListerWatcher) List(options metav1.ListOptions) (runtime.Object, error) {
	return &corev1.PodList{}, nil
}

func (m *mockListerWatcher) Watch(options metav1.ListOptions) (watch.Interface, error) {
	return watch.NewFake(), nil
}

type mockCache struct {
	ctrcache.Cache
}

func (m *mockCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (m *mockCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...ctrcache.InformerGetOption) (ctrcache.Informer, error) {
	informer := cache.NewSharedIndexInformer(
		&mockListerWatcher{},
		&corev1.Pod{}, 
		0,             
		cache.Indexers{},
	)
	return informer, nil
}

func Test_internalCache_ListWithMockCache(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	mockCache := &mockCache{}

	noDeepCopyLister := &noDeepCopyLister{
		cache:  mockCache,
		scheme: scheme,
	}

	ic := &internalCache{
		Cache:            mockCache,
		noDeepCopyLister: noDeepCopyLister,
	}

	ctx := context.Background()
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "PodList",
	})
	listOpts := []client.ListOption{DisableDeepCopy}

	err := ic.List(ctx, list, listOpts...)
	assert.NoError(t, err)
}

func TestMain(m *testing.M) {
	flag.Parse()
	m.Run()
}
