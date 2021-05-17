package apps

import (
	"fmt"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
			ds, err := tester.CreateDaemonSet(tester.NewDaemonSet(dsName, label, framework.OldImage, appsv1alpha1.DaemonSetUpdateStrategy{}))
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

			err = c.CoreV1().Pods(ns).Delete(pod.Name, nil)
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
			ds := tester.NewDaemonSet(dsName, complexLabel, framework.OldImage, appsv1alpha1.DaemonSetUpdateStrategy{})
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
			ds, err := tester.CreateDaemonSet(tester.NewDaemonSet(dsName, label, framework.OldImage, appsv1alpha1.DaemonSetUpdateStrategy{}))
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
			_, err = c.CoreV1().Pods(ns).UpdateStatus(&pod)
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error failing a daemon pod")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to revive")

			ginkgo.By("Wait for the failed daemon pod to be completely deleted.")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.WaitFailedDaemonPodDeleted(&pod))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for the failed daemon pod to be completely deleted")
		})
	})
})
