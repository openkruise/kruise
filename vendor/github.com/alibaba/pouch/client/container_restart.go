package client

import (
	"context"
	"net/url"
)

// ContainerRestart restarts a running container.
func (client *APIClient) ContainerRestart(ctx context.Context, name string, timeout string) error {
	q := url.Values{}
	q.Add("t", timeout)

	resp, err := client.post(ctx, "/containers/"+name+"/restart", q, nil, nil)
	ensureCloseReader(resp)

	return err
}
