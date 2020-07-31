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
	"io/ioutil"
	"net/http"
	"path"
	"sync"

	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
)

var (
	once    sync.Once
	client  *http.Client
	initErr error
)

func Checker(_ *http.Request) error {
	once.Do(func() {
		var caCert []byte
		certDir := webhookutil.GetCertDir()
		caCert, initErr = ioutil.ReadFile(path.Join(certDir, "ca-cert.pem"))
		if initErr != nil {
			return
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caCertPool,
				},
			},
		}
	})
	if initErr != nil {
		return initErr
	}

	url := fmt.Sprintf("https://localhost:%d/healthz", webhookutil.GetPort())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	return nil
}
