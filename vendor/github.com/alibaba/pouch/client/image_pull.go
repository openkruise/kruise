package client

import (
	"context"
	"io"
	"net/url"
)

// ImagePull requests daemon to pull an image from registry.
func (client *APIClient) ImagePull(ctx context.Context, name, tag, encodedAuth string) (io.ReadCloser, error) {
	q := url.Values{}
	q.Set("fromImage", name)
	q.Set("tag", tag)

	headers := map[string][]string{}
	if encodedAuth != "" {
		headers["X-Registry-Auth"] = []string{encodedAuth}
	}
	resp, err := client.post(ctx, "/images/create", q, nil, headers)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
