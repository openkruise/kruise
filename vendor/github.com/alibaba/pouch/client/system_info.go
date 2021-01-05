package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// SystemInfo requests daemon for system info.
func (client *APIClient) SystemInfo(ctx context.Context) (*types.SystemInfo, error) {
	resp, err := client.get(ctx, "/info", nil, nil)
	if err != nil {
		return nil, err
	}

	info := &types.SystemInfo{}
	err = decodeBody(info, resp.Body)
	ensureCloseReader(resp)

	return info, err
}
