/*
Copyright 2022 The Kruise Authors.

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

package podprobe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	criapi "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/probe"
	execprobe "k8s.io/kubernetes/pkg/probe/exec"
	tcpprobe "k8s.io/kubernetes/pkg/probe/tcp"
	"k8s.io/utils/exec"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

const maxProbeMessageLength = 1024

// Prober helps to check the probe(exec, http, tcp) of a container.
type prober struct {
	exec           execprobe.Prober
	tcp            tcpprobe.Prober
	runtimeService criapi.RuntimeService
}

// NewProber creates a Prober, it takes a command runner and
// several container info managers.
func newProber(runtimeService criapi.RuntimeService) *prober {
	return &prober{
		exec:           execprobe.New(),
		tcp:            tcpprobe.New(),
		runtimeService: runtimeService,
	}
}

// probe probes the container.
func (pb *prober) probe(p *appsv1alpha1.ContainerProbeSpec, probeKey probeKey, containerRuntimeStatus *runtimeapi.ContainerStatus, containerID string) (appsv1alpha1.ProbeState, string, error) {
	result, msg, err := pb.runProbe(p, probeKey, containerRuntimeStatus, containerID)
	if bytes.Count([]byte(msg), nil)-1 > maxProbeMessageLength {
		msg = msg[:maxProbeMessageLength]
	}
	if err != nil || (result != probe.Success && result != probe.Warning) {
		return appsv1alpha1.ProbeFailed, msg, err
	}
	return appsv1alpha1.ProbeSucceeded, msg, nil
}

func (pb *prober) runProbe(p *appsv1alpha1.ContainerProbeSpec, probeKey probeKey, containerRuntimeStatus *runtimeapi.ContainerStatus, containerID string) (probe.Result, string, error) {
	timeSecond := p.TimeoutSeconds
	if timeSecond <= 0 {
		timeSecond = 1
	}
	timeout := time.Duration(timeSecond) * time.Second
	// current only support exec
	// todo: http
	if p.Exec != nil {
		return pb.exec.Probe(pb.newExecInContainer(containerID, p.Exec.Command, timeout))
	}
	// support tcp socket probe handler
	if p.TCPSocket != nil {
		port := p.TCPSocket.Port.IntValue()
		host := p.TCPSocket.Host
		if host == "" {
			host = probeKey.podIP
		}
		klog.InfoS("TCP-Probe Host", "host", host, "port", port, "timeout", timeout)
		return pb.tcp.Probe(host, port, timeout)
	}
	klog.InfoS("Failed to find probe builder for container", "containerName", containerRuntimeStatus.Metadata.Name)
	return probe.Unknown, "", fmt.Errorf("missing probe handler for %s", containerRuntimeStatus.Metadata.Name)
}

type execInContainer struct {
	// run executes a command in a container. Combined stdout and stderr output is always returned. An
	// error is returned if one occurred.
	run    func() ([]byte, error)
	writer io.Writer
}

func (pb *prober) newExecInContainer(containerID string, cmd []string, timeout time.Duration) exec.Cmd {
	return &execInContainer{run: func() ([]byte, error) {
		stdout, stderr, err := pb.runtimeService.ExecSync(context.TODO(), containerID, cmd, timeout)
		if err != nil {
			return stderr, err
		}
		return stdout, nil
	}}
}

func (eic *execInContainer) Run() error {
	return nil
}

func (eic *execInContainer) CombinedOutput() ([]byte, error) {
	return eic.run()
}

func (eic *execInContainer) Output() ([]byte, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (eic *execInContainer) SetDir(dir string) {
	// unimplemented
}

func (eic *execInContainer) SetStdin(in io.Reader) {
	// unimplemented
}

func (eic *execInContainer) SetStdout(out io.Writer) {
	eic.writer = out
}

func (eic *execInContainer) SetStderr(out io.Writer) {
	eic.writer = out
}

func (eic *execInContainer) SetEnv(env []string) {
	// unimplemented
}

func (eic *execInContainer) Stop() {
	// unimplemented
}

func (eic *execInContainer) Start() error {
	data, err := eic.run()
	if eic.writer != nil {
		eic.writer.Write(data)
	}
	return err
}

func (eic *execInContainer) Wait() error {
	return nil
}

func (eic *execInContainer) StdoutPipe() (io.ReadCloser, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (eic *execInContainer) StderrPipe() (io.ReadCloser, error) {
	return nil, fmt.Errorf("unimplemented")
}
