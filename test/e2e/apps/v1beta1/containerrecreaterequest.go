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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	frameworkv1beta1 "github.com/openkruise/kruise/test/e2e/framework/v1beta1"
)

var _ = ginkgo.Describe("ContainerRecreateRequest", ginkgo.Label("ContainerRecreateRequest", "v1beta1", "operation"), func() {
	f := frameworkv1beta1.NewDefaultFramework("containerrecreaterequests-v1beta1")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *frameworkv1beta1.ContainerRecreateTester
	var err error
	var pods []*v1.Pod
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = frameworkv1beta1.NewContainerRecreateTester(c, kc, ns)
		randStr = rand.String(10)
	})

	ginkgo.AfterEach(func() {
		err = tester.CleanAllTestResources()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("v1beta1 API", func() {

		ginkgo.It("recreates simple containers via v1beta1 client", func() {
			ginkgo.By("Create CloneSet and wait Pods ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 2, []v1.Container{
				{
					Name:  "app",
					Image: common.WebserverImage,
				},
				{
					Name:  "sidecar",
					Image: common.AgnhostImage,
				},
			})

			{
				ginkgo.By("Create v1beta1 CRR for pods[0], recreate container: app")
				pod := pods[0]
				crr := &appsv1beta1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-0"},
					Spec: appsv1beta1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1beta1.ContainerRecreateRequestContainer{
							{Name: "app"},
						},
						Strategy:                &appsv1beta1.ContainerRecreateRequestStrategy{MinStartedSeconds: 5},
						TTLSecondsAfterFinished: ptr.To[int32](3),
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Labels[appsv1beta1.ContainerRecreateRequestPodUIDKey]).Should(gomega.Equal(string(pod.UID)))
				gomega.Expect(crr.Labels[appsv1beta1.ContainerRecreateRequestNodeNameKey]).Should(gomega.Equal(pod.Spec.NodeName))
				gomega.Expect(crr.Labels[appsv1beta1.ContainerRecreateRequestActiveKey]).Should(gomega.Equal("true"))
				gomega.Expect(crr.Spec.Strategy.FailurePolicy).Should(gomega.Equal(appsv1beta1.ContainerRecreateRequestFailurePolicyFail))
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))

				ginkgo.By("Wait CRR recreate completion")
				crr = tester.WaitForCRRCompleted(crr.Name, 70*time.Second)
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1beta1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1beta1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1beta1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))

				ginkgo.By("Check Pod containers recreated and started for minStartedSeconds")
				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appCS := util.GetContainerStatus("app", pod)
				gomega.Expect(appCS.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[0].StatusContext.ContainerID))
				gomega.Expect(appCS.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(crr.Status.CompletionTime.Sub(appCS.State.Running.StartedAt.Time)).Should(gomega.BeNumerically(">", 4*time.Second))

				ginkgo.By("Wait CRR deleted by TTL")
				gomega.Eventually(func() bool {
					_, err = tester.GetCRR(crr.Name)
					return err != nil && errors.IsNotFound(err)
				}, 6*time.Second, 2*time.Second).Should(gomega.Equal(true))
			}

			{
				ginkgo.By("Create v1beta1 CRR for pods[1], recreate containers: app and sidecar")
				pod := pods[1]
				crr := &appsv1beta1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-1"},
					Spec: appsv1beta1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1beta1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))
				gomega.Expect(crr.Spec.Containers[1].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("sidecar", pod).ContainerID))

				crr = tester.WaitForCRRCompleted(crr.Name, 60*time.Second)
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1beta1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1beta1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1beta1.ContainerRecreateRequestSucceeded, IsKilled: true},
					{Name: "sidecar", Phase: appsv1beta1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))

				ginkgo.By("Check Pod containers recreated")
				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appCS := util.GetContainerStatus("app", pod)
				sidecarCS := util.GetContainerStatus("sidecar", pod)
				gomega.Expect(appCS.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[0].StatusContext.ContainerID))
				gomega.Expect(sidecarCS.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[1].StatusContext.ContainerID))
				gomega.Expect(appCS.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(sidecarCS.RestartCount).Should(gomega.Equal(int32(1)))
			}
		})

		ginkgo.It("v1beta1 status.containerStatusSnapshot is populated during recreating phase", func() {
			ginkgo.By("Create CloneSet and wait Pods ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 1, []v1.Container{
				{
					Name:  "app",
					Image: common.WebserverImage,
					Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{
						Exec: &v1.ExecAction{Command: []string{"sleep", "10"}},
					}},
				},
			})

			pod := pods[0]
			crr := &appsv1beta1.ContainerRecreateRequest{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-snap-" + randStr},
				Spec: appsv1beta1.ContainerRecreateRequestSpec{
					PodName:    pod.Name,
					Containers: []appsv1beta1.ContainerRecreateRequestContainer{{Name: "app"}},
				},
			}
			crr, err = tester.CreateCRR(crr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait until CRR enters Recreating phase and check containerStatusSnapshot")
			gomega.Eventually(func() int {
				crr, err = tester.GetCRR(crr.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if crr.Status.Phase == appsv1beta1.ContainerRecreateRequestRecreating {
					return len(crr.Status.ContainerStatusSnapshot)
				}
				return -1
			}, 30*time.Second, time.Second).Should(gomega.BeNumerically(">", 0))

			klog.Infof("CRR containerStatusSnapshot: %v", util.DumpJSON(crr.Status.ContainerStatusSnapshot))
			gomega.Expect(crr.Status.ContainerStatusSnapshot[0].Name).Should(gomega.Equal("app"))
			gomega.Expect(crr.Status.ContainerStatusSnapshot[0].ContainerID).ShouldNot(gomega.BeEmpty())

			ginkgo.By("Wait CRR completion")
			tester.WaitForCRRCompleted(crr.Name, 60*time.Second)
		})

		ginkgo.It("orderedRecreate works via v1beta1", func() {
			ginkgo.By("Create CloneSet and wait Pods ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 1, []v1.Container{
				{
					Name:  "app",
					Image: common.WebserverImage,
					Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{
						Exec: &v1.ExecAction{Command: []string{"sleep", "5"}},
					}},
				},
				{
					Name:  "sidecar",
					Image: common.AgnhostImage,
				},
			})

			pod := pods[0]
			crr := &appsv1beta1.ContainerRecreateRequest{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-ordered-" + randStr},
				Spec: appsv1beta1.ContainerRecreateRequestSpec{
					PodName: pod.Name,
					Containers: []appsv1beta1.ContainerRecreateRequestContainer{
						{Name: "app"},
						{Name: "sidecar"},
					},
					Strategy: &appsv1beta1.ContainerRecreateRequestStrategy{OrderedRecreate: true},
				},
			}
			crr, err = tester.CreateCRR(crr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			crr = tester.WaitForCRRCompleted(crr.Name, 90*time.Second)
			gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1beta1.ContainerRecreateRequestContainerRecreateState{
				{Name: "app", Phase: appsv1beta1.ContainerRecreateRequestSucceeded, IsKilled: true},
				{Name: "sidecar", Phase: appsv1beta1.ContainerRecreateRequestSucceeded, IsKilled: true},
			}))

			ginkgo.By("Check sidecar recreated after app was ready")
			pod, err = tester.GetPod(pod.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			appCS := util.GetContainerStatus("app", pod)
			sidecarCS := util.GetContainerStatus("sidecar", pod)
			gomega.Expect(sidecarCS.LastTerminationState.Terminated).ShouldNot(gomega.BeNil())
			interval := sidecarCS.LastTerminationState.Terminated.FinishedAt.Sub(appCS.LastTerminationState.Terminated.FinishedAt.Time)
			gomega.Expect(interval >= 5*time.Second).Should(gomega.Equal(true))
		})

		ginkgo.It("serves v1alpha1-created status as v1beta1 typed fields", func() {
			ginkgo.By("Create CloneSet and wait Pods ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 1, []v1.Container{
				{
					Name:  "app",
					Image: common.WebserverImage,
					Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{
						Exec: &v1.ExecAction{Command: []string{"sleep", "8"}},
					}},
				},
				{
					Name:  "sidecar",
					Image: common.AgnhostImage,
					Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{
						Exec: &v1.ExecAction{Command: []string{"sleep", "8"}},
					}},
				},
			})

			pod := pods[0]
			crrName := fmt.Sprintf("crr-xver-%s", randStr)
			crr := &appsv1alpha1.ContainerRecreateRequest{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crrName},
				Spec: appsv1alpha1.ContainerRecreateRequestSpec{
					PodName: pod.Name,
					Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
						{Name: "app"},
						{Name: "sidecar"},
					},
					Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
						UnreadyGracePeriodSeconds: ptr.To(int64(1)),
					},
				},
			}
			_, err = kc.AppsV1alpha1().ContainerRecreateRequests(ns).Create(context.TODO(), crr, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Verify v1beta1 status exposes typed containerStatusSnapshot and PodUnreadyAcquired condition")
			gomega.Eventually(func() bool {
				fetched, err2 := tester.GetCRR(crrName)
				gomega.Expect(err2).NotTo(gomega.HaveOccurred())
				if fetched.Status.Phase != appsv1beta1.ContainerRecreateRequestRecreating ||
					len(fetched.Status.ContainerStatusSnapshot) == 0 {
					return false
				}
				for _, condition := range fetched.Status.Conditions {
					if condition.Type == appsv1beta1.ContainerRecreateRequestPodUnreadyAcquiredType &&
						condition.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, 60*time.Second, time.Second).Should(gomega.Equal(true))

			ginkgo.By("Verify legacy status annotations are absent on v1beta1 object")
			fetched, err := tester.GetCRR(crrName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(fetched.Annotations[appsv1beta1.ContainerRecreateRequestSyncContainerStatusesKey]).Should(gomega.Equal(""))
			gomega.Expect(fetched.Annotations[appsv1beta1.ContainerRecreateRequestUnreadyAcquiredKey]).Should(gomega.Equal(""))

			tester.WaitForCRRCompleted(crrName, 60*time.Second)
		})

	})
})
