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
	"sync"

	dockerapi "github.com/docker/docker/client"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/util"
)

func NewDockerContainerService(runtimeURI string) (ContainerService, error) {
	d := &dockerContainerService{runtimeURI: runtimeURI}
	if err := d.createRuntimeClientIfNecessary(); err != nil {
		return nil, err
	}
	return d, nil
}

type dockerContainerService struct {
	sync.Mutex
	runtimeURI string
	client     *dockerapi.Client
}

func (d *dockerContainerService) createRuntimeClientIfNecessary() error {
	d.Lock()
	defer d.Unlock()
	if d.client != nil {
		return nil
	}
	c, err := dockerapi.NewClient(d.runtimeURI, "1.23", nil, nil)
	if err != nil {
		return err
	}
	d.client = c
	return nil
}

func (d *dockerContainerService) handleRuntimeError(err error) {
	if daemonutil.FilterCloseErr(err) {
		d.Lock()
		defer d.Unlock()
		d.client = nil
	}
}

func (d *dockerContainerService) ContainerStatus(ctx context.Context, containerID string) (*ContainerStatus, error) {
	var err error
	if err = d.createRuntimeClientIfNecessary(); err != nil {
		return nil, err
	}

	containerJSON, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		d.handleRuntimeError(err)
		return nil, err
	} else if containerJSON.Config == nil {
		return nil, fmt.Errorf("no config found in %s container inspect from docker: %v", containerID, util.DumpJSON(containerJSON))
	}
	return &ContainerStatus{Env: containerJSON.Config.Env}, nil
}
