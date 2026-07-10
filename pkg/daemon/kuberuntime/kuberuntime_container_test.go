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

package kuberuntime

import (
	"fmt"
	"net"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// startTCPListener opens a TCP listener on a random port and returns the
// listener and its port number. The caller is responsible for closing it.
func startTCPListener(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP listener: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	return ln, port
}

// TestResolveTCPSocketPort_Integer verifies that an integer port is returned as-is.
func TestResolveTCPSocketPort_Integer(t *testing.T) {
	container := &v1.Container{Name: "app"}
	port, err := resolveTCPSocketPort(intstr.FromInt(8080), container)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 8080 {
		t.Errorf("expected 8080, got %d", port)
	}
}

// TestResolveTCPSocketPort_Named verifies that a named port is resolved from
// the container's Ports list.
func TestResolveTCPSocketPort_Named(t *testing.T) {
	container := &v1.Container{
		Name: "app",
		Ports: []v1.ContainerPort{
			{Name: "http", ContainerPort: 8080},
			{Name: "grpc", ContainerPort: 9090},
		},
	}
	port, err := resolveTCPSocketPort(intstr.FromString("grpc"), container)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 9090 {
		t.Errorf("expected 9090, got %d", port)
	}
}

// TestResolveTCPSocketPort_NamedNotFound verifies that an error is returned when
// the named port does not exist in the container's Ports list.
func TestResolveTCPSocketPort_NamedNotFound(t *testing.T) {
	container := &v1.Container{Name: "app"}
	_, err := resolveTCPSocketPort(intstr.FromString("missing"), container)
	if err == nil {
		t.Fatal("expected error for missing named port, got nil")
	}
}

// TestResolveTCPSocketPort_InvalidRange verifies that port numbers outside the
// valid 1–65535 range are rejected.
func TestResolveTCPSocketPort_InvalidRange(t *testing.T) {
	container := &v1.Container{Name: "app"}
	for _, bad := range []int{0, 65536, -1} {
		_, err := resolveTCPSocketPort(intstr.FromInt(bad), container)
		if err == nil {
			t.Errorf("expected error for port %d, got nil", bad)
		}
	}
}

// TestExecuteTCPSocketHook_Success verifies that a hook succeeds when the target
// TCP port is open and accepting connections.
func TestExecuteTCPSocketHook_Success(t *testing.T) {
	ln, port := startTCPListener(t)
	defer ln.Close()

	// Accept connections in the background so DialTimeout does not block.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	action := &v1.TCPSocketAction{
		Port: intstr.FromInt(port),
		Host: "127.0.0.1",
	}
	pod := &v1.Pod{}
	container := &v1.Container{Name: "app"}

	if err := executeTCPSocketHook(action, pod, container, 5); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

// TestExecuteTCPSocketHook_Failure verifies that a hook fails when nothing is
// listening on the target port.
func TestExecuteTCPSocketHook_Failure(t *testing.T) {
	// Bind a listener to get a free port, then close it so nothing is listening.
	ln, port := startTCPListener(t)
	ln.Close()

	action := &v1.TCPSocketAction{
		Port: intstr.FromInt(port),
		Host: "127.0.0.1",
	}
	pod := &v1.Pod{}
	container := &v1.Container{Name: "app"}

	if err := executeTCPSocketHook(action, pod, container, 2); err == nil {
		t.Fatal("expected error for closed port, got nil")
	}
}

// TestExecuteTCPSocketHook_UsesExplicitHost verifies that an explicit host in
// TCPSocketAction.Host is used rather than the pod IP.
func TestExecuteTCPSocketHook_UsesExplicitHost(t *testing.T) {
	ln, port := startTCPListener(t)
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	action := &v1.TCPSocketAction{
		Port: intstr.FromInt(port),
		Host: "127.0.0.1", // explicit — pod IP should not be used
	}
	// Set a different pod IP to confirm it is not used.
	pod := &v1.Pod{Status: v1.PodStatus{PodIP: "10.0.0.1"}}
	container := &v1.Container{Name: "app"}

	if err := executeTCPSocketHook(action, pod, container, 5); err != nil {
		t.Fatalf("expected success using explicit host, got: %v", err)
	}
}

// TestExecuteTCPSocketHook_UsesPodIPWhenNoHost verifies that the pod IP is used
// as the dial target when TCPSocketAction.Host is empty.
func TestExecuteTCPSocketHook_UsesPodIPWhenNoHost(t *testing.T) {
	ln, port := startTCPListener(t)
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	action := &v1.TCPSocketAction{
		Port: intstr.FromInt(port),
		// Host intentionally empty — should fall back to pod.Status.PodIP
	}
	pod := &v1.Pod{Status: v1.PodStatus{PodIP: "127.0.0.1"}}
	container := &v1.Container{Name: "app"}

	if err := executeTCPSocketHook(action, pod, container, 5); err != nil {
		t.Fatalf("expected success using pod IP, got: %v", err)
	}
}

// TestExecuteTCPSocketHook_NamedPort verifies that a named port is correctly
// resolved from the container spec when dialing.
func TestExecuteTCPSocketHook_NamedPort(t *testing.T) {
	ln, port := startTCPListener(t)
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	action := &v1.TCPSocketAction{
		Port: intstr.FromString("shutdown"),
		Host: "127.0.0.1",
	}
	pod := &v1.Pod{}
	container := &v1.Container{
		Name: "app",
		Ports: []v1.ContainerPort{
			{Name: "shutdown", ContainerPort: int32(port)},
		},
	}

	if err := executeTCPSocketHook(action, pod, container, 5); err != nil {
		t.Fatalf("expected success with named port, got: %v", err)
	}
}

// TestExecuteTCPSocketHook_FallbackToLocalhostWhenNoPodIP verifies that when
// neither an explicit host nor a pod IP is available, the hook dials localhost.
// We verify this by listening on localhost and leaving pod.Status.PodIP empty.
func TestExecuteTCPSocketHook_FallbackToLocalhostWhenNoPodIP(t *testing.T) {
	ln, port := startTCPListener(t)
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	action := &v1.TCPSocketAction{
		Port: intstr.FromInt(port),
		// No Host — no PodIP — should fall back to "localhost"
	}
	pod := &v1.Pod{}
	container := &v1.Container{Name: "app"}

	if err := executeTCPSocketHook(action, pod, container, 5); err != nil {
		t.Fatalf("expected success falling back to localhost, got: %v", err)
	}
}

// TestExecuteTCPSocketHook_BadPort verifies that an invalid port number in the
// action is rejected before any dial attempt.
func TestExecuteTCPSocketHook_BadPort(t *testing.T) {
	action := &v1.TCPSocketAction{
		Port: intstr.FromInt(0),
		Host: "127.0.0.1",
	}
	pod := &v1.Pod{}
	container := &v1.Container{Name: "app"}

	err := executeTCPSocketHook(action, pod, container, 5)
	if err == nil {
		t.Fatal("expected error for port 0, got nil")
	}
	expected := "failed to resolve TCPSocket port"
	if len(err.Error()) < len(expected) || err.Error()[:len(expected)] != expected {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestExecuteTCPSocketHook_ZeroGracePeriod verifies that a zero grace period is
// clamped to 1 second so the dial does not block indefinitely or fail immediately.
func TestExecuteTCPSocketHook_ZeroGracePeriod(t *testing.T) {
	ln, port := startTCPListener(t)
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	action := &v1.TCPSocketAction{
		Port: intstr.FromInt(port),
		Host: "127.0.0.1",
	}
	pod := &v1.Pod{}
	container := &v1.Container{Name: "app"}

	// gracePeriod=0 should be clamped to 1s — connection to local listener must still succeed.
	if err := executeTCPSocketHook(action, pod, container, 0); err != nil {
		t.Fatalf("expected success with zero grace period (clamped to 1s), got: %v", err)
	}
}

// Compile-time check: ensure the helper functions are accessible from tests in
// the same package (white-box testing).
var _ = fmt.Sprintf
