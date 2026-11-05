/*
Copyright 2026 The Kruise Authors.

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

package configuration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	webhooktypes "github.com/openkruise/kruise/pkg/webhook/types"
)

const podSelectorOverride = `[{"targetWebhook":"mpod.kb.io","namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"kube-system"}},"objectSelector":{"matchLabels":{"app":"dind-service","dind-service-hot-upgrade":"true"}}}]`

func basePodWebhook() admissionregistrationv1.MutatingWebhook {
	path := "/mutate-pod"
	fail := admissionregistrationv1.Fail
	timeout := int32(10)
	return admissionregistrationv1.MutatingWebhook{
		Name: "mpod.kb.io",
		NamespaceSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key: "kubernetes.io/metadata.name", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"kube-system"},
		}}},
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			Service: &admissionregistrationv1.ServiceReference{Path: &path}, CABundle: []byte("test-ca"),
		},
		FailurePolicy: &fail, TimeoutSeconds: &timeout,
		Rules: []admissionregistrationv1.RuleWithOperations{{
			Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
			Rule:       admissionregistrationv1.Rule{Resources: []string{"pods"}},
		}},
	}
}

func TestWebhookSelectorDefaultsAndGate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.WebhookSelectorOverrides, false)
	base := []admissionregistrationv1.MutatingWebhook{basePodWebhook()}
	actual, err := applyMutatingWebhookSelectors(base, nil)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(actual).To(gomega.Equal(base))
	_, err = applyMutatingWebhookSelectors(base, map[string]string{WebhookSelectorsAnnotation: podSelectorOverride})
	g.Expect(err).To(gomega.HaveOccurred())
}

func TestWebhookSelectorReplacement(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.WebhookSelectorOverrides, true)
	base := []admissionregistrationv1.MutatingWebhook{basePodWebhook(), {Name: "other.kb.io"}}
	original := base[0].DeepCopy()
	actual, err := applyMutatingWebhookSelectors(base, map[string]string{WebhookSelectorsAnnotation: podSelectorOverride})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(actual).To(gomega.HaveLen(2))
	g.Expect(actual[1]).To(gomega.Equal(base[1]))
	g.Expect(base[0]).To(gomega.Equal(*original))
	ns, err := metav1.LabelSelectorAsSelector(actual[0].NamespaceSelector)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	obj, err := metav1.LabelSelectorAsSelector(actual[0].ObjectSelector)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	for _, tc := range []struct {
		namespace string
		podLabels labels.Set
		matches   bool
	}{
		{"kube-system", labels.Set{"app": "dind-service", "dind-service-hot-upgrade": "true"}, true},
		{"default", labels.Set{"app": "dind-service", "dind-service-hot-upgrade": "true"}, false},
		{"kube-system", labels.Set{"app": "dind-service"}, false},
		{"default", labels.Set{}, false},
	} {
		g.Expect(ns.Matches(labels.Set{"kubernetes.io/metadata.name": tc.namespace}) && obj.Matches(tc.podLabels)).To(gomega.Equal(tc.matches))
	}
	// All fields other than the supplied selectors remain identical.
	actual[0].NamespaceSelector = original.NamespaceSelector
	actual[0].ObjectSelector = original.ObjectSelector
	g.Expect(actual[0]).To(gomega.Equal(*original))
}

func TestPartialAndEmptyWebhookSelectors(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.WebhookSelectorOverrides, true)
	base := basePodWebhook()
	actual, err := applyMutatingWebhookSelectors([]admissionregistrationv1.MutatingWebhook{base}, map[string]string{
		WebhookSelectorsAnnotation: `[{"targetWebhook":"mpod.kb.io","objectSelector":{}}]`,
	})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(actual[0].NamespaceSelector).To(gomega.Equal(base.NamespaceSelector))
	g.Expect(actual[0].ObjectSelector).To(gomega.Equal(&metav1.LabelSelector{}))
	validating := []admissionregistrationv1.ValidatingWebhook{{Name: "vpod.kb.io", NamespaceSelector: base.NamespaceSelector, ObjectSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"keep": "true"}}}}
	v, err := applyValidatingWebhookSelectors(validating, map[string]string{
		WebhookSelectorsAnnotation: `[{"targetWebhook":"vpod.kb.io","namespaceSelector":{}}]`,
	})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(v).To(gomega.HaveLen(1))
	g.Expect(v[0].NamespaceSelector).To(gomega.Equal(&metav1.LabelSelector{}))
	g.Expect(v[0].ObjectSelector).To(gomega.Equal(validating[0].ObjectSelector))
	g.Expect(validating[0].NamespaceSelector).To(gomega.Equal(base.NamespaceSelector))
}

func TestInvalidWebhookSelectorOverrides(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.WebhookSelectorOverrides, true)
	for _, raw := range []string{
		`[`, `{}`, `[] []`,
		`[{"objectSelector":{}}]`,
		`[{"targetWebhook":"mpod.kb.io"}]`,
		`[{"targetWebhook":"mpod.kb.io","objectSelector":null}]`,
		`[{"targetWebhook":"mpod.kb.io","name":"extra.kb.io","objectSelector":{}}]`,
		`[{"targetWebhook":"unknown.kb.io","objectSelector":{}}]`,
		`[{"targetWebhook":"mpod.kb.io","objectSelector":{}},{"targetWebhook":"mpod.kb.io","namespaceSelector":{}}]`,
		`[{"targetWebhook":"mpod.kb.io","objectSelector":{"matchExpressions":[{"key":"app","operator":"In"}]}}]`,
		`[{"targetWebhook":"mpod.kb.io","namespaceSelector":{"matchLabels":{"bad key":"value"}}}]`,
	} {
		t.Run(raw, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			annotations := map[string]string{WebhookSelectorsAnnotation: raw}
			_, err := applyMutatingWebhookSelectors([]admissionregistrationv1.MutatingWebhook{basePodWebhook()}, annotations)
			g.Expect(err).To(gomega.HaveOccurred())
			_, err = applyValidatingWebhookSelectors([]admissionregistrationv1.ValidatingWebhook{{Name: "mpod.kb.io"}}, annotations)
			g.Expect(err).To(gomega.HaveOccurred())
		})
	}
}

func TestEnsureWebhookSelectorLifecycle(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.WebhookSelectorOverrides, true)
	for _, external := range []bool{false, true} {
		t.Run(fmt.Sprintf("external-certs=%t", external), func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableExternalCerts, external)
			g := gomega.NewGomegaWithT(t)
			base := basePodWebhook()
			client := fake.NewSimpleClientset(
				&admissionregistrationv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: mutatingWebhookConfigurationName, Annotations: map[string]string{WebhookSelectorsAnnotation: podSelectorOverride}}, Webhooks: []admissionregistrationv1.MutatingWebhook{base}},
				&admissionregistrationv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: validatingWebhookConfigurationName}},
			)
			handlers := map[string]webhooktypes.HandlerGetter{"/mutate-pod": nil}
			get := func() *admissionregistrationv1.MutatingWebhookConfiguration {
				v, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), mutatingWebhookConfigurationName, metav1.GetOptions{})
				g.Expect(err).NotTo(gomega.HaveOccurred())
				return v
			}
			update := func(v *admissionregistrationv1.MutatingWebhookConfiguration) {
				_, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(context.Background(), v, metav1.UpdateOptions{})
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}
			g.Expect(Ensure(client, handlers, []byte("test-ca"))).To(gomega.Succeed())
			current := get()
			g.Expect(current.Webhooks).To(gomega.HaveLen(1))
			g.Expect(current.Webhooks[0].NamespaceSelector.MatchLabels).To(gomega.HaveKeyWithValue("kubernetes.io/metadata.name", "kube-system"))
			var template []admissionregistrationv1.MutatingWebhook
			g.Expect(json.Unmarshal([]byte(current.Annotations["template"]), &template)).To(gomega.Succeed())
			g.Expect(template[0].NamespaceSelector).To(gomega.Equal(base.NamespaceSelector))
			g.Expect(Ensure(client, handlers, []byte("test-ca"))).To(gomega.Succeed())
			g.Expect(get().Webhooks).To(gomega.Equal(current.Webhooks))
			// Invalid changes must not replace the last effective allowlist.
			current.Annotations[WebhookSelectorsAnnotation] = `[{"targetWebhook":"missing.kb.io","objectSelector":{}}]`
			update(current)
			g.Expect(Ensure(client, handlers, []byte("test-ca"))).NotTo(gomega.Succeed())
			g.Expect(get().Webhooks).To(gomega.Equal(current.Webhooks))
			// Removing the annotation restores both template selectors.
			delete(current.Annotations, WebhookSelectorsAnnotation)
			update(current)
			g.Expect(Ensure(client, handlers, []byte("test-ca"))).To(gomega.Succeed())
			g.Expect(get().Webhooks[0].NamespaceSelector).To(gomega.Equal(base.NamespaceSelector))
			g.Expect(get().Webhooks[0].ObjectSelector).To(gomega.Equal(base.ObjectSelector))
			g.Expect(get().Annotations).NotTo(gomega.HaveKey(webhookSelectorsManagedAnnotation))
		})
	}
}

func TestEnsureValidatingSelectorLifecycle(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.WebhookSelectorOverrides, true)
	for _, external := range []bool{false, true} {
		t.Run(fmt.Sprintf("external-certs=%t", external), func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableExternalCerts, external)
			g := gomega.NewGomegaWithT(t)
			path := "/validate-pod"
			base := admissionregistrationv1.ValidatingWebhook{
				Name:              "vpod.kb.io",
				NamespaceSelector: basePodWebhook().NamespaceSelector,
				ClientConfig:      admissionregistrationv1.WebhookClientConfig{CABundle: []byte("test-ca"), Service: &admissionregistrationv1.ServiceReference{Path: &path}},
			}
			client := fake.NewSimpleClientset(
				&admissionregistrationv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: mutatingWebhookConfigurationName}},
				&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: validatingWebhookConfigurationName, Annotations: map[string]string{
						WebhookSelectorsAnnotation: `[{"targetWebhook":"vpod.kb.io","namespaceSelector":{},"objectSelector":{"matchLabels":{"opt-in":"true"}}}]`,
					}},
					Webhooks: []admissionregistrationv1.ValidatingWebhook{base},
				},
			)
			handlers := map[string]webhooktypes.HandlerGetter{path: nil}
			get := func() *admissionregistrationv1.ValidatingWebhookConfiguration {
				v, err := client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.Background(), validatingWebhookConfigurationName, metav1.GetOptions{})
				g.Expect(err).NotTo(gomega.HaveOccurred())
				return v
			}
			g.Expect(Ensure(client, handlers, []byte("test-ca"))).To(gomega.Succeed())
			current := get()
			g.Expect(current.Webhooks).To(gomega.HaveLen(1))
			g.Expect(current.Webhooks[0].NamespaceSelector).To(gomega.Equal(&metav1.LabelSelector{}))
			g.Expect(current.Webhooks[0].ObjectSelector.MatchLabels).To(gomega.HaveKeyWithValue("opt-in", "true"))
			// Changing to a partial override restores the omitted selector from the template.
			current.Annotations[WebhookSelectorsAnnotation] = `[{"targetWebhook":"vpod.kb.io","objectSelector":{"matchLabels":{"opt-in":"next"}}}]`
			_, err := client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(context.Background(), current, metav1.UpdateOptions{})
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(Ensure(client, handlers, []byte("test-ca"))).To(gomega.Succeed())
			current = get()
			g.Expect(current.Webhooks[0].NamespaceSelector).To(gomega.Equal(base.NamespaceSelector))
			g.Expect(current.Webhooks[0].ObjectSelector.MatchLabels).To(gomega.HaveKeyWithValue("opt-in", "next"))
			delete(current.Annotations, WebhookSelectorsAnnotation)
			_, err = client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(context.Background(), current, metav1.UpdateOptions{})
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(Ensure(client, handlers, []byte("test-ca"))).To(gomega.Succeed())
			g.Expect(get().Webhooks[0].NamespaceSelector).To(gomega.Equal(base.NamespaceSelector))
			g.Expect(get().Webhooks[0].ObjectSelector).To(gomega.BeNil())
			g.Expect(get().Annotations).NotTo(gomega.HaveKey(webhookSelectorsManagedAnnotation))
		})
	}
}
