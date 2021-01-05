package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// RegistryLogin authenticates the server with a given registry to login.
func (client *APIClient) RegistryLogin(ctx context.Context, auth *types.AuthConfig) (*types.AuthResponse, error) {
	resp, err := client.post(ctx, "/auth", nil, auth, nil)
	if err != nil {
		return nil, err
	}

	authResp := &types.AuthResponse{}
	err = decodeBody(authResp, resp.Body)
	ensureCloseReader(resp)

	return authResp, err
}
