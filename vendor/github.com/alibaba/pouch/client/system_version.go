package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// SystemVersion requests daemon for system version.
func (client *APIClient) SystemVersion(ctx context.Context) (*types.SystemVersion, error) {
	resp, err := client.get(ctx, "/version", nil, nil)
	if err != nil {
		return nil, err
	}

	version := &types.SystemVersion{}
	err = decodeBody(version, resp.Body)
	ensureCloseReader(resp)

	return version, err
}
