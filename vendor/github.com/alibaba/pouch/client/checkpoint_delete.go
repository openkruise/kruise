package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerCheckpointDelete deletes a checkpoint from container
func (client *APIClient) ContainerCheckpointDelete(ctx context.Context, name string, options types.CheckpointDeleteOptions) error {
	q := url.Values{}
	if options.CheckpointDir != "" {
		q.Set("dir", options.CheckpointDir)
	}

	resp, err := client.delete(ctx, "/containers/"+name+"/checkpoints/"+options.CheckpointID, q, nil)
	ensureCloseReader(resp)

	return err
}
