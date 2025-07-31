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

package convertor

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestConvertPodTemplateSpec(t *testing.T) {
	template := &v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name:  "nginx",
				Image: "nginx:latest",
			}},
		},
	}
	converted, err := ConvertPodTemplateSpec(template)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(converted.Spec.Containers) != 1 || converted.Spec.Containers[0].Name != "nginx" {
		t.Errorf("Conversion failed, got %+v", converted)
	}
}

func TestConvertPod(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name:  "test",
				Image: "busybox",
			}},
		},
	}
	result, err := ConvertPod(pod)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(result.Spec.Containers) != 1 || result.Spec.Containers[0].Name != "test" {
		t.Errorf("Unexpected result: %+v", result)
	}
}

func TestConvertCoreVolumes(t *testing.T) {
	vols := []v1.Volume{{
		Name: "data",
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}}
	coreVols, err := ConvertCoreVolumes(vols)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(coreVols) != 1 || coreVols[0].Name != "data" {
		t.Errorf("Unexpected result: %+v", coreVols)
	}
}

func TestConvertEphemeralContainer(t *testing.T) {
	ec := []v1.EphemeralContainer{{
		EphemeralContainerCommon: v1.EphemeralContainerCommon{
			Name:  "debugger",
			Image: "busybox",
		},
	}}
	coreEC, err := ConvertEphemeralContainer(ec)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(coreEC) != 1 || coreEC[0].Name != "debugger" {
		t.Errorf("Unexpected result: %+v", coreEC)
	}
}

func TestGetPercentValue(t *testing.T) {
	tests := []struct {
		input    intstr.IntOrString
		expected int
		ok       bool
	}{
		{intstr.FromString("50%"), 50, true},
		{intstr.FromString("100%"), 100, true},
		{intstr.FromString("100"), 0, false},
		{intstr.FromInt(30), 0, false},
		{intstr.FromString("abc%"), 0, false},
	}

	for _, test := range tests {
		val, ok := GetPercentValue(test.input)
		if val != test.expected || ok != test.ok {
			t.Errorf("input: %v, expected (%d, %v), got (%d, %v)", test.input, test.expected, test.ok, val, ok)
		}
	}
}

func TestGetIntOrPercentValue(t *testing.T) {
	tests := []struct {
		input    intstr.IntOrString
		expected int
	}{
		{intstr.FromString("40%"), 40},
		{intstr.FromInt(60), 60},
	}

	for _, test := range tests {
		result := GetIntOrPercentValue(test.input)
		if result != test.expected {
			t.Errorf("input: %v, expected: %d, got: %d", test.input, test.expected, result)
		}
	}
}
