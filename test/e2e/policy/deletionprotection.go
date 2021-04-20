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

package policy

import (
	"fmt"
	"time"

	apps "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
	utilpointer "k8s.io/utils/pointer"
)

const (
	deleteForbiddenMessage = "forbidden by ResourcesProtectionDeletion"
)

var _ = SIGDescribe("DeletionProtection", func() {
	f := framework.NewDefaultFramework("deletionprotection")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var ec apiextensionsclientset.Interface
	var dc dynamic.Interface
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ec = f.ApiExtensionsClientSet
		dc = f.DynamicClient
		ns = f.Namespace.Name
		randStr = rand.String(10)
		_ = ns
	})

	framework.KruiseDescribe("namespace deletion", func() {
		ginkgo.It("should be protected", func() {
			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "kruise-e2e-deletion-protection-" + randStr,
				Labels: map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways},
			}}
			ginkgo.By("Create test namespace " + ns.Name + " with Always")
			_, err := c.CoreV1().Namespaces().Create(ns)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the namespace should be rejected")
			err = c.CoreV1().Namespaces().Delete(ns.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Create a CloneSet in this namespace and wait for pod created")
			tester := framework.NewCloneSetTester(c, kc, ns.Name)
			cs := tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{})
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Patch the namespace deletion to Cascading")
			_, err = c.CoreV1().Namespaces().Patch(ns.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, policyv1alpha1.DeletionProtectionKey, policyv1alpha1.DeletionProtectionTypeCascading)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the namespace should be rejected")
			err = c.CoreV1().Namespaces().Delete(ns.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Scale CloneSet replicas to 0")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Replicas = utilpointer.Int32Ptr(0)
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(0)))

			time.Sleep(time.Second)
			ginkgo.By("Delete the namespace successfully")
			err = c.CoreV1().Namespaces().Delete(ns.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})

	framework.KruiseDescribe("CRD deletion", func() {
		ginkgo.It("should be protected", func() {
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "testdeletioncases.e2e.kruise.io",
					Labels: map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways},
				},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "e2e.kruise.io",
					Scope: apiextensionsv1.NamespaceScoped,
					Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "testdeletioncases", Singular: "testdeletioncase", Kind: "TestDeletionCase"},
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
						Name: "v1beta1",
						Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: apiextensionsv1.JSONSchemaDefinitions{
								"apiVersion": apiextensionsv1.JSONSchemaProps{Type: "string"},
								"kind":       apiextensionsv1.JSONSchemaProps{Type: "string"},
								"metadata":   apiextensionsv1.JSONSchemaProps{Type: "object"},
							},
						}},
						Storage: true,
						Served:  true,
					}},
				},
			}
			ginkgo.By("Create test CRD " + crd.Name + " with Always")
			_, err := ec.ApiextensionsV1().CustomResourceDefinitions().Create(crd)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the CRD should be rejected")
			err = ec.ApiextensionsV1().CustomResourceDefinitions().Delete(crd.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Create a test CR")
			time.Sleep(time.Second)
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion("e2e.kruise.io/v1beta1")
			obj.SetKind("TestDeletionCase")
			obj.SetNamespace(ns)
			obj.SetName("test-cr-" + randStr)
			obj, err = dc.Resource(schema.GroupVersionResource{Group: "e2e.kruise.io", Version: "v1beta1", Resource: "testdeletioncases"}).Namespace(ns).Create(obj, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Patch the CRD deletion to Cascading")
			_, err = ec.ApiextensionsV1().CustomResourceDefinitions().Patch(crd.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, policyv1alpha1.DeletionProtectionKey, policyv1alpha1.DeletionProtectionTypeCascading)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the CRD should be rejected")
			err = ec.ApiextensionsV1().CustomResourceDefinitions().Delete(crd.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Delete the test CR")
			err = dc.Resource(schema.GroupVersionResource{Group: "e2e.kruise.io", Version: "v1beta1", Resource: "testdeletioncases"}).Namespace(ns).Delete(obj.GetName(), &metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the CRD should be successful")
			err = ec.ApiextensionsV1().CustomResourceDefinitions().Delete(crd.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})

	framework.KruiseDescribe("Workload deletion", func() {
		var err error
		ginkgo.It("CloneSet should be protected", func() {
			ginkgo.By("Create a CloneSet with Always")
			tester := framework.NewCloneSetTester(c, kc, ns)
			cs := tester.NewCloneSet("clone-"+randStr, 0, appsv1alpha1.CloneSetUpdateStrategy{})
			cs.Labels = map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways}
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the CloneSet should be rejected")
			err = tester.DeleteCloneSet(cs.Name)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Scale CloneSet replicas to 2 and protection to Cascading")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Replicas = utilpointer.Int32Ptr(2)
				cs.Labels[policyv1alpha1.DeletionProtectionKey] = policyv1alpha1.DeletionProtectionTypeCascading
			})
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(2)))

			ginkgo.By("Delete the CloneSet should be rejected")
			err = tester.DeleteCloneSet(cs.Name)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Scale CloneSet replicas to 0")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Replicas = utilpointer.Int32Ptr(0)
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(0)))

			ginkgo.By("Delete the CloneSet should be successful")
			err = tester.DeleteCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("Deployment should be protected", func() {
			ginkgo.By("Create a Deployment with Always")
			name := "deploy-" + randStr
			deploy := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      name,
					Labels:    map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways},
				},
				Spec: apps.DeploymentSpec{
					Replicas: utilpointer.Int32Ptr(0),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"owner": name}},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"owner": name},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.9.1",
									Env: []v1.EnvVar{
										{Name: "test", Value: "foo"},
									},
								},
							},
						},
					},
				},
			}
			deploy, err = c.AppsV1().Deployments(ns).Create(deploy)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the Deployment should be rejected")
			err = c.AppsV1().Deployments(ns).Delete(deploy.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Scale Deployment replicas to 2 and protection to Cascading")
			deploy, err = c.AppsV1().Deployments(ns).Patch(deploy.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}},"spec":{"replicas":%d}}`, policyv1alpha1.DeletionProtectionKey, policyv1alpha1.DeletionProtectionTypeCascading, 2)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				deploy, err = c.AppsV1().Deployments(ns).Get(deploy.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return deploy.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(2)))

			ginkgo.By("Delete the Deployment should be rejected")
			err = c.AppsV1().Deployments(ns).Delete(deploy.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Scale Deployment replicas to 0")
			deploy, err = c.AppsV1().Deployments(ns).Patch(deploy.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, 0)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				deploy, err = c.AppsV1().Deployments(ns).Get(deploy.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return deploy.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(0)))

			ginkgo.By("Delete the Deployment should successful")
			err = c.AppsV1().Deployments(ns).Delete(deploy.Name, &metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
})
