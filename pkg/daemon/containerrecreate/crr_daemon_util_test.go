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

package containerrecreate

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func makeCRR(podName, podUID string, containers []appsv1beta1.ContainerRecreateRequestContainer, gracePeriod *int64) *appsv1beta1.ContainerRecreateRequest {
	crr := &appsv1beta1.ContainerRecreateRequest{}
	crr.Namespace = "default"
	crr.Name = "test-crr"
	crr.Labels = map[string]string{
		appsv1beta1.ContainerRecreateRequestPodUIDKey: podUID,
	}
	crr.Spec.PodName = podName
	crr.Spec.Containers = containers
	if gracePeriod != nil {
		crr.Spec.Strategy = &appsv1beta1.ContainerRecreateRequestStrategy{
			TerminationGracePeriodSeconds: gracePeriod,
		}
	}
	return crr
}

// TestConvertCRRToPod_PodIPIsSet verifies that the pod IP passed to
// convertCRRToPod is reflected in the resulting fake pod's Status.
func TestConvertCRRToPod_PodIPIsSet(t *testing.T) {
	crr := makeCRR("my-pod", "abc-123", nil, nil)
	pod := convertCRRToPod(crr, "10.0.0.42")

	if pod.Status.PodIP != "10.0.0.42" {
		t.Errorf("expected PodIP 10.0.0.42, got %q", pod.Status.PodIP)
	}
	if pod.Name != "my-pod" {
		t.Errorf("expected pod name my-pod, got %q", pod.Name)
	}
	if string(pod.UID) != "abc-123" {
		t.Errorf("expected UID abc-123, got %q", pod.UID)
	}
}

// TestConvertCRRToPod_EmptyPodIP verifies that an empty pod IP is handled
// gracefully (TCPSocket host fallback logic lives in the daemon executor).
func TestConvertCRRToPod_EmptyPodIP(t *testing.T) {
	crr := makeCRR("my-pod", "abc-123", nil, nil)
	pod := convertCRRToPod(crr, "")

	if pod.Status.PodIP != "" {
		t.Errorf("expected empty PodIP, got %q", pod.Status.PodIP)
	}
}

// TestConvertCRRToPod_DefaultGracePeriod verifies that TerminationGracePeriodSeconds
// defaults to 30s when the CRR strategy does not specify one.
func TestConvertCRRToPod_DefaultGracePeriod(t *testing.T) {
	crr := makeCRR("my-pod", "abc-123", nil, nil)
	pod := convertCRRToPod(crr, "")

	if pod.Spec.TerminationGracePeriodSeconds == nil {
		t.Fatal("expected TerminationGracePeriodSeconds to be set")
	}
	if *pod.Spec.TerminationGracePeriodSeconds != 30 {
		t.Errorf("expected 30, got %d", *pod.Spec.TerminationGracePeriodSeconds)
	}
}

// TestConvertCRRToPod_CustomGracePeriod verifies that a custom grace period from
// the CRR strategy is propagated to the fake pod.
func TestConvertCRRToPod_CustomGracePeriod(t *testing.T) {
	crr := makeCRR("my-pod", "abc-123", nil, ptr.To(int64(60)))
	pod := convertCRRToPod(crr, "")

	if pod.Spec.TerminationGracePeriodSeconds == nil || *pod.Spec.TerminationGracePeriodSeconds != 60 {
		t.Errorf("expected grace period 60")
	}
}

// TestConvertCRRToPod_TCPSocketPreStop verifies that a TCPSocket preStop hook is
// correctly reconstructed in the fake pod's container lifecycle spec.
func TestConvertCRRToPod_TCPSocketPreStop(t *testing.T) {
	containers := []appsv1beta1.ContainerRecreateRequestContainer{
		{
			Name: "app",
			PreStop: &appsv1beta1.CRRProbeHandler{
				TCPSocket: &v1.TCPSocketAction{
					Port: intstr.FromInt(9000),
					Host: "127.0.0.1",
				},
			},
			Ports: []v1.ContainerPort{
				{Name: "shutdown", ContainerPort: 9000},
			},
		},
	}
	crr := makeCRR("my-pod", "abc-123", containers, nil)
	pod := convertCRRToPod(crr, "10.0.0.1")

	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(pod.Spec.Containers))
	}
	c := pod.Spec.Containers[0]
	if c.Lifecycle == nil || c.Lifecycle.PreStop == nil {
		t.Fatal("expected Lifecycle.PreStop to be set")
	}
	if c.Lifecycle.PreStop.TCPSocket == nil {
		t.Fatal("expected TCPSocket to be set in PreStop")
	}
	if c.Lifecycle.PreStop.TCPSocket.Port.IntValue() != 9000 {
		t.Errorf("expected port 9000, got %v", c.Lifecycle.PreStop.TCPSocket.Port)
	}
	if len(c.Ports) != 1 || c.Ports[0].Name != "shutdown" {
		t.Error("expected container port 'shutdown' to be set for named-port resolution")
	}
}

// TestConvertCRRToPod_NoPreStop verifies that a container without a preStop hook
// produces no Lifecycle entry in the fake pod.
func TestConvertCRRToPod_NoPreStop(t *testing.T) {
	containers := []appsv1beta1.ContainerRecreateRequestContainer{
		{Name: "app"},
	}
	crr := makeCRR("my-pod", "abc-123", containers, nil)
	pod := convertCRRToPod(crr, "")

	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(pod.Spec.Containers))
	}
	if pod.Spec.Containers[0].Lifecycle != nil {
		t.Error("expected no Lifecycle for container without preStop")
	}
}
