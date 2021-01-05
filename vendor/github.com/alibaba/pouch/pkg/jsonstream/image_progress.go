package jsonstream

import (
	"fmt"

	"github.com/containerd/containerd/pkg/progress"
)

const (
	// PullStatusDownloading represents downloading status.
	PullStatusDownloading = "downloading"
	// PullStatusWaiting represents waiting status.
	PullStatusWaiting = "waiting"
	// PullStatusResolving represents resolving status.
	PullStatusResolving = "resolving"
	// PullStatusResolved represents resolved status.
	PullStatusResolved = "resolved"
	// PullStatusExists represents exist status.
	PullStatusExists = "exists"
	// PullStatusDone represents done status.
	PullStatusDone = "done"

	// PushStatusUploading represents uploading status.
	PushStatusUploading = "uploading"
)

// ProcessStatus returns the status of download or upload image
//
// NOTE: if the stdout is not terminal, it should only show the reference and
// status without progress bar.
func ProcessStatus(short bool, msg JSONMessage) string {
	if short || msg.Detail == nil {
		return fmt.Sprintf("%s:\t%s\n", msg.ID, msg.Status)
	}

	switch msg.Status {
	case PullStatusResolving, PullStatusWaiting:
		return fmt.Sprintf("%s:\t%s\t%40r\t\n", msg.ID, msg.Status, progress.Bar(0.0))
	case PullStatusDownloading, PushStatusUploading:
		bar := progress.Bar(0)
		current, total := progress.Bytes(msg.Detail.Current), progress.Bytes(msg.Detail.Total)

		if msg.Detail.Total > 0 {
			bar = progress.Bar(float64(current) / float64(total))
		}
		return fmt.Sprintf("%s:\t%s\t%40r\t%8.8s/%s\t\n", msg.ID, msg.Status, bar, current, total)
	default:
		return fmt.Sprintf("%s:\t%s\t%40r\t\n", msg.ID, msg.Status, progress.Bar(1.0))
	}
}
