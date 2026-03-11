/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllerfinder

import (
	"testing"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func Test_getSpecReplicas(t *testing.T) {
	type args struct {
		workload *unstructured.Unstructured
	}
	tests := []struct {
		name    string
		args    args
		want    int32
		wantErr bool
	}{
		{
			name: "replicas exist",
			args: args{
				workload: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"replicas": int64(3),
						},
					},
				},
			},
			want:    3,
			wantErr: false,
		},
		{
			name: "replicas do not exist",
			args: args{
				workload: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{},
					},
				},
			},
			want:    0,
			wantErr: false,
		},
		{
			name: "spec does not exist",
			args: args{
				workload: &unstructured.Unstructured{
					Object: map[string]interface{}{},
				},
			},
			want:    0,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getSpecReplicas(tt.args.workload)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSpecReplicas() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getSpecReplicas() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPodReplicaSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = apps.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	rs := &apps.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rs",
			Namespace: "default",
			UID:       types.UID("rs-uid-123"),
		},
		Spec: apps.ReplicaSetSpec{
			Replicas: ptr.To[int32](3),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rs).Build()
	finder := &ControllerFinder{Client: client}

	ref := ControllerReference{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "test-rs",
		UID:        types.UID("rs-uid-123"),
	}

	result, err := finder.getPodReplicaSet(ref, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Scale != 3 {
		t.Errorf("expected scale 3, got %d", result.Scale)
	}
	if result.APIVersion != "apps/v1" {
		t.Errorf("expected APIVersion apps/v1, got %s", result.APIVersion)
	}
	if result.Kind != "ReplicaSet" {
		t.Errorf("expected Kind ReplicaSet, got %s", result.Kind)
	}
}

func TestGetPodStatefulSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = apps.AddToScheme(scheme)

	ss := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ss",
			Namespace: "default",
			UID:       types.UID("ss-uid-123"),
		},
		Spec: apps.StatefulSetSpec{
			Replicas: ptr.To[int32](5),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ss).Build()
	finder := &ControllerFinder{Client: client}

	ref := ControllerReference{
		APIVersion: "apps/v1",
		Kind:       "StatefulSet",
		Name:       "test-ss",
		UID:        types.UID("ss-uid-123"),
	}

	result, err := finder.getPodStatefulSet(ref, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Scale != 5 {
		t.Errorf("expected scale 5, got %d", result.Scale)
	}
	if result.APIVersion != "apps/v1" {
		t.Errorf("expected APIVersion apps/v1, got %s", result.APIVersion)
	}
	if result.Kind != "StatefulSet" {
		t.Errorf("expected Kind StatefulSet, got %s", result.Kind)
	}
}

func TestGetPodDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = apps.AddToScheme(scheme)

	deploy := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "default",
			UID:       types.UID("deploy-uid-123"),
		},
		Spec: apps.DeploymentSpec{
			Replicas: ptr.To[int32](2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()
	finder := &ControllerFinder{Client: client}

	ref := ControllerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "test-deploy",
		UID:        types.UID("deploy-uid-123"),
	}

	result, err := finder.getPodDeployment(ref, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Scale != 2 {
		t.Errorf("expected scale 2, got %d", result.Scale)
	}
	if result.APIVersion != "apps/v1" {
		t.Errorf("expected APIVersion apps/v1, got %s", result.APIVersion)
	}
	if result.Kind != "Deployment" {
		t.Errorf("expected Kind Deployment, got %s", result.Kind)
	}
}

func TestGetPodReplicationController(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	rc := &corev1.ReplicationController{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rc",
			Namespace: "default",
			UID:       types.UID("rc-uid-123"),
		},
		Spec: corev1.ReplicationControllerSpec{
			Replicas: ptr.To[int32](4),
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rc).Build()
	finder := &ControllerFinder{Client: client}

	ref := ControllerReference{
		APIVersion: "v1",
		Kind:       "ReplicationController",
		Name:       "test-rc",
		UID:        types.UID("rc-uid-123"),
	}

	result, err := finder.getPodReplicationController(ref, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Scale != 4 {
		t.Errorf("expected scale 4, got %d", result.Scale)
	}
	if result.APIVersion != "v1" {
		t.Errorf("expected APIVersion v1, got %s", result.APIVersion)
	}
	if result.Kind != "ReplicationController" {
		t.Errorf("expected Kind ReplicationController, got %s", result.Kind)
	}
}

func TestGetPodKruiseCloneSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)

	cs := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cs",
			Namespace: "default",
			UID:       types.UID("cs-uid-123"),
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: ptr.To[int32](6),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cs).Build()
	finder := &ControllerFinder{Client: client}

	ref := ControllerReference{
		APIVersion: "apps.kruise.io/v1alpha1",
		Kind:       "CloneSet",
		Name:       "test-cs",
		UID:        types.UID("cs-uid-123"),
	}

	result, err := finder.getPodKruiseCloneSet(ref, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Scale != 6 {
		t.Errorf("expected scale 6, got %d", result.Scale)
	}
	if result.APIVersion != "apps.kruise.io/v1alpha1" {
		t.Errorf("expected APIVersion apps.kruise.io/v1alpha1, got %s", result.APIVersion)
	}
	if result.Kind != "CloneSet" {
		t.Errorf("expected Kind CloneSet, got %s", result.Kind)
	}
}

func TestGetPodKruiseStatefulSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)

	ss := &appsv1beta1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kruise-ss",
			Namespace: "default",
			UID:       types.UID("kss-uid-123"),
		},
		Spec: appsv1beta1.StatefulSetSpec{
			Replicas: ptr.To[int32](7),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ss).Build()
	finder := &ControllerFinder{Client: client}

	ref := ControllerReference{
		APIVersion: "apps.kruise.io/v1beta1",
		Kind:       "StatefulSet",
		Name:       "test-kruise-ss",
		UID:        types.UID("kss-uid-123"),
	}

	result, err := finder.getPodKruiseStatefulSet(ref, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Scale != 7 {
		t.Errorf("expected scale 7, got %d", result.Scale)
	}
	if result.APIVersion != "apps.kruise.io/v1beta1" {
		t.Errorf("expected APIVersion apps.kruise.io/v1beta1, got %s", result.APIVersion)
	}
	if result.Kind != "StatefulSet" {
		t.Errorf("expected Kind StatefulSet, got %s", result.Kind)
	}
}
