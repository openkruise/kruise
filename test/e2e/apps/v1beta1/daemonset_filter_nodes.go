package v1beta1

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/utils/pointer"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	"github.com/openkruise/kruise/test/e2e/framework/v1beta1"
)

var _ = ginkgo.Describe("DaemonSet", ginkgo.Label("DaemonSet", "workload"), func() {
	f := v1beta1.NewDefaultFramework("daemonset")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *v1beta1.DaemonSetTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = v1beta1.NewDaemonSetTester(c, kc, ns)
	})

	ginkgo.Context("Basic DaemonSet functionality [DaemonSetBasic]", func() {
		dsName := "e2e-ds"

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentSpecReport().Failed() {
				common.DumpDebugInfo(c, ns)
			}
			common.Logf("Deleting DaemonSet %s/%s in cluster", ns, dsName)
			tester.DeleteDaemonSet(ns, dsName)
		})

		ginkgo.It("should ignore not ready nodes during rolling update when annotation is set", ginkgo.Serial, func() {
			// This test performs destructive cluster-level operations (docker rm nodes)
			// It must run serially to avoid interfering with other tests
			label := map[string]string{v1beta1.DaemonSetNameLabel: dsName}

			ginkgo.By(fmt.Sprintf("Creating DaemonSet %s with ignore-notready-nodes annotation", dsName))
			maxUnavailable := intstr.IntOrString{IntVal: int32(2)}
			ds := tester.NewDaemonSet(dsName, label, common.WebserverImage, appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			})

			ds.Spec.Template.Spec.TerminationGracePeriodSeconds = pointer.Int64Ptr(5)
			ds, err := tester.CreateDaemonSet(ds)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check that daemon pods launch on every node of the cluster.")
			err = wait.PollImmediate(v1beta1.DaemonSetRetryPeriod, v1beta1.DaemonSetRetryTimeout, tester.CheckRunningOnAllNodes(ds))
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error waiting for daemon pod to start")
			filterWorkerNode := func(nodes *v1.NodeList) *v1.NodeList {
				tmp := v1.NodeList{}
				for i, node := range nodes.Items {
					if strings.Contains(node.Name, "-worker") {
						tmp.Items = append(tmp.Items, nodes.Items[i])
					}
				}
				return &tmp
			}

			nodeList := common.GetReadySchedulableNodesOrDie(f.ClientSet)
			nodeList = filterWorkerNode(nodeList)
			// We need at least 2 nodes to meaningfully test ignoring one NotReady node.
			notReadyNodeCount := 2
			if len(nodeList.Items) < notReadyNodeCount {
				ginkgo.Skip("Need at least 2 schedulable nodes for ignore-notready-nodes e2e")
			}
			nodeNotReadySet := sets.NewString()
			for i := 0; i < notReadyNodeCount; i++ {
				node := nodeList.Items[len(nodeList.Items)-1-i]
				nodeNotReadySet.Insert(node.Name)
				ginkgo.By(fmt.Sprintf("Mark node %s NotReady", node.Name))
				err = exec.Command("docker", "stop", node.Name).Run()
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}

			ginkgo.By("wait node to not ready")
			err = wait.PollImmediate(v1beta1.DaemonSetRetryPeriod, v1beta1.DaemonSetRetryTimeout*2, func() (bool, error) {
				notStatus := map[string]bool{}
				nodeList := common.GetReadySchedulableNodesOrDie(f.ClientSet)
				for _, p := range nodeList.Items {
					if nodeNotReadySet.Has(p.Name) {
						notStatus[p.Name] = true
					}
				}
				if len(notStatus) == 0 {
					return true, nil
				}
				return false, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error node not unready")

			ginkgo.By("Migrate the kruise-system pod")
			err = wait.PollImmediate(v1beta1.DaemonSetRetryPeriod, v1beta1.DaemonSetRetryTimeout*2, func() (bool, error) {
				podList, err := c.CoreV1().Pods(util.GetKruiseNamespace()).List(context.TODO(), metav1.ListOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for _, pod := range podList.Items {
					if nodeNotReadySet.Has(pod.Spec.NodeName) && pod.Labels["control-plane"] == "controller-manager" {
						err := c.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64Ptr(0)})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
					}
				}
				return true, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("wait kruise pod ready")
			err = wait.PollImmediate(v1beta1.DaemonSetRetryPeriod*5, v1beta1.DaemonSetRetryTimeout, func() (bool, error) {
				podList, err := c.CoreV1().Pods(util.GetKruiseNamespace()).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(map[string]string{
						"control-plane": "controller-manager",
					}).String(),
				})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if len(podList.Items) == 0 {
					common.Logf("Waiting for the kruise controller pod to become ready")
					return false, nil
				}
				for _, pod := range podList.Items {
					if !podutil.IsPodReady(&pod) {
						return false, nil
					}
				}
				return true, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "kruise controller ok")

			ginkgo.By("Update DaemonSet image to trigger rolling update")
			err = tester.UpdateDaemonSet(ds.Name, func(ds *appsv1beta1.DaemonSet) { // Bottleneck point
				ds.Spec.Template.Spec.Containers[0].Image = common.NewWebserverImage
				ds.Spec.Template.Spec.Affinity = &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "kubernetes.io/hostname",
											Operator: v1.NodeSelectorOpNotIn,
											Values:   nodeNotReadySet.List(),
										},
									},
								},
							},
						},
					},
				}
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "error updating daemonset image")

			ginkgo.By("Ensure daemon pods on the NotReady node are eventually deleted")
			err = wait.PollImmediate(v1beta1.DaemonSetRetryPeriod, v1beta1.DaemonSetRetryTimeout, func() (bool, error) {
				podList, err := tester.ListDaemonPods(label)
				if err != nil {
					return false, err
				}
				for _, p := range podList.Items {
					if nodeNotReadySet.Has(p.Name) && p.DeletionTimestamp == nil {
						common.Logf("Pod %s is still running", p.Name)
						return false, nil
					}
				}
				return true, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "daemon pods on NotReady node were not deleted")

			ginkgo.By("Check that daemon pods images")
			err = wait.PollImmediate(v1beta1.DaemonSetRetryPeriod, v1beta1.DaemonSetRetryTimeout, func() (bool, error) {

				podList, err := tester.ListDaemonPods(label)
				if err != nil {
					return false, err
				}
				for _, p := range podList.Items {
					if !nodeNotReadySet.Has(p.Spec.NodeName) {
						if p.Spec.Containers[0].Image != common.NewWebserverImage {
							common.Logf("Pod %s is not running on node %s", p.Name, p.Spec.NodeName)
							return false, nil
						}
					}
				}
				return true, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "daemonset did not update all pods")

			ginkgo.By("clean unready pods")
			err = wait.PollImmediate(v1beta1.DaemonSetRetryPeriod, v1beta1.DaemonSetRetryTimeout, func() (bool, error) {
				podList, err := c.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
				if err != nil {
					common.Logf("Error listing pods: %v", err)
					return false, err
				}
				for _, pod := range podList.Items {
					if nodeNotReadySet.Has(pod.Spec.NodeName) {
						err := c.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{GracePeriodSeconds: pointer.Int64(0)})
						if err != nil && errors.IsNotFound(err) {
							common.Logf("Pod %s is still running", pod.Name)
							return false, err
						}
					}
				}
				return true, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "daemonset delete unready node")

			ginkgo.By("recover node")
			for _, node := range nodeNotReadySet.List() {
				err = exec.Command("docker", "start", node).Run()
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}

			ginkgo.By("wait all node and pod ready")
			err = wait.PollImmediate(v1beta1.DaemonSetRetryPeriod*5, v1beta1.DaemonSetRetryTimeout, func() (bool, error) {
				nodeList, err := c.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for _, node := range nodeList.Items {
					if !v1beta1.IsNodeReady(&node) {
						return false, nil
					}
				}

				podList, err := c.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for _, pod := range podList.Items {
					if !podutil.IsPodReady(&pod) {
						return false, nil
					}
				}
				return true, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cluster ok")

		})
	})
})
