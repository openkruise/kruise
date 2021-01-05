package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerUpdate updates the configurations of a container.
func (client *APIClient) ContainerUpdate(ctx context.Context, name string, config *types.UpdateConfig) error {
	resp, err := client.post(ctx, "/containers/"+name+"/update", url.Values{}, config, nil)
	ensureCloseReader(resp)

	return err

}
