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

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	fakescale "k8s.io/client-go/scale/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestGetPodReplicaSetClimbsOwnerRef(t *testing.T) {
	rolloutGVK := schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"}
	rolloutUID := types.UID("rollout-uid")
	deployUID := types.UID("deploy-uid")
	standaloneUID := types.UID("standalone-rs-uid")
	selfRSUID := types.UID("self-rs-uid")

	rolloutRS := &appsv1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSet"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "rollout-rs",
			UID:       "rollout-rs-uid",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "argoproj.io/v1alpha1",
				Kind:       "Rollout",
				Name:       "demo-rollout",
				UID:        rolloutUID,
				Controller: ptr.To(true),
			}},
		},
		Spec: appsv1.ReplicaSetSpec{Replicas: ptr.To[int32](2)},
	}
	rolloutObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Rollout",
			"metadata": map[string]interface{}{
				"namespace": "default",
				"name":      "demo-rollout",
				"uid":       string(rolloutUID),
			},
		},
	}
	deployRS := &appsv1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSet"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "deploy-rs",
			UID:       "deploy-rs-uid",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "demo-deploy",
				UID:        deployUID,
				Controller: ptr.To(true),
			}},
		},
		Spec: appsv1.ReplicaSetSpec{Replicas: ptr.To[int32](1)},
	}
	deployObj := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "demo-deploy", UID: deployUID},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](3)},
	}
	standaloneRS := &appsv1.ReplicaSet{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSet"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "standalone-rs", UID: standaloneUID},
		Spec:       appsv1.ReplicaSetSpec{Replicas: ptr.To[int32](4)},
	}
	selfRS := &appsv1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSet"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "self-rs",
			UID:       selfRSUID,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       "self-rs",
				UID:        selfRSUID,
				Controller: ptr.To(true),
			}},
		},
		Spec: appsv1.ReplicaSetSpec{Replicas: ptr.To[int32](2)},
	}

	fakeClient := fake.NewClientBuilder().
		WithObjects(rolloutRS, rolloutObj, deployRS, deployObj, standaloneRS, selfRS).
		Build()

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{rolloutGVK.GroupVersion()})
	mapper.Add(rolloutGVK, meta.RESTScopeNamespace)
	scaleClient := &fakescale.FakeScaleClient{}
	scaleClient.AddReactor("get", "rollouts", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, &autoscalingv1.Scale{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "demo-rollout", UID: rolloutUID},
			Spec:       autoscalingv1.ScaleSpec{Replicas: 2},
			Status:     autoscalingv1.ScaleStatus{Selector: "app=demo"},
		}, nil
	})

	finderWithScale := &ControllerFinder{Client: fakeClient, mapper: mapper, scaleNamespacer: scaleClient}
	finderWithoutMapper := &ControllerFinder{Client: fakeClient}

	tests := []struct {
		name       string
		finder     *ControllerFinder
		rsName     string
		wantKind   string
		wantName   string
		wantScale  int32
		wantSelect string
	}{
		{
			name:       "replicaset owned by rollout resolves via scale subresource",
			finder:     finderWithScale,
			rsName:     "rollout-rs",
			wantKind:   "Rollout",
			wantName:   "demo-rollout",
			wantScale:  2,
			wantSelect: "app=demo",
		},
		{
			name:      "replicaset owned by rollout resolves via unstructured fallback when mapper is nil",
			finder:    finderWithoutMapper,
			rsName:    "rollout-rs",
			wantKind:  "Rollout",
			wantName:  "demo-rollout",
			wantScale: ReplicasUnknown,
		},
		{
			name:      "replicaset owned by deployment keeps resolving deployment",
			finder:    finderWithScale,
			rsName:    "deploy-rs",
			wantKind:  "Deployment",
			wantName:  "demo-deploy",
			wantScale: 3,
		},
		{
			name:      "standalone replicaset resolves to itself",
			finder:    finderWithScale,
			rsName:    "standalone-rs",
			wantKind:  "ReplicaSet",
			wantName:  "standalone-rs",
			wantScale: 4,
		},
		{
			name:      "self-owned replicaset does not recurse",
			finder:    finderWithScale,
			rsName:    "self-rs",
			wantKind:  "ReplicaSet",
			wantName:  "self-rs",
			wantScale: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.finder.GetScaleAndSelectorForRef("apps/v1", "ReplicaSet", "default", tt.rsName, "")
			if err != nil {
				t.Fatalf("GetScaleAndSelectorForRef returned error: %v", err)
			}
			if got == nil {
				t.Fatalf("expected non-nil ScaleAndSelector")
			}
			if got.Kind != tt.wantKind || got.Name != tt.wantName || got.Scale != tt.wantScale {
				t.Fatalf("got (%s, %s, scale=%d), want (%s, %s, scale=%d)",
					got.Kind, got.Name, got.Scale, tt.wantKind, tt.wantName, tt.wantScale)
			}
			if tt.wantSelect != "" && (got.Selector == nil || got.Selector.MatchLabels["app"] != "demo") {
				t.Fatalf("expected selector from scale subresource, got %+v", got.Selector)
			}
		})
	}
}
