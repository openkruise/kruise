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

package client

import (
	"testing"

	"k8s.io/apimachinery/pkg/version"
)

func TestDigitsOnly(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty string", in: "", want: ""},
		{name: "plain digits", in: "28", want: "28"},
		{name: "gke build metadata suffix", in: "32+gke", want: "32"},
		{name: "eks build metadata suffix", in: "30+eks", want: "30"},
		{name: "trailing plus", in: "28+", want: "28"},
		{name: "major with plus", in: "1+", want: "1"},
		{name: "all non-numeric", in: "+++", want: ""},
		{name: "letters only", in: "abc", want: ""},
		{name: "leading digits then letters", in: "1a2b3c", want: "1"},
		{name: "zero", in: "0", want: "0"},
		{name: "large number", in: "132", want: "132"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := digitsOnly(tc.in); got != tc.want {
				t.Errorf("digitsOnly(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestShouldUpdateResourceByResize(t *testing.T) {
	original := curVersion
	t.Cleanup(func() {
		curVersion = original
	})

	cases := []struct {
		name    string
		version *version.Info
		want    bool
	}{
		{
			name:    "exactly 1.32.0 returns true",
			version: &version.Info{Major: "1", Minor: "32", GitVersion: "v1.32.0"},
			want:    true,
		},
		{
			name:    "above 1.32 returns true",
			version: &version.Info{Major: "1", Minor: "33", GitVersion: "v1.33.0"},
			want:    true,
		},
		{
			name:    "below 1.32 returns false",
			version: &version.Info{Major: "1", Minor: "31", GitVersion: "v1.31.0"},
			want:    false,
		},
		{
			name:    "gke-style 1.28+gke.1 is cleaned to 1.28.1 and returns false",
			version: &version.Info{Major: "1", Minor: "28+", GitVersion: "v1.28+gke.1"},
			want:    false,
		},
		{
			name:    "gke-style 1.32+gke.1 is cleaned to 1.32.1 and returns true",
			version: &version.Info{Major: "1", Minor: "32+", GitVersion: "v1.32+gke.1"},
			want:    true,
		},
		{
			name:    "eks-style 1.30+eks.1 is cleaned to 1.30.1 and returns false",
			version: &version.Info{Major: "1", Minor: "30+", GitVersion: "v1.30+eks.1"},
			want:    false,
		},
		{
			name:    "1.28+.1 is cleaned to 1.28.1 and returns false",
			version: &version.Info{Major: "1", Minor: "28+", GitVersion: "v1.28+.1"},
			want:    false,
		},
		{
			name:    "extra segment after patch is folded into patch and cleaned to 1.32.0",
			version: &version.Info{Major: "1", Minor: "32", GitVersion: "v1.32.0.extra"},
			want:    true,
		},
		{
			name:    "empty GitVersion recovers from panic and returns false",
			version: &version.Info{Major: "1", Minor: ""},
			want:    false,
		},
		{
			name:    "non-semver garbage recovers from panic and returns false",
			version: &version.Info{Major: "1", Minor: "32", GitVersion: "garbage"},
			want:    false,
		},
		{
			name:    "missing patch segment recovers from panic and returns false",
			version: &version.Info{Major: "1", Minor: "32", GitVersion: "1.32"},
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			curVersion = tc.version
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ShouldUpdateResourceByResize leaked a panic for GitVersion=%q: %v",
						tc.version.GitVersion, r)
				}
			}()
			if got := ShouldUpdateResourceByResize(); got != tc.want {
				t.Errorf("ShouldUpdateResourceByResize() for GitVersion=%q = %v, want %v",
					tc.version.GitVersion, got, tc.want)
			}
		})
	}
}
