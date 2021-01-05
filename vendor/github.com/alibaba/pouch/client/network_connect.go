package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// NetworkConnect connects a container to a network.
func (client *APIClient) NetworkConnect(ctx context.Context, network string, req *types.NetworkConnect) error {
	resp, err := client.post(ctx, "/networks/"+network+"/connect", nil, req, nil)
	if err != nil {
		return err
	}

	ensureCloseReader(resp)
	return nil
}
