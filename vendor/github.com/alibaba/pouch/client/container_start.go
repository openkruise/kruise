package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerStart starts a created container.
func (client *APIClient) ContainerStart(ctx context.Context, name string, options types.ContainerStartOptions) error {
	query := url.Values{}
	if len(options.DetachKeys) != 0 {
		query.Set("detachKeys", options.DetachKeys)
	}
	if len(options.CheckpointID) != 0 {
		query.Set("checkpoint", options.CheckpointID)
	}
	if len(options.CheckpointDir) != 0 {
		query.Set("checkpoint-dir", options.CheckpointDir)
	}

	resp, err := client.post(ctx, "/containers/"+name+"/start", query, nil, nil)
	ensureCloseReader(resp)

	return err
}
