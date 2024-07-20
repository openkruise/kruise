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
	"github.com/openkruise/kruise/pkg/webhook/util/generator"
	v1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		expect               ExpectOut
	}{
		{
			name:                 "test get certs from mutating webhook configuration",
			needRefreshCertCache: true,
			expect: ExpectOut{
				changed:      true,
				errorHappens: false,
			},
		},
		{
			name:                 "test get certs from cache",
			needRefreshCertCache: false,
			expect: ExpectOut{
				changed:      false,
				errorHappens: false,
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
	wh := &v1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: mutatingWebhookConfigurationName,
		},
		Webhooks: []v1.MutatingWebhook{
			{
				ClientConfig: v1.WebhookClientConfig{
					CABundle: certs.CACert,
				},
			},
		},
	}

	_, err = client.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.TODO(), wh, metav1.CreateOptions{})
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
			})
			g.Expect(err).NotTo(gomega.HaveOccurred())
			externalCerts, changed, err := externalCertWriter.EnsureCert(dnsName)
			g.Expect(externalCerts.CACert).Should(gomega.Equal(certs.CACert))
			g.Expect(changed).Should(gomega.Equal(tc.expect.changed))
			g.Expect(err != nil).Should(gomega.Equal(tc.expect.errorHappens))
		})
	}
}

func RefreshCurrentExternalCertsCache() {
	currentExternalCerts = nil
}

func UpdateCurrentExternalCertsCache(externalCerts *generator.Artifacts) {
	currentExternalCerts.CACert = externalCerts.CACert
}
