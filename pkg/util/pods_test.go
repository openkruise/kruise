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

package util

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestMergeVolumeMounts(t *testing.T) {
	original := []v1.VolumeMount{
		{
			MountPath: "/origin-1",
		},
		{
			MountPath: "/share",
		},
	}
	additional := []v1.VolumeMount{
		{
			MountPath: "/addition-1",
		},
		{
			MountPath: "/share",
		},
	}

	volumeMounts := MergeVolumeMounts(original, additional)
	excepts := []string{"/origin-1", "/share", "/addition-1"}
	for i, except := range excepts {
		if volumeMounts[i].MountPath != except {
			t.Fatalf("except VolumeMount(%s), but get %s", except, volumeMounts[i].MountPath)
		}
	}
}

func TestMergeEnvVars(t *testing.T) {
	original := []v1.EnvVar{
		{
			Name: "origin-1",
		},
		{
			Name: "share",
		},
	}
	additional := []v1.EnvVar{
		{
			Name: "addition-1",
		},
		{
			Name: "share",
		},
	}

	envVars := MergeEnvVar(original, additional)
	excepts := []string{"origin-1", "share", "addition-1"}
	for i, except := range excepts {
		if envVars[i].Name != except {
			t.Fatalf("except EnvVar(%s), but get %s", except, envVars[i].Name)
		}
	}
}

func TestMergeVolumes(t *testing.T) {
	original := []v1.Volume{
		{
			Name: "origin-1",
		},
		{
			Name: "share",
		},
	}
	additional := []v1.Volume{
		{
			Name: "addition-1",
		},
		{
			Name: "share",
		},
	}

	volumes := MergeVolumes(original, additional)
	excepts := []string{"origin-1", "share", "addition-1"}
	for i, except := range excepts {
		if volumes[i].Name != except {
			t.Fatalf("except EnvVar(%s), but get %s", except, volumes[i].Name)
		}
	}
}
