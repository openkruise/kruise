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
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	utilpointer "k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
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
		ginkgo.It("ns should be protected", func() {
			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:   "kruise-e2e-deletion-protection-" + randStr,
				Labels: map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways},
			}}
			ginkgo.By("Create test namespace " + ns.Name + " with Always")
			_, err := c.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the namespace should be rejected")
			err = c.CoreV1().Namespaces().Delete(context.TODO(), ns.Name, metav1.DeleteOptions{})
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
			_, err = c.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, policyv1alpha1.DeletionProtectionKey, policyv1alpha1.DeletionProtectionTypeCascading)), metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the namespace should be rejected")
			err = c.CoreV1().Namespaces().Delete(context.TODO(), ns.Name, metav1.DeleteOptions{})
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

			ginkgo.By("Create a PVC in this namespace")
			nodeList, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(nodeList.Items)).ShouldNot(gomega.BeZero())
			nodeName := nodeList.Items[0].Name
			pvcName := "pvc-" + randStr
			storageClassName := "standard"
			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns.Name,
					Name:      pvcName,
					Annotations: map[string]string{
						"volume.kubernetes.io/selected-node": nodeName,
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("50Mi"),
						},
					},
					StorageClassName: &storageClassName,
				},
			}
			_, err = c.CoreV1().PersistentVolumeClaims(ns.Name).Create(context.TODO(), pvc, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				pvc, err := c.CoreV1().PersistentVolumeClaims(ns.Name).Get(context.TODO(), pvcName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return pvc.Status.Phase == v1.ClaimBound
			}, 15*time.Second, time.Second).Should(gomega.BeTrue())

			ginkgo.By("Delete the namespace should be rejected")
			err = c.CoreV1().Namespaces().Delete(context.TODO(), ns.Name, metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Delete the PVC just created")
			err = c.CoreV1().PersistentVolumeClaims(ns.Name).Delete(context.TODO(), pvcName, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(3 * time.Second)
			ginkgo.By("Delete the namespace successfully")
			err = c.CoreV1().Namespaces().Delete(context.TODO(), ns.Name, metav1.DeleteOptions{})
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
			_, err := ec.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), crd, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the CRD should be rejected")
			err = ec.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), crd.Name, metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Create a test CR")
			time.Sleep(time.Second)
			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion("e2e.kruise.io/v1beta1")
			obj.SetKind("TestDeletionCase")
			obj.SetNamespace(ns)
			obj.SetName("test-cr-" + randStr)
			obj, err = dc.Resource(schema.GroupVersionResource{Group: "e2e.kruise.io", Version: "v1beta1", Resource: "testdeletioncases"}).Namespace(ns).Create(context.TODO(), obj, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Patch the CRD deletion to Cascading")
			_, err = ec.ApiextensionsV1().CustomResourceDefinitions().Patch(context.TODO(), crd.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, policyv1alpha1.DeletionProtectionKey, policyv1alpha1.DeletionProtectionTypeCascading)), metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the CRD should be rejected")
			err = ec.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), crd.Name, metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Delete the test CR")
			err = dc.Resource(schema.GroupVersionResource{Group: "e2e.kruise.io", Version: "v1beta1", Resource: "testdeletioncases"}).Namespace(ns).Delete(context.TODO(), obj.GetName(), metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the CRD should be successful")
			err = ec.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), crd.Name, metav1.DeleteOptions{})
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
									Image: NginxImage,
									Env: []v1.EnvVar{
										{Name: "test", Value: "foo"},
									},
								},
							},
						},
					},
				},
			}
			deploy, err = c.AppsV1().Deployments(ns).Create(context.TODO(), deploy, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the Deployment should be rejected")
			err = c.AppsV1().Deployments(ns).Delete(context.TODO(), deploy.Name, metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Scale Deployment replicas to 2 and protection to Cascading")
			deploy, err = c.AppsV1().Deployments(ns).Patch(context.TODO(), deploy.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}},"spec":{"replicas":%d}}`, policyv1alpha1.DeletionProtectionKey, policyv1alpha1.DeletionProtectionTypeCascading, 2)), metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				deploy, err = c.AppsV1().Deployments(ns).Get(context.TODO(), deploy.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return deploy.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(2)))

			ginkgo.By("Delete the Deployment should be rejected")
			err = c.AppsV1().Deployments(ns).Delete(context.TODO(), deploy.Name, metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Scale Deployment replicas to 0")
			deploy, err = c.AppsV1().Deployments(ns).Patch(context.TODO(), deploy.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, 0)), metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				deploy, err = c.AppsV1().Deployments(ns).Get(context.TODO(), deploy.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return deploy.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(0)))

			ginkgo.By("Delete the Deployment should successful")
			err = c.AppsV1().Deployments(ns).Delete(context.TODO(), deploy.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})

	framework.KruiseDescribe("Service deletion", func() {
		ginkgo.It("should be protected", func() {
			ginkgo.By("Create a Service with Always")
			name := "svc-" + randStr
			svc := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      name,
					Labels:    map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways},
				},
				Spec: v1.ServiceSpec{
					Selector: map[string]string{"owner": "foo"},
					Ports: []v1.ServicePort{
						{Port: 80, Name: "http", Protocol: v1.ProtocolTCP},
					},
				},
			}
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), svc, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the Service should be rejected")
			err = c.CoreV1().Services(ns).Delete(context.TODO(), svc.Name, metav1.DeleteOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Patch the Service deletion to null")
			_, err = c.CoreV1().Services(ns).Patch(context.TODO(), svc.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`, policyv1alpha1.DeletionProtectionKey)), metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the Service successfully")
			err = c.CoreV1().Services(ns).Delete(context.TODO(), svc.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})

	framework.KruiseDescribe("Ingress deletion", func() {
		ginkgo.It("should be protected", func() {
			ginkgo.By("Create a Ingress with Always")
			name := "ing-" + randStr
			pathType := networkingv1.PathTypePrefix
			ing := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: ns,
					Labels:    map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							Host: "foo.bar.com",
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: &pathType,
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "test",
													Port: networkingv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			_, err := c.NetworkingV1().Ingresses(ns).Create(context.TODO(), ing, metav1.CreateOptions{})

			// for the cluster using old Kubernetes version, use networking.k8s.io/v1beta1 instead of networking.k8s.io/v1 to create Ingress resource
			if err != nil && err.Error() == "the server could not find the requested resource" {
				err = nil
				pathType := networkingv1beta1.PathTypePrefix
				ing := &networkingv1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: ns,
						Labels:    map[string]string{policyv1alpha1.DeletionProtectionKey: policyv1alpha1.DeletionProtectionTypeAlways},
					},
					Spec: networkingv1beta1.IngressSpec{
						Rules: []networkingv1beta1.IngressRule{
							{
								Host: "foo.bar.com",
								IngressRuleValue: networkingv1beta1.IngressRuleValue{
									HTTP: &networkingv1beta1.HTTPIngressRuleValue{
										Paths: []networkingv1beta1.HTTPIngressPath{
											{
												Path:     "/",
												PathType: &pathType,
												Backend: networkingv1beta1.IngressBackend{
													ServiceName: "test",
													ServicePort: intstr.FromInt(80),
												},
											},
										},
									},
								},
							},
						},
					},
				}
				_, err = c.NetworkingV1beta1().Ingresses(ns).Create(context.TODO(), ing, metav1.CreateOptions{})
			}
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the Ingress should be rejected")
			err = c.NetworkingV1().Ingresses(ns).Delete(context.TODO(), ing.Name, metav1.DeleteOptions{})
			if err != nil && err.Error() == "the server could not find the requested resource" {
				err = nil
				err = c.NetworkingV1beta1().Ingresses(ns).Delete(context.TODO(), ing.Name, metav1.DeleteOptions{})
			}
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(deleteForbiddenMessage))

			ginkgo.By("Patch the Ingress deletion to null")
			_, err = c.NetworkingV1().Ingresses(ns).Patch(context.TODO(), ing.Name, types.StrategicMergePatchType,
				[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`, policyv1alpha1.DeletionProtectionKey)), metav1.PatchOptions{})
			if err != nil && err.Error() == "the server could not find the requested resource" {
				err = nil
				_, err = c.NetworkingV1beta1().Ingresses(ns).Patch(context.TODO(), ing.Name, types.StrategicMergePatchType,
					[]byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`, policyv1alpha1.DeletionProtectionKey)), metav1.PatchOptions{})
			}
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Delete the Ingress successfully")
			err = c.NetworkingV1().Ingresses(ns).Delete(context.TODO(), ing.Name, metav1.DeleteOptions{})
			if err != nil && err.Error() == "the server could not find the requested resource" {
				err = nil
				err = c.NetworkingV1beta1().Ingresses(ns).Delete(context.TODO(), ing.Name, metav1.DeleteOptions{})
			}
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
})
