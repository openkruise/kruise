/*
Copyright 2022 The Kruise Authors.

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
	"context"
	"fmt"
	"time"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/webhook/pod/mutating"
	"github.com/openkruise/kruise/test/e2e/framework"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	utilpointer "k8s.io/utils/pointer"
)

var (
	podStateOsTopologyLabel   = "kubernetes.io/os"
	podStateNodeTopologyLabel = "kubernetes.io/hostname"
)

var _ = SIGDescribe("PersistentPodState", func() {
	f := framework.NewDefaultFramework("persistentpodstates")
	var ns string
	var c clientset.Interface
	var a apiextensionsclientset.Interface
	var d dynamic.Interface
	var kc kruiseclientset.Interface
	var tester *framework.PersistentPodStateTester

	ginkgo.BeforeEach(func() {
		a = f.ApiExtensionsClientSet
		c = f.ClientSet
		d = f.DynamicClient
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewPersistentPodStateTester(c, kc, d, a)
	})

	framework.KruiseDescribe("PersistentPodState functionality", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
		})

		ginkgo.It("statefulset node topology", func() {
			// create statefulset
			sts := tester.NewBaseStatefulset(ns)
			sts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] = "true"
			sts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] = podStateOsTopologyLabel
			sts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] = podStateNodeTopologyLabel
			ginkgo.By(fmt.Sprintf("Creating kruise Statefulset %s", sts.Name))
			tester.CreateStatefulset(sts)
			time.Sleep(time.Second * 3)

			// check PersistentPodState in staticIP
			ginkgo.By(fmt.Sprintf("check PersistentPodState(%s/%s)", sts.Namespace, sts.Name))
			pods, err := tester.ListPodsInKruiseSts(sts)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			staticIP, err := kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(staticIP.Status.PodStates).To(gomega.HaveLen(len(pods)))
			currentStates := staticIP.Status.DeepCopy().PodStates
			for _, pod := range pods {
				podState, ok := staticIP.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.Equal(true))
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(2))
				node, err := c.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for k, v := range podState.NodeTopologyLabels {
					gomega.Expect(v).To(gomega.Equal(node.Labels[k]))
				}
				gomega.Expect(podState.NodeName).To(gomega.Equal(node.Name))
			}

			// upgrade statefulSet.container[x].image
			sts.Spec.Template.Spec.Containers[0].Image = "busybox:1.35"
			tester.UpdateStatefulset(sts)
			time.Sleep(time.Second * 3)

			ginkgo.By(fmt.Sprintf("check PersistentPodState(%s/%s)", sts.Namespace, sts.Name))
			pods, err = tester.ListPodsInKruiseSts(sts)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			staticIP, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(staticIP.Status.PodStates).To(gomega.HaveLen(len(pods)))
			gomega.Expect(staticIP.Status.PodStates).To(gomega.Equal(currentStates))
			for _, pod := range pods {
				gomega.Expect(pod.Annotations[mutating.InjectedPersistentPodStateKey]).To(gomega.Equal(staticIP.Name))
				podState, ok := staticIP.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.Equal(true))
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(2))
				node, err := c.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for k, v := range podState.NodeTopologyLabels {
					gomega.Expect(v).To(gomega.Equal(node.Labels[k]))
				}
				gomega.Expect(podState.NodeName).To(gomega.Equal(node.Name))
			}

			// scale down, 5 -> 1
			sts.Spec.Replicas = utilpointer.Int32Ptr(1)
			tester.UpdateStatefulset(sts)
			time.Sleep(time.Second * 3)

			ginkgo.By(fmt.Sprintf("check PersistentPodState(%s/%s)", sts.Namespace, sts.Name))
			pods, err = tester.ListPodsInKruiseSts(sts)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			staticIP, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(staticIP.Status.PodStates).To(gomega.HaveLen(len(pods)))
			for _, pod := range pods {
				gomega.Expect(pod.Annotations[mutating.InjectedPersistentPodStateKey]).To(gomega.Equal(staticIP.Name))
				podState, ok := staticIP.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.Equal(true))
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(2))
				node, err := c.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for k, v := range podState.NodeTopologyLabels {
					gomega.Expect(v).To(gomega.Equal(node.Labels[k]))
				}
				gomega.Expect(podState.NodeName).To(gomega.Equal(node.Name))
			}

			// scale up, 1 -> 5
			sts.Spec.Replicas = utilpointer.Int32Ptr(5)
			tester.UpdateStatefulset(sts)
			time.Sleep(time.Second * 3)

			ginkgo.By(fmt.Sprintf("check PersistentPodState(%s/%s)", sts.Namespace, sts.Name))
			pods, err = tester.ListPodsInKruiseSts(sts)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			staticIP, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(staticIP.Status.PodStates).To(gomega.HaveLen(len(pods)))
			for _, pod := range pods {
				podState, ok := staticIP.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.Equal(true))
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(2))
				node, err := c.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for k, v := range podState.NodeTopologyLabels {
					gomega.Expect(v).To(gomega.Equal(node.Labels[k]))
				}
				gomega.Expect(podState.NodeName).To(gomega.Equal(node.Name))
			}
			currentStates = staticIP.Status.DeepCopy().PodStates

			// delete auto generate annotation
			delete(sts.Annotations, appsv1alpha1.AnnotationAutoGeneratePersistentPodState)
			tester.UpdateStatefulset(sts)
			time.Sleep(time.Second * 3)
			_, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())

			// add auto generate annotation
			sts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] = "true"
			tester.UpdateStatefulset(sts)
			time.Sleep(time.Second * 3)

			ginkgo.By(fmt.Sprintf("check PersistentPodState(%s/%s)", sts.Namespace, sts.Name))
			pods, err = tester.ListPodsInKruiseSts(sts)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			staticIP, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(staticIP.Status.PodStates).To(gomega.HaveLen(len(pods)))
			gomega.Expect(staticIP.Status.PodStates).To(gomega.Equal(currentStates))
			for _, pod := range pods {
				podState, ok := staticIP.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.Equal(true))
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(2))
				node, err := c.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for k, v := range podState.NodeTopologyLabels {
					gomega.Expect(v).To(gomega.Equal(node.Labels[k]))
				}
				gomega.Expect(podState.NodeName).To(gomega.Equal(node.Name))
			}

			delete(sts.Annotations, appsv1alpha1.AnnotationRequiredPersistentTopology)
			tester.UpdateStatefulset(sts)
			time.Sleep(time.Second * 3)

			ginkgo.By(fmt.Sprintf("check PersistentPodState(%s/%s)", sts.Namespace, sts.Name))
			pods, err = tester.ListPodsInKruiseSts(sts)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			staticIP, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(staticIP.Status.PodStates).To(gomega.HaveLen(len(pods)))
			for _, pod := range pods {
				podState, ok := staticIP.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.Equal(true))
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(1))
				node, err := c.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for k, v := range podState.NodeTopologyLabels {
					gomega.Expect(k).To(gomega.Equal(podStateNodeTopologyLabel))
					gomega.Expect(v).To(gomega.Equal(node.Labels[k]))
				}
				gomega.Expect(podState.NodeName).To(gomega.Equal(node.Name))
			}
			currentStates = staticIP.Status.DeepCopy().PodStates

			// update persistentPodState
			staticIP, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			staticIP.Spec.PersistentPodStateRetentionPolicy = appsv1alpha1.PersistentPodStateRetentionPolicyWhenDeleted
			_, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Update(context.TODO(), staticIP, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 3)

			// scale down 5 -> 1
			sts.Spec.Replicas = utilpointer.Int32Ptr(1)
			tester.UpdateStatefulset(sts)
			time.Sleep(time.Second * 3)

			ginkgo.By(fmt.Sprintf("check PersistentPodState(%s/%s)", sts.Namespace, sts.Name))
			pods, err = tester.ListPodsInKruiseSts(sts)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			staticIP, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(staticIP.Status.PodStates).To(gomega.Equal(currentStates))
			for _, pod := range pods {
				gomega.Expect(pod.Annotations[mutating.InjectedPersistentPodStateKey]).To(gomega.Equal(staticIP.Name))
				podState, ok := staticIP.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.Equal(true))
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(1))
				node, err := c.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for k, v := range podState.NodeTopologyLabels {
					gomega.Expect(k).To(gomega.Equal(podStateNodeTopologyLabel))
					gomega.Expect(v).To(gomega.Equal(node.Labels[k]))
				}
				gomega.Expect(podState.NodeName).To(gomega.Equal(node.Name))
			}

			// check validate webhook
			other := &appsv1alpha1.PersistentPodState{
				TypeMeta: metav1.TypeMeta{
					APIVersion: appsv1alpha1.GroupVersion.String(),
					Kind:       "PersistentPodState",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      "pps-test",
				},
				Spec: appsv1alpha1.PersistentPodStateSpec{
					TargetReference: appsv1alpha1.TargetReference{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       sts.Name,
					},
					RequiredPersistentTopology: &appsv1alpha1.NodeTopologyTerm{
						NodeTopologyKeys: []string{podStateNodeTopologyLabel},
					},
					PreferredPersistentTopology: []appsv1alpha1.PreferredTopologyTerm{
						{
							Weight: 100,
							Preference: appsv1alpha1.NodeTopologyTerm{
								NodeTopologyKeys: []string{podStateOsTopologyLabel},
							},
						},
					},
				},
			}
			_, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Create(context.TODO(), other, metav1.CreateOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())

			// delete sts
			err = c.AppsV1().StatefulSets(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 3)
			_, err = kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())

			ginkgo.By("statefulset node topology done")
		})

		ginkgo.It("statefulsetlike node topology", func() {
			crdExist, err := tester.CheckStatefulsetLikeCRDExist()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if !crdExist {
				ginkgo.By("crd statefilsetlike.kruise.io not exist, skip")
				return
			}
			// create statefulset
			tester.AddStatefulsetLikeClusterRoleConf()
			sts := tester.NewBaseStatefulsetLikeTest(ns)
			sts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] = "true"
			sts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] = podStateOsTopologyLabel
			sts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] = podStateNodeTopologyLabel
			sts.Annotations[appsv1alpha1.AnnotationPersistentPodAnnotations] = "name"
			ginkgo.By(fmt.Sprintf("Creating Statefulset like %s", sts.Name))
			tester.CreateStatefulsetLikeCRD(ns)
			sts = tester.CreateStatefulsetLike(sts)
			tester.CreateDynamicWatchWhiteList([]schema.GroupVersionKind{schema.GroupVersionKind{
				Group:   framework.StatefulSetLikeTestKind.Group,
				Kind:    framework.StatefulSetLikeTestKind.Kind,
				Version: framework.StatefulSetLikeTestKind.Version,
			}})
			tester.CreateStatefulsetLikePods(sts)
			ginkgo.By(fmt.Sprintf("check PersistentPodState(%s/%s)", sts.Namespace, sts.Name))
			tester.UpdateStatefulsetLikeStatus(sts)
			time.Sleep(time.Second * 10)

			// check PersistentPodState in staticIP
			pods, err := tester.ListPodsInNamespace(sts.Namespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			staticIP, err := kc.AppsV1alpha1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(staticIP.Status.PodStates).To(gomega.HaveLen(len(pods)))
			currentStates := staticIP.Status.DeepCopy().PodStates
			for _, pod := range pods {
				podState, ok := staticIP.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.Equal(true))
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(2))
				node, err := c.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for k, v := range podState.NodeTopologyLabels {
					gomega.Expect(v).To(gomega.Equal(node.Labels[k]))
				}
				gomega.Expect(podState.NodeName).To(gomega.Equal(node.Name))
			}

			gomega.Expect(currentStates).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			for _, state := range currentStates {
				gomega.Expect(state.Annotations).To(gomega.HaveLen(1))
			}

			ginkgo.By("statefulsetlike node topology done")
		})
	})
})
