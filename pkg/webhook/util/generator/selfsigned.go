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

package generator

import (
	"crypto"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"time"

	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
)

const (
	rsaKeySize = 2048
)

// ServiceToCommonName generates the CommonName for the certificate when using a k8s service.
func ServiceToCommonName(serviceNamespace, serviceName string) string {
	return fmt.Sprintf("%s.%s.svc", serviceName, serviceNamespace)
}

// SelfSignedCertGenerator implements the certGenerator interface.
// It provisions self-signed certificates.
type SelfSignedCertGenerator struct {
	caKey  []byte
	caCert []byte
}

var _ CertGenerator = &SelfSignedCertGenerator{}

// SetCA sets the PEM-encoded CA private key and CA cert for signing the generated serving cert.
func (cp *SelfSignedCertGenerator) SetCA(caKey, caCert []byte) {
	cp.caKey = caKey
	cp.caCert = caCert
}

// Generate creates and returns a CA certificate, certificate and
// key for the server. serverKey and serverCert are used by the server
// to establish trust for clients, CA certificate is used by the
// client to verify the server authentication chain.
// The cert will be valid for 365 days.
func (cp *SelfSignedCertGenerator) Generate(commonName string) (*Artifacts, error) {
	var signingKey *rsa.PrivateKey
	var signingCert *x509.Certificate
	var valid bool
	var err error

	valid, signingKey, signingCert = cp.validCACert()
	if !valid {
		signingKey, err = NewPrivateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to create the CA private key: %v", err)
		}
		signingCert, err = cert.NewSelfSignedCACert(cert.Config{CommonName: "webhook-cert-ca"}, signingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create the CA cert: %v", err)
		}
	}

	hostIP := net.ParseIP(commonName)
	var altIPs []net.IP
	DNSNames := []string{"localhost"}
	if hostIP.To4() != nil {
		altIPs = append(altIPs, hostIP.To4())
	} else {
		DNSNames = append(DNSNames, commonName)
	}

	key, err := NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to create the private key: %v", err)
	}
	signedCert, err := NewSignedCert(
		cert.Config{
			CommonName: commonName,
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			AltNames:   cert.AltNames{IPs: altIPs, DNSNames: DNSNames},
		},
		key, signingCert, signingKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create the cert: %v", err)
	}
	return &Artifacts{
		Key:    EncodePrivateKeyPEM(key),
		Cert:   EncodeCertPEM(signedCert),
		CAKey:  EncodePrivateKeyPEM(signingKey),
		CACert: EncodeCertPEM(signingCert),
	}, nil
}

func (cp *SelfSignedCertGenerator) validCACert() (bool, *rsa.PrivateKey, *x509.Certificate) {
	if !ValidCACert(cp.caKey, cp.caCert, cp.caCert, "",
		time.Now().AddDate(1, 0, 0)) {
		return false, nil, nil
	}

	var ok bool
	key, err := keyutil.ParsePrivateKeyPEM(cp.caKey)
	if err != nil {
		return false, nil, nil
	}
	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return false, nil, nil
	}

	certs, err := cert.ParseCertsPEM(cp.caCert)
	if err != nil {
		return false, nil, nil
	}
	if len(certs) != 1 {
		return false, nil, nil
	}
	return true, privateKey, certs[0]
}

// NewPrivateKey creates an RSA private key
func NewPrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(cryptorand.Reader, rsaKeySize)
}

// NewSignedCert creates a signed certificate using the given CA certificate and key
func NewSignedCert(cfg cert.Config, key crypto.Signer, caCert *x509.Certificate, caKey crypto.Signer) (*x509.Certificate, error) {
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}
	if len(cfg.Usages) == 0 {
		return nil, errors.New("must specify at least one ExtKeyUsage")
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    caCert.NotBefore,
		NotAfter:     time.Now().Add(time.Hour * 24 * 365 * 10).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.Usages,
	}
	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

// EncodePrivateKeyPEM returns PEM-encoded private key data
func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  keyutil.RSAPrivateKeyBlockType,
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(&block)
}

// EncodeCertPEM returns PEM-encoded certificate data
func EncodeCertPEM(ct *x509.Certificate) []byte {
	block := pem.Block{
		Type:  cert.CertificateBlockType,
		Bytes: ct.Raw,
	}
	return pem.EncodeToMemory(&block)
}
