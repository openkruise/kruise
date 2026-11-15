/*
Copyright 2026 The Kruise Authors.

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

package workloadspread

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openkruise/kruise/pkg/util/configuration"
)

func TestWatchableCustomWorkloads(t *testing.T) {
	complete := configuration.CustomWorkload{
		GroupVersionKind: schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"},
		ReplicasPath:     "spec.replicas",
	}
	noVersion := configuration.CustomWorkload{
		GroupVersionKind: schema.GroupVersionKind{Group: "argoproj.io", Kind: "Rollout"},
		ReplicasPath:     "spec.replicas",
	}
	noKind := configuration.CustomWorkload{
		GroupVersionKind: schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1"},
	}

	cases := []struct {
		name string
		in   []configuration.CustomWorkload
		want int
	}{
		{name: "empty whitelist", in: nil, want: 0},
		{name: "complete entry is kept", in: []configuration.CustomWorkload{complete}, want: 1},
		{name: "entry without version is dropped", in: []configuration.CustomWorkload{noVersion}, want: 0},
		{name: "entry without kind is dropped", in: []configuration.CustomWorkload{noKind}, want: 0},
		{name: "bad entries do not drop good ones", in: []configuration.CustomWorkload{noVersion, complete, noKind}, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := watchableCustomWorkloads(tc.in)
			if len(got) != tc.want {
				t.Fatalf("expected %d watchable workloads, got %d: %v", tc.want, len(got), got)
			}
			for _, workload := range got {
				if workload.Version == "" || workload.Kind == "" {
					t.Fatalf("incomplete entry was kept: %v", workload)
				}
			}
		})
	}
}
