package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerCommit creates an image from a container.
func (client *APIClient) ContainerCommit(ctx context.Context, name string, options types.ContainerCommitOptions) (*types.ContainerCommitResp, error) {
	q := url.Values{}
	q.Set("container", name)
	q.Set("repo", options.Repository)
	q.Set("tag", options.Tag)
	q.Set("comment", options.Comment)
	q.Set("author", options.Author)

	response := &types.ContainerCommitResp{}
	resp, err := client.post(ctx, "/commit", q, nil, nil)
	if err != nil {
		return response, err
	}

	err = decodeBody(response, resp.Body)
	ensureCloseReader(resp)

	return response, err
}
