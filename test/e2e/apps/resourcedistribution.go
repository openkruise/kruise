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
	"encoding/base64"
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"
	"github.com/openkruise/kruise/test/e2e/framework"

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
		secretName = "test-secret-1"
		tester = framework.NewResourceDistributionTester(c, kc)
	})

	framework.KruiseDescribe("ResourceDistribution distributing functionality [ResourceDistributionInject]", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all ResourceDistribution in cluster")
		})

		ginkgo.It("When no namespace matches with ResourceDistribution", func() {
			nsPrefix1 := "resourcedistribution-e2e-test-ns-test1"

			// clear resource to avoid conflict
			tester.DeleteResourceDistributions(nsPrefix1)
			tester.DeleteNamespaces(nsPrefix1)

			// create ResourceDistribution
			resourceDistribution := tester.NewBaseResourceDistribution(nsPrefix1) //tester.NewResourceDistribution(ns)
			// ResourceDistribution no matched namespaces
			resourceDistribution.Spec.Targets.NamespaceLabelSelector.MatchLabels["group"] = "nomatched"
			ginkgo.By(fmt.Sprintf("Creating ResourceDistribution %s", resourceDistribution.Name))
			tester.CreateResourceDistribution(resourceDistribution)

			// create namespace
			namespace := tester.NewBaseNamespace(ns)
			ginkgo.By(fmt.Sprintf("Creating Namespace(%s)", namespace.Name))
			tester.CreateNamespaces(namespace)

			// get namespaces
			namespaces, err := tester.GetSelectedNamespaces(&resourceDistribution.Spec.Targets)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(namespaces).To(gomega.HaveLen(0))

			tester.DeleteResourceDistributions(nsPrefix1)
			tester.DeleteNamespaces(nsPrefix1)
			ginkgo.By(fmt.Sprintf("test done when no namespace matches with ResourceDistribution"))
		})

		ginkgo.It("When namespaces do not exist", func() {
			nsPrefix2 := "resourcedistribution-e2e-test-ns-test2"

			// clear resource to avoid conflict
			tester.DeleteResourceDistributions(nsPrefix2)
			tester.DeleteNamespaces(nsPrefix2)

			// create ResourceDistribution
			resourceDistribution := tester.NewBaseResourceDistribution(nsPrefix2)
			resourceDistribution.Spec.Targets.NamespaceLabelSelector = metav1.LabelSelector{}
			ginkgo.By(fmt.Sprintf("Creating ResourceDistribution %s", resourceDistribution.Name))
			tester.CreateResourceDistribution(resourceDistribution)

			// get namespaces
			targetNamespaces, err := tester.GetSelectedNamespaces(&resourceDistribution.Spec.Targets)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(targetNamespaces).To(gomega.HaveLen(4))

			for _, namespace := range targetNamespaces {
				object, err := tester.GetSecret(namespace, secretName, true)
				ginkgo.By(fmt.Sprintf("checking secret(%s.%s).", namespace, secretName))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(object).ShouldNot(gomega.BeNil())
			}

			// clear namespaces
			tester.DeleteResourceDistributions(nsPrefix2)
			tester.DeleteNamespaces(nsPrefix2)
			ginkgo.By("test done when namespaces do not exist")
		})

		ginkgo.It("When namespace is created after ResourceDistribution", func() {
			nsPrefix3 := "resourcedistribution-e2e-test-ns-test3"

			// clear resource to avoid conflict
			tester.DeleteResourceDistributions(nsPrefix3)
			tester.DeleteNamespaces(nsPrefix3)

			// create ResourceDistribution
			resourceDistribution := tester.NewBaseResourceDistribution(nsPrefix3)
			resourceDistribution.Spec.Targets.IncludedNamespaces = nil
			resourceDistribution.Spec.Targets.NamespaceLabelSelector.MatchLabels = map[string]string{
				"group": "seven",
			}
			ginkgo.By(fmt.Sprintf("Creating ResourceDistribution %s %v", resourceDistribution.Name, resourceDistribution.Spec.Targets.NamespaceLabelSelector.MatchLabels))
			tester.CreateResourceDistribution(resourceDistribution)

			// create matched Namespaces
			tester.CreateNamespaces(&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsPrefix3 + "-test",
					Labels: map[string]string{
						"group": "seven",
					},
				},
			})

			// get namespaces
			namespaces, err := tester.GetSelectedNamespaces(&resourceDistribution.Spec.Targets)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ginkgo.By(fmt.Sprintf("target namespaces %v\n", namespaces))
			gomega.Expect(namespaces).To(gomega.HaveLen(1))
			// resource must exist
			resource, err := tester.GetSecret(namespaces[0], secretName, true)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(resource).ShouldNot(gomega.BeNil())

			//clear all resources in cluster
			tester.DeleteResourceDistributions(nsPrefix3)
			tester.DeleteNamespaces(nsPrefix3)
			ginkgo.By("test done when namespace is created after ResourceDistribution")
		})

		ginkgo.It("Test for other situations", func() {
			nsPrefix4 := "resourcedistribution-e2e-test-ns-test4"

			// clear resource to avoid conflict
			tester.DeleteResourceDistributions(nsPrefix4)
			tester.DeleteNamespaces(nsPrefix4)

			// build ResourceDistribution object
			resourceDistribution := tester.NewBaseResourceDistribution(nsPrefix4)
			cases := []struct {
				name          string
				getResources  func() []*corev1.Secret
				getNamespaces func() []*corev1.Namespace
			}{
				{
					name: "normal resource distribution case",
					getNamespaces: func() []*corev1.Namespace {
						return []*corev1.Namespace{
							&corev1.Namespace{ // for create
								ObjectMeta: metav1.ObjectMeta{
									Name: nsPrefix4 + "-1",
									Labels: map[string]string{
										"group":       "one",
										"environment": "develop",
									},
								},
							},
							&corev1.Namespace{ // for create
								ObjectMeta: metav1.ObjectMeta{
									Name: nsPrefix4 + "-2",
									Labels: map[string]string{
										"group":       "one",
										"environment": "develop",
									},
								},
							},
							&corev1.Namespace{ // for ExcludedNamespaces
								ObjectMeta: metav1.ObjectMeta{
									Name: nsPrefix4 + "-3",
									Labels: map[string]string{
										"group":       "one",
										"environment": "develop",
									},
								},
							},
							&corev1.Namespace{ // for create
								ObjectMeta: metav1.ObjectMeta{
									Name: nsPrefix4 + "-4",
									Labels: map[string]string{
										"group":       "one",
										"environment": "test",
									},
								},
							},
							&corev1.Namespace{ // for delete
								ObjectMeta: metav1.ObjectMeta{
									Name: nsPrefix4 + "-5",
									Labels: map[string]string{
										"group":       "two",
										"environment": "test",
									},
								},
							},
						}
					},
					getResources: func() []*corev1.Secret {
						secretContent := []byte(base64.StdEncoding.EncodeToString([]byte("myUsername:myPassword")))
						return []*corev1.Secret{
							{ // for update
								ObjectMeta: metav1.ObjectMeta{
									Name:      secretName,
									Namespace: nsPrefix4 + "-1",
									Annotations: map[string]string{
										utils.SourceResourceDistributionOfResource: resourceDistribution.Name,
									},
								},
								Type: "Opaque",
								Data: map[string][]byte{
									".dockerconfigjson": secretContent,
								},
							},
							{ // for delete
								ObjectMeta: metav1.ObjectMeta{
									Name:      secretName,
									Namespace: nsPrefix4 + "-5",
									Annotations: map[string]string{
										utils.SourceResourceDistributionOfResource: resourceDistribution.Name,
									},
								},
								Type: "Opaque",
								Data: map[string][]byte{
									".dockerconfigjson": secretContent,
								},
							},
						}
					},
				},
			}

			for _, cs := range cases {
				ginkgo.By(cs.name)

				allNamespaces := cs.getNamespaces()
				ginkgo.By(fmt.Sprintf("Creating Namespaces"))
				tester.CreateNamespaces(allNamespaces...)

				resources := cs.getResources()
				ginkgo.By(fmt.Sprintf("Creating Resources"))
				tester.CreateSecretResources(resources...)

				ginkgo.By(fmt.Sprintf("Creating ResourceDistribution %s", resourceDistribution.Name))
				tester.CreateResourceDistribution(resourceDistribution)

				targetNamespaces, err := tester.GetSelectedNamespaces(&resourceDistribution.Spec.Targets)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(targetNamespaces).To(gomega.HaveLen(3))
				ginkgo.By(fmt.Sprintf("ResourceDistribution target namespaces %v", targetNamespaces))

				done := make(map[string]struct{})
				for _, namespace := range targetNamespaces {
					done[namespace] = struct{}{}
					object, err := tester.GetSecret(namespace, secretName, true)
					ginkgo.By(fmt.Sprintf("checking secret(%s.%s).", namespace, secretName))
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}

				for _, namespace := range allNamespaces {
					if _, ok := done[namespace.Name]; ok {
						continue
					}
					object, err := tester.GetSecret(namespace.Name, resources[0].Name, false)
					gomega.Expect(errors.IsNotFound(err)).Should(gomega.BeTrue())
					gomega.Expect(object).Should(gomega.BeNil())
				}

				tester.DeleteResourceDistributions(nsPrefix4)
				tester.DeleteNamespaces(nsPrefix4)
			}
			ginkgo.By("Test done for other situations")
		})
	})
})
