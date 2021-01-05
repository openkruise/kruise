package client

import (
	"context"
	"errors"
	"io"
	"net/url"

	"github.com/alibaba/pouch/pkg/reference"
)

// ImagePush requests daemon to push an image to registry.
func (client *APIClient) ImagePush(ctx context.Context, ref, encodedAuth string) (io.ReadCloser, error) {
	namedRef, err := reference.Parse(ref)
	if err != nil {
		return nil, err
	}

	if reference.IsCanonicalDigested(namedRef) {
		return nil, errors.New("cannot push a digest reference format image")
	}

	tag := ""
	if v, ok := namedRef.(reference.Tagged); ok {
		tag = v.Tag()
	}

	q := url.Values{}
	if tag != "" {
		q.Set("tag", tag)
	}

	headers := map[string][]string{}
	if encodedAuth != "" {
		headers["X-Registry-Auth"] = []string{encodedAuth}
	}
	resp, err := client.post(ctx, "/images/"+namedRef.Name()+"/push", q, nil, headers)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
