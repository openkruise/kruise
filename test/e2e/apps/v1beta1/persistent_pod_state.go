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
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/webhook/pod/mutating"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	fw "github.com/openkruise/kruise/test/e2e/framework/v1beta1"
)

var (
	ppsBetaOsTopologyLabel   = "kubernetes.io/os"
	ppsBetaNodeTopologyLabel = "kubernetes.io/hostname"
)

var _ = ginkgo.Describe("PersistentPodState", ginkgo.Label("PersistentPodState", "operation"), func() {
	f := fw.NewDefaultFramework("persistentpodstates-v1beta1")
	var ns string
	var c clientset.Interface
	var a apiextensionsclientset.Interface
	var d dynamic.Interface
	var kc kruiseclientset.Interface
	var tester *fw.PersistentPodStateTester

	ginkgo.BeforeEach(func() {
		a = f.ApiExtensionsClientSet
		c = f.ClientSet
		d = f.DynamicClient
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = fw.NewPersistentPodStateTester(c, kc, d, a)
	})

	ginkgo.Context("PersistentPodState v1beta1", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentSpecReport().Failed() {
				common.DumpDebugInfo(c, ns)
			}
		})

		ginkgo.It("statefulset node topology with v1beta1 storage", func() {
			sts := tester.NewBaseStatefulset(ns)
			sts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] = "true"
			sts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] = ppsBetaOsTopologyLabel
			sts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] = ppsBetaNodeTopologyLabel
			ginkgo.By(fmt.Sprintf("Creating StatefulSet %s", sts.Name))
			tester.CreateStatefulset(sts)

			ginkgo.By("verify auto-generated PersistentPodState uses v1beta1 keys field")
			var pps *appsv1beta1.PersistentPodState
			gomega.Eventually(func() error {
				var err error
				pps, err = kc.AppsV1beta1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if pps.Spec.RequiredPersistentTopology == nil || len(pps.Spec.RequiredPersistentTopology.Keys) == 0 {
					return fmt.Errorf("required topology keys not ready yet")
				}
				if len(pps.Spec.PreferredPersistentTopology) == 0 || len(pps.Spec.PreferredPersistentTopology[0].Preference.Keys) == 0 {
					return fmt.Errorf("preferred topology keys not ready yet")
				}
				return nil
			}, common.PodStartShortTimeout, common.Poll).Should(gomega.Succeed())
			gomega.Expect(pps.Spec.RequiredPersistentTopology.Keys).To(gomega.ContainElement(ppsBetaOsTopologyLabel))
			gomega.Expect(pps.Spec.PreferredPersistentTopology).To(gomega.HaveLen(1))
			gomega.Expect(pps.Spec.PreferredPersistentTopology[0].Preference.Keys).To(gomega.ContainElement(ppsBetaNodeTopologyLabel))

			var pods []*corev1.Pod
			gomega.Eventually(func() int {
				var err error
				pods, err = tester.ListPodsInKruiseSts(sts)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(pods)
			}, common.PodStartShortTimeout, common.Poll).Should(gomega.Equal(int(*sts.Spec.Replicas)))

			gomega.Eventually(func() int {
				pps, err := kc.AppsV1beta1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(pps.Status.PodStates)
			}, common.PodStartShortTimeout, common.Poll).Should(gomega.Equal(len(pods)))

			pps, err := kc.AppsV1beta1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				podState, ok := pps.Status.PodStates[pod.Name]
				gomega.Expect(ok).To(gomega.BeTrue())
				gomega.Expect(podState.NodeTopologyLabels).To(gomega.HaveLen(2))
				gomega.Expect(pod.Annotations[mutating.InjectedPersistentPodStateKey]).To(gomega.Equal(pps.Name))
			}

			sts.Spec.Template.Spec.Containers[0].Image = "busybox:1.35"
			tester.UpdateStatefulset(sts)
			time.Sleep(3 * time.Second)

			pps, err = kc.AppsV1beta1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pps.Status.PodStates).To(gomega.HaveLen(len(pods)))

			sts.Spec.Replicas = ptr.To[int32](1)
			tester.UpdateStatefulset(sts)
			time.Sleep(3 * time.Second)

			pps, err = kc.AppsV1beta1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			pods, err = tester.ListPodsInKruiseSts(sts)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pps.Status.PodStates).To(gomega.HaveLen(len(pods)))

			other := &appsv1beta1.PersistentPodState{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      "pps-test",
				},
				Spec: appsv1beta1.PersistentPodStateSpec{
					TargetReference: appsv1beta1.TargetReference{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       sts.Name,
					},
					RequiredPersistentTopology: &appsv1beta1.NodeTopologyTerm{
						Keys: []string{ppsBetaNodeTopologyLabel},
					},
					PreferredPersistentTopology: []appsv1beta1.PreferredTopologyTerm{
						{
							Weight: 100,
							Preference: appsv1beta1.NodeTopologyTerm{
								Keys: []string{ppsBetaOsTopologyLabel},
							},
						},
					},
				},
			}
			_, err = kc.AppsV1beta1().PersistentPodStates(sts.Namespace).Create(context.TODO(), other, metav1.CreateOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())

			err = c.AppsV1().StatefulSets(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(3 * time.Second)
			_, err = kc.AppsV1beta1().PersistentPodStates(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
		})

		ginkgo.It("cross-version: v1alpha1 nodeTopologyKeys converts to v1beta1 keys", func() {
			sts := tester.NewBaseStatefulset(ns)
			sts.Name = "pps-cross-version-sts"
			sts.Spec.Selector.MatchLabels["app"] = "pps-cross"
			sts.Spec.Template.ObjectMeta.Labels["app"] = "pps-cross"
			tester.CreateStatefulset(sts)
			time.Sleep(3 * time.Second)

			ginkgo.By("create PersistentPodState via v1alpha1 with nodeTopologyKeys")
			alphaPPS := &appsv1alpha1.PersistentPodState{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      "pps-cross-version",
				},
				Spec: appsv1alpha1.PersistentPodStateSpec{
					TargetReference: appsv1alpha1.TargetReference{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       sts.Name,
					},
					RequiredPersistentTopology: &appsv1alpha1.NodeTopologyTerm{
						NodeTopologyKeys: []string{ppsBetaNodeTopologyLabel},
					},
					PreferredPersistentTopology: []appsv1alpha1.PreferredTopologyTerm{
						{
							Weight: 100,
							Preference: appsv1alpha1.NodeTopologyTerm{
								NodeTopologyKeys: []string{ppsBetaOsTopologyLabel},
							},
						},
					},
				},
			}
			_, err := kc.AppsV1alpha1().PersistentPodStates(ns).Create(context.TODO(), alphaPPS, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("read via v1beta1 and verify keys field (not nodeTopologyKeys)")
			var betaPPS *appsv1beta1.PersistentPodState
			gomega.Eventually(func() error {
				var getErr error
				betaPPS, getErr = kc.AppsV1beta1().PersistentPodStates(ns).Get(context.TODO(), alphaPPS.Name, metav1.GetOptions{})
				if getErr != nil {
					return getErr
				}
				if betaPPS.Spec.RequiredPersistentTopology == nil {
					return fmt.Errorf("required topology not converted yet")
				}
				if len(betaPPS.Spec.RequiredPersistentTopology.Keys) == 0 {
					return fmt.Errorf("keys not populated yet")
				}
				return nil
			}, common.PodStartShortTimeout, common.Poll).Should(gomega.Succeed())
			gomega.Expect(betaPPS.Spec.RequiredPersistentTopology.Keys).To(gomega.Equal([]string{ppsBetaNodeTopologyLabel}))
			gomega.Expect(betaPPS.Spec.PreferredPersistentTopology[0].Preference.Keys).To(gomega.Equal([]string{ppsBetaOsTopologyLabel}))

			ginkgo.By("round-trip write v1beta1 keys and read v1alpha1 nodeTopologyKeys")
			betaPPS.Spec.RequiredPersistentTopology.Keys = []string{ppsBetaOsTopologyLabel, ppsBetaNodeTopologyLabel}
			_, err = kc.AppsV1beta1().PersistentPodStates(ns).Update(context.TODO(), betaPPS, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			var alphaRead *appsv1alpha1.PersistentPodState
			gomega.Eventually(func() []string {
				var getErr error
				alphaRead, getErr = kc.AppsV1alpha1().PersistentPodStates(ns).Get(context.TODO(), alphaPPS.Name, metav1.GetOptions{})
				gomega.Expect(getErr).NotTo(gomega.HaveOccurred())
				if alphaRead.Spec.RequiredPersistentTopology == nil {
					return nil
				}
				return alphaRead.Spec.RequiredPersistentTopology.NodeTopologyKeys
			}, common.PodStartShortTimeout, common.Poll).Should(
				gomega.Equal([]string{ppsBetaOsTopologyLabel, ppsBetaNodeTopologyLabel}))

			err = kc.AppsV1beta1().PersistentPodStates(ns).Delete(context.TODO(), alphaPPS.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			err = c.AppsV1().StatefulSets(sts.Namespace).Delete(context.TODO(), sts.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
})
