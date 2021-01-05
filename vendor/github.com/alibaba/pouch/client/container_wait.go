package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerWait pauses execution until a container exits.
// It returns the API status code as response of its readiness.
func (client *APIClient) ContainerWait(ctx context.Context, name string) (types.ContainerWaitOKBody, error) {
	resp, err := client.post(ctx, "/containers/"+name+"/wait", nil, nil, nil)

	if err != nil {
		return types.ContainerWaitOKBody{}, err
	}

	var response types.ContainerWaitOKBody
	err = decodeBody(&response, resp.Body)
	ensureCloseReader(resp)
	return response, err
}
