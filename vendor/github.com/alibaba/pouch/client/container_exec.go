package client

import (
	"bufio"
	"context"
	"net"
	"net/url"

	"github.com/alibaba/pouch/apis/types"
)

// ContainerCreateExec creates exec process.
func (client *APIClient) ContainerCreateExec(ctx context.Context, name string, config *types.ExecCreateConfig) (*types.ExecCreateResp, error) {
	response, err := client.post(ctx, "/containers/"+name+"/exec", url.Values{}, config, nil)
	if err != nil {
		return nil, err
	}

	body := &types.ExecCreateResp{}
	err = decodeBody(body, response.Body)
	ensureCloseReader(response)

	return body, err
}

// ContainerStartExec starts exec process.
func (client *APIClient) ContainerStartExec(ctx context.Context, execid string, config *types.ExecStartConfig) (net.Conn, *bufio.Reader, error) {
	header := map[string][]string{
		"Content-Type": {"text/plain"},
	}

	return client.hijack(ctx, "/exec/"+execid+"/start", url.Values{}, config, header)
}

// ContainerExecInspect get exec info with a specified exec id.
func (client *APIClient) ContainerExecInspect(ctx context.Context, execid string) (*types.ContainerExecInspect, error) {
	resp, err := client.get(ctx, "/exec/"+execid+"/json", nil, nil)
	if err != nil {
		return nil, err
	}

	body := &types.ContainerExecInspect{}
	err = decodeBody(body, resp.Body)
	ensureCloseReader(resp)

	return body, err
}
