/*
Copyright 2023 The Kruise Authors.

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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	utilpointer "k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework"
)

var _ = SIGDescribe("PullImages", func() {
	f := framework.NewDefaultFramework("imagelistpulljobs")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var testerForNodeImage *framework.NodeImageTester
	var testerForImageListPullJob *framework.ImageListPullJobTester
	var testerForImagePullJob *framework.ImagePullJobTester
	var nodes []*v1.Node
	var imagePullJobs *appsv1alpha1.ImagePullJobList

	f.AfterEachActions = []func(){
		func() {
			// Print debug info if it fails
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				imagePullJobList, err := testerForImagePullJob.ListJobs(ns)
				if err != nil {
					framework.Logf("[FAILURE_DEBUG] List ImagePullJobs in %s error: %v", ns, err)
				} else {
					framework.Logf("[FAILURE_DEBUG] List ImagePullJobs in %s: %v", ns, util.DumpJSON(imagePullJobList))
				}
				imageListPullJobList, err := testerForImageListPullJob.ListJobs(ns)
				if err != nil {
					framework.Logf("[FAILURE_DEBUG] List ImageListPullJobs in %s error: %v", ns, err)
				} else {
					framework.Logf("[FAILURE_DEBUG] List ImageListPullJobs in %s: %v", ns, util.DumpJSON(imageListPullJobList))
				}
			}
		},
	}

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		testerForNodeImage = framework.NewNodeImageTester(c, kc)
		testerForImageListPullJob = framework.NewImageListPullJobTester(c, kc)
		testerForImagePullJob = framework.NewImagePullJobTester(c, kc)
		err := testerForNodeImage.CreateFakeNodeImageIfNotPresent()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		nodes, err = testerForNodeImage.ExpectNodes()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		err := testerForNodeImage.DeleteFakeNodeImage()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = testerForImagePullJob.DeleteAllJobs(ns)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	framework.KruiseDescribe("ImageListPullJob pulling images functionality [ImageListPullJob]", func() {
		var baseJob *appsv1alpha1.ImageListPullJob
		intorstr4 := intstr.FromInt(4)

		ginkgo.BeforeEach(func() {
			baseJob = &appsv1alpha1.ImageListPullJob{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "test-imagelistpulljob"}}
		})

		framework.ConformanceIt("create an always job to pull two images on all real nodes", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1alpha1.ImageListPullJobSpec{
				Images: []string{NginxImage, BusyboxImage},
				ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
					Selector: &appsv1alpha1.ImagePullJobNodeSelector{LabelSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: framework.FakeNodeImageLabelKey, Operator: metav1.LabelSelectorOpDoesNotExist},
					}}},
					PullPolicy: &appsv1alpha1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism:     &intorstr4,
					ImagePullPolicy: appsv1alpha1.PullAlways,
					CompletionPolicy: appsv1alpha1.CompletionPolicy{
						Type:                    appsv1alpha1.Always,
						ActiveDeadlineSeconds:   utilpointer.Int64Ptr(50),
						TTLSecondsAfterFinished: utilpointer.Int32Ptr(20),
					},
				},
			}
			err := testerForImageListPullJob.CreateJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("Wait %d imagepulljob created", len(job.Spec.Images)))
			gomega.Eventually(func() int32 {
				imagePullJobs, err = testerForImagePullJob.ListJobs(job.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return int32(len(imagePullJobs.Items))
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(job.Spec.Images))))

			ginkgo.By("Desired should be equal to number of imagepulljobs [2]")
			gomega.Eventually(func() int32 {
				job, err = testerForImageListPullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(job.Spec.Images))))

			ginkgo.By("Wait completed in 270s")
			gomega.Eventually(func() bool {
				job, err = testerForImageListPullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.CompletionTime != nil
			}, 270*time.Second, 3*time.Second).Should(gomega.Equal(true))
			gomega.Expect(job.Status.Succeeded).To(gomega.Equal(int32(len(job.Spec.Images))))

			ginkgo.By("Wait clean in 25s")
			gomega.Eventually(func() bool {
				_, err = testerForImageListPullJob.GetJob(job)
				return err != nil && errors.IsNotFound(err)
			}, 25*time.Second, 2*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check imagepulljob should be cleaned")
			gomega.Eventually(func() bool {
				imagePullJobs, err := testerForImagePullJob.ListJobs(job.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(imagePullJobs.Items) > 0
			}, 10*time.Second, time.Second).Should(gomega.Equal(false))
		})

		framework.ConformanceIt("create an always job to pull two images on one real node", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1alpha1.ImageListPullJobSpec{
				Images: []string{NewNginxImage, BusyboxImage},
				ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
					Selector: &appsv1alpha1.ImagePullJobNodeSelector{Names: []string{nodes[0].Name}},
					PullPolicy: &appsv1alpha1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism:     &intorstr4,
					ImagePullPolicy: appsv1alpha1.PullIfNotPresent,
					CompletionPolicy: appsv1alpha1.CompletionPolicy{
						Type: appsv1alpha1.Always,
					},
				},
			}
			err := testerForImageListPullJob.CreateJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Desired should be equal to 2")
			gomega.Eventually(func() int32 {
				job, err = testerForImageListPullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(job.Spec.Images))))

			ginkgo.By("Wait completed in 360s")
			gomega.Eventually(func() bool {
				job, err = testerForImageListPullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.CompletionTime != nil
			}, 360*time.Second, 10*time.Second).Should(gomega.Equal(true))
			gomega.Expect(job.Status.Succeeded).To(gomega.Equal(int32(len(job.Spec.Images))))

			ginkgo.By("Delete job")
			err = testerForImageListPullJob.DeleteJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check imagepulljob should be cleaned")
			time.Sleep(3 * time.Second)
			gomega.Eventually(func() bool {
				imagePullJobLister, err := testerForImagePullJob.ListJobs(job.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				var imagePullJobs []*appsv1alpha1.ImagePullJob
				for i := range imagePullJobLister.Items {
					pullJob := &imagePullJobLister.Items[i]
					if metav1.IsControlledBy(pullJob, job) {
						imagePullJobs = append(imagePullJobs, pullJob)
					}
					fmt.Printf("waiting imagePullJob GC: %v", imagePullJobs)
				}
				return len(imagePullJobs) == 0
			}, time.Minute, time.Second).Should(gomega.BeTrue())
		})

		framework.ConformanceIt("create an always job to pull an image on all nodes", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1alpha1.ImageListPullJobSpec{
				Images: []string{WebserverImage},
				ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
					PullPolicy: &appsv1alpha1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism:     &intorstr4,
					ImagePullPolicy: appsv1alpha1.PullAlways,
					CompletionPolicy: appsv1alpha1.CompletionPolicy{
						Type: appsv1alpha1.Always,
					},
				},
			}
			err := testerForImageListPullJob.CreateJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("Wait %d imagepulljob created", len(job.Spec.Images)))
			gomega.Eventually(func() int32 {
				imagePullJobs, err := testerForImagePullJob.ListJobs(job.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return int32(len(imagePullJobs.Items))
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(job.Spec.Images))))

			ginkgo.By("Desired should be equal to 1")
			gomega.Eventually(func() int32 {
				job, err = testerForImageListPullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(job.Spec.Images))))

			// mock a completed imagepulljob
			time.Sleep(10 * time.Second)
			err = testerForImageListPullJob.FailNodeImageFast("fake-nodeimage")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait completed in 180s")
			gomega.Eventually(func() bool {
				job, err = testerForImageListPullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.CompletionTime != nil
			}, 180*time.Second, 3*time.Second).Should(gomega.Equal(true))
			gomega.Expect(len(job.Status.FailedImageStatuses)).To(gomega.Equal(len(job.Spec.Images)))

		})

		framework.ConformanceIt("create a never job to pull an image on all nodes", func() {
			job := baseJob.DeepCopy()
			job.Spec = appsv1alpha1.ImageListPullJobSpec{
				Images: []string{WebserverImage},
				ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
					PullPolicy: &appsv1alpha1.PullPolicy{
						TimeoutSeconds: utilpointer.Int32Ptr(50),
						BackoffLimit:   utilpointer.Int32Ptr(2),
					},
					Parallelism:     &intorstr4,
					ImagePullPolicy: appsv1alpha1.PullIfNotPresent,
					CompletionPolicy: appsv1alpha1.CompletionPolicy{
						Type: appsv1alpha1.Never,
					},
				},
			}
			err := testerForImageListPullJob.CreateJob(job)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By(fmt.Sprintf("Wait %d imagepulljob created", len(job.Spec.Images)))
			gomega.Eventually(func() int32 {
				imagePullJobs, err = testerForImagePullJob.ListJobs(job.Namespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return int32(len(imagePullJobs.Items))
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(job.Spec.Images))))

			ginkgo.By("Desired should be equal to 1")
			gomega.Eventually(func() int32 {
				job, err = testerForImageListPullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Desired
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(len(job.Spec.Images))))

			// mock a completed imagepulljob
			time.Sleep(10 * time.Second)
			err = testerForImageListPullJob.FailNodeImageFast("fake-nodeimage")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait completed in 180s")
			gomega.Eventually(func() int32 {
				job, err = testerForImageListPullJob.GetJob(job)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return job.Status.Completed
			}, 180*time.Second, 3*time.Second).Should(gomega.Equal(int32(len(job.Spec.Images))))
			gomega.Expect(len(job.Status.FailedImageStatuses)).To(gomega.Equal(len(job.Spec.Images)))

		})
	})

})
