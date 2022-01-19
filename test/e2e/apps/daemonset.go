package apps

import (
	"context"
	"fmt"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = SIGDescribe("DaemonSet", func() {
	f := framework.NewDefaultFramework("daemonset")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.DaemonSetTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewDaemonSetTester(c, kc, ns)
	})

	framework.KruiseDescribe("Basic DaemonSet functionality [DaemonSetBasic]", func() {
		dsName := "e2e-ds"

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting DaemonSet %s.%s in cluster", ns, dsName)
			tester.DeleteDaemonSet(ns, dsName)
		})

		/*
		  Testname: DaemonSet-Creation
		  Description: A conformant Kubernetes distribution MUST support the creation of DaemonSets. When a DaemonSet
		  Pod is deleted, the DaemonSet controller MUST create a replacement Pod.
		*/
		framework.ConformanceIt("should run and stop simple daemon", func() {
			label := map[string]string{framework.DaemonSetNameLabel: dsName}

			ginkgo.By(fmt.Sprintf("Creating simple DaemonSet %q", dsName))
			ds, err := tester.CreateDaemonSet(tester.NewDaemonSet(dsName, label, WebserverImage, appsv1alpha1.DaemonSetUpdateStrategy{}))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check that daemon pods launch on every node of the cluster.")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))

			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to start")
			err = tester.CheckDaemonStatus(dsName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Stop a daemon pod, check that the daemon pod is revived.")
			podList, err := tester.ListDaemonPods(label)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(podList.Items)).To(gomega.BeNumerically(">", 0))
			pod := podList.Items[0]

			err = c.CoreV1().Pods(ns).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to revive")
		})

		/*
		  Testname: DaemonSet-NodeSelection
		  Description: A conformant Kubernetes distribution MUST support DaemonSet Pod node selection via label
		  selectors.
		*/
		framework.ConformanceIt("should run and stop complex daemon", func() {
			complexLabel := map[string]string{framework.DaemonSetNameLabel: dsName}
			nodeSelector := map[string]string{framework.DaemonSetColorLabel: "blue"}
			framework.Logf("Creating daemon %q with a node selector", dsName)
			ds := tester.NewDaemonSet(dsName, complexLabel, WebserverImage, appsv1alpha1.DaemonSetUpdateStrategy{})
			ds.Spec.Template.Spec.NodeSelector = nodeSelector
			ds, err := tester.CreateDaemonSet(ds)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Initially, daemon pods should not be running on any nodes.")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnNoNodes(ds))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pods to be running on no nodes")

			ginkgo.By("Change node label to blue, check that daemon pod is launched.")
			nodeList := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
			gomega.Expect(len(nodeList.Items)).To(gomega.BeNumerically(">", 0))
			newNode, err := tester.SetDaemonSetNodeLabels(nodeList.Items[0].Name, nodeSelector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error setting labels on node")
			daemonSetLabels, _ := tester.SeparateDaemonSetNodeLabels(newNode.Labels)
			gomega.Expect(len(daemonSetLabels)).To(gomega.Equal(1))
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckDaemonPodOnNodes(ds, []string{newNode.Name}))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pods to be running on new nodes")
			err = tester.CheckDaemonStatus(dsName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Update the node label to green, and wait for daemons to be unscheduled")
			nodeSelector[framework.DaemonSetColorLabel] = "green"
			greenNode, err := tester.SetDaemonSetNodeLabels(nodeList.Items[0].Name, nodeSelector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error removing labels on node")
			gomega.Expect(wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnNoNodes(ds))).
				NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to not be running on nodes")

			ginkgo.By("Update DaemonSet node selector to green, and change its update strategy to RollingUpdate")
			patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"nodeSelector":{"%s":"%s"}}},"updateStrategy":{"type":"RollingUpdate"}}}`,
				framework.DaemonSetColorLabel, greenNode.Labels[framework.DaemonSetColorLabel])
			ds, err = tester.PatchDaemonSet(dsName, types.MergePatchType, []byte(patch))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error patching daemon set")
			daemonSetLabels, _ = tester.SeparateDaemonSetNodeLabels(greenNode.Labels)
			gomega.Expect(len(daemonSetLabels)).To(gomega.Equal(1))
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckDaemonPodOnNodes(ds, []string{greenNode.Name}))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pods to be running on new nodes")
			err = tester.CheckDaemonStatus(dsName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		/*
		  Testname: DaemonSet-FailedPodCreation
		  Description: A conformant Kubernetes distribution MUST create new DaemonSet Pods when they fail.
		*/
		framework.ConformanceIt("should retry creating failed daemon pods", func() {
			label := map[string]string{framework.DaemonSetNameLabel: dsName}

			ginkgo.By(fmt.Sprintf("Creating a simple DaemonSet %q", dsName))
			ds, err := tester.CreateDaemonSet(tester.NewDaemonSet(dsName, label, WebserverImage, appsv1alpha1.DaemonSetUpdateStrategy{}))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check that daemon pods launch on every node of the cluster.")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to start")
			err = tester.CheckDaemonStatus(dsName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Set a daemon pod's phase to 'Failed', check that the daemon pod is revived.")
			podList, err := tester.ListDaemonPods(label)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			pod := podList.Items[0]
			pod.ResourceVersion = ""
			pod.Status.Phase = v1.PodFailed
			_, err = c.CoreV1().Pods(ns).UpdateStatus(context.TODO(), &pod, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error failing a daemon pod")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to revive")

			ginkgo.By("Wait for the failed daemon pod to be completely deleted.")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.WaitFailedDaemonPodDeleted(&pod))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for the failed daemon pod to be completely deleted")
		})

		framework.ConformanceIt("should only inplace image if update daemonset image with inplace update strategy", func() {
			label := map[string]string{framework.DaemonSetNameLabel: dsName}

			ginkgo.By(fmt.Sprintf("Creating simple DaemonSet %q", dsName))
			maxUnavailable := intstr.IntOrString{IntVal: int32(5)}
			ds, err := tester.CreateDaemonSet(tester.NewDaemonSet(dsName, label, WebserverImage, appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					Type:           appsv1alpha1.InplaceRollingUpdateType,
					MaxUnavailable: &maxUnavailable,
				},
			}))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check that daemon pods launch on every node of the cluster.")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))

			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to start")
			err = tester.CheckDaemonStatus(dsName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Get all old daemon node")
			oldNodeList := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
			gomega.Expect(len(oldNodeList.Items)).To(gomega.BeNumerically(">", 0))

			ginkgo.By("Get all old daemon pods")
			oldPodList, err := tester.ListDaemonPods(label)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(oldPodList.Items)).To(gomega.BeNumerically(">", 0))

			//change pods container image
			err = tester.UpdateDaemonSet(ds.Name, func(ds *appsv1alpha1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error to update daemon")

			ginkgo.By("Compare container info")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.GetNewPodsToCheckImage(label, NewWebserverImage))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error for pod image")

			ginkgo.By("Get all New daemon node")
			newNodeList := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
			gomega.Expect(len(newNodeList.Items)).To(gomega.BeNumerically(">", 0))

			ginkgo.By("Compare Node info")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckPodStayInNode(oldNodeList, newNodeList))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error for node info")

			ginkgo.By("Get all new daemon pods")
			newPodList, err := tester.ListDaemonPods(label)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(newPodList.Items)).To(gomega.BeNumerically(">", 0))

			ginkgo.By("All pods should have readiness gate")
			gomega.Expect(tester.DaemonPodHasReadinessGate(newPodList.Items)).Should(gomega.Equal(true))

			ginkgo.By("No pod recreate during updating")
			gomega.Expect(tester.CheckPodHasNotRecreate(oldPodList.Items, newPodList.Items)).Should(gomega.Equal(true))
		})

		framework.ConformanceIt("Should upgrade inplace if update image and recreate if update others", func() {
			label := map[string]string{framework.DaemonSetNameLabel: dsName}

			cases := []struct {
				updateFn       func(ds *appsv1alpha1.DaemonSet)
				expectRecreate bool
			}{
				{
					updateFn: func(ds *appsv1alpha1.DaemonSet) {
						ds.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
					},
					expectRecreate: true,
				},
				{
					updateFn: func(ds *appsv1alpha1.DaemonSet) {
						ds.Spec.Template.Spec.Containers[0].Env = []v1.EnvVar{
							{Name: "foo", Value: "bar"},
						}
					},
					expectRecreate: false,
				},
			}

			ginkgo.By(fmt.Sprintf("Creating simple DaemonSet %q", dsName))
			maxUnavailable := intstr.IntOrString{IntVal: int32(20)}
			ds, err := tester.CreateDaemonSet(tester.NewDaemonSet(dsName, label, WebserverImage, appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					Type:           appsv1alpha1.InplaceRollingUpdateType,
					MaxUnavailable: &maxUnavailable,
				},
			}))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check that daemon pods launch on every node of the cluster")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))

			for _, tc := range cases {
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to start")
				err = tester.CheckDaemonStatus(dsName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				ginkgo.By("Get all old daemon node")
				oldNodeList := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
				gomega.Expect(len(oldNodeList.Items)).To(gomega.BeNumerically(">", 0))

				ginkgo.By("Get all old daemon pods")
				oldPodList, err := tester.ListDaemonPods(label)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(len(oldPodList.Items)).To(gomega.BeNumerically(">", 0))

				// update daemonset spec
				ginkgo.By("Update daemonset spec")
				err = tester.UpdateDaemonSet(ds.Name, tc.updateFn)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error to update daemon")

				// sleep for a while make sure
				time.Sleep(5 * time.Second)

				ginkgo.By("Compare container info")
				err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.GetNewPodsToCheckImage(label, NewWebserverImage))
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error for pod image")

				ginkgo.By("Get all New daemon node")
				newNodeList := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
				gomega.Expect(len(newNodeList.Items)).To(gomega.BeNumerically(">", 0))

				ginkgo.By("Compare Node info")
				err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckPodStayInNode(oldNodeList, newNodeList))
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error for node info")

				ginkgo.By("Wait for daemonset ready")
				err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckDaemonReady(dsName))

				ginkgo.By("Get all new daemon pods")
				newPodList, err := tester.ListDaemonPods(label)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(len(newPodList.Items)).To(gomega.BeNumerically(">", 0))

				ginkgo.By("All pods should have readiness gate")
				gomega.Expect(tester.DaemonPodHasReadinessGate(newPodList.Items)).Should(gomega.Equal(true))

				ginkgo.By(fmt.Sprintf("No pod recreate during updating, expect %v", tc.expectRecreate))
				gomega.Expect(tester.CheckPodHasNotRecreate(oldPodList.Items, newPodList.Items)).Should(gomega.Equal(tc.expectRecreate))
			}
		})
	})
})
