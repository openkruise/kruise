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
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = SIGDescribe("BroadcastJob", func() {
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

	framework.KruiseDescribe("BroadcastJob dispatching", func() {

		ginkgo.It("succeeds for parallelism < number of node", func() {
			ginkgo.By("Create Fake Node " + randStr)
			fakeNode, err := nodeTester.CreateFakeNode(randStr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Create BroadcastJob job-" + randStr)
			parallelism := intstr.FromInt(1)
			job := &appsv1alpha1.BroadcastJob{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + randStr},
				Spec: appsv1alpha1.BroadcastJobSpec{
					Parallelism: &parallelism,
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Tolerations: []v1.Toleration{{Key: framework.E2eFakeKey, Operator: v1.TolerationOpEqual, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
							Containers: []v1.Container{{
								Name:    "pi",
								Image:   "perl",
								Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(1000)"},
							}},
							RestartPolicy: v1.RestartPolicyNever,
						},
					},
					CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
				},
			}

			job, err = tester.CreateBroadcastJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			nodes, err := nodeTester.ListRealNodesWithFake(job.Spec.Template.Spec.Tolerations)
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
					fakePod.Status.Phase = v1.PodSucceeded
					_, err = c.CoreV1().Pods(ns).UpdateStatus(fakePod)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				}

				return len(pods)
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(len(nodes)))

			gomega.Eventually(func() int32 {
				job, err = tester.GetBroadcastJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Succeeded
			}, 30*time.Second, time.Second).Should(gomega.Equal(int32(len(nodes))))
		})
	})
})
