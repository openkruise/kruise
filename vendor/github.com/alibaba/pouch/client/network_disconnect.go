package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// NetworkDisconnect disconnect a network from a container.
func (client *APIClient) NetworkDisconnect(ctx context.Context, networkID, containerID string, force bool) error {
	disconnectParams := &types.NetworkDisconnect{
		Container: containerID,
		Force:     force,
	}

	resp, err := client.post(ctx, "/networks/"+networkID+"/disconnect", nil, disconnectParams, nil)
	ensureCloseReader(resp)

	return err
}
