package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// VolumeCreate creates a volume.
func (client *APIClient) VolumeCreate(ctx context.Context, config *types.VolumeCreateConfig) (*types.VolumeInfo, error) {
	resp, err := client.post(ctx, "/volumes/create", nil, config, nil)
	if err != nil {
		return nil, err
	}

	volume := &types.VolumeInfo{}

	err = decodeBody(volume, resp.Body)
	ensureCloseReader(resp)

	return volume, err
}
