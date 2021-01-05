package jsonstream

import (
	"context"
	"sync"
	"time"

	"github.com/containerd/containerd/remotes/docker"
)

// PushJobs defines a job in upload progress
type PushJobs struct {
	jobs    map[string]struct{}
	ordered []string
	tracker docker.StatusTracker
	mu      sync.Mutex
}

// NewPushJobs news a PushJobs
func NewPushJobs(tracker docker.StatusTracker) *PushJobs {
	return &PushJobs{
		jobs:    make(map[string]struct{}),
		tracker: tracker,
	}
}

// Add adds a ref in upload job
func (j *PushJobs) Add(ref string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if _, ok := j.jobs[ref]; ok {
		return
	}
	j.ordered = append(j.ordered, ref)
	j.jobs[ref] = struct{}{}
}

// Status gets PushJobs statuses
func (j *PushJobs) Status() []JSONMessage {
	j.mu.Lock()
	defer j.mu.Unlock()

	statuses := make([]JSONMessage, 0, len(j.jobs))
	for _, name := range j.ordered {
		si := JSONMessage{
			ID: name,
		}

		status, err := j.tracker.GetStatus(name)
		if err != nil {
			si.Status = "waiting"
		} else {
			si.Detail = &ProgressDetail{
				Current: status.Offset,
				Total:   status.Total,
			}
			si.StartedAt = status.StartedAt
			si.UpdatedAt = status.UpdatedAt
			if status.Offset >= status.Total {
				if status.UploadUUID == "" {
					si.Status = "done"
				} else {
					si.Status = "committing"
				}
			} else {
				si.Status = "uploading"
			}
		}
		statuses = append(statuses, si)
	}

	return statuses
}

// PushProcess translates upload progress to json stream
func PushProcess(ctx context.Context, ongoing *PushJobs, stream *JSONStream) {
	var (
		ticker = time.NewTicker(100 * time.Millisecond)
		done   bool
	)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, si := range ongoing.Status() {
				stream.WriteObject(si)
			}

			if done {
				return
			}
		case <-ctx.Done():
			done = true // allow ui to update once more
		}
	}
}
