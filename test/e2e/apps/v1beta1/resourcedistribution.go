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

package v1beta1

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	rdutils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	framework "github.com/openkruise/kruise/test/e2e/framework/v1beta1"
)

var _ = ginkgo.Describe("ResourceDistribution", ginkgo.Serial, ginkgo.Label("ResourceDistribution", "v1beta1"), func() {
	f := framework.NewDefaultFramework("resourcedistribution-v1beta1")
	var ns, secretName string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.ResourceDistributionTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		secretName = "resourcedistribution-e2e-test-secret"
		tester = framework.NewResourceDistributionTester(c, kc)
	})

	ginkgo.Context("ResourceDistribution v1beta1 distributing functionality [ResourceDistributionInject]", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentSpecReport().Failed() {
				common.DumpDebugInfo(c, ns)
			}
		})

		// ----------------------------------------------------------------
		// namespace event checker
		// ----------------------------------------------------------------
		ginkgo.It("namespace event checker", func() {
			prefix := "rd-beta-e2e-test1"
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			// pre-create a namespace that will be label-updated to match
			namespaceUpdateMatch := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: prefix + "-update-matched"},
			}
			tester.CreateNamespaces(namespaceUpdateMatch)

			// create ResourceDistribution targeting by namespaceSelector only
			rd := tester.NewBaseResourceDistribution(prefix)
			rd.Spec.Targets.IncludedNamespaces.List = nil
			rd.Spec.Targets.NamespaceSelector.MatchLabels = map[string]string{prefix: "seven"}
			tester.CreateResourceDistribution(rd)

			// create a namespace that already matches the selector
			tester.CreateNamespaces(&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   prefix + "-creat-matched",
					Labels: map[string]string{prefix: "seven"},
				},
			})

			// update existing namespace to match the selector
			namespaceUpdateMatch.Labels = map[string]string{prefix: "seven"}
			tester.UpdateNamespace(namespaceUpdateMatch)

			ginkgo.By("waiting for namespace create and update...")
			gomega.Eventually(func() int {
				matched, _, err := tester.GetNamespaceForDistributor(&rd.Spec.Targets)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return matched.Len()
			}, time.Minute, time.Second).Should(gomega.Equal(2))

			ginkgo.By("checking created secret in every matched namespace...")
			matched, _, err := tester.GetNamespaceForDistributor(&rd.Spec.Targets)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for nsName := range matched {
				gomega.Eventually(func() error {
					_, err := tester.GetSecret(nsName, secretName, true)
					return err
				}, time.Minute, time.Second).ShouldNot(gomega.HaveOccurred())
			}

			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)
			ginkgo.By("Done!")
		})

		// ----------------------------------------------------------------
		// resource event checker — reconciles direct edits and deletes
		// ----------------------------------------------------------------
		ginkgo.It("resource event checker", func() {
			prefix := "rd-beta-e2e-test2"
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			namespaces := []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-2"}},
			}
			tester.CreateNamespaces(namespaces...)

			rd := tester.NewBaseResourceDistribution(prefix)
			tester.CreateResourceDistribution(rd)

			var secret *corev1.Secret
			gomega.Eventually(func() error {
				var err error
				secret, err = tester.GetSecret(namespaces[0].Name, secretName, true)
				return err
			}, time.Minute, time.Second).Should(gomega.BeNil())

			ginkgo.By("update resource directly — controller must revert it...")
			secret.StringData = map[string]string{"updated": "yes"}
			err := tester.UpdateSecret(secret)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() error {
				secret, err = tester.PollSecret(namespaces[0].Name, secretName)
				if err != nil {
					return err
				}
				if len(secret.StringData) != 0 {
					return fmt.Errorf("secret not yet reverted")
				}
				return nil
			}, time.Minute, time.Second).Should(gomega.BeNil())

			ginkgo.By("delete resource directly — controller must recreate it...")
			err = tester.DeleteSecret(secret.Namespace, secret.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() error {
				_, err = tester.PollSecret(namespaces[0].Name, secretName)
				return err
			}, time.Minute, time.Second).Should(gomega.BeNil())

			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)
			ginkgo.By("Done!")
		})

		// ----------------------------------------------------------------
		// full CRUD cycle — mirrors the v1alpha1 "functionality checker"
		// ----------------------------------------------------------------
		ginkgo.It("resourcedistribution functionality checker", func() {
			prefix := "rd-beta-e2e-test3"
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			rd := tester.NewBaseResourceDistribution(prefix)
			cases := []struct {
				name          string
				getNamespaces func() []*corev1.Namespace
			}{
				{
					name: "normal resource distribution case",
					getNamespaces: func() []*corev1.Namespace {
						return []*corev1.Namespace{
							{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-1", Labels: map[string]string{"e2e-rd-group": "one", "environment": "develop"}}},
							{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-2", Labels: map[string]string{"e2e-rd-group": "one", "environment": "develop"}}},
							{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-3", Labels: map[string]string{"e2e-rd-group": "one", "environment": "develop"}}},
							{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-4", Labels: map[string]string{"e2e-rd-group": "one", "environment": "test"}}},
							{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-5", Labels: map[string]string{"e2e-rd-group": "two", "environment": "test"}}},
						}
					},
				},
			}

			for _, cs := range cases {
				ginkgo.By(cs.name)
				allNamespaces := cs.getNamespaces()
				ginkgo.By("creating namespaces")
				tester.CreateNamespaces(allNamespaces...)

				ginkgo.By(fmt.Sprintf("creating ResourceDistribution %s", rd.Name))
				tester.CreateResourceDistribution(rd)

				var err error
				var matched, unmatched sets.String
				gomega.Eventually(func() int {
					matched, unmatched, err = tester.GetNamespaceForDistributor(&rd.Spec.Targets)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return matched.Len()
				}, time.Minute, time.Second).Should(gomega.Equal(4))

				gomega.Eventually(func() int32 {
					rd, err = tester.GetResourceDistribution(rd.Name, true)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return rd.Status.Succeeded
				}, time.Minute, time.Second).Should(gomega.Equal(int32(matched.Len())))
				gomega.Expect(rd.Status.Desired).Should(gomega.Equal(rd.Status.Succeeded))

				ginkgo.By("checking distributed secret hash annotation...")
				sha256Hash := sha256.Sum256(rd.Spec.Resource.Raw)
				consistentVersion := hex.EncodeToString(sha256Hash[:])
				for nsName := range matched {
					obj, err := tester.GetSecret(nsName, secretName, true)
					ginkgo.By(fmt.Sprintf("checking distributed secret(%s/%s)", nsName, secretName))
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(obj.Annotations).ShouldNot(gomega.BeNil())
					gomega.Expect(obj.Annotations[rdutils.ResourceHashCodeAnnotation]).To(gomega.Equal(consistentVersion))
				}

				ginkgo.By("checking unmatched namespaces have no secret...")
				for nsName := range unmatched {
					if !strings.HasPrefix(nsName, prefix) {
						continue
					}
					obj, err := tester.GetSecret(nsName, secretName, false)
					gomega.Expect(apierrors.IsNotFound(err)).Should(gomega.BeTrue())
					gomega.Expect(obj).Should(gomega.BeNil())
				}

				ginkgo.By("checking status conditions (all False)...")
				gomega.Expect(rd.Status.Conditions).To(gomega.HaveLen(6))
				for _, cond := range rd.Status.Conditions {
					gomega.Expect(cond.Status).To(gomega.Equal(appsv1beta1.ResourceDistributionConditionFalse))
				}

				ginkgo.By("deleting one included namespace — status should reflect it...")
				tester.DeleteNamespace(allNamespaces[0])
				gomega.Eventually(func() int {
					mice, err := tester.GetResourceDistribution(rd.Name, true)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return len(mice.Status.Conditions[5].FailedNamespaces)
				}, time.Minute, time.Second).Should(gomega.Equal(1))

				ginkgo.By("updating spec.targets to narrow scope...")
				rd.Spec.Targets.IncludedNamespaces.List = []appsv1beta1.ResourceDistributionNamespace{{Name: prefix + "-2"}}
				rd.Spec.Targets.NamespaceSelector.MatchLabels = map[string]string{"e2e-rd-group": "two"}
				tester.UpdateResourceDistribution(rd)
				gomega.Eventually(func() int32 {
					mice, err := tester.GetResourceDistribution(rd.Name, true)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return mice.Status.Succeeded
				}, time.Minute, time.Second).Should(gomega.Equal(int32(2)))

				matched, unmatched, err = tester.GetNamespaceForDistributor(&rd.Spec.Targets)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(matched.Len()).Should(gomega.Equal(2))

				ginkgo.By("checking secrets exist in new matched namespaces...")
				for nsName := range matched {
					obj, err := tester.GetSecret(nsName, secretName, true)
					ginkgo.By(fmt.Sprintf("checking distributed secret(%s/%s)", nsName, secretName))
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(obj).ShouldNot(gomega.BeNil())
				}
				ginkgo.By("checking secrets removed from now-unmatched namespaces...")
				for nsName := range unmatched {
					if !strings.HasPrefix(nsName, prefix) {
						continue
					}
					obj, err := tester.GetSecret(nsName, secretName, false)
					gomega.Expect(apierrors.IsNotFound(err)).Should(gomega.BeTrue())
					gomega.Expect(obj).Should(gomega.BeNil())
				}

				ginkgo.By("deleting ResourceDistribution — all managed secrets must be removed...")
				tester.DeleteResourceDistributions(prefix)
				for nsName := range matched {
					if !strings.HasPrefix(nsName, prefix) {
						continue
					}
					gomega.Eventually(func() bool {
						_, err := tester.GetSecret(nsName, secretName, false)
						return apierrors.IsNotFound(err)
					}, time.Minute, time.Second).Should(gomega.BeTrue())
				}

				ginkgo.By("Done!")
				tester.DeleteNamespaces(prefix)
			}
		})

		// ----------------------------------------------------------------
		// allNamespaces excludes kube-system and kube-public by default
		// ----------------------------------------------------------------
		ginkgo.It("allNamespaces excludes system namespaces by default", func() {
			prefix := "rd-beta-e2e-test4"
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			tester.CreateNamespaces(
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-1"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-2"}},
			)

			rd := &appsv1beta1.ResourceDistribution{
				ObjectMeta: metav1.ObjectMeta{Name: prefix},
				Spec: appsv1beta1.ResourceDistributionSpec{
					Resource: runtime.RawExtension{Raw: []byte(`{
						"apiVersion":"v1","kind":"ConfigMap",
						"metadata":{"name":"rd-beta-cm"},"data":{"key":"value"}
					}`)},
					Targets: appsv1beta1.ResourceDistributionTargets{AllNamespaces: true},
				},
			}
			tester.CreateResourceDistribution(rd)

			ginkgo.By("verifying test namespaces receive the ConfigMap...")
			for _, nsName := range []string{prefix + "-1", prefix + "-2"} {
				gomega.Eventually(func() error {
					_, err := c.CoreV1().ConfigMaps(nsName).Get(context.TODO(), "rd-beta-cm", metav1.GetOptions{})
					return err
				}, 2*time.Minute, time.Second).ShouldNot(gomega.HaveOccurred())
			}

			ginkgo.By("verifying kube-system has NO ConfigMap...")
			gomega.Consistently(func() bool {
				_, err := c.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "rd-beta-cm", metav1.GetOptions{})
				return apierrors.IsNotFound(err)
			}, 10*time.Second, time.Second).Should(gomega.BeTrue(), "kube-system must be excluded from allNamespaces")

			ginkgo.By("verifying kube-public has NO ConfigMap...")
			gomega.Consistently(func() bool {
				_, err := c.CoreV1().ConfigMaps("kube-public").Get(context.TODO(), "rd-beta-cm", metav1.GetOptions{})
				return apierrors.IsNotFound(err)
			}, 10*time.Second, time.Second).Should(gomega.BeTrue(), "kube-public must be excluded from allNamespaces")

			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)
			ginkgo.By("Done!")
		})

		// ----------------------------------------------------------------
		// cross-version: v1alpha1 client creates, v1beta1 client reads
		// ----------------------------------------------------------------
		ginkgo.It("cross-version compatibility: v1alpha1 create, v1beta1 read", func() {
			prefix := "rd-beta-e2e-test5"
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			tester.CreateNamespaces(
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-1"}},
			)

			alphaRD := &appsv1alpha1.ResourceDistribution{
				ObjectMeta: metav1.ObjectMeta{Name: prefix},
				Spec: appsv1alpha1.ResourceDistributionSpec{
					Resource: runtime.RawExtension{Raw: []byte(`{
						"apiVersion":"v1","kind":"ConfigMap",
						"metadata":{"name":"rd-cross-version-cm"},"data":{"k":"v"}
					}`)},
					Targets: appsv1alpha1.ResourceDistributionTargets{
						IncludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
							List: []appsv1alpha1.ResourceDistributionNamespace{{Name: prefix + "-1"}},
						},
					},
				},
			}
			_, err := kc.AppsV1alpha1().ResourceDistributions().Create(context.TODO(), alphaRD, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("reading back via v1beta1 client and verifying conversion populated namespaceSelector correctly...")
			gomega.Eventually(func() error {
				betaRD, err := kc.AppsV1beta1().ResourceDistributions().Get(context.TODO(), prefix, metav1.GetOptions{})
				if err != nil {
					return err
				}
				sel := betaRD.Spec.Targets.NamespaceSelector
				if len(sel.MatchLabels) != 0 || len(sel.MatchExpressions) != 0 {
					return fmt.Errorf("expected empty NamespaceSelector after conversion, got matchLabels=%v matchExpressions=%v", sel.MatchLabels, sel.MatchExpressions)
				}
				if len(betaRD.Spec.Targets.IncludedNamespaces.List) == 0 {
					return fmt.Errorf("IncludedNamespaces lost during alpha→beta conversion")
				}
				return nil
			}, time.Minute, time.Second).Should(gomega.Succeed())

			ginkgo.By("verifying ConfigMap is distributed to the target namespace...")
			gomega.Eventually(func() error {
				_, err := c.CoreV1().ConfigMaps(prefix+"-1").Get(context.TODO(), "rd-cross-version-cm", metav1.GetOptions{})
				return err
			}, 2*time.Minute, time.Second).Should(gomega.Succeed())

			_ = kc.AppsV1beta1().ResourceDistributions().Delete(context.TODO(), prefix, metav1.DeleteOptions{})
			tester.DeleteNamespaces(prefix)
			ginkgo.By("Done!")
		})

		// ----------------------------------------------------------------
		// namespaceSelector-only targeting
		// ----------------------------------------------------------------
		ginkgo.It("distributes resources by namespaceSelector label", func() {
			prefix := "rd-beta-e2e-test6"
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			tester.CreateNamespaces(
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-match-1", Labels: map[string]string{"env": "staging"}}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-match-2", Labels: map[string]string{"env": "staging"}}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-no-match", Labels: map[string]string{"env": "prod"}}},
			)

			rd := &appsv1beta1.ResourceDistribution{
				ObjectMeta: metav1.ObjectMeta{Name: prefix},
				Spec: appsv1beta1.ResourceDistributionSpec{
					Resource: runtime.RawExtension{Raw: []byte(`{
						"apiVersion":"v1","kind":"ConfigMap",
						"metadata":{"name":"rd-selector-cm"},"data":{"env":"staging"}
					}`)},
					Targets: appsv1beta1.ResourceDistributionTargets{
						NamespaceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "staging"},
						},
					},
				},
			}
			tester.CreateResourceDistribution(rd)

			ginkgo.By("verifying ConfigMap lands in both matching namespaces...")
			for _, nsName := range []string{prefix + "-match-1", prefix + "-match-2"} {
				gomega.Eventually(func() error {
					_, err := c.CoreV1().ConfigMaps(nsName).Get(context.TODO(), "rd-selector-cm", metav1.GetOptions{})
					return err
				}, 2*time.Minute, time.Second).Should(gomega.Succeed())
			}

			ginkgo.By("verifying non-matching namespace has no ConfigMap...")
			gomega.Consistently(func() bool {
				_, err := c.CoreV1().ConfigMaps(prefix+"-no-match").Get(context.TODO(), "rd-selector-cm", metav1.GetOptions{})
				return apierrors.IsNotFound(err)
			}, 10*time.Second, time.Second).Should(gomega.BeTrue())

			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)
			ginkgo.By("Done!")
		})

		// ----------------------------------------------------------------
		// distribute resources with includedNamespaces and excludedNamespaces
		// (original smoke test — kept in place)
		// ----------------------------------------------------------------
		ginkgo.It("should distribute resources with the beta api", func() {
			prefix := fmt.Sprintf("rd-beta-%d", time.Now().UnixNano())
			namespaces := []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-3"}},
			}
			for _, namespace := range namespaces {
				_, err := c.CoreV1().Namespaces().Create(context.TODO(), namespace, metav1.CreateOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
			defer cleanupNamespaces(c, prefix)

			rd := &appsv1beta1.ResourceDistribution{
				ObjectMeta: metav1.ObjectMeta{Name: prefix},
				Spec: appsv1beta1.ResourceDistributionSpec{
					Resource: runtime.RawExtension{Raw: []byte(`{
						"apiVersion":"v1","kind":"Secret",
						"metadata":{"name":"resourcedistribution-e2e-beta-secret"},
						"type":"Opaque","data":{"test":"dGVzdA=="}
					}`)},
					Targets: appsv1beta1.ResourceDistributionTargets{
						IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
							List: []appsv1beta1.ResourceDistributionNamespace{
								{Name: prefix + "-1"},
								{Name: prefix + "-2"},
							},
						},
						ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
							List: []appsv1beta1.ResourceDistributionNamespace{
								{Name: prefix + "-3"},
							},
						},
					},
				},
			}
			rd, err := kc.AppsV1beta1().ResourceDistributions().Create(context.TODO(), rd, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer func() {
				_ = kc.AppsV1beta1().ResourceDistributions().Delete(context.TODO(), rd.Name, metav1.DeleteOptions{})
			}()

			gomega.Eventually(func(g gomega.Gomega) {
				current, err := kc.AppsV1beta1().ResourceDistributions().Get(context.TODO(), rd.Name, metav1.GetOptions{})
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(current.Status.Succeeded).To(gomega.Equal(int32(2)))
				g.Expect(current.Status.Desired).To(gomega.Equal(int32(2)))
			}, 2*time.Minute, time.Second).Should(gomega.Succeed())

			for _, nsName := range []string{prefix + "-1", prefix + "-2"} {
				gomega.Eventually(func() error {
					secret, err := c.CoreV1().Secrets(nsName).Get(context.TODO(), "resourcedistribution-e2e-beta-secret", metav1.GetOptions{})
					if err != nil {
						return err
					}
					if secret.Annotations[rdutils.SourceResourceDistributionOfResource] != rd.Name {
						return fmt.Errorf("secret %s/%s missing source annotation", nsName, secret.Name)
					}
					return nil
				}, 2*time.Minute, time.Second).Should(gomega.Succeed())
			}

			_, err = c.CoreV1().Secrets(prefix+"-3").Get(context.TODO(), "resourcedistribution-e2e-beta-secret", metav1.GetOptions{})
			gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
		})
	})
})

func cleanupNamespaces(c clientset.Interface, prefix string) {
	namespaces, err := c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		common.Logf("list namespaces failed: %v", err)
		return
	}
	for _, namespace := range namespaces.Items {
		if strings.HasPrefix(namespace.Name, prefix) {
			_ = c.CoreV1().Namespaces().Delete(context.TODO(), namespace.Name, metav1.DeleteOptions{})
		}
	}
}
