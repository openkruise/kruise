package client

import (
	"context"
	"io/ioutil"
)

// SystemPing shows whether server is ok.
func (client *APIClient) SystemPing(ctx context.Context) (string, error) {
	resp, err := client.get(ctx, "/_ping", nil, nil)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
