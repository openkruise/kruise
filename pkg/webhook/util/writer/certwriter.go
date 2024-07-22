/*
Copyright 2020 The Kruise Authors.
Copyright 2018 The Kubernetes Authors.

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

package writer

import (
	"errors"
	"time"

	"k8s.io/klog/v2"

	"github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/generator"
)

const (
	// CAKeyName is the name of the CA private key
	CAKeyName = "ca-key.pem"
	// CACertName is the name of the CA certificate
	CACertName = "ca-cert.pem"
	// ServerKeyName is the name of the server private key
	ServerKeyName  = "key.pem"
	ServerKeyName2 = "tls.key"
	// ServerCertName is the name of the serving certificate
	ServerCertName  = "cert.pem"
	ServerCertName2 = "tls.crt"
)

// CertWriter provides method to handle webhooks.
type CertWriter interface {
	// EnsureCert provisions the cert for the webhookClientConfig.
	EnsureCert(dnsName string) (*generator.Artifacts, bool, error)
}

// handleCommon ensures the given webhook has a proper certificate.
// It uses the given certReadWriter to read and (or) write the certificate.
func handleCommon(dnsName string, ch certReadWriter) (*generator.Artifacts, bool, error) {
	if len(dnsName) == 0 {
		return nil, false, errors.New("dnsName should not be empty")
	}
	if ch == nil {
		return nil, false, errors.New("certReaderWriter should not be nil")
	}

	certs, changed, err := createIfNotExists(ch)
	if err != nil {
		return nil, changed, err
	}

	// Recreate the cert if it's invalid.
	renewBefore := util.GetRenewBeforeTime()
	valid := validCert(certs, dnsName, time.Now().Add(renewBefore))
	if !valid {
		klog.Info("cert is invalid or expired, regenerating a new one")
		certs, err = ch.overwrite(certs.ResourceVersion)
		if err != nil {
			return nil, false, err
		}
		changed = true
	}
	return certs, changed, nil
}

func createIfNotExists(ch certReadWriter) (*generator.Artifacts, bool, error) {
	// Try to read first
	certs, err := ch.read()
	if isNotFound(err) {
		// Create if not exists
		certs, err = ch.write()
		// This may happen if there is another racer.
		if isAlreadyExists(err) {
			certs, err = ch.read()
		}
		return certs, true, err
	}
	return certs, false, err
}

// certReadWriter provides methods for reading and writing certificates.
type certReadWriter interface {
	// read a webhook name and returns the certs for it.
	read() (*generator.Artifacts, error)
	// write the certs and return the certs it wrote.
	write() (*generator.Artifacts, error)
	// overwrite the existing certs and return the certs it wrote.
	overwrite(resourceVersion string) (*generator.Artifacts, error)
}

func validCert(certs *generator.Artifacts, dnsName string, expired time.Time) bool {
	if certs == nil {
		return false
	}
	return generator.ValidCACert(certs.Key, certs.Cert, certs.CACert, dnsName, expired)
}
