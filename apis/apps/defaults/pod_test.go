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

package defaults

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openkruise/kruise/pkg/features"
	_ "github.com/openkruise/kruise/pkg/util/feature"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

func TestSetDefaultPodSpec_DefaultHostNetworkHostPortsEnabled(t *testing.T) {
	// Enable the feature gate
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.DefaultHostNetworkHostPortsInPodTemplates, true)()

	tests := []struct {
		name     string
		input    *corev1.PodSpec
		expected *corev1.PodSpec
	}{
		{
			name: "HostNetwork enabled, HostPort not set in containers",
			input: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								Protocol:      corev1.ProtocolTCP,
							},
							{
								ContainerPort: 443,
							},
						},
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      80,
								Protocol:      corev1.ProtocolTCP,
							},
							{
								ContainerPort: 443,
								HostPort:      443,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
		{
			name: "HostNetwork enabled, HostPort already set in containers",
			input: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      8080,
							},
						},
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      8080,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
		{
			name: "HostNetwork enabled, HostPort not set in initContainers",
			input: &corev1.PodSpec{
				HostNetwork: true,
				InitContainers: []corev1.Container{
					{
						Name:  "init-container",
						Image: "busybox",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 8080,
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
							},
						},
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: true,
				InitContainers: []corev1.Container{
					{
						Name:  "init-container",
						Image: "busybox",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 8080,
								HostPort:      8080,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      80,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
		{
			name: "HostNetwork disabled, HostPort should not be set",
			input: &corev1.PodSpec{
				HostNetwork: false,
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
							},
						},
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: false,
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      0,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
		{
			name: "HostNetwork enabled, no ports defined",
			input: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
					},
				},
			},
		},
		{
			name: "HostNetwork enabled, multiple containers with mixed ports",
			input: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
							},
							{
								ContainerPort: 443,
								HostPort:      8443,
							},
						},
					},
					{
						Name:  "redis",
						Image: "redis:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 6379,
							},
						},
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      80,
								Protocol:      corev1.ProtocolTCP,
							},
							{
								ContainerPort: 443,
								HostPort:      8443,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
					{
						Name:            "redis",
						Image:           "redis:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 6379,
								HostPort:      6379,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDefaultPodSpec(tt.input)
			validatePodSpec(t, tt.input, tt.expected)
		})
	}
}

func TestSetDefaultPodSpec_DefaultHostNetworkHostPortsDisabled(t *testing.T) {
	// Disable the feature gate
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.DefaultHostNetworkHostPortsInPodTemplates, false)()

	tests := []struct {
		name     string
		input    *corev1.PodSpec
		expected *corev1.PodSpec
	}{
		{
			name: "HostNetwork enabled but feature disabled, HostPort should not be set",
			input: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
							},
						},
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      0,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
		{
			name: "HostNetwork enabled but feature disabled, existing HostPort should remain",
			input: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      8080,
							},
						},
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: true,
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      8080,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
		{
			name: "HostNetwork enabled but feature disabled, initContainers should not get HostPort",
			input: &corev1.PodSpec{
				HostNetwork: true,
				InitContainers: []corev1.Container{
					{
						Name:  "init",
						Image: "busybox",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 8080,
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
							},
						},
					},
				},
			},
			expected: &corev1.PodSpec{
				HostNetwork: true,
				InitContainers: []corev1.Container{
					{
						Name:  "init",
						Image: "busybox",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 8080,
								HostPort:      0,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
								HostPort:      0,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDefaultPodSpec(tt.input)
			validatePodSpec(t, tt.input, tt.expected)
		})
	}
}

