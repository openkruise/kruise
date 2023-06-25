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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	criapi "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/probe"
	execprobe "k8s.io/kubernetes/pkg/probe/exec"
	httpprobe "k8s.io/kubernetes/pkg/probe/http"
	tcpprobe "k8s.io/kubernetes/pkg/probe/tcp"
	"k8s.io/utils/exec"
)

const maxProbeMessageLength = 1024

// Prober helps to check the probe(exec, http, tcp) of a container.
type prober struct {
	exec           execprobe.Prober
	tcp            tcpprobe.Prober
	http           httpprobe.Prober
	runtimeService criapi.RuntimeService
}

// NewProber creates a Prober, it takes a command runner and
// several container info managers.
func newProber(runtimeService criapi.RuntimeService) *prober {
	return &prober{
		exec:           execprobe.New(),
		tcp:            tcpprobe.New(),
		http:           httpprobe.New(false),
		runtimeService: runtimeService,
	}
}

// probe probes the container.
func (pb *prober) probe(p *appsv1alpha1.ContainerProbeSpec, container *runtimeapi.ContainerStatus, containerID string, hostIP string) (appsv1alpha1.ProbeState, string, error) {
	result, msg, err := pb.runProbe(p, container, containerID, hostIP)
	if bytes.Count([]byte(msg), nil)-1 > maxProbeMessageLength {
		msg = msg[:maxProbeMessageLength]
	}
	if err != nil || (result != probe.Success && result != probe.Warning) {
		return appsv1alpha1.ProbeFailed, msg, err
	}
	return appsv1alpha1.ProbeSucceeded, msg, nil
}

func (pb *prober) runProbe(p *appsv1alpha1.ContainerProbeSpec, container *runtimeapi.ContainerStatus, containerID string, hostIP string) (probe.Result, string, error) {
	timeSecond := p.TimeoutSeconds
	if timeSecond <= 0 {
		timeSecond = 1
	}
	timeout := time.Duration(timeSecond) * time.Second
	// for probing using exec method
	if p.Exec != nil {
		return pb.exec.Probe(pb.newExecInContainer(containerID, p.Exec.Command, timeout))
	}

	// for probing using tcp method
	if p.TCPSocket.Port.IntVal != 0 {
		if p.TCPSocket.Host != "" {
			return pb.tcp.Probe(p.TCPSocket.Host, p.TCPSocket.Port.IntValue(), timeout)
		} else {
			return pb.tcp.Probe(hostIP, p.TCPSocket.Port.IntValue(), timeout)
		}
	}

	// for probing using http method
	if p.HTTPGet.Path != "" {
		var u url.URL
		var header http.Header
		u.Scheme = "http"
		u.Path = p.HTTPGet.Path
		if p.HTTPGet.Host != "" {
			u.Host = p.HTTPGet.Host
		} else {
			u.Host = hostIP
		}
		return pb.http.Probe(&u, header, timeout)
	}

	klog.InfoS("Failed to find probe builder for container", "containerName", container.Metadata.Name)
	return probe.Unknown, "", fmt.Errorf("missing probe handler for %s", container.Metadata.Name)
}

type execInContainer struct {
	// run executes a command in a container. Combined stdout and stderr output is always returned. An
	// error is returned if one occurred.
	run    func() ([]byte, error)
	writer io.Writer
}

func (pb *prober) newExecInContainer(containerID string, cmd []string, timeout time.Duration) exec.Cmd {
	return &execInContainer{run: func() ([]byte, error) {
		stdout, stderr, err := pb.runtimeService.ExecSync(containerID, cmd, timeout)
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
