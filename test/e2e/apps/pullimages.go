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
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	utilpointer "k8s.io/utils/pointer"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework"
)

var _ = SIGDescribe("PullImage", func() {
	f := framework.NewDefaultFramework("pullimages")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var testerForNodeImage *framework.NodeImageTester
	var testerForImagePullJob *framework.ImagePullJobTester
	var nodes []*v1.Node

	f.AfterEachActions = []func(){
		func() {
			// Print debug info if it fails
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				nodeImageList, err := testerForNodeImage.ListNodeImages()
				if err != nil {
					framework.Logf("[FAILURE_DEBUG] List NodeImages error: %v", err)
				} else {
					framework.Logf("[FAILURE_DEBUG] List NodeImages: %v", util.DumpJSON(nodeImageList))
				}
				imagePullJobList, err := testerForImagePullJob.ListJobs(ns)
				if err != nil {
					framework.Logf("[FAILURE_DEBUG] List ImagePullJobs in %s error: %v", ns, err)
				} else {
					framework.Logf("[FAILURE_DEBUG] List ImagePullJobs in %s: %v", ns, util.DumpJSON(imagePullJobList))
				}
			}
		},
	}

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		testerForNodeImage = framework.NewNodeImageTester(c, kc)
		testerForImagePullJob = framework.NewImagePullJobTester(c, kc)

		err := testerForNodeImage.CreateFakeNodeImageIfNotPresent()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		nodes, err = testerForNodeImage.ExpectNodes()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		err := testerForNodeImage.DeleteFakeNodeImage()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	framework.KruiseDescribe("ImagePullJob pulling images functionality [ImagePullJob]", func() {
		var baseJob *appsv1beta1.ImagePullJob
		intorstr4 := intstr.FromInt(4)

		ginkgo.BeforeEach(func() {
			baseJob = &appsv1beta1.ImagePullJob{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "test-imagepulljob"}}
		})

		framework.ConformanceIt("create an always job to pull an image on all real nodes", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1beta1.ImagePullJobSpec{
				Image: NginxImage,
				ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
					Selector: &appsv1beta1.ImagePullJobNodeSelector{LabelSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: framework.FakeNodeImageLabelKey, Operator: metav1.LabelSelectorOpDoesNotExist},
					}}},
					PullPolicy: &appsv1beta1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism: &intorstr4,
					CompletionPolicy: appsv1beta1.CompletionPolicy{
						Type:                    appsv1beta1.Always,
						ActiveDeadlineSeconds:   utilpointer.Int64Ptr(50),
						TTLSecondsAfterFinished: utilpointer.Int32Ptr(20),
					},
					PullSecrets: []string{"test-pull-secret"},
				},
			}
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      "test-pull-secret",
				},
				Type: v1.SecretTypeDockercfg,
				Data: map[string][]byte{
					v1.DockerConfigKey: []byte(`{"auths":{"docker.io/library/nginx":{"username":"echoserver","password":"test","auth":"ZWNob3NlcnZlcjp0ZXN0"}}}`),
				},
			}
			_, err := c.CoreV1().Secrets(ns).Create(context.TODO(), secret, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			err = testerForImagePullJob.CreateJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ginkgo.By("Desired should be equal to number of nodes")
			gomega.Eventually(func() int32 {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(nodes))))

			ginkgo.By("Wait completed in 180s")
			gomega.Eventually(func() bool {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.CompletionTime != nil
			}, 180*time.Second, 3*time.Second).Should(gomega.Equal(true))
			gomega.Expect(job.Status.Succeeded).To(gomega.Equal(int32(len(nodes))))

			ginkgo.By("Wait clean in 25s")
			gomega.Eventually(func() bool {
				_, err = testerForImagePullJob.GetJob(job)
				return err != nil && errors.IsNotFound(err)
			}, 25*time.Second, 2*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check image should be cleaned in NodeImage")
			gomega.Eventually(func() bool {
				found, err := testerForNodeImage.IsImageInSpec(job.Spec.Image, nodes[0].Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return found
			}, 10*time.Second, time.Second).Should(gomega.Equal(false))
		})

		framework.ConformanceIt("create an always job to pull an image on one real node", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1beta1.ImagePullJobSpec{
				Image: NewNginxImage,
				ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
					Selector: &appsv1beta1.ImagePullJobNodeSelector{Names: []string{nodes[0].Name}},
					PullPolicy: &appsv1beta1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism: &intorstr4,
					CompletionPolicy: appsv1beta1.CompletionPolicy{
						Type: appsv1beta1.Always,
					},
				},
			}
			err := testerForImagePullJob.CreateJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Desired should be equal to 1")
			gomega.Eventually(func() int32 {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait completed in 180s")
			gomega.Eventually(func() bool {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.CompletionTime != nil
			}, 180*time.Second, 3*time.Second).Should(gomega.Equal(true))
			gomega.Expect(job.Status.Succeeded).To(gomega.Equal(int32(1)))

			ginkgo.By("Delete job")
			err = testerForImagePullJob.DeleteJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check image should be cleaned in NodeImage")
			gomega.Eventually(func() bool {
				found, err := testerForNodeImage.IsImageInSpec(job.Spec.Image, nodes[0].Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return found
			}, 3*time.Second, time.Second).Should(gomega.Equal(false))
		})

		framework.ConformanceIt("create a never job to pull an image on all nodes", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1beta1.ImagePullJobSpec{
				Image: WebserverImage,
				ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
					PullPolicy: &appsv1beta1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism: &intorstr4,
					CompletionPolicy: appsv1beta1.CompletionPolicy{
						Type: appsv1beta1.Never,
					},
				},
			}
			err := testerForImagePullJob.CreateJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Desired should be equal to 1+len(nodes)")
			gomega.Eventually(func() int32 {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(nodes) + 1)))

			ginkgo.By(fmt.Sprintf("Wait %d succeeded", len(nodes)))
			gomega.Eventually(func() int32 {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Succeeded
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(len(nodes))))
			gomega.Expect(job.Status.CompletionTime == nil).To(gomega.Equal(true))

			//ginkgo.By("Wait 1 failed in 80s")
			//gomega.Eventually(func() int32 {
			//	job, err = testerForImagePullJob.GetJob(job)
			//	gomega.Expect(err).NotTo(gomega.HaveOccurred())
			//	return job.Status.Failed
			//}, 80*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))
			//gomega.Expect(len(job.Status.FailedNodes)).To(gomega.Equal(1))
		})

		framework.ConformanceIt("create two jobs to pull a same image", func() {
			ginkgo.By("Create job1")
			job1 := baseJob.DeepCopy()
			job1.Name = baseJob.Name + "-1"
			job1.Spec = appsv1beta1.ImagePullJobSpec{
				Image: NewWebserverImage,
				ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
					Selector: &appsv1beta1.ImagePullJobNodeSelector{Names: []string{nodes[0].Name}},
					PullPolicy: &appsv1beta1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism: &intorstr4,
					CompletionPolicy: appsv1beta1.CompletionPolicy{
						Type: appsv1beta1.Never,
					},
				},
			}
			err := testerForImagePullJob.CreateJob(job1)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Desired should be equal to 1")
			gomega.Eventually(func() int32 {
				job1, err = testerForImagePullJob.GetJob(job1)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job1.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait job1 completed in 60s")
			gomega.Eventually(func() int32 {
				job1, err = testerForImagePullJob.GetJob(job1)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job1.Status.Succeeded
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait until job1 has created over 70s (> response timeout)")
			time.Until(job1.CreationTimestamp.Add(70 * time.Second))

			ginkgo.By("Create job2")
			job2 := baseJob.DeepCopy()
			job2.Name = baseJob.Name + "-2"
			job2.Spec = appsv1beta1.ImagePullJobSpec{
				Image: NewWebserverImage,
				ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
					Selector: &appsv1beta1.ImagePullJobNodeSelector{Names: []string{nodes[0].Name}},
					PullPolicy: &appsv1beta1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism: &intorstr4,
					CompletionPolicy: appsv1beta1.CompletionPolicy{
						Type: appsv1beta1.Never,
					},
				},
			}
			err = testerForImagePullJob.CreateJob(job2)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait job2 completed in 10s")
			gomega.Eventually(func() int32 {
				job2, err = testerForImagePullJob.GetJob(job2)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job2.Status.Succeeded
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))
		})
	})

})
