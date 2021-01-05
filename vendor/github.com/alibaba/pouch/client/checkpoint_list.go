package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerCheckpointList lists all checkpoints of a container
func (client *APIClient) ContainerCheckpointList(ctx context.Context, name string, options types.CheckpointListOptions) ([]string, error) {
	q := url.Values{}
	if options.CheckpointDir != "" {
		q.Set("dir", options.CheckpointDir)
	}

	resp, err := client.get(ctx, "/containers/"+name+"/checkpoints", q, nil)
	if err != nil {
		return nil, err
	}

	list := make([]string, 0)
	err = decodeBody(&list, resp.Body)
	ensureCloseReader(resp)

	return list, err
}
