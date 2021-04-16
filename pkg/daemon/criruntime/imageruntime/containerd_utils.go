/*
Copyright 2021 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package imageruntime

import (
	"context"
	"sync"
	"time"

	"github.com/alibaba/pouch/pkg/jsonstream"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/remotes"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// fetchProgress reads Ingester status and write status into stream.
func fetchProgress(ctx context.Context, cs content.Store, ongoing *fetchJobs, stream *jsonstream.JSONStream) error {
	var (
		ticker     = time.NewTicker(300 * time.Millisecond)
		start      = time.Now()
		progresses = map[string]jsonstream.JSONMessage{}
		done       bool
	)
	defer ticker.Stop()

outer:
	for {
		select {
		case <-ticker.C:
			resolved := jsonstream.PullStatusResolved
			if !ongoing.isResolved() {
				resolved = jsonstream.PullStatusResolving
			}
			progresses[ongoing.name] = jsonstream.JSONMessage{
				ID:     ongoing.name,
				Status: resolved,
				Detail: &jsonstream.ProgressDetail{},
			}
			keys := []string{ongoing.name}

			activeSeen := map[string]struct{}{}
			if !done {
				actives, err := cs.ListStatuses(ctx, "")
				if err != nil {
					continue
				}
				// update status of active entries!
				for _, active := range actives {
					progresses[active.Ref] = jsonstream.JSONMessage{
						ID:     active.Ref,
						Status: jsonstream.PullStatusDownloading,
						Detail: &jsonstream.ProgressDetail{
							Current: active.Offset,
							Total:   active.Total,
						},
						StartedAt: active.StartedAt,
						UpdatedAt: active.UpdatedAt,
					}
					activeSeen[active.Ref] = struct{}{}
				}
			}

			// now, update the items in jobs that are not in active
			for _, j := range ongoing.jobs() {
				key := remotes.MakeRefKey(ctx, j)
				keys = append(keys, key)
				if _, ok := activeSeen[key]; ok {
					continue
				}

				status, ok := progresses[key]
				if !done && (!ok || status.Status == jsonstream.PullStatusDownloading) {
					info, err := cs.Info(ctx, j.Digest)
					if err != nil {
						if !errdefs.IsNotFound(err) {
							continue outer
						} else {
							progresses[key] = jsonstream.JSONMessage{
								ID:     key,
								Status: jsonstream.PullStatusWaiting,
							}
						}
					} else if info.CreatedAt.After(start) {
						progresses[key] = jsonstream.JSONMessage{
							ID:     key,
							Status: jsonstream.PullStatusDone,
							Detail: &jsonstream.ProgressDetail{
								Current: info.Size,
								Total:   info.Size,
							},
							UpdatedAt: info.CreatedAt,
						}
					} else {
						progresses[key] = jsonstream.JSONMessage{
							ID:     key,
							Status: jsonstream.PullStatusExists,
						}
					}
				} else if done {
					if ok {
						if status.Status != jsonstream.PullStatusDone &&
							status.Status != jsonstream.PullStatusExists {

							status.Status = jsonstream.PullStatusDone
							progresses[key] = status
						}
					} else {
						progresses[key] = jsonstream.JSONMessage{
							ID:     key,
							Status: jsonstream.PullStatusDone,
						}
					}
				}
			}

			for _, key := range keys {
				stream.WriteObject(progresses[key])
			}

			if done {
				return nil
			}
		case <-ctx.Done():
			done = true // allow ui to update once more
		}
	}
}

type fetchJobs struct {
	name     string
	added    map[digest.Digest]struct{}
	descs    []ocispec.Descriptor
	mu       sync.Mutex
	resolved bool
}

func newFetchJobs(name string) *fetchJobs {
	return &fetchJobs{
		name:  name,
		added: map[digest.Digest]struct{}{},
	}
}

func (j *fetchJobs) add(desc ocispec.Descriptor) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.resolved = true

	if _, ok := j.added[desc.Digest]; ok {
		return
	}
	j.descs = append(j.descs, desc)
	j.added[desc.Digest] = struct{}{}
}

func (j *fetchJobs) jobs() []ocispec.Descriptor {
	j.mu.Lock()
	defer j.mu.Unlock()

	var descs []ocispec.Descriptor
	return append(descs, j.descs...)
}

func (j *fetchJobs) isResolved() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.resolved
}
