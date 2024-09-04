package apps

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	daemonutil "k8s.io/kubernetes/pkg/controller/daemon/util"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/test/e2e/framework"
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
			framework.Logf("Deleting DaemonSet %s/%s in cluster", ns, dsName)
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

		framework.ConformanceIt("should upgrade one by one on steps if there is pre-delete hook", func() {
			label := map[string]string{framework.DaemonSetNameLabel: dsName}
			hookKey := "my-pre-delete"

			ginkgo.By(fmt.Sprintf("Creating DaemonSet %q with pre-delete hook", dsName))
			maxUnavailable := intstr.IntOrString{IntVal: int32(1)}
			ads := tester.NewDaemonSet(dsName, label, WebserverImage, appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					Type:           appsv1alpha1.InplaceRollingUpdateType,
					MaxUnavailable: &maxUnavailable,
				},
			})
			ads.Spec.Lifecycle = &appspub.Lifecycle{PreDelete: &appspub.LifecycleHook{LabelsHandler: map[string]string{hookKey: "true"}}}
			ads.Spec.Template.Labels = map[string]string{framework.DaemonSetNameLabel: dsName, hookKey: "true"}
			ads.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceCPU: resource.MustParse("100m"),
				},
			}
			ds, err := tester.CreateDaemonSet(ads)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check that daemon pods launch on every node of the cluster")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to start")

			err = tester.CheckDaemonStatus(dsName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			oldPodList, err := tester.ListDaemonPods(label)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Update daemonset resources")
			err = tester.UpdateDaemonSet(ds.Name, func(ads *appsv1alpha1.DaemonSet) {
				ads.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU: resource.MustParse("120m"),
					},
				}
				// when enable InPlaceWorkloadVerticalScaling feature, just resize request will not delete pod
				ads.Spec.Template.Spec.Containers[0].Env = append(ads.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{Name: "test", Value: "test"})
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error to update daemon")

			var newHash string
			gomega.Eventually(func() int64 {
				ads, err = tester.GetDaemonSet(dsName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				newHash = ads.Status.DaemonSetHash
				return ads.Status.ObservedGeneration
			}, time.Second*30, time.Second*3).Should(gomega.Equal(int64(2)))

			ginkgo.By("There should be one pod with PreparingDelete and no pods been deleted")
			var preDeletingPod *v1.Pod
			var podList *v1.PodList
			gomega.Eventually(func() int {
				podList, err = tester.ListDaemonPods(label)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				var preDeletingCount int
				for i := range podList.Items {
					pod := &podList.Items[i]
					if lifecycle.GetPodLifecycleState(pod) == appspub.LifecycleStatePreparingDelete {
						preDeletingCount++
						preDeletingPod = pod
					}
				}
				return preDeletingCount
			}, time.Second*30, time.Second*3).Should(gomega.Equal(1))

			gomega.Expect(tester.SortPodNames(podList)).To(gomega.Equal(tester.SortPodNames(oldPodList)))

			ginkgo.By("Remove the hook label and wait it to be recreated")
			patch := fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`, hookKey)
			_, err = tester.PatchPod(preDeletingPod.Name, types.StrategicMergePatchType, []byte(patch))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			var getErr error
			gomega.Eventually(func() bool {
				_, getErr = tester.GetPod(preDeletingPod.Name)
				return errors.IsNotFound(getErr)
			}, time.Second*60, time.Second).Should(gomega.Equal(true), fmt.Sprintf("get error %v", getErr))

			gomega.Eventually(func() int {
				podList, err = tester.ListDaemonPods(label)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				var newVersionCount int
				for i := range podList.Items {
					pod := &podList.Items[i]
					if daemonutil.IsPodUpdated(pod, newHash, nil) {
						newVersionCount++
					} else if lifecycle.GetPodLifecycleState(pod) == appspub.LifecycleStatePreparingDelete {
						preDeletingPod = pod
					}
				}
				return newVersionCount
			}, time.Second*60, time.Second).Should(gomega.Equal(1))

		})

		framework.ConformanceIt("should successfully surging update daemonset with minReadySeconds", func() {
			label := map[string]string{framework.DaemonSetNameLabel: dsName}

			ginkgo.By(fmt.Sprintf("Creating DaemonSet %q", dsName))
			maxSurge := intstr.FromString("100%")
			maxUnavailable := intstr.FromInt(0)
			ds := tester.NewDaemonSet(dsName, label, WebserverImage, appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			})
			ds.Spec.MinReadySeconds = 10
			ds, err := tester.CreateDaemonSet(ds)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check that daemon pods launch on every node of the cluster.")
			err = wait.PollImmediate(framework.DaemonSetRetryPeriod, framework.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))

			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to start")
			err = tester.CheckDaemonStatus(dsName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ds, err = tester.GetDaemonSet(dsName)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Get all old daemon pods on nodes")
			oldNodeToDaemonPods, err := tester.GetNodesToDaemonPods(label)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(oldNodeToDaemonPods).To(gomega.HaveLen(int(ds.Status.DesiredNumberScheduled)))

			nodeNameList := sets.NewString()
			for nodeName, pods := range oldNodeToDaemonPods {
				nodeNameList.Insert(nodeName)
				gomega.Expect(pods).To(gomega.HaveLen(1))
				gomega.Expect(podutil.IsPodReady(pods[0])).To(gomega.BeTrue())
			}

			//change pods container image
			err = tester.UpdateDaemonSet(ds.Name, func(ds *appsv1alpha1.DaemonSet) {
				ds.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error to update daemon")

			ginkgo.By("Check all surging Pods created")
			err = wait.PollImmediate(time.Second, time.Minute, func() (done bool, err error) {
				nodeToDaemonPods, err := tester.GetNodesToDaemonPods(label)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(nodeToDaemonPods).To(gomega.HaveLen(int(ds.Status.DesiredNumberScheduled)))

				for _, nodeName := range nodeNameList.List() {
					pods := nodeToDaemonPods[nodeName]
					if len(pods) < 2 {
						continue
					}

					for _, pod := range pods {
						if pod.Name == oldNodeToDaemonPods[nodeName][0].Name {
							gomega.Expect(pod.DeletionTimestamp).To(gomega.BeNil())
						}
					}
					nodeNameList.Delete(nodeName)
				}
				return nodeNameList.Len() == 0, nil
			})

			ginkgo.By("Check all old Pods deleted")
			err = wait.PollImmediate(time.Second, time.Minute, func() (done bool, err error) {
				nodeToDaemonPods, err := tester.GetNodesToDaemonPods(label)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(nodeToDaemonPods).To(gomega.HaveLen(int(ds.Status.DesiredNumberScheduled)))

				finished := true
				for nodeName, pods := range nodeToDaemonPods {
					if len(pods) != 1 {
						finished = false
						continue
					}

					gomega.Expect(pods[0].Name).NotTo(gomega.Equal(oldNodeToDaemonPods[nodeName][0].Name))
					gomega.Expect(podutil.IsPodReady(pods[0])).To(gomega.BeTrue())
					c := podutil.GetPodReadyCondition(pods[0].Status)
					gomega.Expect(int32(time.Since(c.LastTransitionTime.Time) / time.Second)).To(gomega.BeNumerically(">", ds.Spec.MinReadySeconds))
				}
				return finished, nil
			})
		})
	})
})
