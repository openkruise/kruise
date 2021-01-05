package client

import (
	"context"
)

// VolumeRemove removes a volume.
func (client *APIClient) VolumeRemove(ctx context.Context, name string) error {
	resp, err := client.delete(ctx, "/volumes/"+name, nil, nil)
	ensureCloseReader(resp)

	return err
}
