package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// DaemonUpdate requests daemon to update daemon config.
func (client *APIClient) DaemonUpdate(ctx context.Context, config *types.DaemonUpdateConfig) error {
	resp, err := client.post(ctx, "/daemon/update", nil, config, nil)
	if err != nil {
		return err
	}

	ensureCloseReader(resp)
	return nil
}
