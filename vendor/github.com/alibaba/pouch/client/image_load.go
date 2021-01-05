package client

import (
	"context"
	"io"
	"net/url"
)

// ImageLoad requests daemon to load an image from tarstream.
func (client *APIClient) ImageLoad(ctx context.Context, imageName string, reader io.Reader) error {
	q := url.Values{}
	if imageName != "" {
		q.Set("name", imageName)
	}

	headers := map[string][]string{}
	headers["Content-Type"] = []string{"application/x-tar"}

	resp, err := client.postRawData(ctx, "/images/load", q, reader, headers)
	if err != nil {
		return err
	}

	ensureCloseReader(resp)
	return nil
}
