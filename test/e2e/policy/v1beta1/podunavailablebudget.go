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
	apps "k8s.io/api/apps/v1"
	policy "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	frameworkv1alpha1 "github.com/openkruise/kruise/test/e2e/framework/v1alpha1"
)

var _ = ginkgo.Describe("PodUnavailableBudget", ginkgo.Label("PodUnavailableBudget", "policy", "v1beta1"), func() {
	f := frameworkv1alpha1.NewDefaultFramework("podunavailablebudget-v1beta1")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *frameworkv1alpha1.PodUnavailableBudgetTester
	var sidecarTester *frameworkv1alpha1.SidecarSetTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = frameworkv1alpha1.NewPodUnavailableBudgetTester(c, kc)
		sidecarTester = frameworkv1alpha1.NewSidecarSetTester(c, kc)
	})

	ginkgo.AfterEach(func() {
		if ginkgo.CurrentSpecReport().Failed() {
			common.DumpDebugInfo(c, ns)
		}
		tester.DeletePubs(ns)
		tester.DeleteDeployments(ns)
		tester.DeleteCloneSets(ns)
		sidecarTester.DeleteSidecarSets(ns)
	})

	ginkgo.It("PodUnavailableBudget selector pods and ignoredPodSelector allow force delete", func() {
		pub := &policyv1beta1.PodUnavailableBudget{
			TypeMeta: metav1.TypeMeta{
				APIVersion: policyv1beta1.GroupVersion.String(),
				Kind:       "PodUnavailableBudget",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      "webserver-pub",
			},
			Spec: policyv1beta1.PodUnavailableBudgetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"pub-controller": "true",
					},
				},
				MaxUnavailable: &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 0,
				},
				IgnoredPodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kruise.io/force-deletable": "true",
					},
				},
			},
		}
		ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
		_, err := kc.PolicyV1beta1().PodUnavailableBudgets(pub.Namespace).Create(context.TODO(), pub, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		deployment := tester.NewBaseDeployment(ns)
		deployment.Spec.Replicas = ptr.To(int32(1))
		ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deployment.Namespace, deployment.Name))
		tester.CreateDeployment(deployment)

		ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
		gomega.Eventually(func() apps.DeploymentStatus {
			current, err := c.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			return current.Status
		}, 30*time.Second, time.Second).Should(gomega.WithTransform(func(status apps.DeploymentStatus) int32 {
			return status.ReadyReplicas
		}, gomega.Equal(int32(1))))

		gomega.Eventually(func() int32 {
			current, err := kc.PolicyV1beta1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			return current.Status.UnavailableAllowed
		}, 30*time.Second, time.Second).Should(gomega.Equal(int32(0)))

		pods, err := sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(pods).To(gomega.HaveLen(1))

		ginkgo.By("Verifying PUB blocks eviction before the ignored selector matches")
		err = c.CoreV1().Pods(deployment.Namespace).Evict(context.TODO(), &policy.Eviction{
			ObjectMeta: metav1.ObjectMeta{Name: pods[0].Name},
		})
		gomega.Expect(apierrors.IsTooManyRequests(err)).To(gomega.BeTrue(), "expected PUB to block eviction with 429, got %v", err)

		ginkgo.By("Labeling the pod so ignoredPodSelector exempts it from protection")
		patch := []byte(`{"metadata":{"labels":{"kruise.io/force-deletable":"true"}}}`)
		_, err = c.CoreV1().Pods(deployment.Namespace).Patch(context.TODO(), pods[0].Name, types.MergePatchType, patch, metav1.PatchOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		gomega.Eventually(func() error {
			return c.CoreV1().Pods(deployment.Namespace).Evict(context.TODO(), &policy.Eviction{
				ObjectMeta: metav1.ObjectMeta{Name: pods[0].Name},
			})
		}, 10*time.Second, time.Second).Should(gomega.Succeed())
	})
})
