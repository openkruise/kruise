package client

import (
	"context"
	"net/url"
	"strings"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerTop shows process information from within a container.
func (client *APIClient) ContainerTop(ctx context.Context, name string, arguments []string) (types.ContainerProcessList, error) {
	response := types.ContainerProcessList{}
	query := url.Values{}
	if len(arguments) > 0 {
		query.Set("ps_args", strings.Join(arguments, " "))
	}

	resp, err := client.get(ctx, "/containers/"+name+"/top", query, nil)
	if err != nil {
		return response, err
	}

	err = decodeBody(&response, resp.Body)
	ensureCloseReader(resp)
	return response, err
}
