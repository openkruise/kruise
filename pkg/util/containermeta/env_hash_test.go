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

package containermeta

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func fakeEnvGetter(m map[string]string) EnvGetter {
	return func(key string) (string, error) {
		return m[key], nil
	}
}

func TestEnvFromMetadataHasher(t *testing.T) {
	objMeta := &metav1.ObjectMeta{
		Labels: map[string]string{
			"l1": "foo",
			"l2": "bar",
		},
	}

	cases := []struct {
		c1            *v1.Container
		c2            *v1.Container
		realEnvs      map[string]string
		expectedEqual bool
	}{
		{
			c1: &v1.Container{Env: []v1.EnvVar{
				{Name: "e1", Value: "a"},
				{Name: "e2", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['l1']"}}},
			}},
			c2: &v1.Container{Env: []v1.EnvVar{
				{Name: "e1", Value: "a"},
				{Name: "e2", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['l1']"}}},
				{Name: "e3", Value: "c"},
			}},
			realEnvs:      map[string]string{"e2": "bar"},
			expectedEqual: false,
		},
		{
			c1: &v1.Container{Env: []v1.EnvVar{
				{Name: "e1", Value: "a"},
				{Name: "e2", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['l1']"}}},
			}},
			c2: &v1.Container{Env: []v1.EnvVar{
				{Name: "e2", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['l1']"}}},
				{Name: "e1", Value: "a"},
			}},
			realEnvs:      map[string]string{"e2": "foo"},
			expectedEqual: true,
		},
	}

	hasher := NewEnvFromMetadataHasher()
	for i, tc := range cases {
		expectHash := hasher.GetExpectHash(tc.c1, objMeta)
		realHash, _ := hasher.GetCurrentHash(tc.c2, fakeEnvGetter(tc.realEnvs))
		if (expectHash == realHash) != tc.expectedEqual {
			t.Fatalf("#%v failed, expected hash %v, real hash %v", i, expectHash, realHash)
		}
	}
}
