package client

import (
	"context"
	"net/url"
)

// ContainerRename renames a container.
func (client *APIClient) ContainerRename(ctx context.Context, id string, name string) error {
	q := url.Values{}
	q.Add("name", name)

	resp, err := client.post(ctx, "/containers/"+id+"/rename", q, nil, nil)
	ensureCloseReader(resp)

	return err
}
