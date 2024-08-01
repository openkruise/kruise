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
	"context"
	"testing"

	"github.com/onsi/gomega"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/generator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureCert(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	type ExpectOut struct {
		changed      bool
		errorHappens bool
	}
	teatCases := []struct {
		name                 string
		needRefreshCertCache bool
		secret               types.NamespacedName
		expect               ExpectOut
	}{
		{
			name:                 "test get certs from secret",
			needRefreshCertCache: true,
			secret:               types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
			expect: ExpectOut{
				changed:      true,
				errorHappens: false,
			},
		},
		{
			name:                 "test get certs from cache",
			needRefreshCertCache: false,
			secret:               types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
			expect: ExpectOut{
				changed:      false,
				errorHappens: false,
			},
		},
		{
			name:                 "test secret not found",
			needRefreshCertCache: true,
			secret:               types.NamespacedName{Namespace: "default", Name: "not-existed-secret"},
			expect: ExpectOut{
				changed:      false,
				errorHappens: true,
			},
		},
	}
	dnsName := "kruise-webhook-service.svc"
	client := fake.NewSimpleClientset()

	// generate certs and secret
	certGenerator := &generator.SelfSignedCertGenerator{}
	// certs expire after 10 years
	certs, err := certGenerator.Generate(dnsName)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: webhookutil.GetNamespace(),
			Name:      webhookutil.GetSecretName(),
		},
		Data: map[string][]byte{
			ExternalCAKey:      certs.CAKey,
			ExternalCACert:     certs.CACert,
			ExternalServerKey:  certs.Key,
			ExternalServerCert: certs.Cert,
		},
	}
	_, err = client.CoreV1().Secrets(webhookutil.GetNamespace()).Create(context.TODO(), &secret, metav1.CreateOptions{})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	for _, tc := range teatCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needRefreshCertCache {
				RefreshCurrentExternalCertsCache()
			} else {
				UpdateCurrentExternalCertsCache(certs)
			}

			externalCertWriter, err := NewExternalCertWriter(ExternalCertWriterOptions{
				Clientset: client,
				Secret:    &types.NamespacedName{Namespace: tc.secret.Namespace, Name: tc.secret.Name},
			})
			g.Expect(err).NotTo(gomega.HaveOccurred())
			_, changed, err := externalCertWriter.EnsureCert(dnsName)
			g.Expect(changed).Should(gomega.Equal(tc.expect.changed))
			g.Expect(err != nil).Should(gomega.Equal(tc.expect.errorHappens))
		})
	}
}

func RefreshCurrentExternalCertsCache() {
	currentExternalCerts = nil
}

func UpdateCurrentExternalCertsCache(externalCerts *generator.Artifacts) {
	currentExternalCerts = externalCerts
}
