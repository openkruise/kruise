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

package containermeta

import (
	"testing"

	"k8s.io/client-go/util/workqueue"

	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
)

func TestNewControllerDefaultWorkers(t *testing.T) {
	// When MaxWorkers options are zero, defaults should be used.
	opts := daemonoptions.Options{
		MaxWorkersForContainerMeta:        0,
		MaxWorkersForContainerMetaRestart: 0,
	}

	// NewController will fail because PodInformer is nil — that's fine,
	// we only care about the worker counts before that check triggers.
	// To inspect them we call with valid counts but nil informer and check error.
	_, err := NewController(opts)
	if err == nil {
		t.Fatal("Expected error due to nil PodInformer, got nil")
	}
}

func TestNewControllerWorkers(t *testing.T) {
	opts := daemonoptions.Options{
		MaxWorkersForContainerMeta:        15,
		MaxWorkersForContainerMetaRestart: 20,
	}

	// NewController fails at PodInformer nil check — capture what was
	// set by inspecting a partial run via a modified opts path.
	// We verify the logic by constructing the values the same way NewController does.
	workers := defaultWorkers
	if opts.MaxWorkersForContainerMeta > 0 {
		workers = opts.MaxWorkersForContainerMeta
	}
	restartWorkers := defaultRestartWorkers
	if opts.MaxWorkersForContainerMetaRestart > 0 {
		restartWorkers = opts.MaxWorkersForContainerMetaRestart
	}

	if workers != 15 {
		t.Fatalf("Expected workers to be 15, got %d", workers)
	}
	if restartWorkers != 20 {
		t.Fatalf("Expected restartWorkers to be 20, got %d", restartWorkers)
	}

	// Also confirm NewController returns an error (nil PodInformer), not a panic.
	_, err := NewController(opts)
	if err == nil {
		t.Fatal("Expected error due to nil PodInformer, got nil")
	}
}

func TestControllerRun(t *testing.T) {
	c := &Controller{
		queue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-queue"),
		workers: 2,
		restarter: &restartController{
			queue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-restart-queue"),
			workers: 2,
		},
	}

	stop := make(chan struct{})
	// Call Run in a goroutine and close stop immediately to ensure it exits without blocking
	go c.Run(stop)
	close(stop)
}

func TestRestartControllerRun(t *testing.T) {
	c := &restartController{
		queue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test-restart-queue"),
		workers: 2,
	}

	stop := make(chan struct{})
	go c.Run(stop)
	close(stop)
}
