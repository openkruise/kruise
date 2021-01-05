package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerUpgrade upgrade a container with new image and args.
func (client *APIClient) ContainerUpgrade(ctx context.Context, name string, config *types.ContainerUpgradeConfig) error {
	resp, err := client.post(ctx, "/containers/"+name+"/upgrade", url.Values{}, config, nil)
	ensureCloseReader(resp)

	return err
}
