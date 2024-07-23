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
	SecretCertWriter = "secret"
)

// secretCertWriter provisions the certificate by reading and writing to the k8s secrets.
type secretCertWriter struct {
	*SecretCertWriterOptions

	// dnsName is the DNS name that the certificate is for.
	dnsName string
}

// SecretCertWriterOptions is options for constructing a secretCertWriter.
type SecretCertWriterOptions struct {
	// client talks to a kubernetes cluster for creating the secret.
	Clientset clientset.Interface
	// certGenerator generates the certificates.
	CertGenerator generator.CertGenerator
	// secret points the secret that contains certificates that written by the CertWriter.
	Secret *types.NamespacedName
}

var _ CertWriter = &secretCertWriter{}

func (ops *SecretCertWriterOptions) setDefaults() {
	if ops.CertGenerator == nil {
		ops.CertGenerator = &generator.SelfSignedCertGenerator{}
	}
}

func (ops *SecretCertWriterOptions) validate() error {
	if ops.Clientset == nil {
		return errors.New("client must be set in SecretCertWriterOptions")
	}
	if ops.Secret == nil {
		return errors.New("secret must be set in SecretCertWriterOptions")
	}
	return nil
}

// NewSecretCertWriter constructs a CertWriter that persists the certificate in a k8s secret.
func NewSecretCertWriter(ops SecretCertWriterOptions) (CertWriter, error) {
	ops.setDefaults()
	err := ops.validate()
	if err != nil {
		return nil, err
	}
	return &secretCertWriter{SecretCertWriterOptions: &ops}, nil
}

// EnsureCert provisions certificates for a webhookClientConfig by writing the certificates to a k8s secret.
func (s *secretCertWriter) EnsureCert(dnsName string) (*generator.Artifacts, bool, error) {
	// Create or refresh the certs based on clientConfig
	s.dnsName = dnsName
	return handleCommon(s.dnsName, s)
}

var _ certReadWriter = &secretCertWriter{}

func (s *secretCertWriter) buildSecret() (*corev1.Secret, *generator.Artifacts, error) {
	certs, err := s.CertGenerator.Generate(s.dnsName)
	if err != nil {
		return nil, nil, err
	}
	secret := certsToSecret(certs, *s.Secret)
	return secret, certs, err
}

func (s *secretCertWriter) write() (*generator.Artifacts, error) {
	secret, certs, err := s.buildSecret()
	if err != nil {
		return nil, err
	}
	_, err = s.Clientset.CoreV1().Secrets(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil, alreadyExistError{err}
	}
	return certs, err
}

func (s *secretCertWriter) overwrite(resourceVersion string) (*generator.Artifacts, error) {
	secret, certs, err := s.buildSecret()
	if err != nil {
		return nil, err
	}
	secret.ResourceVersion = resourceVersion
	secret, err = s.Clientset.CoreV1().Secrets(secret.Namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "Cert writer update secret failed")
		return nil, err
	}
	klog.InfoS("Cert writer update secret resourceVersion",
		"name", secret.Name, "from", resourceVersion, "to", secret.ResourceVersion,
	)
	return certs, nil
}

func (s *secretCertWriter) read() (*generator.Artifacts, error) {
	secret, err := s.Clientset.CoreV1().Secrets(s.Secret.Namespace).Get(context.TODO(), s.Secret.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, notFoundError{err}
	}
	if err != nil {
		return nil, err
	}
	certs := secretToCerts(secret)
	if certs.CACert != nil && certs.CAKey != nil {
		// Store the CA for next usage.
		s.CertGenerator.SetCA(certs.CAKey, certs.CACert)
	}
	return certs, nil
}

func secretToCerts(secret *corev1.Secret) *generator.Artifacts {
	ret := &generator.Artifacts{
		ResourceVersion: secret.ResourceVersion,
	}
	if secret.Data != nil {
		ret.CAKey = secret.Data[CAKeyName]
		ret.CACert = secret.Data[CACertName]
		ret.Cert = secret.Data[ServerCertName]
		ret.Key = secret.Data[ServerKeyName]
	}
	return ret
}

func certsToSecret(certs *generator.Artifacts, sec types.NamespacedName) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: sec.Namespace,
			Name:      sec.Name,
		},
		Data: map[string][]byte{
			CAKeyName:       certs.CAKey,
			CACertName:      certs.CACert,
			ServerKeyName:   certs.Key,
			ServerKeyName2:  certs.Key,
			ServerCertName:  certs.Cert,
			ServerCertName2: certs.Cert,
		},
	}
}
