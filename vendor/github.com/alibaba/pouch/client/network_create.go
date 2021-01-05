package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// NetworkCreate creates a network.
func (client *APIClient) NetworkCreate(ctx context.Context, req *types.NetworkCreateConfig) (*types.NetworkCreateResp, error) {
	resp, err := client.post(ctx, "/networks/create", nil, req, nil)
	if err != nil {
		return nil, err
	}

	network := &types.NetworkCreateResp{}

	err = decodeBody(network, resp.Body)
	ensureCloseReader(resp)

	return network, err
}
