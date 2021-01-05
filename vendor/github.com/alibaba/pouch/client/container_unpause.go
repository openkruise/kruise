package client

import "context"

// ContainerUnpause unpauses a container.
func (client *APIClient) ContainerUnpause(ctx context.Context, name string) error {
	resp, err := client.post(ctx, "/containers/"+name+"/unpause", nil, nil, nil)
	ensureCloseReader(resp)

	return err
}
