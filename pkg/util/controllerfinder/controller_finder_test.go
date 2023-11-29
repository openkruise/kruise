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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
