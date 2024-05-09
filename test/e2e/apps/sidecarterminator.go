package apps

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
)

var _ = SIGDescribe("SidecarTerminator", func() {
	f := framework.NewDefaultFramework("sidecarterminator")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		randStr = rand.String(10)
	})

	framework.KruiseDescribe("SidecarTerminator checker", func() {
		ginkgo.It("job and broadcast job with sidecar", func() {
			mainContainer := v1.Container{
				Name:            "main",
				Image:           "busybox:latest",
				ImagePullPolicy: v1.PullIfNotPresent,
				Command:         []string{"/bin/sh", "-c", "sleep 5"},
			}
			sidecarContainer := v1.Container{
				Name:            "sidecar",
				Image:           "nginx:latest",
				ImagePullPolicy: v1.PullIfNotPresent,
				Env: []v1.EnvVar{
					{
						Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
						Value: "true",
					},
				},
			}
			sidecarContainerNeverStop := v1.Container{
				Name:            "sidecar",
				Image:           "busybox:latest",
				ImagePullPolicy: v1.PullIfNotPresent,
				Command:         []string{"/bin/sh", "-c", "sleep 10000"},
				Env: []v1.EnvVar{
					{
						Name:  appsv1alpha1.KruiseTerminateSidecarEnv,
						Value: "true",
					},
				},
			}

			cases := []struct {
				name        string
				createJob   func(str string) metav1.Object
				checkStatus func(object metav1.Object) bool
			}{
				{
					name: "native job, restartPolicy=Never",
					createJob: func(str string) metav1.Object {
						job := &batchv1.Job{
							ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
							Spec: batchv1.JobSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											*mainContainer.DeepCopy(),
											*sidecarContainer.DeepCopy(),
										},
										RestartPolicy: v1.RestartPolicyNever,
									},
								},
							},
						}
						job, err := c.BatchV1().Jobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job
					},
					checkStatus: func(object metav1.Object) bool {
						job, err := c.BatchV1().Jobs(object.GetNamespace()).
							Get(context.TODO(), object.GetName(), metav1.GetOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						for _, cond := range job.Status.Conditions {
							if cond.Type == batchv1.JobComplete {
								return true
							}
						}
						return false
					},
				},
				{
					name: "native job, restartPolicy=Never, main succeeded, sidecar never stop",
					createJob: func(str string) metav1.Object {
						job := &batchv1.Job{
							ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
							Spec: batchv1.JobSpec{
								Template: v1.PodTemplateSpec{
									ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"with-sidecar-neverstop": "true"}},
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											*mainContainer.DeepCopy(),
											*sidecarContainerNeverStop.DeepCopy(),
										},
										RestartPolicy: v1.RestartPolicyNever,
									},
								},
							},
						}
						job, err := c.BatchV1().Jobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job
					},
					checkStatus: func(object metav1.Object) bool {
						// assert job status
						job, err := c.BatchV1().Jobs(object.GetNamespace()).
							Get(context.TODO(), object.GetName(), metav1.GetOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						for _, cond := range job.Status.Conditions {
							if cond.Type == batchv1.JobComplete {
								return true
							}
						}
						// assert pod status
						pods, err := c.CoreV1().Pods(object.GetNamespace()).List(context.TODO(), metav1.ListOptions{LabelSelector: "with-sidecar-neverstop=true"})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						if len(pods.Items) == 1 && pods.Items[0].Status.Phase == v1.PodSucceeded {
							return true
						}
						return false
					},
				},
				{
					name: "native job, restartPolicy=OnFailure, main failed",
					createJob: func(str string) metav1.Object {
						job := &batchv1.Job{
							ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
							Spec: batchv1.JobSpec{
								BackoffLimit: pointer.Int32Ptr(3),
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											func() v1.Container {
												main := mainContainer.DeepCopy()
												main.Command = []string{"/bin/sh", "-c", "exit 1"}
												return *main
											}(),
											*sidecarContainer.DeepCopy(),
										},
										RestartPolicy: v1.RestartPolicyOnFailure,
									},
								},
							},
						}
						job, err := c.BatchV1().Jobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job
					},
					checkStatus: func(object metav1.Object) bool {
						job, err := c.BatchV1().Jobs(object.GetNamespace()).
							Get(context.TODO(), object.GetName(), metav1.GetOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						for _, cond := range job.Status.Conditions {
							if cond.Type == batchv1.JobFailed {
								return true
							}
						}
						return false
					},
				},
				{
					name: "native job, restartPolicy=OnFailure, main succeeded",
					createJob: func(str string) metav1.Object {
						job := &batchv1.Job{
							ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
							Spec: batchv1.JobSpec{
								BackoffLimit: pointer.Int32Ptr(3),
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											*mainContainer.DeepCopy(),
											*sidecarContainer.DeepCopy(),
										},
										RestartPolicy: v1.RestartPolicyOnFailure,
									},
								},
							},
						}
						job, err := c.BatchV1().Jobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job
					},
					checkStatus: func(object metav1.Object) bool {
						job, err := c.BatchV1().Jobs(object.GetNamespace()).
							Get(context.TODO(), object.GetName(), metav1.GetOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						for _, cond := range job.Status.Conditions {
							if cond.Type == batchv1.JobComplete {
								return true
							}
						}
						return false
					},
				},
				{
					name: "BroadcastJob, restartPolicy=Never",
					createJob: func(str string) metav1.Object {
						job := &appsv1alpha1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
							Spec: appsv1alpha1.BroadcastJobSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											*mainContainer.DeepCopy(),
											*sidecarContainer.DeepCopy(),
										},
										RestartPolicy: v1.RestartPolicyNever,
									},
								},
								CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
							},
						}
						job, err := kc.AppsV1alpha1().BroadcastJobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job
					},
					checkStatus: func(object metav1.Object) bool {
						job, err := kc.AppsV1alpha1().BroadcastJobs(object.GetNamespace()).
							Get(context.TODO(), object.GetName(), metav1.GetOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job.Status.Phase == appsv1alpha1.PhaseCompleted
					},
				},
				{
					name: "BroadcastJob, restartPolicy=OnFailure, main failed",
					createJob: func(str string) metav1.Object {
						job := &appsv1alpha1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
							Spec: appsv1alpha1.BroadcastJobSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											func() v1.Container {
												main := mainContainer.DeepCopy()
												main.Command = []string{"/bin/sh", "-c", "exit 1"}
												return *main
											}(),
											*sidecarContainer.DeepCopy(),
										},
										RestartPolicy: v1.RestartPolicyOnFailure,
									},
								},
								CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
							},
						}
						job, err := kc.AppsV1alpha1().BroadcastJobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job
					},
					checkStatus: func(object metav1.Object) bool {
						job, err := kc.AppsV1alpha1().BroadcastJobs(object.GetNamespace()).
							Get(context.TODO(), object.GetName(), metav1.GetOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job.Status.Phase == appsv1alpha1.PhaseFailed
					},
				},
				{
					name: "BroadcastJob, restartPolicy=OnFailure, main succeeded",
					createJob: func(str string) metav1.Object {
						job := &appsv1alpha1.BroadcastJob{
							ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
							Spec: appsv1alpha1.BroadcastJobSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											*mainContainer.DeepCopy(),
											*sidecarContainer.DeepCopy(),
										},
										RestartPolicy: v1.RestartPolicyOnFailure,
									},
								},
								CompletionPolicy: appsv1alpha1.CompletionPolicy{Type: appsv1alpha1.Always},
							},
						}
						job, err := kc.AppsV1alpha1().BroadcastJobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job
					},
					checkStatus: func(object metav1.Object) bool {
						job, err := kc.AppsV1alpha1().BroadcastJobs(object.GetNamespace()).
							Get(context.TODO(), object.GetName(), metav1.GetOptions{})
						gomega.Expect(err).NotTo(gomega.HaveOccurred())
						return job.Status.Phase == appsv1alpha1.PhaseCompleted
					},
				},
			}

			for i := range cases {
				ginkgo.By(cases[i].name)
				ginkgo.By("Create job-" + randStr)
				job := cases[i].createJob(fmt.Sprintf("%v-%v", randStr, i))
				time.Sleep(time.Second)
				gomega.Eventually(func() bool {
					return cases[i].checkStatus(job)
				}, 10*time.Minute, time.Second).Should(gomega.BeTrue())
			}
		})
		/*
			ginkgo.It("use in-place update strategy to kill containers", func() {
				mainContainer := v1.Container{
					Name:            "main",
					Image:           "busybox:latest",
					ImagePullPolicy: v1.PullIfNotPresent,
					Command:         []string{"/bin/sh", "-c", "sleep 5"},
				}
				sidecarContainer := v1.Container{
					Name:            "sidecar",
					Image:           "nginx:latest",
					ImagePullPolicy: v1.PullIfNotPresent,
					Env: []v1.EnvVar{
						{
							Name:  stutil.KruiseTerminateSidecarEnv,
							Value: stutil.InplaceUpdateONLYStrategy,
						},
						{
							Name:  stutil.KruiseTerminateSidecarWithImageEnv,
							Value: "minchou/exit:v1.0",
						},
						{
							Name:  stutil.KruiseTerminateSidecarWithDownwardAPI,
							Value: "true",
						},
					},
				}

				cases := []struct {
					name        string
					createJob   func(str string) metav1.Object
					checkStatus func(object metav1.Object) bool
				}{
					{
						name: "native job, no command",
						createJob: func(str string) metav1.Object {
							job := &batchv1.Job{
								ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
								Spec: batchv1.JobSpec{
									Template: v1.PodTemplateSpec{
										Spec: v1.PodSpec{
											Containers: []v1.Container{
												*mainContainer.DeepCopy(),
												*sidecarContainer.DeepCopy(),
											},
											RestartPolicy: v1.RestartPolicyNever,
										},
									},
								},
							}
							fmt.Println("Batch Job")
							job, err := c.BatchV1().Jobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
							gomega.Expect(err).NotTo(gomega.HaveOccurred())
							return job
						},
						checkStatus: func(object metav1.Object) bool {
							job, err := c.BatchV1().Jobs(object.GetNamespace()).
								Get(context.TODO(), object.GetName(), metav1.GetOptions{})
							gomega.Expect(err).NotTo(gomega.HaveOccurred())
							for _, cond := range job.Status.Conditions {
								if cond.Type == batchv1.JobComplete {
									return true
								}
							}
							return false
						},
					},
					{
						name: "native job, 3 commands",
						createJob: func(str string) metav1.Object {
							job := &batchv1.Job{
								ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
								Spec: batchv1.JobSpec{
									BackoffLimit: pointer.Int32Ptr(3),
									Template: v1.PodTemplateSpec{
										Spec: v1.PodSpec{
											Containers: []v1.Container{
												func() v1.Container {
													main := mainContainer.DeepCopy()
													main.Command = []string{"/bin/sh", "-c", "exit 1"}
													return *main
												}(),
												func() v1.Container {
													sidecar := sidecarContainer.DeepCopy()
													sidecar.Command = []string{"/bin/sh", "-c", "sleep 10000"}
													return *sidecar
												}(),
											},
											RestartPolicy: v1.RestartPolicyOnFailure,
										},
									},
								},
							}
							job, err := c.BatchV1().Jobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
							gomega.Expect(err).NotTo(gomega.HaveOccurred())
							return job
						},
						checkStatus: func(object metav1.Object) bool {
							job, err := c.BatchV1().Jobs(object.GetNamespace()).
								Get(context.TODO(), object.GetName(), metav1.GetOptions{})
							gomega.Expect(err).NotTo(gomega.HaveOccurred())
							for _, cond := range job.Status.Conditions {
								if cond.Type == batchv1.JobFailed {
									return true
								}
							}
							return false
						},
					},
					{
						name: "native job, 4 commands",
						createJob: func(str string) metav1.Object {
							job := &batchv1.Job{
								ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "job-" + str},
								Spec: batchv1.JobSpec{
									BackoffLimit: pointer.Int32Ptr(3),
									Template: v1.PodTemplateSpec{
										Spec: v1.PodSpec{
											Containers: []v1.Container{
												*mainContainer.DeepCopy(),
												func() v1.Container {
													sidecar := sidecarContainer.DeepCopy()
													sidecar.Command = []string{"/bin/sh", "-c", "sleep 10000"}
													sidecar.Args = []string{`echo "hello"`}
													return *sidecar
												}(),
											},
											RestartPolicy: v1.RestartPolicyOnFailure,
										},
									},
								},
							}
							job, err := c.BatchV1().Jobs(ns).Create(context.TODO(), job, metav1.CreateOptions{})
							gomega.Expect(err).NotTo(gomega.HaveOccurred())
							return job
						},
						checkStatus: func(object metav1.Object) bool {
							job, err := c.BatchV1().Jobs(object.GetNamespace()).
								Get(context.TODO(), object.GetName(), metav1.GetOptions{})
							gomega.Expect(err).NotTo(gomega.HaveOccurred())
							for _, cond := range job.Status.Conditions {
								if cond.Type == batchv1.JobComplete {
									return true
								}
							}
							return false
						},
					},
				}

				for i := range cases {
					ginkgo.By(cases[i].name)
					ginkgo.By("Create job-" + randStr)
					job := cases[i].createJob(fmt.Sprintf("%v-%v", randStr, i))
					time.Sleep(time.Second)
					gomega.Eventually(func() bool {
						return cases[i].checkStatus(job)
					}, 5*time.Minute, time.Second).Should(gomega.BeTrue())
				}
			})
		*/
	})
})
