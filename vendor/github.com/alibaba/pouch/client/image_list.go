package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/filters"
	"github.com/alibaba/pouch/apis/types"
)

// ImageList requests daemon to list all images
func (client *APIClient) ImageList(ctx context.Context, filter filters.Args) ([]types.ImageInfo, error) {
	query := url.Values{}

	if filter.Len() > 0 {
		filtersJSON, err := filters.ToParam(filter)
		if err != nil {
			return nil, err
		}

		query.Set("filters", filtersJSON)
	}

	resp, err := client.get(ctx, "/images/json", query, nil)
	if err != nil {
		return nil, err
	}

	imageList := []types.ImageInfo{}

	err = decodeBody(&imageList, resp.Body)
	ensureCloseReader(resp)

	return imageList, err
}