func TestSetDefaultPodSpec_ImagePullPolicy(t *testing.T) {
	tests := []struct {
		name     string
		input    *corev1.PodSpec
		expected corev1.PullPolicy
	}{
		{
			name: "ImagePullPolicy not set, should default to Always",
			input: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
			},
			expected: corev1.PullAlways,
		},
		{
			name: "ImagePullPolicy already set to IfNotPresent",
			input: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullIfNotPresent,
					},
				},
			},
			expected: corev1.PullIfNotPresent,
		},
		{
			name: "ImagePullPolicy already set to Never",
			input: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:            "nginx",
						Image:           "nginx:latest",
						ImagePullPolicy: corev1.PullNever,
					},
				},
			},
			expected: corev1.PullNever,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDefaultPodSpec(tt.input)
			if tt.input.Containers[0].ImagePullPolicy != tt.expected {
				t.Errorf("Expected ImagePullPolicy %v, got %v", tt.expected, tt.input.Containers[0].ImagePullPolicy)
			}
		})
	}
}

func TestSetDefaultPodSpec_ProtocolDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    *corev1.PodSpec
		expected corev1.Protocol
	}{
		{
			name: "Protocol not set in container port",
			input: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 80,
							},
						},
					},
				},
			},
			expected: corev1.ProtocolTCP,
		},
		{
			name: "Protocol not set in init container port",
			input: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name:  "init",
						Image: "busybox",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 8080,
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
			},
			expected: corev1.ProtocolTCP,
		},
		{
			name: "Protocol not set in ephemeral container port",
			input: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
				EphemeralContainers: []corev1.EphemeralContainer{
					{
						EphemeralContainerCommon: corev1.EphemeralContainerCommon{
							Name:  "debug",
							Image: "busybox",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 9999,
								},
							},
						},
					},
				},
			},
			expected: corev1.ProtocolTCP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDefaultPodSpec(tt.input)
			if len(tt.input.Containers) > 0 && len(tt.input.Containers[0].Ports) > 0 {
				if tt.input.Containers[0].Ports[0].Protocol != tt.expected {
					t.Errorf("Expected Protocol %v in containers, got %v", tt.expected, tt.input.Containers[0].Ports[0].Protocol)
				}
			}
			if len(tt.input.InitContainers) > 0 && len(tt.input.InitContainers[0].Ports) > 0 {
				if tt.input.InitContainers[0].Ports[0].Protocol != tt.expected {
					t.Errorf("Expected Protocol %v in initContainers, got %v", tt.expected, tt.input.InitContainers[0].Ports[0].Protocol)
				}
			}
			if len(tt.input.EphemeralContainers) > 0 && len(tt.input.EphemeralContainers[0].Ports) > 0 {
				if tt.input.EphemeralContainers[0].Ports[0].Protocol != tt.expected {
					t.Errorf("Expected Protocol %v in ephemeralContainers, got %v", tt.expected, tt.input.EphemeralContainers[0].Ports[0].Protocol)
				}
			}
		})
	}
}

func TestSetDefaultPodSpec_ProbeDefaults(t *testing.T) {
	tests := []struct {
		name  string
		input *corev1.PodSpec
	}{
		{
			name: "HTTPGet probe with defaults",
			input: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromInt(8080),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/ready",
									Port: intstr.FromInt(8080),
								},
							},
						},
						StartupProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/startup",
									Port: intstr.FromInt(8080),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "InitContainer with probes",
			input: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name:  "init",
						Image: "busybox",
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromInt(8080),
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			SetDefaultPodSpec(tt.input)
		})
	}
}

func TestSetDefaultPodVolumes(t *testing.T) {
	tests := []struct {
		name  string
		input []corev1.Volume
	}{
		{
			name: "HostPath volume",
			input: []corev1.Volume{
				{
					Name: "host-vol",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/data",
						},
					},
				},
			},
		},
		{
			name: "Secret volume",
			input: []corev1.Volume{
				{
					Name: "secret-vol",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "my-secret",
						},
					},
				},
			},
		},
		{
			name: "ConfigMap volume",
			input: []corev1.Volume{
				{
					Name: "config-vol",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "my-config",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			SetDefaultPodVolumes(tt.input)
		})
	}
}

