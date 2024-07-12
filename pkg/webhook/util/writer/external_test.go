package writer

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/openkruise/kruise/pkg/webhook/util"
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
		// renewBefore controls whether the cert is expired
		renewBefore time.Duration
		secret      types.NamespacedName
		expect      ExpectOut
	}{
		{
			name:                 "test get unexpired certs from secret",
			needRefreshCertCache: true,
			renewBefore:          time.Hour,
			secret:               types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
			expect: ExpectOut{
				changed:      true,
				errorHappens: false,
			},
		},
		{
			name:                 "test get unexpired certs from cache",
			needRefreshCertCache: false,
			renewBefore:          time.Hour,
			secret:               types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
			expect: ExpectOut{
				changed:      false,
				errorHappens: false,
			},
		},
		{
			name:                 "test get expired certs from secret",
			needRefreshCertCache: true,
			renewBefore:          20 * time.Hour * 24 * 365, // 20 years
			secret:               types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
			expect: ExpectOut{
				changed:      false,
				errorHappens: true,
			},
		},
		{
			name:                 "test secret not found",
			needRefreshCertCache: true,
			renewBefore:          time.Hour,
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
			util.SetRenewBeforeTime(tc.renewBefore)

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
