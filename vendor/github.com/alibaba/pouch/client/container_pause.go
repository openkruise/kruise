package client

import "context"

// ContainerPause pauses a container.
func (client *APIClient) ContainerPause(ctx context.Context, name string) error {
	resp, err := client.post(ctx, "/containers/"+name+"/pause", nil, nil, nil)
	ensureCloseReader(resp)

	return err
}
