package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// VolumeInspect inspects a volume.
func (client *APIClient) VolumeInspect(ctx context.Context, name string) (*types.VolumeInfo, error) {
	resp, err := client.get(ctx, "/volumes/"+name, nil, nil)
	if err != nil {
		return nil, err
	}

	volume := &types.VolumeInfo{}

	err = decodeBody(volume, resp.Body)
	ensureCloseReader(resp)

	return volume, err
}
