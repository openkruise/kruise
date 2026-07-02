package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestGetEmptyObjectWithKey(t *testing.T) {
	tests := []struct {
		name     string
		object   client.Object
		wantType client.Object
	}{
		{
			name: "Pod",
			object: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "default", Labels: map[string]string{"app": "test"}},
				Spec:       v1.PodSpec{NodeName: "node-1"},
			},
			wantType: &v1.Pod{},
		},
		{
			name: "Service",
			object: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "kube-system"},
			},
			wantType: &v1.Service{},
		},
		{
			name: "Ingress",
			object: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "my-ingress", Namespace: "web"},
			},
			wantType: &networkingv1.Ingress{},
		},
		{
			name: "Deployment",
			object: &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "prod"},
			},
			wantType: &apps.Deployment{},
		},
		{
			name: "ReplicaSet",
			object: &apps.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Name: "my-rs", Namespace: "default"},
			},
			wantType: &apps.ReplicaSet{},
		},
		{
			name: "StatefulSet (upstream)",
			object: &apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "my-sts", Namespace: "default"},
			},
			wantType: &apps.StatefulSet{},
		},
		{
			name: "CloneSet",
			object: &appsv1alpha1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{Name: "my-clone", Namespace: "default"},
			},
			wantType: &appsv1alpha1.CloneSet{},
		},
		{
			name: "StatefulSet (kruise v1beta1)",
			object: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "my-kruise-sts", Namespace: "default"},
			},
			wantType: &appsv1beta1.StatefulSet{},
		},
		{
			name: "DaemonSet (kruise)",
			object: &appsv1alpha1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "my-ds", Namespace: "default"},
			},
			wantType: &appsv1alpha1.DaemonSet{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetEmptyObjectWithKey(tt.object)
			assert.IsType(t, tt.wantType, result)
			assert.Equal(t, tt.object.GetName(), result.GetName())
			assert.Equal(t, tt.object.GetNamespace(), result.GetNamespace())

			// Verify no data leaks — labels should be empty on the result
			assert.Empty(t, result.GetLabels())
		})
	}
}

func TestGetEmptyObjectWithKeyUnstructured(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "Foo"}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName("my-foo")
	obj.SetNamespace("bar")
	obj.SetLabels(map[string]string{"extra": "data"})

	result := GetEmptyObjectWithKey(obj)
	u, ok := result.(*unstructured.Unstructured)
	assert.True(t, ok)
	assert.Equal(t, gvk, u.GroupVersionKind())
	assert.Equal(t, "my-foo", u.GetName())
	assert.Equal(t, "bar", u.GetNamespace())
	// Labels should not carry over
	assert.Empty(t, u.GetLabels())
}
