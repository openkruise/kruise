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
	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	utilpointer "k8s.io/utils/pointer"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
)

var _ = SIGDescribe("PodUnavailableBudget", func() {
	f := framework.NewDefaultFramework("podunavailablebudget")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.PodUnavailableBudgetTester
	var sidecarTester *framework.SidecarSetTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewPodUnavailableBudgetTester(c, kc)
		sidecarTester = framework.NewSidecarSetTester(c, kc)
	})

	framework.KruiseDescribe("podUnavailableBudget functionality [podUnavailableBudget]", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all PodUnavailableBudgets and Deployments in cluster")
			tester.DeletePubs(ns)
			tester.DeleteDeployments(ns)
			tester.DeleteCloneSets(ns)
			sidecarTester.DeleteSidecarSets(ns)
		})

		ginkgo.It("PodUnavailableBudget selector no matched pods", func() {
			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			deployment.Spec.Selector.MatchLabels["pub-controller"] = "false"
			deployment.Spec.Template.Labels["pub-controller"] = "false"
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// create pub
			pub := tester.NewBasePub(ns)
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)
			time.Sleep(time.Second * 5)

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   0,
				CurrentAvailable:   0,
				TotalReplicas:      0,
			}
			setPubStatus(expectStatus)
			var err error
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))
			ginkgo.By("PodUnavailableBudget selector no matched pods done")
		})

		ginkgo.It("PodUnavailableBudget selector pods and delete deployment ignore", func() {
			// create pub
			pub := tester.NewBasePub(ns)
			pub.Spec.MaxUnavailable = &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 0,
			}
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)
			time.Sleep(time.Second * 3)

			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			deployment.Spec.Replicas = utilpointer.Int32Ptr(1)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   1,
				CurrentAvailable:   1,
				TotalReplicas:      1,
			}
			setPubStatus(expectStatus)
			var err error
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// pods that contain annotations[pod.kruise.io/pub-no-protect]="true" will be ignore
			// and will no longer check the pub quota
			pods, err := sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			err = c.CoreV1().Pods(deployment.Namespace).Evict(context.TODO(), &policy.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name: pods[0].Name,
				},
			})
			gomega.Expect(err).To(gomega.HaveOccurred())
			// add annotations
			pods, err = sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			podIn := pods[0]
			if podIn.Annotations == nil {
				podIn.Annotations = map[string]string{}
			}
			podIn.Annotations[policyv1alpha1.PodPubNoProtectionAnnotation] = "true"
			_, err = c.CoreV1().Pods(deployment.Namespace).Update(context.TODO(), podIn, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second)
			err = c.CoreV1().Pods(deployment.Namespace).Evict(context.TODO(), &policy.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name: pods[0].Name,
				},
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 5)

			// delete deployment
			ginkgo.By(fmt.Sprintf("Deleting Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			err = c.AppsV1().Deployments(deployment.Namespace).Delete(context.TODO(), deployment.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   0,
				CurrentAvailable:   0,
				TotalReplicas:      0,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			pods, err = sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(0))

			ginkgo.By("PodUnavailableBudget selector pods and delete deployment reject done")
		})

		ginkgo.It("PodUnavailableBudget selector pods and scale down deployment ignore", func() {
			// create pub
			pub := tester.NewBasePub(ns)
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)

			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			deployment.Spec.Replicas = utilpointer.Int32Ptr(4)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   3,
				CurrentAvailable:   4,
				TotalReplicas:      4,
			}
			setPubStatus(expectStatus)
			var err error
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// scale down deployment
			ginkgo.By(fmt.Sprintf("scale down Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			deployment.Spec.Replicas = utilpointer.Int32Ptr(0)
			_, err = c.AppsV1().Deployments(deployment.Namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				DesiredAvailable: 0,
				TotalReplicas:    0,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := pub.Status.DeepCopy()
				setPubStatus(nowStatus)
				expectStatus.UnavailableAllowed = nowStatus.UnavailableAllowed
				expectStatus.CurrentAvailable = nowStatus.CurrentAvailable
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			gomega.Eventually(func() []*corev1.Pod {
				pods, err := sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return pods
			}).Should(gomega.HaveLen(0))

			ginkgo.By("PodUnavailableBudget selector pods and scale down deployment reject done")
		})

		ginkgo.It("PodUnavailableBudget targetReference pods, update failed image and block", func() {
			// create pub
			pub := tester.NewBasePub(ns)
			pub.Spec.Selector = nil
			pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "webserver",
			}
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)

			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   1,
				CurrentAvailable:   2,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			var err error
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update failed image
			ginkgo.By(fmt.Sprintf("update Deployment(%s/%s) failed image", deployment.Namespace, deployment.Name))
			deployment.Spec.Template.Spec.Containers[0].Image = InvalidImage
			_, err = c.AppsV1().Deployments(deployment.Namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 5)

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   1,
				CurrentAvailable:   1,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// check now pod
			pods, err := sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			noUpdatePods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if pod.Spec.Containers[0].Image == InvalidImage || !pod.DeletionTimestamp.IsZero() {
					continue
				}
				noUpdatePods = append(noUpdatePods, *pod)
			}
			gomega.Expect(noUpdatePods).To(gomega.HaveLen(1))

			// update success image
			ginkgo.By(fmt.Sprintf("update Deployment(%s/%s) success image", deployment.Namespace, deployment.Name))
			deployment.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			_, err = c.AppsV1().Deployments(deployment.Namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			tester.WaitForDeploymentReadyAndRunning(deployment)

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   1,
				CurrentAvailable:   2,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 60*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			//check pods
			pods, err = sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			newPods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if !pod.DeletionTimestamp.IsZero() {
					continue
				}
				gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal(NewWebserverImage))
				newPods = append(newPods, *pod)
			}
			gomega.Expect(newPods).To(gomega.HaveLen(2))

			// add unavailable label
			labelKey := fmt.Sprintf("%sdata", appspub.PubUnavailablePodLabelPrefix)
			labelBody := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, labelKey, "true")
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), newPods[0].Name, types.MergePatchType, []byte(labelBody), metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   1,
				CurrentAvailable:   1,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 60*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update pod image, ignore
			podIn1, err := c.CoreV1().Pods(ns).Get(context.TODO(), newPods[0].Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			podIn1.Spec.Containers[0].Image = WebserverImage
			_, err = c.CoreV1().Pods(ns).Update(context.TODO(), podIn1, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// add unavailable label reject
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), newPods[1].Name, types.MergePatchType, []byte(labelBody), metav1.PatchOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			// update pod image, reject
			podIn2, err := c.CoreV1().Pods(ns).Get(context.TODO(), newPods[1].Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			podIn2.Spec.Containers[0].Image = WebserverImage
			_, err = c.CoreV1().Pods(ns).Update(context.TODO(), podIn2, metav1.UpdateOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())

			// add pub protect operation DELETE
			annotationBody := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"%s"}}}`, policyv1alpha1.PubProtectOperationAnnotation, policyv1alpha1.PubDeleteOperation)
			_, err = kc.PolicyV1alpha1().PodUnavailableBudgets(ns).Patch(context.TODO(), pub.Name, types.MergePatchType, []byte(annotationBody), metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 3)
			// update pod image, allow
			podIn2, err = c.CoreV1().Pods(ns).Get(context.TODO(), newPods[1].Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			podIn2.Spec.Containers[0].Image = WebserverImage
			_, err = c.CoreV1().Pods(ns).Update(context.TODO(), podIn2, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 3)

			// check pod image
			pods, err = sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				if !pod.DeletionTimestamp.IsZero() {
					continue
				}
				gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal(WebserverImage))
			}

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   1,
				CurrentAvailable:   1,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 60*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// delete unavailable label
			deleteLabelBody := fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`, labelKey)
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), newPods[0].Name, types.StrategicMergePatchType, []byte(deleteLabelBody), metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   1,
				CurrentAvailable:   2,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 60*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			ginkgo.By("PodUnavailableBudget targetReference pods, update failed image and block done")
		})

		ginkgo.It("PodUnavailableBudget selector two deployments, deployment.strategy.maxUnavailable=25% and maxSurge=25%, pub.spec.maxUnavailable=25%, and update success image", func() {
			// create pub
			var err error
			pub := tester.NewBasePub(ns)
			pub.Spec.MaxUnavailable = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "25%",
			}
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)

			// create deployment1
			deployment := tester.NewBaseDeployment(ns)
			deployment.Spec.Strategy.RollingUpdate.MaxUnavailable = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "25%",
			}
			deployment.Spec.Strategy.RollingUpdate.MaxSurge = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "25%",
			}
			deployment.Spec.Replicas = utilpointer.Int32Ptr(5)
			deploymentIn1 := deployment.DeepCopy()
			deploymentIn1.Name = fmt.Sprintf("%s-1", deploymentIn1.Name)
			ginkgo.By(fmt.Sprintf("Creating Deployment1(%s/%s)", deploymentIn1.Namespace, deploymentIn1.Name))
			tester.CreateDeployment(deploymentIn1)
			// create deployment2
			deploymentIn2 := deployment.DeepCopy()
			deploymentIn2.Name = fmt.Sprintf("%s-2", deploymentIn1.Name)
			ginkgo.By(fmt.Sprintf("Creating Deployment2(%s/%s)", deploymentIn2.Namespace, deploymentIn2.Name))
			tester.CreateDeployment(deploymentIn2)

			ginkgo.By(fmt.Sprintf("PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 3,
				DesiredAvailable:   7,
				CurrentAvailable:   10,
				TotalReplicas:      10,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update success image
			ginkgo.By("update Deployment-1 and deployment-2 with success image")
			deploymentIn1.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			_, err = c.AppsV1().Deployments(deploymentIn1.Namespace).Update(context.TODO(), deploymentIn1, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			deploymentIn2.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			_, err = c.AppsV1().Deployments(deploymentIn2.Namespace).Update(context.TODO(), deploymentIn2, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// wait 1 seconds, and check deployment, pub Status
			ginkgo.By("wait 1 seconds, and check deployment, pub Status")
			time.Sleep(time.Second)
			// check deployment
			tester.WaitForDeploymentReadyAndRunning(deploymentIn1)
			tester.WaitForDeploymentReadyAndRunning(deploymentIn2)
			// check pods
			pods, err := sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal(NewWebserverImage))
			}
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 3,
				DesiredAvailable:   7,
				CurrentAvailable:   10,
				TotalReplicas:      10,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 5*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// scale down replicas = 0
			ginkgo.By("scale down Deployment-1 replicas to 0")
			deploymentIn1.Spec.Replicas = utilpointer.Int32(0)
			_, err = c.AppsV1().Deployments(deploymentIn1.Namespace).Update(context.TODO(), deploymentIn1, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			tester.WaitForDeploymentReadyAndRunning(deploymentIn1)
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 2,
				DesiredAvailable:   3,
				CurrentAvailable:   5,
				TotalReplicas:      5,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 5*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// delete deployment directly
			err = c.AppsV1().Deployments(deploymentIn2.Namespace).Delete(context.TODO(), deploymentIn2.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 3)
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   0,
				CurrentAvailable:   0,
				TotalReplicas:      0,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 5*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			ginkgo.By("PodUnavailableBudget selector two deployments, deployment.strategy.maxUnavailable=100%, pub.spec.maxUnavailable=50%, and update success image done")
		})

		ginkgo.It("PodUnavailableBudget selector SidecarSet, inject sidecar container, update failed sidecar image, block", func() {
			// create pub
			pub := tester.NewBasePub(ns)
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)

			// create sidecarset
			sidecarSet := sidecarTester.NewBaseSidecarSet(ns)
			sidecarSet.Spec.InitContainers = nil
			sidecarSet.Spec.Namespace = ns
			sidecarSet.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "webserver",
				},
			}
			sidecarSet.Spec.Containers = []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:    "nginx-sidecar",
						Image:   NginxImage,
						Command: []string{"tail", "-f", "/dev/null"},
					},
				},
			}
			sidecarSet.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
				MaxUnavailable: &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "100%",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			var err error
			sidecarSet, err = sidecarTester.CreateSidecarSet(sidecarSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second)

			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			deployment.Spec.Replicas = utilpointer.Int32Ptr(5)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			time.Sleep(time.Second)
			// check sidecarSet inject sidecar container
			ginkgo.By("check sidecarSet inject sidecar container and pub status")
			pods, err := sidecarTester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			pod := pods[0]
			gomega.Expect(pod.Spec.Containers).To(gomega.HaveLen(len(deployment.Spec.Template.Spec.Containers) + len(sidecarSet.Spec.Containers)))

			//check pub status
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   4,
				CurrentAvailable:   5,
				TotalReplicas:      5,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update sidecar container failed image
			ginkgo.By("update sidecar container failed image")
			sidecarSet, err = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sidecarSet.Spec.Containers[0].Image = InvalidImage
			sidecarTester.UpdateSidecarSet(sidecarSet)

			// wait 1 seconds, and check sidecarSet upgrade block
			ginkgo.By("wait 1 seconds, and check sidecarSet upgrade block")
			time.Sleep(time.Second)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      5,
				UpdatedPods:      1,
				UpdatedReadyPods: 0,
				ReadyPods:        4,
			}
			sidecarTester.WaitForSidecarSetMinReadyAndUpgrade(sidecarSet, except, 4)
			//check pub status
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   4,
				CurrentAvailable:   4,
				TotalReplicas:      5,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() int {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				return len(nowStatus.UnavailablePods)
			}, 20*time.Second, time.Second).Should(gomega.Equal(1))
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 20*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update sidecar container success image
			ginkgo.By("update sidecar container success image")
			sidecarSet, err = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sidecarSet.Spec.Containers[0].Image = NewNginxImage
			sidecarTester.UpdateSidecarSet(sidecarSet)

			time.Sleep(time.Second)
			// check sidecarSet upgrade success
			ginkgo.By("check sidecarSet upgrade success")
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      5,
				UpdatedPods:      5,
				UpdatedReadyPods: 5,
				ReadyPods:        5,
			}
			sidecarTester.WaitForSidecarSetMinReadyAndUpgrade(sidecarSet, except, 4)
			//check pub status
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   4,
				CurrentAvailable:   5,
				TotalReplicas:      5,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			ginkgo.By("PodUnavailableBudget selector pods, inject sidecar container, update failed sidecar image, block done")
		})

		ginkgo.It("PodUnavailableBudget selector cloneSet, strategy.type=recreate, update failed image and block", func() {
			// create pub
			pub := tester.NewBasePub(ns)
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)

			// create cloneset
			cloneset := tester.NewBaseCloneSet(ns)
			ginkgo.By(fmt.Sprintf("Creating CloneSet(%s/%s)", cloneset.Namespace, cloneset.Name))
			cloneset = tester.CreateCloneSet(cloneset)

			// wait 10 seconds
			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   1,
				CurrentAvailable:   2,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			var err error
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update failed image
			ginkgo.By(fmt.Sprintf("update CloneSet(%s/%s) with failed image", cloneset.Namespace, cloneset.Name))
			cloneset.Spec.Template.Spec.Containers[0].Image = InvalidImage
			_, err = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Update(context.TODO(), cloneset, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			//wait 20 seconds
			ginkgo.By(fmt.Sprintf("waiting 20 seconds, and check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   1,
				CurrentAvailable:   1,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// check now pod
			pods, err := sidecarTester.GetSelectorPods(cloneset.Namespace, cloneset.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			noUpdatePods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if pod.Spec.Containers[0].Image == InvalidImage || !pod.DeletionTimestamp.IsZero() {
					continue
				}
				noUpdatePods = append(noUpdatePods, *pod)
			}
			gomega.Expect(noUpdatePods).To(gomega.HaveLen(1))

			// update success image
			ginkgo.By(fmt.Sprintf("update CloneSet(%s/%s) success image", cloneset.Namespace, cloneset.Name))
			cloneset, _ = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Get(context.TODO(), cloneset.Name, metav1.GetOptions{})
			cloneset.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			cloneset, err = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Update(context.TODO(), cloneset, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			tester.WaitForCloneSetMinReadyAndRunning([]*appsv1alpha1.CloneSet{cloneset}, 1)

			// check pub status
			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   1,
				CurrentAvailable:   2,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			//check pods
			pods, err = sidecarTester.GetSelectorPods(cloneset.Namespace, cloneset.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			newPods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if !pod.DeletionTimestamp.IsZero() || pod.Spec.Containers[0].Image != NewWebserverImage {
					continue
				}
				newPods = append(newPods, *pod)
			}
			gomega.Expect(newPods).To(gomega.HaveLen(2))
			ginkgo.By("PodUnavailableBudget selector cloneSet, update failed image and block done")
		})

		ginkgo.It("PodUnavailableBudget selector cloneSet, strategy.type=in-place, update failed image and block", func() {
			// create pub
			pub := tester.NewBasePub(ns)
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)

			// create cloneset
			cloneset := tester.NewBaseCloneSet(ns)
			cloneset.Spec.UpdateStrategy.Type = appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType
			ginkgo.By(fmt.Sprintf("Creating CloneSet(%s/%s)", cloneset.Namespace, cloneset.Name))
			cloneset = tester.CreateCloneSet(cloneset)

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   1,
				CurrentAvailable:   2,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			var err error
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update failed image
			ginkgo.By(fmt.Sprintf("update CloneSet(%s/%s) with failed image", cloneset.Namespace, cloneset.Name))
			cloneset.Spec.Template.Spec.Containers[0].Image = InvalidImage
			_, err = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Update(context.TODO(), cloneset, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			//wait 20 seconds
			ginkgo.By(fmt.Sprintf("waiting 20 seconds, and check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   1,
				CurrentAvailable:   1,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// check now pod
			pods, err := sidecarTester.GetSelectorPods(cloneset.Namespace, cloneset.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			noUpdatePods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if pod.Spec.Containers[0].Image == InvalidImage || !pod.DeletionTimestamp.IsZero() {
					continue
				}
				noUpdatePods = append(noUpdatePods, *pod)
			}
			gomega.Expect(noUpdatePods).To(gomega.HaveLen(1))

			// update success image
			ginkgo.By(fmt.Sprintf("update CloneSet(%s/%s) success image", cloneset.Namespace, cloneset.Name))
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				cloneset, err = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Get(context.TODO(), cloneset.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				cloneset.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
				cloneset, err = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Update(context.TODO(), cloneset, metav1.UpdateOptions{})
				return err
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			tester.WaitForCloneSetMinReadyAndRunning([]*appsv1alpha1.CloneSet{cloneset}, 1)

			//wait 20 seconds
			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 1,
				DesiredAvailable:   1,
				CurrentAvailable:   2,
				TotalReplicas:      2,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			//check pods
			pods, err = sidecarTester.GetSelectorPods(cloneset.Namespace, cloneset.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			newPods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if !pod.DeletionTimestamp.IsZero() || pod.Spec.Containers[0].Image != NewWebserverImage {
					continue
				}
				newPods = append(newPods, *pod)
			}
			gomega.Expect(newPods).To(gomega.HaveLen(2))
			ginkgo.By("PodUnavailableBudget selector cloneSet, update failed image and block done")
		})

		ginkgo.It("PodUnavailableBudget selector two cloneSets, strategy.type=in-place, update success image", func() {
			// create pub
			pub := tester.NewBasePub(ns)
			pub.Spec.MaxUnavailable = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "20%",
			}
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)

			// create cloneset1
			cloneset := tester.NewBaseCloneSet(ns)
			cloneset.Spec.Replicas = utilpointer.Int32Ptr(5)
			cloneset.Spec.UpdateStrategy.Type = appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType
			clonesetIn1 := cloneset.DeepCopy()
			clonesetIn1.Name = fmt.Sprintf("%s-1", clonesetIn1.Name)
			ginkgo.By(fmt.Sprintf("Creating CloneSet1(%s/%s)", clonesetIn1.Namespace, clonesetIn1.Name))
			clonesetIn1 = tester.CreateCloneSet(clonesetIn1)
			//create cloneSet2
			clonesetIn2 := cloneset.DeepCopy()
			clonesetIn2.Name = fmt.Sprintf("%s-2", clonesetIn2.Name)
			ginkgo.By(fmt.Sprintf("Creating CloneSet2(%s/%s)", clonesetIn2.Namespace, clonesetIn2.Name))
			clonesetIn2 = tester.CreateCloneSet(clonesetIn2)

			// wait 10 seconds
			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 2,
				DesiredAvailable:   8,
				CurrentAvailable:   10,
				TotalReplicas:      10,
			}
			setPubStatus(expectStatus)
			var err error
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update failed image
			ginkgo.By(fmt.Sprintf("update CloneSet(%s/%s) with failed image", cloneset.Namespace, cloneset.Name))
			clonesetIn1.Spec.Template.Spec.Containers[0].Image = InvalidImage
			_, err = kc.AppsV1alpha1().CloneSets(clonesetIn1.Namespace).Update(context.TODO(), clonesetIn1, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			clonesetIn2.Spec.Template.Spec.Containers[0].Image = InvalidImage
			_, err = kc.AppsV1alpha1().CloneSets(clonesetIn2.Namespace).Update(context.TODO(), clonesetIn2, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			//wait 20 seconds
			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 0,
				DesiredAvailable:   8,
				CurrentAvailable:   8,
				TotalReplicas:      10,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			time.Sleep(5 * time.Second)
			// check now pod
			pods, err := sidecarTester.GetSelectorPods(cloneset.Namespace, cloneset.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			noUpdatePods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if pod.Spec.Containers[0].Image == InvalidImage || !pod.DeletionTimestamp.IsZero() {
					continue
				}
				noUpdatePods = append(noUpdatePods, *pod)
			}
			gomega.Expect(noUpdatePods).To(gomega.HaveLen(8))

			// update success image
			ginkgo.By(fmt.Sprintf("update CloneSet(%s/%s) success image", cloneset.Namespace, cloneset.Name))
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				clonesetIn1, err = kc.AppsV1alpha1().CloneSets(clonesetIn1.Namespace).Get(context.TODO(), clonesetIn1.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				clonesetIn1.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
				clonesetIn1, err = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Update(context.TODO(), clonesetIn1, metav1.UpdateOptions{})
				return err
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// update success image
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				clonesetIn2, err = kc.AppsV1alpha1().CloneSets(clonesetIn2.Namespace).Get(context.TODO(), clonesetIn2.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				clonesetIn2.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
				clonesetIn2, err = kc.AppsV1alpha1().CloneSets(clonesetIn2.Namespace).Update(context.TODO(), clonesetIn2, metav1.UpdateOptions{})
				return err
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			tester.WaitForCloneSetMinReadyAndRunning([]*appsv1alpha1.CloneSet{clonesetIn1, clonesetIn2}, 7)

			// check pub status
			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 2,
				DesiredAvailable:   8,
				CurrentAvailable:   10,
				TotalReplicas:      10,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			//check pods
			pods, err = sidecarTester.GetSelectorPods(cloneset.Namespace, cloneset.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			newPods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if pod.DeletionTimestamp.IsZero() && pod.Spec.Containers[0].Image == NewWebserverImage {
					newPods = append(newPods, *pod)
				}
			}
			gomega.Expect(newPods).To(gomega.HaveLen(10))
			ginkgo.By("PodUnavailableBudget selector two cloneSets, strategy.type=in-place, update success image done")
		})

		ginkgo.It("PodUnavailableBudget selector cloneSet and sidecarSet, strategy.type=in-place, update success image", func() {
			// create pub
			pub := tester.NewBasePub(ns)
			pub.Spec.MaxUnavailable = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "20%",
			}
			ginkgo.By(fmt.Sprintf("Creating PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name))
			tester.CreatePub(pub)

			// create sidecarSet
			sidecarSet := sidecarTester.NewBaseSidecarSet(ns)
			sidecarSet.Spec.InitContainers = nil
			sidecarSet.Spec.Namespace = ns
			sidecarSet.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "webserver",
				},
			}
			sidecarSet.Spec.Containers = []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:    "nginx-sidecar",
						Image:   NginxImage,
						Command: []string{"tail", "-f", "/dev/null"},
					},
				},
			}
			sidecarSet.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
				MaxUnavailable: &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "100%",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			sidecarSet, _ = sidecarTester.CreateSidecarSet(sidecarSet)

			// create cloneset
			cloneset := tester.NewBaseCloneSet(ns)
			cloneset.Spec.UpdateStrategy.Type = appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType
			cloneset.Spec.Replicas = utilpointer.Int32Ptr(10)
			ginkgo.By(fmt.Sprintf("Creating CloneSet(%s/%s)", cloneset.Namespace, cloneset.Name))
			cloneset = tester.CreateCloneSet(cloneset)

			time.Sleep(time.Second)
			// check sidecarSet inject sidecar container
			ginkgo.By("check sidecarSet inject sidecar container and pub status")
			pods, err := sidecarTester.GetSelectorPods(cloneset.Namespace, cloneset.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			pod := pods[0]
			gomega.Expect(pod.Spec.Containers).To(gomega.HaveLen(len(cloneset.Spec.Template.Spec.Containers) + len(sidecarSet.Spec.Containers)))

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus := &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 2,
				DesiredAvailable:   8,
				CurrentAvailable:   10,
				TotalReplicas:      10,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			// update success image
			ginkgo.By(fmt.Sprintf("update CloneSet(%s/%s) success image", cloneset.Namespace, cloneset.Name))
			cloneset, _ = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Get(context.TODO(), cloneset.Name, metav1.GetOptions{})
			cloneset.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			cloneset, err = kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Update(context.TODO(), cloneset, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// update sidecar container success image
			ginkgo.By("update sidecar container success image")
			sidecarSet.Spec.Containers[0].Image = NewNginxImage
			sidecarTester.UpdateSidecarSet(sidecarSet)
			time.Sleep(time.Second)
			tester.WaitForCloneSetMinReadyAndRunning([]*appsv1alpha1.CloneSet{cloneset}, 2)
			exceptSidecarSetStatus := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      10,
				UpdatedPods:      10,
				UpdatedReadyPods: 10,
				ReadyPods:        10,
			}
			sidecarTester.WaitForSidecarSetMinReadyAndUpgrade(sidecarSet, exceptSidecarSetStatus, 2)

			ginkgo.By(fmt.Sprintf("check PodUnavailableBudget(%s/%s) Status", pub.Namespace, pub.Name))
			expectStatus = &policyv1alpha1.PodUnavailableBudgetStatus{
				UnavailableAllowed: 2,
				DesiredAvailable:   8,
				CurrentAvailable:   10,
				TotalReplicas:      10,
			}
			setPubStatus(expectStatus)
			gomega.Eventually(func() *policyv1alpha1.PodUnavailableBudgetStatus {
				pub, err = kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				nowStatus := &pub.Status
				setPubStatus(nowStatus)
				return nowStatus
			}, 30*time.Second, time.Second).Should(gomega.Equal(expectStatus))

			//check pods
			pods, err = sidecarTester.GetSelectorPods(cloneset.Namespace, cloneset.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			newPods := make([]corev1.Pod, 0)
			for _, pod := range pods {
				if pod.DeletionTimestamp.IsZero() && pod.Spec.Containers[1].Image == NewWebserverImage && pod.Spec.Containers[0].Image == NewNginxImage {
					newPods = append(newPods, *pod)
				}
			}
			gomega.Expect(newPods).To(gomega.HaveLen(10))
			ginkgo.By("PodUnavailableBudget selector cloneSet, update failed image and block done")
		})
	})
})

func setPubStatus(status *policyv1alpha1.PodUnavailableBudgetStatus) {
	status.DisruptedPods = nil
	status.UnavailablePods = nil
	status.ObservedGeneration = 0
}
