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
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

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

func TestGetContainer(t *testing.T) {
	tests := []struct {
		pod       *v1.Pod
		name      string
		container *v1.Container
	}{
		// case 0: found exist container
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "c0",
							Image: "c0",
						},
						{
							Name:  "c1",
							Image: "c1",
						},
					},
				},
			},
			name: "c1",
			container: &v1.Container{
				Name:  "c1",
				Image: "c1",
			},
		},

		// case 1: found exist init container
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{
						{
							Name:  "c0",
							Image: "c0",
						},
						{
							Name:  "c1",
							Image: "c1",
						},
					},
				},
			},
			name: "c1",
			container: &v1.Container{
				Name:  "c1",
				Image: "c1",
			},
		},

		// case 2: pod is nil
		{
			pod:       nil,
			name:      "c2",
			container: nil,
		},

		// case 3: not found container name
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "c0",
							Image: "c0",
						},
						{
							Name:  "c1",
							Image: "c1",
						},
					},
				},
			},
			name:      "c2",
			container: nil,
		},
	}

	for i, test := range tests {
		expect := test.container
		actual := GetContainer(test.name, test.pod)
		if !reflect.DeepEqual(expect, actual) {
			t.Fatalf("case %d: expect container(%v), but get %v", i, expect, actual)
		}
	}
}

func TestGetContainerStatus(t *testing.T) {
	tests := []struct {
		pod             *v1.Pod
		name            string
		containerStatus *v1.ContainerStatus
	}{
		// case 0: found exist container
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:  "c0",
							Image: "c0",
						},
						{
							Name:  "c1",
							Image: "c1",
						},
					},
				},
			},
			name: "c1",
			containerStatus: &v1.ContainerStatus{
				Name:  "c1",
				Image: "c1",
			},
		},

		// case 1: pod is nil
		{
			pod:             nil,
			name:            "c2",
			containerStatus: nil,
		},

		// case 2: not found container name
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:  "c0",
							Image: "c0",
						},
						{
							Name:  "c1",
							Image: "c1",
						},
					},
				},
			},
			name:            "c2",
			containerStatus: nil,
		},
	}

	for i, test := range tests {
		expect := test.containerStatus
		actual := GetContainerStatus(test.name, test.pod)
		if !reflect.DeepEqual(expect, actual) {
			t.Fatalf("case %d: expect containerStatus(%v), but get %v", i, expect, actual)
		}
	}
}

func TestGetPodContainerImageIDs(t *testing.T) {
	tests := []struct {
		pod       *v1.Pod
		cImageIDs map[string]string
	}{
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:    "c-1",
							ImageID: "registry.cn-hangzhou.aliyuncs.com/acs/minecraft-demo@sha256:f68fd7d5e6133c511b374a38f7dbc35acedce1d177dd78fba1d62d6264d5cba0",
						},
						{
							Name:    "c-2",
							ImageID: "docker-pullable://busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
						},
					},
				},
			},
			cImageIDs: map[string]string{
				"c-1": "registry.cn-hangzhou.aliyuncs.com/acs/minecraft-demo@sha256:f68fd7d5e6133c511b374a38f7dbc35acedce1d177dd78fba1d62d6264d5cba0",
				"c-2": "busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
			},
		},
	}

	for i, test := range tests {
		expect := test.cImageIDs
		actual := GetPodContainerImageIDs(test.pod)
		if !reflect.DeepEqual(expect, actual) {
			t.Fatalf("case %d: expect cImageIDs(%v), but get %v", i, expect, actual)
		}
	}
}

func TestIsPodContainerDigestEqual(t *testing.T) {
	tests := []struct {
		pod        *v1.Pod
		containers sets.String
		result     bool
	}{
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "c-1",
							Image: "registry.cn-hangzhou.aliyuncs.com/acs/minecraft-demo@sha256:f68fd7d5e6133c511b374a38f7dbc35acedce1d177dd78fba1d62d6264d5cba0",
						},
						{
							Name:  "c-2",
							Image: "busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
						},
					},
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:    "c-1",
							ImageID: "registry.cn-hangzhou.aliyuncs.com/acs/minecraft-demo@sha256:f68fd7d5e6133c511b374a38f7dbc35acedce1d177dd78fba1d62d6264d5cba0",
						},
						{
							Name:    "c-2",
							ImageID: "docker-pullable://busybox@sha256:a9286defaba7b3a519d585ba0e37d0b2cbee74ebfe590960b0b1d6a5e97d1e1d",
						},
					},
				},
			},
			containers: sets.NewString("c-1", "c-2"),
			result:     true,
		},
	}

	for i, test := range tests {
		expect := test.result
		actual := IsPodContainerDigestEqual(test.containers, test.pod)
		if !reflect.DeepEqual(expect, actual) {
			t.Fatalf("case %d: expect result(%v), but get %v", i, expect, actual)
		}
	}
}

