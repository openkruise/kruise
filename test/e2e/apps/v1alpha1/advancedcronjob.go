/*
Copyright 2025 The Kruise Authors.

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

package v1alpha1

import (
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"

	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	"github.com/openkruise/kruise/test/e2e/framework/v1alpha1"
)

var _ = ginkgo.Describe("AdvancedCronJob", ginkgo.Label("AdvancedCronJob", "cronjob", "workload"), func() {
	f := v1alpha1.NewDefaultFramework("advancedcronjobs")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *v1alpha1.AdvancedCronJobTester
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = v1alpha1.NewAdvancedCronJobTester(c, kc, ns)
		randStr = rand.String(10)
	})

	ginkgo.AfterEach(func() {
		if ginkgo.CurrentSpecReport().Failed() {
			common.DumpDebugInfo(c, ns)
		}
		common.Logf("Deleting all AdvancedCronJobs in ns %v", ns)
		tester.DeleteAllAdvancedCronJobs()
	})

	ginkgo.Context("Basic AdvancedCronJob functionality [AdvancedCronJobBasic]", func() {
		ginkgo.It("should create and schedule AdvancedCronJob with Job template", func() {
			acjName := "acj-job-" + randStr
			// Use a schedule that runs every minute for testing
			schedule := "*/1 * * * *"

			ginkgo.By("Creating AdvancedCronJob with Job template")
			acj := tester.CreateTestAdvancedCronJobWithJobTemplate(acjName, schedule)
			gomega.Expect(acj).NotTo(gomega.BeNil())
			gomega.Expect(acj.Spec.Schedule).To(gomega.Equal(schedule))
			gomega.Expect(acj.Spec.Template.JobTemplate).NotTo(gomega.BeNil())

			ginkgo.By("Waiting for AdvancedCronJob to have last schedule time")
			tester.WaitForAdvancedCronJobToHaveLastScheduleTime(acj)

			ginkgo.By("Waiting for jobs to be created")
			gomega.Eventually(func() int {
				jobs, err := tester.GetJobsCreatedByAdvancedCronJob(acj)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(jobs)
			}, 2*time.Minute, 10*time.Second).Should(gomega.BeNumerically(">=", 1))

			ginkgo.By("Verifying job execution")
			jobs, err := tester.GetJobsCreatedByAdvancedCronJob(acj)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(jobs)).To(gomega.BeNumerically(">=", 1))

			// Wait for at least one job to complete
			if len(jobs) > 0 {
				tester.WaitForJobToComplete(jobs[0].Name)
			}
		})

		ginkgo.It("should create and schedule AdvancedCronJob with BroadcastJob template", func() {
			acjName := "acj-broadcast-" + randStr
			// Use a schedule that runs every minute for testing
			schedule := "*/1 * * * *"

			ginkgo.By("Creating AdvancedCronJob with BroadcastJob template")
			acj := tester.CreateTestAdvancedCronJobWithBroadcastJobTemplate(acjName, schedule)
			gomega.Expect(acj).NotTo(gomega.BeNil())
			gomega.Expect(acj.Spec.Schedule).To(gomega.Equal(schedule))
			gomega.Expect(acj.Spec.Template.BroadcastJobTemplate).NotTo(gomega.BeNil())

			ginkgo.By("Waiting for AdvancedCronJob to have last schedule time")
			tester.WaitForAdvancedCronJobToHaveLastScheduleTime(acj)

			ginkgo.By("Waiting for broadcast jobs to be created")
			gomega.Eventually(func() int {
				broadcastJobs, err := tester.GetBroadcastJobsCreatedByAdvancedCronJob(acj)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(broadcastJobs)
			}, 2*time.Minute, 10*time.Second).Should(gomega.BeNumerically(">=", 1))

			ginkgo.By("Verifying broadcast job execution")
			broadcastJobs, err := tester.GetBroadcastJobsCreatedByAdvancedCronJob(acj)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(broadcastJobs)).To(gomega.BeNumerically(">=", 1))

			// Wait for at least one broadcast job to complete
			if len(broadcastJobs) > 0 {
				tester.WaitForBroadcastJobToComplete(broadcastJobs[0].Name)
			}
		})
	})

	ginkgo.Context("AdvancedCronJob pause functionality [AdvancedCronJobPause]", func() {
		ginkgo.It("should pause and resume AdvancedCronJob", func() {
			acjName := "acj-pause-" + randStr
			schedule := "*/1 * * * *"

			ginkgo.By("Creating AdvancedCronJob")
			acj := tester.CreateTestAdvancedCronJobWithJobTemplate(acjName, schedule)
			gomega.Expect(acj).NotTo(gomega.BeNil())

			ginkgo.By("Waiting for initial job to be created")
			gomega.Eventually(func() int {
				jobs, err := tester.GetJobsCreatedByAdvancedCronJob(acj)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(jobs)
			}, 2*time.Minute, 10*time.Second).Should(gomega.BeNumerically(">=", 1))

			ginkgo.By("Pausing AdvancedCronJob")
			updatedAcj, err := tester.PatchAdvancedCronJobPaused(acj.Name, true)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(updatedAcj.Spec.Paused).To(gomega.Equal(boolPtr(true)))

			pausedJobs, err := tester.GetJobsCreatedByAdvancedCronJob(acj)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			pausedJobCount := len(pausedJobs)
			common.Logf("Job count after pausing: %d", pausedJobCount)

			ginkgo.By("Waiting to ensure no new jobs are created while paused")
			// Wait for 2 minutes to ensure no new jobs are created while paused
			time.Sleep(120 * time.Second)

			// Check that no new jobs were created during the pause period
			stillPausedJobs, err := tester.GetJobsCreatedByAdvancedCronJob(acj)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			stillPausedJobCount := len(stillPausedJobs)
			common.Logf("Job count after pause period: %d", stillPausedJobCount)

			// The job count should not increase during the pause period
			gomega.Expect(stillPausedJobCount).To(gomega.Equal(pausedJobCount),
				"Job count should not increase while paused")

			ginkgo.By("Resuming AdvancedCronJob")
			updatedAcj, err = tester.PatchAdvancedCronJobPaused(acj.Name, false)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(updatedAcj.Spec.Paused).To(gomega.Equal(boolPtr(false)))

			ginkgo.By("Waiting for new jobs to be created after resume")
			// Wait for new jobs to be created after resume
			gomega.Eventually(func() int {
				jobs, err := tester.GetJobsCreatedByAdvancedCronJob(acj)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(jobs)
			}, 3*time.Minute, 10*time.Second).Should(gomega.BeNumerically(">", stillPausedJobCount))

			// Verify that jobs are being created again after resume
			finalJobs, err := tester.GetJobsCreatedByAdvancedCronJob(acj)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			finalJobCount := len(finalJobs)
			common.Logf("Final job count: %d", finalJobCount)
			gomega.Expect(finalJobCount).To(gomega.BeNumerically(">", stillPausedJobCount),
				"New jobs should be created after resume")
		})
	})

})

// Helper functions
func boolPtr(b bool) *bool { return &b }
