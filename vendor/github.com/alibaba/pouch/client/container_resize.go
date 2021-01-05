package client

import (
	"context"
	"net/url"
)

// ContainerResize resizes the size of container tty.
func (client *APIClient) ContainerResize(ctx context.Context, name, height, width string) error {
	query := url.Values{}
	query.Set("h", height)
	query.Set("w", width)

	resp, err := client.post(ctx, "/containers/"+name+"/resize", query, nil, nil)
	ensureCloseReader(resp)

	return err
}
