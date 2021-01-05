package client

import (
	"context"
	"net/url"
)

// ImageRemove deletes an image.
func (client *APIClient) ImageRemove(ctx context.Context, name string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "true")
	}

	resp, err := client.delete(ctx, "/images/"+name, q, nil)
	ensureCloseReader(resp)

	return err
}
