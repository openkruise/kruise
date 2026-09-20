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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// getScaleController must not fail the caller when the RESTMapper can't resolve the owner's
// kind at all (e.g. its CRD isn't installed) -- that's a "can't determine" case, not an error,
// and should be treated like any other unresolved owner (nil, nil).
func Test_getScaleController_unresolvableKind(t *testing.T) {
	finder := &ControllerFinder{mapper: meta.NewDefaultRESTMapper(nil)}
	scale, err := finder.getScaleController(ControllerReference{
		APIVersion: "argoproj.io/v1alpha1",
		Kind:       "Rollout",
		Name:       "demo",
	}, "test")
	if err != nil {
		t.Fatalf("expected no error for an unresolvable kind, got: %v", err)
	}
	if scale != nil {
		t.Fatalf("expected nil ScaleAndSelector for an unresolvable kind, got: %+v", scale)
	}
}

// getPodReplicaSet must not fall back to treating the replicaSet itself as the workload when
// its owner is a Deployment that no longer exists (e.g. mid-delete-cascade) -- that must still
// resolve to nil, nil like before this PR's generalization, or callers that use the resolved
// workload's scale to decide when a PUB's status has drained to zero will never converge.
func Test_getPodReplicaSet_deletedDeploymentOwner(t *testing.T) {
	replicas := int32(1)
	rs := &apps.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-1",
			Namespace: "test",
			UID:       "rs-uid",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "deploy-1",
					UID:        "deploy-uid",
					Controller: ptr.To(true),
				},
			},
		},
		Spec: apps.ReplicaSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		},
	}
	testClient := fake.NewClientBuilder().WithObjects(rs).Build()
	finder := &ControllerFinder{Client: testClient}

	scale, err := finder.getPodReplicaSet(ControllerReference{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "rs-1",
	}, "test")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if scale != nil {
		t.Fatalf("expected nil ScaleAndSelector when the Deployment owner is gone, got: %+v", scale)
	}
}

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
