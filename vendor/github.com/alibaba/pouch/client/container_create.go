package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerCreate creates a new container based in the given configuration.
func (client *APIClient) ContainerCreate(ctx context.Context, config types.ContainerConfig, hostConfig *types.HostConfig, networkingConfig *types.NetworkingConfig, containerName string) (*types.ContainerCreateResp, error) {
	createConfig := types.ContainerCreateConfig{
		ContainerConfig:  config,
		HostConfig:       hostConfig,
		NetworkingConfig: networkingConfig,
	}

	q := url.Values{}
	if containerName != "" {
		q.Set("name", containerName)
	}

	resp, err := client.post(ctx, "/containers/create", q, createConfig, nil)
	if err != nil {
		return nil, err
	}

	container := &types.ContainerCreateResp{}

	err = decodeBody(container, resp.Body)
	ensureCloseReader(resp)

	return container, err
}
