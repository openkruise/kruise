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
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	utilpointer "k8s.io/utils/pointer"
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
		var baseJob *appsv1alpha1.ImagePullJob
		intorstr4 := intstr.FromInt(4)

		ginkgo.BeforeEach(func() {
			baseJob = &appsv1alpha1.ImagePullJob{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "test-imagepulljob"}}
		})

		ginkgo.It("create an always job to pull an image on all real nodes", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1alpha1.ImagePullJobSpec{
				Image: "nginx:1.9.1",
				Selector: &appsv1alpha1.ImagePullJobNodeSelector{LabelSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: framework.FakeNodeImageLabelKey, Operator: metav1.LabelSelectorOpDoesNotExist},
				}}},
				PullPolicy: &appsv1alpha1.PullPolicy{
					TimeoutSeconds: utilpointer.Int32Ptr(50),
					BackoffLimit:   utilpointer.Int32Ptr(2),
				},
				Parallelism: &intorstr4,
				CompletionPolicy: appsv1alpha1.CompletionPolicy{
					Type:                    appsv1alpha1.Always,
					ActiveDeadlineSeconds:   utilpointer.Int64Ptr(50),
					TTLSecondsAfterFinished: utilpointer.Int32Ptr(20),
				},
			}
			err := testerForImagePullJob.CreateJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Desired should be equal to number of nodes")
			gomega.Eventually(func() int32 {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(nodes))))

			ginkgo.By("Wait completed in 60s")
			gomega.Eventually(func() bool {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.CompletionTime != nil
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))
			gomega.Expect(job.Status.Succeeded).To(gomega.Equal(int32(len(nodes))))

			ginkgo.By("Wait clean in 25s")
			gomega.Eventually(func() bool {
				_, err = testerForImagePullJob.GetJob(job)
				return err != nil && errors.IsNotFound(err)
			}, 25*time.Second, 2*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check image should be cleaned in NodeImage")
			found, err := testerForNodeImage.IsImageInSpec(job.Spec.Image, nodes[0].Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(found).To(gomega.Equal(false))
		})

		ginkgo.It("create an always job to pull an image on one real node", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1alpha1.ImagePullJobSpec{
				Image:    "nginx:1.9.2",
				Selector: &appsv1alpha1.ImagePullJobNodeSelector{Names: []string{nodes[0].Name}},
				PullPolicy: &appsv1alpha1.PullPolicy{
					TimeoutSeconds: utilpointer.Int32Ptr(50),
					BackoffLimit:   utilpointer.Int32Ptr(2),
				},
				Parallelism: &intorstr4,
				CompletionPolicy: appsv1alpha1.CompletionPolicy{
					Type: appsv1alpha1.Always,
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

			ginkgo.By("Wait completed in 60s")
			gomega.Eventually(func() bool {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.CompletionTime != nil
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))
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

		ginkgo.It("create a never job to pull an image on all nodes", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1alpha1.ImagePullJobSpec{
				Image: "nginx:1.9.3",
				PullPolicy: &appsv1alpha1.PullPolicy{
					TimeoutSeconds: utilpointer.Int32Ptr(50),
					BackoffLimit:   utilpointer.Int32Ptr(2),
				},
				Parallelism: &intorstr4,
				CompletionPolicy: appsv1alpha1.CompletionPolicy{
					Type: appsv1alpha1.Never,
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

			ginkgo.By("Wait failed in 65s")
			gomega.Eventually(func() int32 {
				job, err = testerForImagePullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Failed
			}, 65*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))
			gomega.Expect(len(job.Status.FailedNodes)).To(gomega.Equal(1))

			ginkgo.By(fmt.Sprintf("Expect %d succeeded", len(nodes)))
			gomega.Expect(job.Status.Succeeded).To(gomega.Equal(int32(len(nodes))))
			gomega.Expect(job.Status.CompletionTime == nil).To(gomega.Equal(true))
		})
	})

})
