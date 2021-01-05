package client

import (
	"context"

	"github.com/alibaba/pouch/apis/types"
)

// ImageHistory requests daemon to get history of an image.
func (client *APIClient) ImageHistory(ctx context.Context, name string) ([]types.HistoryResultItem, error) {
	history := []types.HistoryResultItem{}

	resp, err := client.get(ctx, "/images/"+name+"/history", nil, nil)
	if err != nil {
		return history, err
	}

	defer ensureCloseReader(resp)
	err = decodeBody(&history, resp.Body)
	return history, err
}
