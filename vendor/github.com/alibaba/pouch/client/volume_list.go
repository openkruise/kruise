package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/filters"
	"github.com/alibaba/pouch/apis/types"
)

// VolumeList returns the list of volumes.
func (client *APIClient) VolumeList(ctx context.Context, filter filters.Args) (*types.VolumeListResp, error) {
	query := url.Values{}
	if filter.Len() > 0 {
		filtersJSON, err := filters.ToParam(filter)
		if err != nil {
			return nil, err
		}

		query.Set("filters", filtersJSON)
	}

	resp, err := client.get(ctx, "/volumes", query, nil)
	if err != nil {
		return nil, err
	}

	volumeListResp := &types.VolumeListResp{}

	err = decodeBody(volumeListResp, resp.Body)
	ensureCloseReader(resp)

	return volumeListResp, err
}
