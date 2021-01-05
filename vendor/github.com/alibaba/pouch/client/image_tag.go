package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/alibaba/pouch/pkg/reference"
)

// ImageTag creates tag for the image.
func (client *APIClient) ImageTag(ctx context.Context, image string, tag string) error {
	if _, err := reference.Parse(image); err != nil {
		return fmt.Errorf("the image reference (%s) is not valid reference", image)
	}

	ref, err := reference.Parse(tag)
	if err != nil {
		return fmt.Errorf("the tag reference (%s) is not valid reference", tag)
	}

	if _, ok := ref.(reference.Digested); ok {
		return fmt.Errorf("refusing to create a tag with a digest reference")
	}

	q := url.Values{}
	q.Set("repo", ref.Name())
	if tagRef, ok := ref.(reference.Tagged); ok {
		q.Set("tag", tagRef.Tag())
	}

	resp, err := client.post(ctx, fmt.Sprintf("/images/%s/tag", image), q, nil, nil)
	ensureCloseReader(resp)
	return err
}