func TestDefaultHostNetworkPorts(t *testing.T) {
	tests := []struct {
		name     string
		input    []corev1.Container
		expected []corev1.Container
	}{
		{
			name: "Single container with single port, HostPort not set",
			input: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
						},
					},
				},
			},
			expected: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
							HostPort:      80,
						},
					},
				},
			},
		},
		{
			name: "Single container with multiple ports, HostPort not set",
			input: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
						},
						{
							ContainerPort: 443,
						},
					},
				},
			},
			expected: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
							HostPort:      80,
						},
						{
							ContainerPort: 443,
							HostPort:      443,
						},
					},
				},
			},
		},
		{
			name: "Single container with HostPort already set",
			input: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
							HostPort:      8080,
						},
					},
				},
			},
			expected: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
							HostPort:      8080,
						},
					},
				},
			},
		},
		{
			name: "Multiple containers with mixed HostPort settings",
			input: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
						},
						{
							ContainerPort: 443,
							HostPort:      8443,
						},
					},
				},
				{
					Name:  "redis",
					Image: "redis:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 6379,
						},
					},
				},
			},
			expected: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 80,
							HostPort:      80,
						},
						{
							ContainerPort: 443,
							HostPort:      8443,
						},
					},
				},
				{
					Name:  "redis",
					Image: "redis:latest",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 6379,
							HostPort:      6379,
						},
					},
				},
			},
		},
		{
			name: "Container with no ports",
			input: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
			expected: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultHostNetworkPorts(&tt.input)
			validateContainers(t, tt.input, tt.expected)
		})
	}
}

// Helper function to validate PodSpec
func validatePodSpec(t *testing.T, actual, expected *corev1.PodSpec) {
	if actual.HostNetwork != expected.HostNetwork {
		t.Errorf("Expected HostNetwork %v, got %v", expected.HostNetwork, actual.HostNetwork)
	}

	validateContainers(t, actual.Containers, expected.Containers)
	validateContainers(t, actual.InitContainers, expected.InitContainers)
}

// Helper function to validate containers
func validateContainers(t *testing.T, actual, expected []corev1.Container) {
	if len(actual) != len(expected) {
		t.Fatalf("Expected %d containers, got %d", len(expected), len(actual))
	}

	for i := range actual {
		if actual[i].Name != expected[i].Name {
			t.Errorf("Container %d: Expected Name %s, got %s", i, expected[i].Name, actual[i].Name)
		}
		if actual[i].Image != expected[i].Image {
			t.Errorf("Container %d: Expected Image %s, got %s", i, expected[i].Image, actual[i].Image)
		}
		if expected[i].ImagePullPolicy != "" && actual[i].ImagePullPolicy != expected[i].ImagePullPolicy {
			t.Errorf("Container %d: Expected ImagePullPolicy %s, got %s", i, expected[i].ImagePullPolicy, actual[i].ImagePullPolicy)
		}

		if len(actual[i].Ports) != len(expected[i].Ports) {
			t.Fatalf("Container %d: Expected %d ports, got %d", i, len(expected[i].Ports), len(actual[i].Ports))
		}

		for j := range actual[i].Ports {
			if actual[i].Ports[j].ContainerPort != expected[i].Ports[j].ContainerPort {
				t.Errorf("Container %d, Port %d: Expected ContainerPort %d, got %d",
					i, j, expected[i].Ports[j].ContainerPort, actual[i].Ports[j].ContainerPort)
			}
			if actual[i].Ports[j].HostPort != expected[i].Ports[j].HostPort {
				t.Errorf("Container %d, Port %d: Expected HostPort %d, got %d",
					i, j, expected[i].Ports[j].HostPort, actual[i].Ports[j].HostPort)
			}
			if actual[i].Ports[j].Protocol != expected[i].Ports[j].Protocol {
				t.Errorf("Container %d, Port %d: Expected Protocol %s, got %s",
					i, j, expected[i].Ports[j].Protocol, actual[i].Ports[j].Protocol)
			}
		}
	}
}
