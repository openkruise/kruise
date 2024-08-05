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
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	utilpodreadiness "github.com/openkruise/kruise/pkg/util/podreadiness"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	utilpointer "k8s.io/utils/pointer"
)

var _ = SIGDescribe("ContainerRecreateRequest", func() {
	f := framework.NewDefaultFramework("containerrecreaterequests")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.ContainerRecreateTester
	var err error
	var pods []*v1.Pod
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewContainerRecreateTester(c, kc, ns)
		randStr = rand.String(10)
	})

	ginkgo.AfterEach(func() {
		err = tester.CleanAllTestResources()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	framework.KruiseDescribe("ContainerRecreateRequest", func() {

		framework.ConformanceIt("recreates simple containers", func() {

			ginkgo.By("Create CloneSet and wait Pods ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 2, []v1.Container{
				{
					Name:  "app",
					Image: WebserverImage,
				},
				{
					Name:  "sidecar",
					Image: AgnhostImage,
				},
			})

			{
				ginkgo.By("Create CRR for pods[0], recreate container: app")
				pod := pods[0]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-0"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
						},
						Strategy:                &appsv1alpha1.ContainerRecreateRequestStrategy{MinStartedSeconds: 5},
						TTLSecondsAfterFinished: utilpointer.Int32Ptr(3),
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey]).Should(gomega.Equal(string(pod.UID)))
				gomega.Expect(crr.Labels[appsv1alpha1.ContainerRecreateRequestNodeNameKey]).Should(gomega.Equal(pod.Spec.NodeName))
				gomega.Expect(crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]).Should(gomega.Equal("true"))
				gomega.Expect(crr.Spec.Strategy.FailurePolicy).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestFailurePolicyFail))
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))
				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 70*time.Second, time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, 1*time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true}}))

				ginkgo.By("Check Pod containers recreated and started for minStartedSeconds")
				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appContainerStatus := util.GetContainerStatus("app", pod)
				gomega.Expect(appContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[0].StatusContext.ContainerID))
				gomega.Expect(appContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(crr.Status.CompletionTime.Sub(appContainerStatus.State.Running.StartedAt.Time)).Should(gomega.BeNumerically(">", 4*time.Second))

				ginkgo.By("Wait CRR deleted by TTL")
				gomega.Eventually(func() bool {
					_, err = tester.GetCRR(crr.Name)
					return err != nil && errors.IsNotFound(err)
				}, 6*time.Second, 2*time.Second).Should(gomega.Equal(true))
			}

			{
				ginkgo.By("Create CRR for pods[1], recreate container: app and sidecar")
				pod := pods[1]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-1"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))
				gomega.Expect(crr.Spec.Containers[1].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("sidecar", pod).ContainerID))

				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 60*time.Second, 3*time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, 1*time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))

				ginkgo.By("Check Pod containers recreated")
				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appContainerStatus := util.GetContainerStatus("app", pod)
				sidecarContainerStatus := util.GetContainerStatus("sidecar", pod)
				gomega.Expect(appContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[0].StatusContext.ContainerID))
				gomega.Expect(sidecarContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[1].StatusContext.ContainerID))
				gomega.Expect(appContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(sidecarContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
			}
		})

		framework.ConformanceIt("recreates containers with postStartHook", func() {

			ginkgo.By("Create CloneSet and wait Pods ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 2, []v1.Container{
				{
					Name:  "app",
					Image: WebserverImage,
					Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{
						Exec: &v1.ExecAction{Command: []string{"sleep", "5"}},
					}},
				},
				{
					Name:  "sidecar",
					Image: AgnhostImage,
				},
			})
			time.Sleep(time.Second * 3)

			{
				ginkgo.By("Create CRR for pods[0], recreate container: app(postStartHook) and sidecar")
				pod := pods[0]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-0"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))
				gomega.Expect(crr.Spec.Containers[1].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("sidecar", pod).ContainerID))

				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 60*time.Second, 3*time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				klog.Infof("CRR info(%s)", util.DumpJSON(crr))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, time.Second).Should(gomega.Equal(""))

				ginkgo.By("Check Pod containers recreated")
				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appContainerStatus := util.GetContainerStatus("app", pod)
				sidecarContainerStatus := util.GetContainerStatus("sidecar", pod)
				gomega.Expect(appContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[0].StatusContext.ContainerID))
				gomega.Expect(sidecarContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[1].StatusContext.ContainerID))
				gomega.Expect(appContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(sidecarContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))

				ginkgo.By("Check Pod sidecar container recreated not waiting for app container ready")
				interval := sidecarContainerStatus.LastTerminationState.Terminated.FinishedAt.Sub(appContainerStatus.LastTerminationState.Terminated.FinishedAt.Time)
				gomega.Expect(interval < 3*time.Second).Should(gomega.Equal(true))
			}

			{
				ginkgo.By("Create CRR for pods[1] with orderedRecreate, recreate container: app(postStartHook) and sidecar")
				pod := pods[1]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-1"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
						Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
							OrderedRecreate: true,
						},
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))
				gomega.Expect(crr.Spec.Containers[1].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("sidecar", pod).ContainerID))

				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 60*time.Second, 3*time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, 1*time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))

				ginkgo.By("Check Pod containers recreated")
				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appContainerStatus := util.GetContainerStatus("app", pod)
				sidecarContainerStatus := util.GetContainerStatus("sidecar", pod)
				gomega.Expect(appContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[0].StatusContext.ContainerID))
				gomega.Expect(sidecarContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[1].StatusContext.ContainerID))
				gomega.Expect(appContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(sidecarContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))

				ginkgo.By("Check Pod sidecar container recreated after app container ready")
				interval := sidecarContainerStatus.LastTerminationState.Terminated.FinishedAt.Sub(appContainerStatus.LastTerminationState.Terminated.FinishedAt.Time)
				gomega.Expect(interval >= 5*time.Second).Should(gomega.Equal(true))
			}

		})

		framework.ConformanceIt("recreates containers with preStopHook", func() {

			ginkgo.By("Create CloneSet and wait Pods ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 3, []v1.Container{
				{
					Name:  "app",
					Image: WebserverImage,
					Lifecycle: &v1.Lifecycle{PreStop: &v1.LifecycleHandler{
						Exec: &v1.ExecAction{Command: []string{"sleep", "8"}},
					}},
				},
				{
					Name:  "sidecar",
					Image: AgnhostImage,
				},
			})

			{
				ginkgo.By("Create CRR for pods[0], recreate container: app(preStopHook) and sidecar")
				pod := pods[0]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-0"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))
				gomega.Expect(crr.Spec.Containers[1].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("sidecar", pod).ContainerID))

				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 60*time.Second, 3*time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, 1*time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))

				ginkgo.By("Check Pod containers recreated")
				gomega.Eventually(func() bool {
					pod, err = tester.GetPod(pod.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					appContainerStatus := util.GetContainerStatus("app", pod)
					sidecarContainerStatus := util.GetContainerStatus("sidecar", pod)
					return appContainerStatus.ContainerID != crr.Spec.Containers[0].StatusContext.ContainerID &&
						sidecarContainerStatus.ContainerID != crr.Spec.Containers[1].StatusContext.ContainerID
				}, 60*time.Second, 3*time.Second).Should(gomega.BeTrue())

				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appContainerStatus := util.GetContainerStatus("app", pod)
				sidecarContainerStatus := util.GetContainerStatus("sidecar", pod)
				gomega.Expect(appContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(sidecarContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(sidecarContainerStatus.LastTerminationState.Terminated).ShouldNot(gomega.BeNil())
				ginkgo.By("Check Pod app container stopped after preStop")
				interval := sidecarContainerStatus.LastTerminationState.Terminated.FinishedAt.Sub(crr.CreationTimestamp.Time)
				gomega.Expect(interval >= 8*time.Second).Should(gomega.Equal(true))
			}

			{
				ginkgo.By("Create CRR for pods[1] with activeDeadlineSeconds=3, recreate container: app(preStopHook) and sidecar")
				pod := pods[1]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-1"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
						ActiveDeadlineSeconds: utilpointer.Int64Ptr(3),
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))
				gomega.Expect(crr.Spec.Containers[1].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("sidecar", pod).ContainerID))

				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 5*time.Second, 1*time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, 1*time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestPending},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestPending},
				}))

				ginkgo.By("Check the app container should still be recreated")
				gomega.Eventually(func() int32 {
					pod, err = tester.GetPod(pod.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return util.GetContainerStatus("app", pod).RestartCount
				}, 60*time.Second, 1*time.Second).Should(gomega.Equal(int32(1)))
				gomega.Eventually(func() bool {
					pod, err = tester.GetPod(pod.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return podutil.IsPodReady(pod)
				}, 10*time.Second, 1*time.Second).Should(gomega.Equal(true))

				crr, err = tester.GetCRR(crr.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestRecreating, IsKilled: true},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestPending},
				}))
			}

			{
				ginkgo.By("Create CRR for pods[2] with unreadyGracePeriodSeconds=true, recreate container: app(preStopHook) and sidecar")
				pod := pods[2]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-2"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
						Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
							TerminationGracePeriodSeconds: utilpointer.Int64Ptr(3),
							UnreadyGracePeriodSeconds:     utilpointer.Int64Ptr(1),
						},
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				ginkgo.By("Check Pod not-ready and Kruise readiness condition message contains CRR")
				gomega.Eventually(func() *v1.PodCondition {
					pod, err = tester.GetPod(pod.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					condition := utilpodreadiness.GetReadinessCondition(pod)
					if condition == nil {
						return nil
					}
					return &v1.PodCondition{
						Status:  condition.Status,
						Message: condition.Message,
					}
				}, 2*time.Second, 10*time.Millisecond).Should(gomega.Equal(&v1.PodCondition{
					Status:  v1.ConditionFalse,
					Message: fmt.Sprintf(`[{"userAgent":"ContainerRecreateRequest","key":"%s/%s"}]`, ns, crr.Name),
				}))

				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 60*time.Second, time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))

				ginkgo.By("Check Kruise readiness condition True")
				gomega.Eventually(func() v1.ConditionStatus {
					pod, err = tester.GetPod(pod.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					condition := utilpodreadiness.GetReadinessCondition(pod)
					return condition.Status
				}, 3*time.Second, time.Second).Should(gomega.Equal(v1.ConditionTrue))
				gomega.Eventually(func() bool {
					pod, err = tester.GetPod(pod.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return podutil.IsPodReady(pod)
				}, 10*time.Second, time.Second).Should(gomega.Equal(true))

				ginkgo.By("Check the app container stopped before preStop finished")
				interval := util.GetContainerStatus("app", pod).LastTerminationState.Terminated.FinishedAt.Sub(crr.CreationTimestamp.Time)
				gomega.Expect(interval < 8*time.Second).Should(gomega.Equal(true))
			}

		})

		framework.ConformanceIt("recreates containers by force", func() {
			ginkgo.By("Create CloneSet and wait Pods ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 2, []v1.Container{
				{
					Name:  "app",
					Image: WebserverImage,
					Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{
						Exec: &v1.ExecAction{Command: []string{"sleep", "5"}},
					}},
				},
				{
					Name:  "sidecar",
					Image: AgnhostImage,
				},
			})

			{
				ginkgo.By("Create CRR for pods[0], recreate container: app(postStartHook) and sidecar by force")
				pod := pods[0]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-0"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
						Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
							ForceRecreate: true,
						},
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))

				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 60*time.Second, 3*time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, time.Second).Should(gomega.Equal(""))

				ginkgo.By("Check Pod containers recreated")
				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appContainerStatus := util.GetContainerStatus("app", pod)
				sidecarContainerStatus := util.GetContainerStatus("sidecar", pod)
				gomega.Expect(sidecarContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[1].StatusContext.ContainerID))
				gomega.Expect(appContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(sidecarContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))

				ginkgo.By("Check Pod sidecar container recreated not waiting for app container ready")
				interval := sidecarContainerStatus.LastTerminationState.Terminated.FinishedAt.Sub(appContainerStatus.LastTerminationState.Terminated.FinishedAt.Time)
				gomega.Expect(interval < 3*time.Second).Should(gomega.Equal(true))
			}

			{
				ginkgo.By("Create CRR for pods[1] with orderedRecreate by force, recreate container: app(postStartHook) and sidecar")
				pod := pods[1]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-1"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "app"},
							{Name: "sidecar"},
						},
						Strategy: &appsv1alpha1.ContainerRecreateRequestStrategy{
							OrderedRecreate: true,
							ForceRecreate:   true,
						},
					},
				}
				crr, err = tester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("app", pod).ContainerID))
				gomega.Expect(crr.Spec.Containers[1].StatusContext.ContainerID).Should(gomega.Equal(util.GetContainerStatus("sidecar", pod).ContainerID))

				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 60*time.Second, 3*time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = tester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, 1*time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{
					{Name: "app", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
					{Name: "sidecar", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true},
				}))

				ginkgo.By("Check Pod containers recreated")
				pod, err = tester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				appContainerStatus := util.GetContainerStatus("app", pod)
				sidecarContainerStatus := util.GetContainerStatus("sidecar", pod)
				gomega.Expect(appContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[0].StatusContext.ContainerID))
				gomega.Expect(sidecarContainerStatus.ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[1].StatusContext.ContainerID))
				gomega.Expect(appContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(sidecarContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))

				ginkgo.By("Check Pod sidecar container recreated after app container ready")
				interval := sidecarContainerStatus.LastTerminationState.Terminated.FinishedAt.Sub(appContainerStatus.LastTerminationState.Terminated.FinishedAt.Time)
				gomega.Expect(interval >= 5*time.Second).Should(gomega.Equal(true))
			}
		})
	})
})
