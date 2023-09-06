/*
Copyright 2021 The Kruise Authors.

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

package apps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"
	"github.com/openkruise/kruise/test/e2e/framework"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = SIGDescribe("ResourceDistribution", func() {
	f := framework.NewDefaultFramework("resourcedistribution")
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

	framework.KruiseDescribe("ResourceDistribution distributing functionality [ResourceDistributionInject]", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all ResourceDistribution in cluster")
		})

		framework.ConformanceIt("namespace event checker", func() {
			prefix := "resourcedistribution-e2e-test1"
			// clean resource to avoid conflict
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			// to be updated
			namespaceUpdateMatch := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: prefix + "-update-matched",
				},
			}
			tester.CreateNamespaces(namespaceUpdateMatch)

			// create ResourceDistribution
			resourceDistribution := tester.NewBaseResourceDistribution(prefix)
			resourceDistribution.Spec.Targets.IncludedNamespaces.List = nil
			resourceDistribution.Spec.Targets.NamespaceLabelSelector.MatchLabels = map[string]string{prefix: "seven"}
			tester.CreateResourceDistribution(resourceDistribution)

			// create matched Namespaces
			tester.CreateNamespaces(&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   prefix + "-creat-matched",
					Labels: map[string]string{prefix: "seven"},
				},
			})

			// update namespace to match distributor
			namespaceUpdateMatch.Labels = map[string]string{prefix: "seven"}
			tester.UpdateNamespace(namespaceUpdateMatch)

			// check matched namespace
			ginkgo.By("waiting for namespace create and update...")
			gomega.Eventually(func() int {
				matchedNamespaces, _, err := tester.GetNamespaceForDistributor(&resourceDistribution.Spec.Targets)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return matchedNamespaces.Len()
			}, time.Minute, time.Second).Should(gomega.Equal(2))

			ginkgo.By("checking created secret...")
			matchedNamespaces, _, err := tester.GetNamespaceForDistributor(&resourceDistribution.Spec.Targets)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for namespace := range matchedNamespaces {
				gomega.Eventually(func() error {
					_, err := tester.GetSecret(namespace, secretName, true)
					return err
				}, time.Minute, time.Second).ShouldNot(gomega.HaveOccurred())
			}

			//clear all resources in cluster
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)
			ginkgo.By("Done!")
		})

		framework.ConformanceIt("resource event checker", func() {
			prefix := "resourcedistribution-e2e-test2"
			// clean resource to avoid conflict
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			namespaces := []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: prefix + "-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: prefix + "-2",
					},
				},
			}
			tester.CreateNamespaces(namespaces...)

			// create ResourceDistribution
			resourceDistribution := tester.NewBaseResourceDistribution(prefix)
			tester.CreateResourceDistribution(resourceDistribution)

			var err error
			var secret *corev1.Secret
			gomega.Eventually(func() error {
				secret, err = tester.GetSecret(namespaces[0].Name, secretName, true)
				return err
			}, time.Minute, time.Second).Should(gomega.BeNil())

			// If resource was modified directly, resourceDistribution should modify it back
			ginkgo.By("update resource directly...")
			secret.StringData = map[string]string{
				"updated": "yes",
			}
			err = tester.UpdateSecret(secret)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int {
				secret, err = tester.GetSecret(namespaces[0].Name, secretName, true)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(secret.StringData)
			}, time.Minute, time.Second).Should(gomega.Equal(0))

			// If resource was deleted directly, resourceDistribution should create it again
			ginkgo.By("delete resource directly...")
			err = tester.DeleteSecret(secret.Namespace, secret.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() error {
				secret, err = tester.GetSecret(namespaces[0].Name, secretName, true)
				return err
			}, time.Minute, time.Second).Should(gomega.BeNil())

			//clear all resources in cluster
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)
			ginkgo.By("Done!")
		})

		framework.ConformanceIt("resourcedistribution functionality checker", func() {
			prefix := "resourcedistribution-e2e-test3"
			// clean resource to avoid conflict
			tester.DeleteResourceDistributions(prefix)
			tester.DeleteNamespaces(prefix)

			// build ResourceDistribution object
			resourceDistribution := tester.NewBaseResourceDistribution(prefix)
			cases := []struct {
				name          string
				getNamespaces func() []*corev1.Namespace
			}{
				{
					name: "normal resource distribution case",
					getNamespaces: func() []*corev1.Namespace {
						return []*corev1.Namespace{
							&corev1.Namespace{ // for create
								ObjectMeta: metav1.ObjectMeta{
									Name: prefix + "-1",
									Labels: map[string]string{
										"e2e-rd-group": "one",
										"environment":  "develop",
									},
								},
							},
							&corev1.Namespace{ // for create
								ObjectMeta: metav1.ObjectMeta{
									Name: prefix + "-2",
									Labels: map[string]string{
										"e2e-rd-group": "one",
										"environment":  "develop",
									},
								},
							},
							&corev1.Namespace{ // for ExcludedNamespaces
								ObjectMeta: metav1.ObjectMeta{
									Name: prefix + "-3",
									Labels: map[string]string{
										"e2e-rd-group": "one",
										"environment":  "develop",
									},
								},
							},
							&corev1.Namespace{ // for create
								ObjectMeta: metav1.ObjectMeta{
									Name: prefix + "-4",
									Labels: map[string]string{
										"e2e-rd-group": "one",
										"environment":  "test",
									},
								},
							},
							&corev1.Namespace{ // for delete
								ObjectMeta: metav1.ObjectMeta{
									Name: prefix + "-5",
									Labels: map[string]string{
										"e2e-rd-group": "two",
										"environment":  "test",
									},
								},
							},
						}
					},
				},
			}

			for _, cs := range cases {
				ginkgo.By(cs.name)
				allNamespaces := cs.getNamespaces()
				ginkgo.By("creating namespaces")
				tester.CreateNamespaces(allNamespaces...)

				ginkgo.By(fmt.Sprintf("Creating ResourceDistribution %s", resourceDistribution.Name))
				tester.CreateResourceDistribution(resourceDistribution)

				var err error
				var matchedNamespaces sets.String
				var unmatchedNamespaces sets.String
				// ensure namespaces have been created
				gomega.Eventually(func() int {
					matchedNamespaces, unmatchedNamespaces, err = tester.GetNamespaceForDistributor(&resourceDistribution.Spec.Targets)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return matchedNamespaces.Len()
				}, time.Minute, time.Second).Should(gomega.Equal(4))
				// ensure all desired resources have been created
				gomega.Eventually(func() int32 {
					resourceDistribution, err = tester.GetResourceDistribution(resourceDistribution.Name, true)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return resourceDistribution.Status.Succeeded
				}, time.Minute, time.Second).Should(gomega.Equal(int32(len(matchedNamespaces))))
				gomega.Expect(resourceDistribution.Status.Desired).Should(gomega.Equal(resourceDistribution.Status.Succeeded))

				// checking created and updated resources
				ginkgo.By("checking created and updated resources...")
				md5Hash := sha256.Sum256(resourceDistribution.Spec.Resource.Raw)
				consistentVersion := hex.EncodeToString(md5Hash[:])
				for namespace := range matchedNamespaces {
					object, err := tester.GetSecret(namespace, secretName, true)
					ginkgo.By(fmt.Sprintf("checking distributed secret(%s/%s).", namespace, secretName))
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(object.GetAnnotations()).ShouldNot(gomega.BeNil())
					version := object.Annotations[utils.ResourceHashCodeAnnotation]
					gomega.Expect(version).To(gomega.Equal(consistentVersion))
				}

				// checking deleted secrets
				ginkgo.By("checking deleted secrets...")
				for namespace := range unmatchedNamespaces {
					// only focus on this e2e test
					if !strings.HasPrefix(namespace, prefix) {
						continue
					}
					object, err := tester.GetSecret(namespace, secretName, false)
					gomega.Expect(errors.IsNotFound(err)).Should(gomega.BeTrue())
					gomega.Expect(object).Should(gomega.BeNil())
				}

				// check status.conditions
				ginkgo.By("checking conditions...")
				gomega.Expect(resourceDistribution.Status.Conditions).To(gomega.HaveLen(6))
				gomega.Expect(resourceDistribution.Status.Conditions[0].Status).To(gomega.Equal(appsv1alpha1.ResourceDistributionConditionFalse))
				gomega.Expect(resourceDistribution.Status.Conditions[1].Status).To(gomega.Equal(appsv1alpha1.ResourceDistributionConditionFalse))
				gomega.Expect(resourceDistribution.Status.Conditions[2].Status).To(gomega.Equal(appsv1alpha1.ResourceDistributionConditionFalse))
				gomega.Expect(resourceDistribution.Status.Conditions[3].Status).To(gomega.Equal(appsv1alpha1.ResourceDistributionConditionFalse))
				gomega.Expect(resourceDistribution.Status.Conditions[4].Status).To(gomega.Equal(appsv1alpha1.ResourceDistributionConditionFalse))
				gomega.Expect(resourceDistribution.Status.Conditions[5].Status).To(gomega.Equal(appsv1alpha1.ResourceDistributionConditionFalse))

				// checking after some included namespaces is deleted
				tester.DeleteNamespace(allNamespaces[0])
				gomega.Eventually(func() int {
					mice, err := tester.GetResourceDistribution(resourceDistribution.Name, true)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return len(mice.Status.Conditions[5].FailedNamespaces)
				}, time.Minute, time.Second).Should(gomega.Equal(1))
				gomega.Expect(resourceDistribution.Status.Desired).Should(gomega.Equal(resourceDistribution.Status.Succeeded))

				// checking after updating spec.targets
				resourceDistribution.Spec.Targets.IncludedNamespaces.List = []appsv1alpha1.ResourceDistributionNamespace{{Name: prefix + "-2"}}
				resourceDistribution.Spec.Targets.NamespaceLabelSelector.MatchLabels = map[string]string{"e2e-rd-group": "two"}
				tester.UpdateResourceDistribution(resourceDistribution)
				gomega.Eventually(func() int32 {
					mice, err := tester.GetResourceDistribution(resourceDistribution.Name, true)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return mice.Status.Succeeded
				}, time.Minute, time.Second).Should(gomega.Equal(int32(2)))

				matchedNamespaces, unmatchedNamespaces, err = tester.GetNamespaceForDistributor(&resourceDistribution.Spec.Targets)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(matchedNamespaces.Len()).Should(gomega.Equal(2))

				ginkgo.By("checking created secrets...")
				for namespace := range matchedNamespaces {
					object, err := tester.GetSecret(namespace, secretName, true)
					ginkgo.By(fmt.Sprintf("checking distributed secret(%s/%s).", namespace, secretName))
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(object).ShouldNot(gomega.BeNil())
					version := object.Annotations[utils.ResourceHashCodeAnnotation]
					gomega.Expect(version).To(gomega.Equal(consistentVersion))
				}
				ginkgo.By("checking deleted secrets...")
				for namespace := range unmatchedNamespaces {
					if !strings.HasPrefix(namespace, prefix) {
						continue
					}
					object, err := tester.GetSecret(namespace, secretName, false)
					gomega.Expect(errors.IsNotFound(err)).Should(gomega.BeTrue())
					gomega.Expect(object).Should(gomega.BeNil())
				}

				ginkgo.By("checking all matched namespaces after distributor was deleted...")
				tester.DeleteResourceDistributions(prefix)
				for namespace := range matchedNamespaces {
					if !strings.HasPrefix(namespace, prefix) {
						continue
					}
					gomega.Eventually(func() bool {
						_, err := tester.GetSecret(namespace, secretName, false)
						return errors.IsNotFound(err)
					}, time.Minute, time.Second).Should(gomega.BeTrue())
				}
				ginkgo.By("Done!")
				tester.DeleteNamespaces(prefix)
			}
		})
	})
})
