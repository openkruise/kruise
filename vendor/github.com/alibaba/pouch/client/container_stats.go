package client

import (
	"context"
	"fmt"
	"io"
	"net/url"
)

// ContainerStats return the stats related container in an io.ReadCloser.
func (client *APIClient) ContainerStats(ctx context.Context, name string, stream bool) (io.ReadCloser, error) {
	query := url.Values{}
	if stream {
		query.Set("stream", "1")
	}
	resp, err := client.get(ctx, fmt.Sprintf("/containers/%s/stats", name), query, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
