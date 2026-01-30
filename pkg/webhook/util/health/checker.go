/*
Copyright 2020 The Kruise Authors.

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

package health

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"

	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
)

var (
	caCertFilePath = path.Join(webhookutil.GetCertDir(), "ca-cert.pem")

	// Initialization state: 0=uninit, 1=initializing, 2=success, 3=failed
	initState   int32
	initLock    sync.Mutex // Protects initialization attempt
	initErr     error      // Stores last initialization error
	lock        sync.Mutex // Protects client access
	client      *http.Client
	certWatcher *fsnotify.Watcher
)

func loadHTTPClientWithCACert() error {
	caCert, err := os.ReadFile(caCertFilePath)
	if err != nil {
		return err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	lock.Lock()
	defer lock.Unlock()
	client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}
	return nil
}

func watchCACert(watcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-watcher.Events:
			// Channel is closed.
			if !ok {
				return
			}

			// Only care about events which may modify the contents of the file.
			if !(isWrite(event) || isRemove(event) || isCreate(event)) {
				continue
			}

			klog.InfoS("Watched ca-cert", "eventName", event.Name, "operation", event.Op)

			// If the file was removed, re-add the watch.
			if isRemove(event) {
				if err := watcher.Add(event.Name); err != nil {
					klog.ErrorS(err, "Failed to re-watch ca-cert", "eventName", event.Name)
				}
			}

			if err := loadHTTPClientWithCACert(); err != nil {
				klog.ErrorS(err, "Failed to reload ca-cert", "eventName", event.Name)
			}

		case err, ok := <-watcher.Errors:
			// Channel is closed.
			if !ok {
				return
			}
			klog.ErrorS(err, "Failed to watch ca-cert")
		}
	}
}

func isWrite(event fsnotify.Event) bool {
	return event.Op&fsnotify.Write == fsnotify.Write
}

func isCreate(event fsnotify.Event) bool {
	return event.Op&fsnotify.Create == fsnotify.Create
}

func isRemove(event fsnotify.Event) bool {
	return event.Op&fsnotify.Remove == fsnotify.Remove
}

// tryInitialize attempts to initialize the health checker.
// Returns nil on success, error on failure.
// Safe to call multiple times - uses initLock for concurrency control.
func tryInitialize() error {
	// Fast path: already initialized successfully
	if atomic.LoadInt32(&initState) == 2 {
		return nil
	}

	initLock.Lock()
	defer initLock.Unlock()

	// Check again after acquiring lock (double-checked locking)
	currentState := atomic.LoadInt32(&initState)
	if currentState == 2 {
		return nil // Another goroutine succeeded
	}
	if currentState == 1 {
		return fmt.Errorf("initialization in progress by another goroutine")
	}

	// Mark as initializing
	atomic.StoreInt32(&initState, 1)

	// Attempt to load CA cert
	if err := loadHTTPClientWithCACert(); err != nil {
		atomic.StoreInt32(&initState, 3) // Failed
		initErr = fmt.Errorf("failed to load CA cert: %w", err)
		klog.ErrorS(initErr, "Health checker initialization failed")
		return initErr
	}

	// Attempt to create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		atomic.StoreInt32(&initState, 3)
		initErr = fmt.Errorf("failed to create fsnotify watcher: %w", err)
		klog.ErrorS(initErr, "Health checker initialization failed")
		return initErr
	}

	// Attempt to watch cert file
	if err = watcher.Add(caCertFilePath); err != nil {
		watcher.Close() // Clean up
		atomic.StoreInt32(&initState, 3)
		initErr = fmt.Errorf("failed to watch %s: %w", caCertFilePath, err)
		klog.ErrorS(initErr, "Health checker initialization failed")
		return initErr
	}

	// Success!
	certWatcher = watcher
	go watchCACert(watcher)
	atomic.StoreInt32(&initState, 2)
	initErr = nil
	klog.InfoS("Health checker initialized successfully")
	return nil
}

func Checker(_ *http.Request) error {
	// Attempt initialization (idempotent, returns immediately if already initialized)
	if err := tryInitialize(); err != nil {
		return fmt.Errorf("health checker not initialized: %w", err)
	}

	// Proceed with health check
	url := fmt.Sprintf("https://localhost:%d/healthz", webhookutil.GetPort())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	lock.Lock()
	defer lock.Unlock()
	if client == nil {
		return fmt.Errorf("http client not initialized")
	}
	_, err = client.Do(req)
	return err
}
