package client

import (
	"context"
)

// NetworkRemove removes a network.
func (client *APIClient) NetworkRemove(ctx context.Context, networkID string) error {
	resp, err := client.delete(ctx, "/networks/"+networkID, nil, nil)
	if err != nil {
		return err
	}

	ensureCloseReader(resp)
	return err
}
