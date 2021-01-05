package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// NetworkInspect inspects a network.
func (client *APIClient) NetworkInspect(ctx context.Context, networkID string) (*types.NetworkInspectResp, error) {
	resp, err := client.get(ctx, "/networks/"+networkID, nil, nil)
	if err != nil {
		return nil, err
	}

	network := &types.NetworkInspectResp{}

	err = decodeBody(network, resp.Body)
	ensureCloseReader(resp)

	return network, err
}
