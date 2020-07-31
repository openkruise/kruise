/*
Copyright 2020 The Kruise Authors.

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

package daemonset

import (
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_maxRevision(t *testing.T) {
	type args struct {
		histories []*apps.ControllerRevision
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "GetMaxRevision",
			args: args{
				histories: []*apps.ControllerRevision{
					{
						Revision: 123456789,
					},
					{
						Revision: 213456789,
					},
					{
						Revision: 312456789,
					},
				},
			},
			want: 312456789,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxRevision(tt.args.histories); got != tt.want {
				t.Errorf("maxRevision() = %v, want %v", got, tt.want)
			}
			t.Logf("maxRevision() = %v", tt.want)
		})
	}
}

func TestGetTemplateGeneration(t *testing.T) {
	type args struct {
		ds *appsv1alpha1.DaemonSet
	}
	constNum := int64(1000)
	tests := []struct {
		name    string
		args    args
		want    *int64
		wantErr bool
	}{
		{
			name: "GetTemplateGeneration",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							apps.DeprecatedTemplateGeneration: "1000",
						},
					},
					Spec:   appsv1alpha1.DaemonSetSpec{},
					Status: appsv1alpha1.DaemonSetStatus{},
				},
			},
			want:    &constNum,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTemplateGeneration(tt.args.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTemplateGeneration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if *got != *tt.want {
				t.Errorf("GetTemplateGeneration() = %v, want %v", got, tt.want)
			}
		})
	}
}