func TestSetPodConditionIfMsgChanged(t *testing.T) {
	tests := []struct {
		pod        *v1.Pod
		condition  v1.PodCondition
		conditions []v1.PodCondition
	}{
		// case 0: existed condition status changed
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    "type-0",
							Status:  v1.ConditionTrue,
							Message: "Msg-0",
						},
						{
							Type:   "type-1",
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			condition: v1.PodCondition{
				Type:   "type-0",
				Status: v1.ConditionFalse,
			},
			conditions: []v1.PodCondition{
				{
					Type:   "type-0",
					Status: v1.ConditionFalse,
				},
				{
					Type:   "type-1",
					Status: v1.ConditionFalse,
				},
			},
		},

		// case 1: add a new condition
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    "type-0",
							Status:  v1.ConditionTrue,
							Message: "Msg-0",
						},
						{
							Type:   "type-1",
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			condition: v1.PodCondition{
				Type:   "type-2",
				Status: v1.ConditionFalse,
			},
			conditions: []v1.PodCondition{
				{
					Type:    "type-0",
					Status:  v1.ConditionTrue,
					Message: "Msg-0",
				},
				{
					Type:   "type-1",
					Status: v1.ConditionFalse,
				},
				{
					Type:   "type-2",
					Status: v1.ConditionFalse,
				},
			},
		},

		// case 2: existed condition status not changed, but message changed
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    "type-0",
							Status:  v1.ConditionTrue,
							Message: "Msg-0",
						},
						{
							Type:   "type-1",
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			condition: v1.PodCondition{
				Type:    "type-0",
				Status:  v1.ConditionTrue,
				Message: "Msg-Changed",
			},
			conditions: []v1.PodCondition{
				{
					Type:    "type-0",
					Status:  v1.ConditionTrue,
					Message: "Msg-Changed",
				},
				{
					Type:   "type-1",
					Status: v1.ConditionFalse,
				},
			},
		},
	}

	for i, test := range tests {
		expect := test.conditions
		SetPodConditionIfMsgChanged(test.pod, test.condition)
		actual := test.pod.Status.Conditions
		if !reflect.DeepEqual(expect, actual) {
			t.Fatalf("case %d: expect Conditions(%s), but get %s", i, expect, actual)
		}
	}
}

func TestSetPodCondition(t *testing.T) {
	tests := []struct {
		pod        *v1.Pod
		condition  v1.PodCondition
		conditions []v1.PodCondition
	}{
		// case 0: existed condition status changed
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    "type-0",
							Status:  v1.ConditionTrue,
							Message: "Msg-0",
						},
						{
							Type:   "type-1",
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			condition: v1.PodCondition{
				Type:   "type-0",
				Status: v1.ConditionFalse,
			},
			conditions: []v1.PodCondition{
				{
					Type:   "type-0",
					Status: v1.ConditionFalse,
				},
				{
					Type:   "type-1",
					Status: v1.ConditionFalse,
				},
			},
		},

		// case 1: add a new condition
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    "type-0",
							Status:  v1.ConditionTrue,
							Message: "Msg-0",
						},
						{
							Type:   "type-1",
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			condition: v1.PodCondition{
				Type:   "type-2",
				Status: v1.ConditionFalse,
			},
			conditions: []v1.PodCondition{
				{
					Type:    "type-0",
					Status:  v1.ConditionTrue,
					Message: "Msg-0",
				},
				{
					Type:   "type-1",
					Status: v1.ConditionFalse,
				},
				{
					Type:   "type-2",
					Status: v1.ConditionFalse,
				},
			},
		},

		// case 2: existed condition status not changed, message should not be changed
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    "type-0",
							Status:  v1.ConditionTrue,
							Message: "Msg-0",
						},
						{
							Type:   "type-1",
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			condition: v1.PodCondition{
				Type:    "type-0",
				Status:  v1.ConditionTrue,
				Message: "Msg-Changed",
			},
			conditions: []v1.PodCondition{
				{
					Type:    "type-0",
					Status:  v1.ConditionTrue,
					Message: "Msg-0",
				},
				{
					Type:   "type-1",
					Status: v1.ConditionFalse,
				},
			},
		},
	}

	for i, test := range tests {
		expect := test.conditions
		SetPodCondition(test.pod, test.condition)
		actual := test.pod.Status.Conditions
		if !reflect.DeepEqual(expect, actual) {
			t.Fatalf("case %d: expect Conditions(%s), but get %s", i, expect, actual)
		}
	}
}

func TestSetPodReadyCondition(t *testing.T) {
	tests := []struct {
		pod            *v1.Pod
		podReadyStatus v1.ConditionStatus
	}{
		// case 0: container not ready
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionFalse,
						},
						{
							Type:   v1.PodReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			podReadyStatus: v1.ConditionFalse,
		},

		// case 1: ReadinessGates exist, but condition not exit
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{
						{
							ConditionType: "type-A",
						},
					},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			podReadyStatus: v1.ConditionFalse,
		},

		// case 2: ReadinessGates exist, but condition not true
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{
						{
							ConditionType: "type-A",
						},
					},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   "type-A",
							Status: v1.ConditionFalse,
						},
						{
							Type:   v1.PodReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			podReadyStatus: v1.ConditionFalse,
		},

		// case 3: ReadinessGates exist, but condition is not true
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{
						{
							ConditionType: "type-A",
						},
					},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   "type-A",
							Status: v1.ConditionFalse,
						},
						{
							Type:   v1.PodReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			podReadyStatus: v1.ConditionFalse,
		},

		// case 4: ReadinessGates exist, and condition is true
		{
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					ReadinessGates: []v1.PodReadinessGate{
						{
							ConditionType: "type-A",
						},
					},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   "type-A",
							Status: v1.ConditionTrue,
						},
						{
							Type:   v1.PodReady,
							Status: v1.ConditionFalse,
						},
						{
							Type:   v1.ContainersReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			podReadyStatus: v1.ConditionTrue,
		},
	}

	for i, test := range tests {
		expect := test.podReadyStatus
		SetPodReadyCondition(test.pod)
		actual := GetCondition(test.pod, v1.PodReady).Status
		if expect != actual {
			t.Fatalf("case %d: expect PodReady Conditions(%s), but get %s", i, expect, actual)
		}
	}
}
