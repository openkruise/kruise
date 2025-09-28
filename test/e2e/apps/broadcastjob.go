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
	"context"
	"time"

	"k8s.io/client-go/util/retry"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ginkgo.Describe("BroadcastJob", ginkgo.Label("BroadcastJob", "job", "workload"), func() {
	f := framework.NewDefaultFramework("broadcastjobs")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.BroadcastJobTester
	var nodeTester *framework.NodeTester
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewBroadcastJobTester(c, kc, ns)
		nodeTester = framework.NewNodeTester(c)
		randStr = rand.String(10)
	})

	ginkgo.AfterEach(func() {
		err := nodeTester.DeleteFakeNode(randStr)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	f.AfterEachActions = []func(){
		func() {
			// Print debug info if it fails
			if ginkgo.CurrentSpecReport().Failed() {
				allNodes, err := nodeTester.ListNodesWithFake()
				if err != nil {
					framework.Logf("[FAILURE_DEBUG] List Nodes error: %v", err)
				} else {
					framework.Logf("[FAILURE_DEBUG] List Nodes: %v", allNodes)
				}
				job, err := tester.GetBroadcastJob("job-" + randStr)
				if err != nil {
					framework.Logf("[FAILURE_DEBUG] Get BroadcastJob %s error: %v", "job-"+randStr, err)
				} else {
					framework.Logf("[FAILURE_DEBUG] Get BroadcastJob: %v", util.DumpJSON(job))
				}
			}
		},
	}

	ginkgo.Context("BroadcastJob dispatching", func() {
		ginkgo.It("succeeds for parallelism < number of node", func() {
			ginkgo.By("Create Fake Node " + randStr)
			fakeNode, err := nodeTester.CreateFakeNode(randStr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Create BroadcastJob job-" + randStr)
			job := &appsv1alpha1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + randStr},
				Spec: appsv1alpha1.BroadcastJobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Tolerations: []v1.Toleration{{Key: framework.E2eFakeKey, Operator: v1.TolerationOpEqual, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
							Containers: []v1.Container{{
								Name:    "box",
								Image:   BusyboxImage,
								Command: []string{"/bin/sh", "-c", "sleep 5"},
							}},
							RestartPolicy: v1.RestartPolicyNever,
						},
					},
					CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
				},
			}

			nodes, err := nodeTester.ListRealNodesWithFake(job.Spec.Template.Spec.Tolerations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			parallelism := intstr.FromInt(max(len(nodes)-1, min(len(nodes), 1)))
			job.Spec.Parallelism = &parallelism

			job, err = tester.CreateBroadcastJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check the status of job")
			gomega.Eventually(func() int32 {
				job, err = tester.GetBroadcastJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 10*time.Second, time.Second).Should(gomega.Equal(int32(len(nodes))))

			gomega.Eventually(func() int {
				pods, err := tester.GetPodsOfJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				var fakePod *v1.Pod
				for _, p := range pods {
					if p.Spec.NodeName == fakeNode.Name {
						fakePod = p
						break
					}
				}
				if fakePod != nil && fakePod.Status.Phase != v1.PodSucceeded {
					ginkgo.By("Try to update Pod " + fakePod.Name + " to Succeeded")
					err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
						fakePod, err := c.CoreV1().Pods(job.Namespace).Get(context.TODO(), fakePod.Name, metav1.GetOptions{})
						if err != nil {
							return err
						}
						fakePod.Status.Phase = v1.PodSucceeded
						_, err = c.CoreV1().Pods(ns).UpdateStatus(context.TODO(), fakePod, metav1.UpdateOptions{})
						return err
					})
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				}

				return len(pods)
			}, 180*time.Second, 3*time.Second).Should(gomega.Equal(len(nodes)))

			gomega.Eventually(func() int32 {
				job, err = tester.GetBroadcastJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Succeeded
			}, 60*time.Second, time.Second).Should(gomega.Equal(int32(len(nodes))))
		})
	})

	ginkgo.Context("BroadcastJob uncordon handling", func() {
		ginkgo.It("creates missing pod after node uncordon", func() {
			// Create fake node
			fakeNode, err := nodeTester.CreateFakeNode(randStr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Cordon the fake node
			ginkgo.By("Cordoning fake node " + fakeNode.Name)
			_, err = c.CoreV1().Nodes().Patch(context.TODO(), fakeNode.Name,
				types.StrategicMergePatchType,
				[]byte(`{"spec":{"unschedulable":true}}`),
				metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Create BroadcastJob job-" + randStr
			job := &appsv1alpha1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + randStr},
				Spec: appsv1alpha1.BroadcastJobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Tolerations: []v1.Toleration{{Key: framework.E2eFakeKey, Operator: v1.TolerationOpEqual, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
							Containers: []v1.Container{{
								Name:    "box",
								Image:   BusyboxImage,
								Command: []string{"/bin/sh", "-c", "sleep 30"},
							}},
							RestartPolicy: v1.RestartPolicyNever,
						},
					},
					CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
				},
			}

			nodes, err := nodeTester.ListRealNodesWithFake(job.Spec.Template.Spec.Tolerations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			totalNodes := int32(len(nodes))
			parallelism := intstr.FromInt(len(nodes))
			job.Spec.Parallelism = &parallelism

			ginkgo.By("Creating BroadcastJob " + job.Name)
			job, err = tester.CreateBroadcastJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Verify desired count equals total nodes - 1 (due to cordoned node)")
			gomega.Eventually(func() int32 {
				job, err = tester.GetBroadcastJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 30*time.Second, time.Second).Should(gomega.Equal(totalNodes - 1))

			ginkgo.By("Verify active pods equals total nodes - 1 (as pod is not created on cordoned node)")
			gomega.Eventually(func() int32 {
				job, err = tester.GetBroadcastJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Active
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(totalNodes - 1))

			// Uncordon the fake node
			ginkgo.By("Uncordoning fake node " + fakeNode.Name)
			_, err = c.CoreV1().Nodes().Patch(context.TODO(), fakeNode.Name,
				types.StrategicMergePatchType,
				[]byte(`{"spec":{"unschedulable":false}}`),
				metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Verify desired count becomes total nodes after uncordon")
			gomega.Eventually(func() int32 {
				job, err = tester.GetBroadcastJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 30*time.Second, time.Second).Should(gomega.Equal(totalNodes))

			ginkgo.By("Verify active pods becomes total nodes after uncordon (missing pod now created)")
			gomega.Eventually(func() int32 {
				job, err = tester.GetBroadcastJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Active
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(totalNodes))
		})
	})
})
