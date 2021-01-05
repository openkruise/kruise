package client

import (
	"context"
	"io"
	"net/url"
)

// ImageSave requests daemon to save an image to a tar archive.
func (client *APIClient) ImageSave(ctx context.Context, imageName string) (io.ReadCloser, error) {
	q := url.Values{}
	q.Set("name", imageName)

	resp, err := client.get(ctx, "/images/save", q, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
