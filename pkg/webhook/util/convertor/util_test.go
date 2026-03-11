/*
Copyright 2025 The Kruise Authors.

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

package convertor

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/apis/core"
)

func TestVolumesFromVolumeClaimTemplates(t *testing.T) {
	cases := []struct {
		name      string
		templates []v1.PersistentVolumeClaim
		expected  []core.Volume
	}{
		{
			name:      "empty templates",
			templates: []v1.PersistentVolumeClaim{},
			expected:  []core.Volume{},
		},
		{
			name: "single template",
			templates: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.VolumeResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			expected: []core.Volume{
				{
					Name: "data",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: "data",
						},
					},
				},
			},
		},
		{
			name: "multiple templates",
			templates: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "logs"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cache"},
				},
			},
			expected: []core.Volume{
				{
					Name: "data",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: "data",
						},
					},
				},
				{
					Name: "logs",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: "logs",
						},
					},
				},
				{
					Name: "cache",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: "cache",
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := VolumesFromVolumeClaimTemplates(tc.templates)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("expected %+v, got %+v", tc.expected, result)
			}
		})
	}
}

func TestGetPercentValue(t *testing.T) {
	cases := []struct {
		name          string
		input         intstr.IntOrString
		expectedValue int
		expectedOk    bool
	}{
		{
			name:          "valid percent 50%",
			input:         intstr.FromString("50%"),
			expectedValue: 50,
			expectedOk:    true,
		},
		{
			name:          "valid percent 100%",
			input:         intstr.FromString("100%"),
			expectedValue: 100,
			expectedOk:    true,
		},
		{
			name:          "valid percent 0%",
			input:         intstr.FromString("0%"),
			expectedValue: 0,
			expectedOk:    true,
		},
		{
			name:          "int value not percent",
			input:         intstr.FromInt32(50),
			expectedValue: 0,
			expectedOk:    false,
		},
		{
			name:          "string without percent sign",
			input:         intstr.FromString("50"),
			expectedValue: 0,
			expectedOk:    false,
		},
		{
			name:          "invalid percent format",
			input:         intstr.FromString("abc%"),
			expectedValue: 0,
			expectedOk:    false,
		},
		{
			name:          "negative percent",
			input:         intstr.FromString("-10%"),
			expectedValue: 0,
			expectedOk:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := GetPercentValue(tc.input)
			if value != tc.expectedValue {
				t.Errorf("expected value %d, got %d", tc.expectedValue, value)
			}
			if ok != tc.expectedOk {
				t.Errorf("expected ok %v, got %v", tc.expectedOk, ok)
			}
		})
	}
}

func TestGetIntOrPercentValue(t *testing.T) {
	cases := []struct {
		name     string
		input    intstr.IntOrString
		expected int
	}{
		{
			name:     "percent value 50%",
			input:    intstr.FromString("50%"),
			expected: 50,
		},
		{
			name:     "percent value 100%",
			input:    intstr.FromString("100%"),
			expected: 100,
		},
		{
			name:     "int value 25",
			input:    intstr.FromInt32(25),
			expected: 25,
		},
		{
			name:     "int value 0",
			input:    intstr.FromInt32(0),
			expected: 0,
		},
		{
			name:     "string without percent falls back to int",
			input:    intstr.FromString("invalid"),
			expected: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetIntOrPercentValue(tc.input)
			if result != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, result)
			}
		})
	}
}
