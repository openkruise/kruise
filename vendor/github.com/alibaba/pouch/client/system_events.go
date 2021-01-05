package client

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/alibaba/pouch/apis/filters"
	"github.com/alibaba/pouch/pkg/utils"
)

// Events returns a stream of events in the daemon in a ReadClosed.
// It's up to the caller to close the stream.
func (client *APIClient) Events(ctx context.Context, since string, until string, f filters.Args) (io.ReadCloser, error) {
	query := url.Values{}
	now := time.Now()

	if since != "" {
		ts, err := utils.GetUnixTimestamp(since, now)
		if err != nil {
			return nil, err
		}
		query.Set("since", ts)
	}

	if until != "" {
		ts, err := utils.GetUnixTimestamp(until, now)
		if err != nil {
			return nil, err
		}
		query.Set("until", ts)
	}

	if f.Len() > 0 {
		filtersJSON, err := filters.ToParam(f)
		if err != nil {
			return nil, err
		}

		query.Set("filters", filtersJSON)
	}

	resp, err := client.get(ctx, "/events", query, nil)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}
