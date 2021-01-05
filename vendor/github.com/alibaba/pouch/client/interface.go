package client

import (
	"bufio"
	"context"
	"io"
	"net"

	"github.com/alibaba/pouch/apis/filters"
	"github.com/alibaba/pouch/apis/types"
)

// CommonAPIClient defines common methods of api client
type CommonAPIClient interface {
	ContainerAPIClient
	ImageAPIClient
	VolumeAPIClient
	SystemAPIClient
	NetworkAPIClient
}

// ContainerAPIClient defines methods of Container client.
type ContainerAPIClient interface {
	ContainerCreate(ctx context.Context, config types.ContainerConfig, hostConfig *types.HostConfig, networkConfig *types.NetworkingConfig, containerName string) (*types.ContainerCreateResp, error)
	ContainerStart(ctx context.Context, name string, options types.ContainerStartOptions) error
	ContainerStop(ctx context.Context, name, timeout string) error
	ContainerRemove(ctx context.Context, name string, options *types.ContainerRemoveOptions) error
	ContainerList(ctx context.Context, option types.ContainerListOptions) ([]*types.Container, error)
	ContainerAttach(ctx context.Context, name string, stdin bool) (net.Conn, *bufio.Reader, error)
	ContainerCreateExec(ctx context.Context, name string, config *types.ExecCreateConfig) (*types.ExecCreateResp, error)
	ContainerStartExec(ctx context.Context, execid string, config *types.ExecStartConfig) (net.Conn, *bufio.Reader, error)
	ContainerExecInspect(ctx context.Context, execid string) (*types.ContainerExecInspect, error)
	ContainerGet(ctx context.Context, name string) (*types.ContainerJSON, error)
	ContainerRename(ctx context.Context, id string, name string) error
	ContainerRestart(ctx context.Context, name string, timeout string) error
	ContainerPause(ctx context.Context, name string) error
	ContainerUnpause(ctx context.Context, name string) error
	ContainerUpdate(ctx context.Context, name string, config *types.UpdateConfig) error
	ContainerUpgrade(ctx context.Context, name string, config *types.ContainerUpgradeConfig) error
	ContainerTop(ctx context.Context, name string, arguments []string) (types.ContainerProcessList, error)
	ContainerLogs(ctx context.Context, name string, options types.ContainerLogsOptions) (io.ReadCloser, error)
	ContainerResize(ctx context.Context, name, height, width string) error
	ContainerWait(ctx context.Context, name string) (types.ContainerWaitOKBody, error)
	ContainerCheckpointCreate(ctx context.Context, name string, options types.CheckpointCreateOptions) error
	ContainerCheckpointList(ctx context.Context, name string, options types.CheckpointListOptions) ([]string, error)
	ContainerCheckpointDelete(ctx context.Context, name string, options types.CheckpointDeleteOptions) error
	ContainerCommit(ctx context.Context, name string, options types.ContainerCommitOptions) (*types.ContainerCommitResp, error)
	ContainerStats(ctx context.Context, name string, stream bool) (io.ReadCloser, error)
}

// ImageAPIClient defines methods of Image client.
type ImageAPIClient interface {
	ImageList(ctx context.Context, filters filters.Args) ([]types.ImageInfo, error)
	ImageInspect(ctx context.Context, name string) (types.ImageInfo, error)
	ImagePull(ctx context.Context, name, tag, encodedAuth string) (io.ReadCloser, error)
	ImageRemove(ctx context.Context, name string, force bool) error
	ImageTag(ctx context.Context, image string, tag string) error
	ImageLoad(ctx context.Context, name string, r io.Reader) error
	ImageSave(ctx context.Context, imageName string) (io.ReadCloser, error)
	ImageHistory(ctx context.Context, name string) ([]types.HistoryResultItem, error)
	ImagePush(ctx context.Context, ref, encodedAuth string) (io.ReadCloser, error)
}

// VolumeAPIClient defines methods of Volume client.
type VolumeAPIClient interface {
	VolumeCreate(ctx context.Context, config *types.VolumeCreateConfig) (*types.VolumeInfo, error)
	VolumeRemove(ctx context.Context, name string) error
	VolumeInspect(ctx context.Context, name string) (*types.VolumeInfo, error)
	VolumeList(ctx context.Context, filter filters.Args) (*types.VolumeListResp, error)
}

// SystemAPIClient defines methods of System client.
type SystemAPIClient interface {
	SystemPing(ctx context.Context) (string, error)
	SystemVersion(ctx context.Context) (*types.SystemVersion, error)
	SystemInfo(ctx context.Context) (*types.SystemInfo, error)
	RegistryLogin(ctx context.Context, auth *types.AuthConfig) (*types.AuthResponse, error)
	DaemonUpdate(ctx context.Context, daemonConfig *types.DaemonUpdateConfig) error
	Events(ctx context.Context, since string, until string, filters filters.Args) (io.ReadCloser, error)
}

// NetworkAPIClient defines methods of Network client.
type NetworkAPIClient interface {
	NetworkCreate(ctx context.Context, req *types.NetworkCreateConfig) (*types.NetworkCreateResp, error)
	NetworkRemove(ctx context.Context, networkID string) error
	NetworkInspect(ctx context.Context, networkID string) (*types.NetworkInspectResp, error)
	NetworkList(ctx context.Context) ([]types.NetworkResource, error)
	NetworkConnect(ctx context.Context, network string, req *types.NetworkConnect) error
	NetworkDisconnect(ctx context.Context, networkID, containerID string, force bool) error
}
