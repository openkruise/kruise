package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerGet returns the detailed information of container.
func (client *APIClient) ContainerGet(ctx context.Context, name string) (*types.ContainerJSON, error) {
	resp, err := client.get(ctx, "/containers/"+name+"/json", nil, nil)
	if err != nil {
		return nil, err
	}

	container := types.ContainerJSON{}
	err = decodeBody(&container, resp.Body)
	ensureCloseReader(resp)

	return &container, err
}
