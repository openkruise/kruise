package client

import (
	"context"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
	"github.com/alibaba/pouch/pkg/utils/filters"
)

// ContainerList returns the list of containers.
func (client *APIClient) ContainerList(ctx context.Context, option types.ContainerListOptions) ([]*types.Container, error) {
	q := url.Values{}

	if option.All {
		q.Set("all", "true")
	}

	if len(option.Filter) > 0 {
		fJSON, err := filters.ToURLParam(option.Filter)
		if err != nil {
			return nil, err
		}
		q.Set("filters", fJSON)
	}

	resp, err := client.get(ctx, "/containers/json", q, nil)
	if err != nil {
		return nil, err
	}

	containers := []*types.Container{}
	err = decodeBody(&containers, resp.Body)
	ensureCloseReader(resp)

	return containers, err
}
