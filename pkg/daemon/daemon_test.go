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

package daemon

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openkruise/kruise/pkg/client"
	"k8s.io/client-go/rest"
)

func TestNewDaemon(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api" || r.URL.Path == "/apis" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"versions": ["v1"]}`))
			return
		}
		if r.URL.Path == "/version" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"major": "1", "minor": "30", "gitVersion": "v1.30.0"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := &rest.Config{Host: server.URL}

	err := client.NewRegistry(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize registry: %v", err)
	}

	os.Setenv("NODE_NAME", "test-node")
	defer os.Unsetenv("NODE_NAME")

	socketDir := t.TempDir()
	socketFile := filepath.Join(socketDir, "docker.sock")
	f, _ := os.Create(socketFile)
	f.Close()

	os.Setenv("HOST_VAR_RUN_DIR", socketDir)
	defer os.Unsetenv("HOST_VAR_RUN_DIR")

	_, err = NewDaemon(cfg, ":0", 1, 2, 3)
	t.Logf("NewDaemon returned: %v", err)
}
