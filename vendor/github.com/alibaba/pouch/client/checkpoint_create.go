package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerCheckpointCreate creates container checkpoint.
func (client *APIClient) ContainerCheckpointCreate(ctx context.Context, name string, options types.CheckpointCreateOptions) error {
	resp, err := client.post(ctx, "/containers/"+name+"/checkpoints", nil, &options, nil)
	ensureCloseReader(resp)

	return err
}
