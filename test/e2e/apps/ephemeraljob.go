package apps

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
	"time"
)

var _ = SIGDescribe("EphemeralJob", func() {
	f := framework.NewDefaultFramework("ehpemeraljobs")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.EphemeralJobTester
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewEphemeralJobTester(c, kc, ns)
		randStr = rand.String(10)
	})

	framework.KruiseDescribe("EphemeralJob Creating", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all EphemeralJob in cluster")
			tester.DeleteEphemeralJobs(ns)
			tester.DeleteDeployments(ns)
		})

		ginkgo.It("create ephemeral running succeed job", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 5, []v1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.9.1",
				},
			})

			ginkgo.By("Create EphemeralJob job-" + randStr)
			job := tester.CreateTestEphemeralJob(randStr, 4, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "busybox:latest",
						Command:                  []string{"sleep", "3"},
						ImagePullPolicy:          v1.PullIfNotPresent,
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			ginkgo.By("Check the status of job")

			gomega.Eventually(func() int32 {
				ejob, err := tester.GetEphemeralJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return ejob.Status.Succeeded
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(int32(4)))
		})

		ginkgo.It("create ephemeral running err command job", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 10, []v1.Container{
				{
					Name:            "nginx",
					Image:           "nginx:1.9.1",
					ImagePullPolicy: v1.PullIfNotPresent,
				},
			})

			ginkgo.By("Create EphemeralJob job-" + randStr)
			job := tester.CreateTestEphemeralJob(randStr, 10, 10, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "busybox:latest",
						ImagePullPolicy:          v1.PullIfNotPresent,
						Command:                  []string{"sdkfjk"},
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			ginkgo.By("Check the status of job")

			gomega.Eventually(func() int32 {
				ejob, err := tester.GetEphemeralJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return ejob.Status.Failed
			}, 60*time.Second, 10*time.Second).Should(gomega.Equal(int32(10)))
		})

		ginkgo.It("create ephemeral running activeDeadlineSeconds", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 1, []v1.Container{
				{
					Name:            "nginx",
					Image:           "nginx:1.9.1",
					ImagePullPolicy: v1.PullIfNotPresent,
				},
			})

			ginkgo.By("Create EphemeralJob job-" + randStr)

			var defaultInt32Value int32 = 1
			var defaultTTLSecondsAfterCreated int64 = 20
			job := &appsv1alpha1.EphemeralJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job-" + randStr},
				Spec: appsv1alpha1.EphemeralJobSpec{
					Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"run": "nginx"}},
					Replicas:    &defaultInt32Value,
					Parallelism: &defaultInt32Value,
					Template: appsv1alpha1.EphemeralContainerTemplateSpec{
						EphemeralContainers: []v1.EphemeralContainer{
							{
								TargetContainerName: "nginx",
								EphemeralContainerCommon: v1.EphemeralContainerCommon{
									Name:                     "debugger",
									Image:                    "busybox:latest",
									Command:                  []string{"sleep", "9999"},
									ImagePullPolicy:          v1.PullIfNotPresent,
									TerminationMessagePolicy: v1.TerminationMessageReadFile,
								},
							}},
					},
					ActiveDeadlineSeconds: &defaultTTLSecondsAfterCreated,
				},
			}

			job = tester.CreateEphemeralJob(job)
			ginkgo.By("Check the status of job")
			gomega.Eventually(func() appsv1alpha1.EphemeralJobPhase {
				ejob, _ := tester.GetEphemeralJob(job.Name)
				return ejob.Status.Phase
			}, 60*time.Second, 10*time.Second).Should(gomega.Equal(appsv1alpha1.EphemeralJobFailed))
		})

		ginkgo.It("create ephemeral running error exit job", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 10, []v1.Container{
				{
					Name:            "nginx",
					Image:           "nginx:1.9.1",
					ImagePullPolicy: v1.PullIfNotPresent,
				},
			})

			ginkgo.By("Create EphemeralJob job-" + randStr)
			job := tester.CreateTestEphemeralJob(randStr, 10, 10, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "busybox:latest",
						ImagePullPolicy:          v1.PullIfNotPresent,
						Command:                  []string{"ls", "sdfewasf"},
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			ginkgo.By("Check the status of job")

			gomega.Eventually(func() int32 {
				ejob, err := tester.GetEphemeralJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return ejob.Status.Failed
			}, 60*time.Second, 10*time.Second).Should(gomega.Equal(int32(10)))
		})

		ginkgo.It("create two ephemeral running job", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 1, []v1.Container{
				{
					Name:            "nginx",
					Image:           "nginx:1.9.1",
					ImagePullPolicy: v1.PullIfNotPresent,
				},
			})

			ginkgo.By("Create EphemeralJob job-" + randStr + "2")

			job1 := tester.CreateTestEphemeralJob(randStr+"2", 1, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "busybox:latest",
						ImagePullPolicy:          v1.PullIfNotPresent,
						Command:                  []string{"sleep", "3000"},
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			tester.CreateTestEphemeralJob("222222", 1, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger2",
						Image:                    "busybox:latest",
						ImagePullPolicy:          v1.PullIfNotPresent,
						Command:                  []string{"sleep", "30"},
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			ginkgo.By("Check the status of job")

			gomega.Eventually(func() int {
				targetPods, err := tester.GetPodsByEjob(job1.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if len(targetPods) == 0 {
					ginkgo.Fail("failed to get target pods")
				}
				targetPod := targetPods[0]
				return len(targetPod.Status.EphemeralContainerStatuses)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(2))
		})

		ginkgo.It("create ephemeral running two job, but to one target", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 1, []v1.Container{
				{
					Name:            "nginx",
					Image:           "nginx:1.9.1",
					ImagePullPolicy: v1.PullIfNotPresent,
				},
			})

			ginkgo.By("Create EphemeralJob job-" + randStr + "2")

			job1 := tester.CreateTestEphemeralJob(randStr+"1", 1, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "busybox:latest",
						ImagePullPolicy:          v1.PullIfNotPresent,
						Command:                  []string{"sleep", "3000"},
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			job2 := tester.CreateTestEphemeralJob(randStr+"2", 1, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    "busybox:latest",
						ImagePullPolicy:          v1.PullIfNotPresent,
						Command:                  []string{"sleep", "3000"},
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			ginkgo.By("Check the status of job")

			gomega.Eventually(func() int {
				job, _ := tester.GetEphemeralJob(job1.Name)
				return int(job.Status.Running)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(1))

			gomega.Eventually(func() int {
				targetPods, err := tester.GetPodsByEjob(job1.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if len(targetPods) == 0 {
					ginkgo.Fail("failed to get target pods")
				}
				targetPod := targetPods[0]
				return len(targetPod.Status.EphemeralContainerStatuses)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(1))

			gomega.Eventually(func() int32 {
				job, _ := tester.GetEphemeralJob(job2.Name)
				return job.Status.Matches
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(int32(0)))
		})
	})

	// checking feature
	framework.KruiseDescribe("EphemeralJob Feature Checking", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all EphemeralJob in cluster")
			tester.DeleteEphemeralJobs(ns)
			tester.DeleteDeployments(ns)
		})

		ginkgo.It("check ttl", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 1, []v1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.9.1",
				},
			})

			ginkgo.By("Create EphemeralJob job-" + randStr)
			var defaultInt32Value int32 = 1
			var defaultTTLSecondsAfterCreated int32 = 5
			var defaultActiveDeadlineSeconds int64 = 5
			job := &appsv1alpha1.EphemeralJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job-" + randStr},
				Spec: appsv1alpha1.EphemeralJobSpec{
					Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"run": "nginx"}},
					Replicas:    &defaultInt32Value,
					Parallelism: &defaultInt32Value,
					Template: appsv1alpha1.EphemeralContainerTemplateSpec{
						EphemeralContainers: []v1.EphemeralContainer{
							{
								TargetContainerName: "nginx",
								EphemeralContainerCommon: v1.EphemeralContainerCommon{
									Name:                     "debugger",
									Image:                    "busybox:latest",
									Command:                  []string{"sleep", "3"},
									ImagePullPolicy:          v1.PullIfNotPresent,
									TerminationMessagePolicy: v1.TerminationMessageReadFile,
								},
							}},
					},
					TTLSecondsAfterFinished: &defaultTTLSecondsAfterCreated,
					ActiveDeadlineSeconds:   &defaultActiveDeadlineSeconds,
				},
			}
			tester.CreateEphemeralJob(job)
			gomega.Expect(tester.CheckEphemeralJobExist(job)).Should(gomega.Equal(true))
			time.Sleep(20 * time.Second)
			gomega.Expect(tester.CheckEphemeralJobExist(job)).Should(gomega.Equal(false))
		})
	})
})
