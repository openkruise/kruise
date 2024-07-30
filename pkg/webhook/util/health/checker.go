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

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"

	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
)

var (
	caCertFilePath = path.Join(webhookutil.GetCertDir(), "ca-cert.pem")

	onceWatch sync.Once
	lock      sync.Mutex
	client    *http.Client
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

func Checker(_ *http.Request) error {
	onceWatch.Do(func() {
		if err := loadHTTPClientWithCACert(); err != nil {
			panic(fmt.Errorf("failed to load ca-cert for the first time: %v", err))
		}
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			panic(fmt.Errorf("failed to new ca-cert watcher: %v", err))
		}
		if err = watcher.Add(caCertFilePath); err != nil {
			panic(fmt.Errorf("failed to add %v into watcher: %v", caCertFilePath, err))
		}
		go watchCACert(watcher)
	})

	url := fmt.Sprintf("https://localhost:%d/healthz", webhookutil.GetPort())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	lock.Lock()
	defer lock.Unlock()
	_, err = client.Do(req)
	return err
}
