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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
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

func Test_implementsScale(t *testing.T) {
	tests := []struct {
		name           string
		gvk            schema.GroupVersionKind
		setupDiscovery func() *fakediscovery.FakeDiscovery
		want           bool
	}{
		{
			name: "Deployment supports scale",
			gvk:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			setupDiscovery: func() *fakediscovery.FakeDiscovery {
				client := fakeclientset.NewSimpleClientset()
				fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
				fakeDiscovery.Resources = []*metav1.APIResourceList{
					{
						GroupVersion: "apps/v1",
						APIResources: []metav1.APIResource{
							{Name: "deployments", Kind: "Deployment"},
							{Name: "deployments/scale", Kind: "Scale"},
						},
					},
				}
				return fakeDiscovery
			},
			want: true,
		},
		{
			name: "ConfigMap does not support scale",
			gvk:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			setupDiscovery: func() *fakediscovery.FakeDiscovery {
				client := fakeclientset.NewSimpleClientset()
				fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
				fakeDiscovery.Resources = []*metav1.APIResourceList{
					{
						GroupVersion: "v1",
						APIResources: []metav1.APIResource{
							{Name: "configmaps", Kind: "ConfigMap"},
						},
					},
				}
				return fakeDiscovery
			},
			want: false,
		},
		{
			name: "nil discovery client returns true",
			gvk:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			setupDiscovery: func() *fakediscovery.FakeDiscovery {
				return nil
			},
			want: true,
		},
		{
			name: "Kind not found in API group",
			gvk:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Unknown"},
			setupDiscovery: func() *fakediscovery.FakeDiscovery {
				client := fakeclientset.NewSimpleClientset()
				fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
				fakeDiscovery.Resources = []*metav1.APIResourceList{
					{
						GroupVersion: "apps/v1",
						APIResources: []metav1.APIResource{
							{Name: "deployments", Kind: "Deployment"},
						},
					},
				}
				return fakeDiscovery
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finder := &ControllerFinder{}
			discovery := tt.setupDiscovery()
			if discovery != nil {
				finder.discoveryClient = discovery
			}

			got := finder.implementsScale(tt.gvk)
			if got != tt.want {
				t.Errorf("implementsScale() = %v, want %v", got, tt.want)
			}
		})
	}
}
