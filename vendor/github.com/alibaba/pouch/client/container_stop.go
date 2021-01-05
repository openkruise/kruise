package client

import (
	"context"
	"net/url"
)

// ContainerStop stops a container.
func (client *APIClient) ContainerStop(ctx context.Context, name string, timeout string) error {
	q := url.Values{}
	q.Add("t", timeout)

	resp, err := client.post(ctx, "/containers/"+name+"/stop", q, nil, nil)
	ensureCloseReader(resp)

	return err
}
