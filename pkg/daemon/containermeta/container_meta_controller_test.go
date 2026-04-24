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

	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
)

func TestNewControllerWorkers(t *testing.T) {
	// Reset the globals back to defaults after test
	defer func() {
		workers = 5
		restartWorkers = 10
	}()

	opts := daemonoptions.Options{
		MaxWorkersForContainerMeta:        15,
		MaxWorkersForContainerMetaRestart: 20,
	}

	_, err := NewController(opts)
	if err == nil {
		t.Fatalf("Expected error due to nil PodInformer, got nil")
	}

	if workers != 15 {
		t.Fatalf("Expected workers to be 15, got %d", workers)
	}

	if restartWorkers != 20 {
		t.Fatalf("Expected restartWorkers to be 20, got %d", restartWorkers)
	}
}
