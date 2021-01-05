package client

import (
	"bufio"
	"context"
	"net"
	"net/url"
)

// ContainerAttach attachs a container.
func (client *APIClient) ContainerAttach(ctx context.Context, name string, stdin bool) (net.Conn, *bufio.Reader, error) {
	q := url.Values{}
	if stdin {
		q.Set("stdin", "1")
	} else {
		q.Set("stdin", "0")
	}

	header := map[string][]string{
		"Content-Type": {"text/plain"},
	}

	return client.hijack(ctx, "/containers/"+name+"/attach", q, nil, header)
}
