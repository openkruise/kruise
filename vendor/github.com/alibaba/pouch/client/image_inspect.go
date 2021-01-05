package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// ImageInspect requests daemon to inspect an image.
func (client *APIClient) ImageInspect(ctx context.Context, name string) (types.ImageInfo, error) {
	image := types.ImageInfo{}

	resp, err := client.get(ctx, "/images/"+name+"/json", nil, nil)
	if err != nil {
		return image, err
	}

	defer ensureCloseReader(resp)
	err = decodeBody(&image, resp.Body)
	return image, err
}
