package apps

import (
	"strconv"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	utilpointer "k8s.io/utils/pointer"
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

		if v, err := c.Discovery().ServerVersion(); err != nil {
			framework.Logf("Failed to discovery server version: %v", err)
		} else if minor, err := strconv.Atoi(v.Minor); err != nil || minor < 20 {
			ginkgo.Skip("Skip EphemeralJob e2e for currently it can only run in K8s >= 1.20, got " + v.String())
		}

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

		// This can't be Conformance yet.
		ginkgo.It("create ephemeral running succeed job", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 5, []v1.Container{
				{
					Name:  "nginx",
					Image: NginxImage,
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
						Image:                    BusyboxImage,
						Command:                  []string{"sleep", "3"},
						ImagePullPolicy:          v1.PullIfNotPresent,
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			ginkgo.By("Check the status of job")

			//time.Sleep(time.Second * 30)
			gomega.Eventually(func() int32 {
				ejob, err := tester.GetEphemeralJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return ejob.Status.Succeeded
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(int32(4)))
		})

		// This can't be Conformance yet.
		ginkgo.It("create ephemeral running err command job", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 10, []v1.Container{
				{
					Name:            "nginx",
					Image:           NginxImage,
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
						Image:                    BusyboxImage,
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

		// This can't be Conformance yet.
		ginkgo.It("create ephemeral running activeDeadlineSeconds", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 1, []v1.Container{
				{
					Name:            "nginx",
					Image:           NginxImage,
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
									Image:                    BusyboxImage,
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

		// This can't be Conformance yet.
		ginkgo.It("create ephemeral running error exit job", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 10, []v1.Container{
				{
					Name:            "nginx",
					Image:           NginxImage,
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
						Image:                    BusyboxImage,
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

		// This can't be Conformance yet.
		ginkgo.It("create two ephemeral running job", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 1, []v1.Container{
				{
					Name:            "nginx",
					Image:           NginxImage,
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
						Image:                    BusyboxImage,
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
						Image:                    BusyboxImage,
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

		// This can't be Conformance yet.
		ginkgo.It("create ephemeral running two job, but to one target", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 1, []v1.Container{
				{
					Name:            "nginx",
					Image:           NginxImage,
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
						Image:                    BusyboxImage,
						ImagePullPolicy:          v1.PullIfNotPresent,
						Command:                  []string{"sleep", "3000"},
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})

			ginkgo.By("Check the status of job")

			gomega.Eventually(func() int {
				job, _ := tester.GetEphemeralJob(job1.Name)
				return int(job.Status.Running)
			}, 180*time.Second, 3*time.Second).Should(gomega.Equal(1))

			gomega.Eventually(func() int {
				targetPods, err := tester.GetPodsByEjob(job1.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if len(targetPods) == 0 {
					ginkgo.Fail("failed to get target pods")
				}
				targetPod := targetPods[0]
				return len(targetPod.Status.EphemeralContainerStatuses)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(1))

			job2 := tester.CreateTestEphemeralJob(randStr+"2", 1, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    BusyboxImage,
						ImagePullPolicy:          v1.PullIfNotPresent,
						Command:                  []string{"sleep", "3000"},
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})
			ginkgo.By("Check whether ephemeral container can updated (not possible yet)")
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

		// This can't be Conformance yet.
		ginkgo.It("check ttl", func() {
			ginkgo.By("Create Deployment and wait Pods ready")
			tester.CreateTestDeployment(randStr, 1, []v1.Container{
				{
					Name:  "nginx",
					Image: NginxImage,
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
									Image:                    BusyboxImage,
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

	framework.KruiseDescribe("Create Ephemeral Container", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all ephemeral container in cluster")
			tester.DeleteEphemeralJobs(ns)
			tester.DeleteDeployments(ns)
		})

		// This can't be Conformance yet.
		ginkgo.It("test ephemeral container in in-place-update", func() {
			// create cloneset
			ginkgo.By("Create CloneSet " + randStr)
			cloneSetTester := framework.NewCloneSetTester(c, kc, ns)
			cs := cloneSetTester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			cs.Spec.Template.Spec.Containers[0].Image = NginxImage
			cs.Spec.Template.ObjectMeta.Labels["run"] = "nginx"
			cs, err := cloneSetTester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = cloneSetTester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = cloneSetTester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			// create ephemeral container
			ginkgo.By("Create EphemeralJob job-" + randStr)
			job := tester.CreateTestEphemeralJob(randStr, 1, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    BusyboxImage,
						Command:                  []string{"sleep", "99999"},
						ImagePullPolicy:          v1.PullIfNotPresent,
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})
			ginkgo.By("Check the status of container")

			gomega.Eventually(func() int32 {
				ejob, err := tester.GetEphemeralJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return ejob.Status.Running
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			pods, err := cloneSetTester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))

			gomega.Eventually(func() int {
				return len(pods[0].Status.EphemeralContainerStatuses)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(1))

			oldPodUID := pods[0].UID
			oldEphemeralContainers := pods[0].Status.EphemeralContainerStatuses[0]

			ginkgo.By("Update image to new nginx")
			err = cloneSetTester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				if cs.Annotations == nil {
					cs.Annotations = map[string]string{}
				}
				cs.Spec.Template.Spec.Containers[0].Image = NewNginxImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for CloneSet generation consistent")
			gomega.Eventually(func() bool {
				cs, err = cloneSetTester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Generation == cs.Status.ObservedGeneration
			}, 10*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Wait for all pods updated and ready")
			gomega.Eventually(func() int32 {
				cs, err = cloneSetTester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.UpdatedReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Verify the ephemeral containerID not changed")
			pods, err = cloneSetTester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))
			newPodUID := pods[0].UID
			newEphemeralContainers := pods[0].Status.ContainerStatuses[0]

			gomega.Expect(oldPodUID).Should(gomega.Equal(newPodUID))
			gomega.Expect(newEphemeralContainers.ContainerID).NotTo(gomega.Equal(oldEphemeralContainers.ContainerID))
		})

		// This can't be Conformance yet.
		ginkgo.It("test ephemeral container in crr", func() {
			// create cloneset
			ginkgo.By("Create CloneSet " + randStr)
			cloneSetTester := framework.NewCloneSetTester(c, kc, ns)
			cs := cloneSetTester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			cs.Spec.Template.Spec.Containers[0].Image = NginxImage
			cs.Spec.Template.ObjectMeta.Labels["run"] = "nginx"
			cs, err := cloneSetTester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = cloneSetTester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = cloneSetTester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			// create ephemeral container
			ginkgo.By("Create EphemeralJob job-" + randStr)
			job := tester.CreateTestEphemeralJob(randStr, 1, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    BusyboxImage,
						Command:                  []string{"sleep", "99999"},
						ImagePullPolicy:          v1.PullIfNotPresent,
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})
			ginkgo.By("Check the status of container")

			gomega.Eventually(func() int32 {
				ejob, err := tester.GetEphemeralJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return ejob.Status.Running
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			pods, err := cloneSetTester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))

			gomega.Eventually(func() int {
				return len(pods[0].Status.EphemeralContainerStatuses)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(1))

			oldPodUID := pods[0].UID
			oldEphemeralContainers := pods[0].Status.EphemeralContainerStatuses[0]

			{
				restartContainerTester := framework.NewContainerRecreateTester(c, kc, ns)
				ginkgo.By("Create CRR for pods[0], recreate container: app")
				pod := pods[0]
				crr := &appsv1alpha1.ContainerRecreateRequest{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "crr-" + randStr + "-0"},
					Spec: appsv1alpha1.ContainerRecreateRequestSpec{
						PodName: pod.Name,
						Containers: []appsv1alpha1.ContainerRecreateRequestContainer{
							{Name: "nginx"},
						},
						Strategy:                &appsv1alpha1.ContainerRecreateRequestStrategy{MinStartedSeconds: 5},
						TTLSecondsAfterFinished: utilpointer.Int32Ptr(99999),
					},
				}
				crr, err = restartContainerTester.CreateCRR(crr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				// wait webhook
				gomega.Eventually(func() string {
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey]
				}, 60*time.Second, 3*time.Second).Should(gomega.Equal(string(pod.UID)))

				gomega.Expect(crr.Labels[appsv1alpha1.ContainerRecreateRequestNodeNameKey]).Should(gomega.Equal(pod.Spec.NodeName))
				gomega.Expect(crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]).Should(gomega.Equal("true"))
				gomega.Expect(crr.Spec.Strategy.FailurePolicy).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestFailurePolicyFail))
				gomega.Expect(crr.Spec.Containers[0].StatusContext.ContainerID).Should(gomega.Equal(pod.Status.ContainerStatuses[0].ContainerID))
				ginkgo.By("Wait CRR recreate completion")
				gomega.Eventually(func() appsv1alpha1.ContainerRecreateRequestPhase {
					crr, err = restartContainerTester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Status.Phase
				}, 70*time.Second, time.Second).Should(gomega.Equal(appsv1alpha1.ContainerRecreateRequestCompleted))
				gomega.Expect(crr.Status.CompletionTime).ShouldNot(gomega.BeNil())
				gomega.Eventually(func() string {
					crr, err = restartContainerTester.GetCRR(crr.Name)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return crr.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey]
				}, 5*time.Second, 1*time.Second).Should(gomega.Equal(""))
				gomega.Expect(crr.Status.ContainerRecreateStates).Should(gomega.Equal([]appsv1alpha1.ContainerRecreateRequestContainerRecreateState{{Name: "nginx", Phase: appsv1alpha1.ContainerRecreateRequestSucceeded, IsKilled: true}}))

				ginkgo.By("Check Pod containers recreated and started for minStartedSeconds")
				pod, err = restartContainerTester.GetPod(pod.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(podutil.IsPodReady(pod)).Should(gomega.Equal(true))
				gomega.Expect(pod.Status.ContainerStatuses[0].ContainerID).ShouldNot(gomega.Equal(crr.Spec.Containers[0].StatusContext.ContainerID))
				gomega.Expect(pod.Status.ContainerStatuses[0].RestartCount).Should(gomega.Equal(int32(1)))
				gomega.Expect(crr.Status.CompletionTime.Sub(pod.Status.ContainerStatuses[0].State.Running.StartedAt.Time)).Should(gomega.BeNumerically(">", 4*time.Second))
			}

			ginkgo.By("Verify the ephemeral containerID not changed")
			pods, err = cloneSetTester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))
			newPodUID := pods[0].UID
			newEphemeralContainers := pods[0].Status.ContainerStatuses[0]

			gomega.Expect(oldPodUID).Should(gomega.Equal(newPodUID))
			gomega.Expect(newEphemeralContainers.ContainerID).NotTo(gomega.Equal(oldEphemeralContainers.ContainerID))
		})

		// This can't be Conformance yet.
		ginkgo.It("test ephemeral container impact main container", func() {
			// create cloneset
			ginkgo.By("Create CloneSet " + randStr)
			cloneSetTester := framework.NewCloneSetTester(c, kc, ns)
			cs := cloneSetTester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			cs.Spec.Template.Spec.Containers[0].Image = NginxImage
			cs.Spec.Template.ObjectMeta.Labels["run"] = "nginx"
			cs, err := cloneSetTester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = cloneSetTester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = cloneSetTester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			pods, err := cloneSetTester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))

			oldPodUID := pods[0].UID
			oldContainers := pods[0].Status.ContainerStatuses[0]

			// create ephemeral container
			ginkgo.By("Create EphemeralJob job-" + randStr)
			job := tester.CreateTestEphemeralJob(randStr, 1, 1, metav1.LabelSelector{
				MatchLabels: map[string]string{
					"run": "nginx",
				}}, []v1.EphemeralContainer{
				{
					TargetContainerName: "nginx",
					EphemeralContainerCommon: v1.EphemeralContainerCommon{
						Name:                     "debugger",
						Image:                    BusyboxImage,
						Command:                  []string{"sleep", "99999"},
						ImagePullPolicy:          v1.PullIfNotPresent,
						TerminationMessagePolicy: v1.TerminationMessageReadFile,
					},
				}})
			ginkgo.By("Check the status of container")

			gomega.Eventually(func() int32 {
				ejob, err := tester.GetEphemeralJob(job.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return ejob.Status.Running
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Verify the main containerID not changed after ephemeral inject")
			pods, err = cloneSetTester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))
			newPodUID := pods[0].UID
			newContainers := pods[0].Status.ContainerStatuses[0]

			gomega.Expect(oldPodUID).Should(gomega.Equal(newPodUID))
			gomega.Expect(newContainers.ContainerID).Should(gomega.Equal(oldContainers.ContainerID))
		})
	})

})
