/*
Copyright 2024 The Kruise Authors.

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
	"bytes"
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openkruise/kruise/pkg/webhook/util/generator"
)

const (
	ExternalCertWriter = "external"
	ExternalCACert     = "ca.crt"
	ExternalCAKey      = "ca.key"
	ExternalServerCert = "tls.crt"
	ExternalServerKey  = "tls.key"
)

var currentExternalCerts *generator.Artifacts

// externalCertWriter provisions the certificate by reading from the k8s secrets.
type externalCertWriter struct {
	*ExternalCertWriterOptions
}

// ExternalCertWriterOptions is options for constructing a externalCertWriter.
type ExternalCertWriterOptions struct {
	// client talks to a kubernetes cluster for creating the secret.
	Clientset clientset.Interface
	// secret points the secret that contains certificates that written by the CertWriter.
	Secret *types.NamespacedName
}

var _ CertWriter = &externalCertWriter{}

func (ops *ExternalCertWriterOptions) validate() error {
	if ops.Clientset == nil {
		return errors.New("client must be set in externalCertWriterOptions")
	}
	if ops.Secret == nil {
		return errors.New("secret must be set in externalCertWriterOptions")
	}
	return nil
}

// NewExternalCertWriter constructs a CertWriter that persists the certificate in a k8s secret.
func NewExternalCertWriter(ops ExternalCertWriterOptions) (CertWriter, error) {
	err := ops.validate()
	if err != nil {
		return nil, err
	}
	return &externalCertWriter{ExternalCertWriterOptions: &ops}, nil
}

// EnsureCert read and validate certs from k8s secret.
func (s *externalCertWriter) EnsureCert(dnsName string) (*generator.Artifacts, bool, error) {
	// Read certs from secrets generated externally
	certs, err := s.read()
	if err != nil {
		return nil, false, err
	}

	// check if the certs are updated since last read
	if currentExternalCerts != nil && compareCerts(certs, currentExternalCerts) {
		klog.Info("external certs are not updated")
		return certs, false, nil
	}

	currentExternalCerts = certs
	return certs, true, nil
}

var _ certReadWriter = &externalCertWriter{}

func (s *externalCertWriter) write() (*generator.Artifacts, error) {
	return nil, nil
}

func (s *externalCertWriter) overwrite(resourceVersion string) (*generator.Artifacts, error) {
	return nil, nil
}

func (s *externalCertWriter) read() (*generator.Artifacts, error) {
	secret, err := s.Clientset.CoreV1().Secrets(s.Secret.Namespace).Get(context.TODO(), s.Secret.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, notFoundError{err}
	}
	if err != nil {
		return nil, err
	}
	certs := externalSecretToCerts(secret)
	return certs, nil
}

func externalSecretToCerts(secret *corev1.Secret) *generator.Artifacts {
	ret := &generator.Artifacts{
		ResourceVersion: secret.ResourceVersion,
	}
	if secret.Data != nil {
		ret.CACert = secret.Data[ExternalCACert]
		ret.CAKey = secret.Data[ExternalCAKey]
		ret.Cert = secret.Data[ExternalServerCert]
		ret.Key = secret.Data[ExternalServerKey]
	}
	return ret
}

func compareCerts(certsA, certsB *generator.Artifacts) bool {
	if !bytes.Equal(certsA.CACert, certsB.CACert) || !bytes.Equal(certsA.CAKey, certsB.CAKey) || !bytes.Equal(certsA.Cert, certsB.Cert) || !bytes.Equal(certsA.Key, certsB.Key) {
		return false
	}
	return true
}
