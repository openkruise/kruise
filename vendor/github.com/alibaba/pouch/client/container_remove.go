package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerRemove removes a container.
func (client *APIClient) ContainerRemove(ctx context.Context, name string, options *types.ContainerRemoveOptions) error {
	q := url.Values{}
	if options.Force {
		q.Set("force", "true")
	}
	if options.Volumes {
		q.Set("v", "true")
	}

	resp, err := client.delete(ctx, "/containers/"+name, q, nil)
	if err != nil {
		return err
	}
	ensureCloseReader(resp)
	return nil
}
