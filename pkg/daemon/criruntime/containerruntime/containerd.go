/*
Copyright 2021 The Kruise Authors.

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

package containerruntime

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/openkruise/kruise/pkg/util"
	"google.golang.org/grpc"
)

const (
	k8sContainerdNamespace = "k8s.io"
)

// NewContainerdContainerService returns containerd-type ContainerService
func NewContainerdContainerService(conn *grpc.ClientConn) (ContainerService, error) {
	client, err := containerd.NewWithConn(conn)
	if err != nil {
		return nil, err
	}
	return &containerdContainerClient{client: client}, nil
}

type containerdContainerClient struct {
	client *containerd.Client
}

func (d *containerdContainerClient) ContainerStatus(ctx context.Context, containerID string) (*ContainerStatus, error) {
	ctx = namespaces.WithNamespace(ctx, k8sContainerdNamespace)
	container, err := d.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}
	spec, err := container.Spec(ctx)
	if err != nil {
		return nil, err
	} else if spec.Process == nil {
		return nil, fmt.Errorf("no process found in %s container spec from containerd: %v", containerID, util.DumpJSON(spec))
	}
	return &ContainerStatus{Env: spec.Process.Env}, nil
}
