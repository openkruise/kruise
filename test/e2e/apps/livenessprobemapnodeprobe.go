package apps

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	livenessprobeUtils "github.com/openkruise/kruise/pkg/util/livenessprobe"
	"github.com/openkruise/kruise/test/e2e/framework"
)

// e2e test for enhanced liveness probe map node probe
var _ = SIGDescribe("EnhancedLivenessProbeMapNodeProbe", func() {
	f := framework.NewDefaultFramework("enhanced-livenessprobe-map-modeprobe")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.ELivenessProbeMapNodeProbeTester
	var pods []*v1.Pod
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewELivenessProbeMapNodeProbeTester(c, kc, ns)
		randStr = rand.String(10)
	})

	ginkgo.AfterEach(func() {
		err := tester.CleanAllTestResources()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	framework.KruiseDescribe("Create pod and check the related nodePodProbe object", func() {

		framework.ConformanceIt("case:1 Create one pod with liveness probe config and check the successful nodePodProbe generated", func() {
			ginkgo.By("New a new container with liveness probe config")
			containerLivenessProbeConfig := v1.Probe{
				ProbeHandler: v1.ProbeHandler{
					Exec: &v1.ExecAction{
						Command: []string{"/bin/sh", "-c", "/healthy.sh"},
					},
				},
				InitialDelaySeconds: 1000,
				TimeoutSeconds:      5,
				PeriodSeconds:       100,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			}
			cName := "app"
			containersProbe := []livenessprobeUtils.ContainerLivenessProbe{
				{
					Name:          cName,
					LivenessProbe: containerLivenessProbeConfig,
				},
			}
			expectContainerProbeConfigRaw, err := json.Marshal(containersProbe)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Create CloneSet and wait 1 pod ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 1, map[string]string{
				alpha1.AnnotationUsingEnhancedLiveness: "true",
			}, nil, []v1.Container{
				{
					Name:          cName,
					Image:         WebserverImage,
					LivenessProbe: &containerLivenessProbeConfig,
				},
			})
			ginkgo.By(fmt.Sprintf("Assert pod annotation %v", alpha1.AnnotationNativeContainerProbeContext))
			pod := pods[0]
			gomega.Eventually(func() string {
				return pod.Annotations[alpha1.AnnotationNativeContainerProbeContext]
			}, 5*time.Second, 1*time.Second).Should(gomega.Equal(string(expectContainerProbeConfigRaw)))

			ginkgo.By("Wait NodePodProbe create completion")
			gomega.Eventually(func() *appsv1alpha1.NodePodProbe {
				gotNodePodProbe, err := tester.GetNodePodProbe(pod.Spec.NodeName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return gotNodePodProbe
			}, 70*time.Second, time.Second).ShouldNot(gomega.BeNil())

			ginkgo.By("Check the generated nodePodProbe config")
			expectNodePodProbeSpec := []appsv1alpha1.PodProbe{
				{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					Probes: []appsv1alpha1.ContainerProbe{
						{
							ContainerName: cName,
							Name:          fmt.Sprintf("%v-%v", pod.Name, cName),
							Probe: appsv1alpha1.ContainerProbeSpec{
								Probe: v1.Probe{
									ProbeHandler: v1.ProbeHandler{
										Exec: &v1.ExecAction{
											Command: []string{"/bin/sh", "-c", "/healthy.sh"},
										},
									},
									InitialDelaySeconds: 1000,
									TimeoutSeconds:      5,
									PeriodSeconds:       100,
									SuccessThreshold:    1,
									FailureThreshold:    3,
								},
							},
						},
					},
					UID: fmt.Sprintf("%v", pod.UID),
				},
			}
			gomega.Eventually(func() string {
				gotNodePodProbe, err := tester.GetNodePodProbe(pod.Spec.NodeName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				podProbe := gotNodePodProbe.Spec.PodProbes
				return util.DumpJSON(podProbe)
			}, 70*time.Second, time.Second).Should(gomega.Equal(util.DumpJSON(expectNodePodProbeSpec)))
		})

		framework.ConformanceIt("case:2 Create one pod with liveness probe config and update the", func() {
			ginkgo.By("New a new container with liveness probe config")
			containerLivenessProbeConfigC1 := v1.Probe{
				ProbeHandler: v1.ProbeHandler{
					Exec: &v1.ExecAction{
						Command: []string{"/bin/sh", "-c", "/healthy.sh"},
					},
				},
				InitialDelaySeconds: 1000,
				TimeoutSeconds:      5,
				PeriodSeconds:       100,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			}
			cNameC1 := "app"
			containersProbe := []livenessprobeUtils.ContainerLivenessProbe{
				{
					Name:          cNameC1,
					LivenessProbe: containerLivenessProbeConfigC1,
				},
			}
			expectContainerProbeConfigRaw, err := json.Marshal(containersProbe)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Create CloneSet and wait 1 pod ready")
			pods = tester.CreateTestCloneSetAndGetPods(randStr, 1, map[string]string{
				alpha1.AnnotationUsingEnhancedLiveness: "true",
			}, nil, []v1.Container{
				{
					Name:          cNameC1,
					Image:         WebserverImage,
					LivenessProbe: &containerLivenessProbeConfigC1,
				},
			})
			ginkgo.By(fmt.Sprintf("Assert pod annotation %v", alpha1.AnnotationNativeContainerProbeContext))
			pod1 := pods[0]
			podNodeName := pod1.Spec.NodeName
			gomega.Eventually(func() string {
				return pod1.Annotations[alpha1.AnnotationNativeContainerProbeContext]
			}, 5*time.Second, 1*time.Second).Should(gomega.Equal(string(expectContainerProbeConfigRaw)))

			ginkgo.By("Wait NodePodProbe create completion")
			gomega.Eventually(func() *appsv1alpha1.NodePodProbe {
				gotNodePodProbe, err := tester.GetNodePodProbe(pod1.Spec.NodeName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return gotNodePodProbe
			}, 70*time.Second, time.Second).ShouldNot(gomega.BeNil())

			ginkgo.By("Check the generated nodePodProbe config")
			expectNodePodProbeSpec := []appsv1alpha1.PodProbe{
				{
					Name:      pod1.Name,
					Namespace: pod1.Namespace,
					Probes: []appsv1alpha1.ContainerProbe{
						{
							ContainerName: cNameC1,
							Name:          fmt.Sprintf("%v-%v", pod1.Name, cNameC1),
							Probe: appsv1alpha1.ContainerProbeSpec{
								Probe: v1.Probe{
									ProbeHandler: v1.ProbeHandler{
										Exec: &v1.ExecAction{
											Command: []string{"/bin/sh", "-c", "/healthy.sh"},
										},
									},
									InitialDelaySeconds: 1000,
									TimeoutSeconds:      5,
									PeriodSeconds:       100,
									SuccessThreshold:    1,
									FailureThreshold:    3,
								},
							},
						},
					},
					UID: fmt.Sprintf("%v", pod1.UID),
				},
			}
			gomega.Eventually(func() string {
				gotNodePodProbe, err := tester.GetNodePodProbe(pod1.Spec.NodeName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				podProbe := gotNodePodProbe.Spec.PodProbes
				return util.DumpJSON(podProbe)
			}, 70*time.Second, time.Second).Should(gomega.Equal(util.DumpJSON(expectNodePodProbeSpec)))

			ginkgo.By("Create the other CloneSet with specified nodeName and wait one pod ready")
			containerLivenessProbeConfigC2 := v1.Probe{
				ProbeHandler: v1.ProbeHandler{
					Exec: &v1.ExecAction{
						Command: []string{"/bin/sh", "-c", "/liveness.sh"},
					},
				},
				InitialDelaySeconds: 2000,
				TimeoutSeconds:      5,
				PeriodSeconds:       200,
				SuccessThreshold:    1,
				FailureThreshold:    3,
			}
			cNameC2 := "app-c2"
			containersProbe = []livenessprobeUtils.ContainerLivenessProbe{
				{
					Name:          cNameC2,
					LivenessProbe: containerLivenessProbeConfigC2,
				},
			}
			expectContainerProbeConfigRaw, err = json.Marshal(containersProbe)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Create CloneSet and wait 1 pod ready")
			randStr = rand.String(10)
			pods = tester.CreateTestCloneSetAffinityNodeAndGetPods(randStr, 1, map[string]string{
				alpha1.AnnotationUsingEnhancedLiveness: "true",
			}, nil, []v1.Container{
				{
					Name:          cNameC2,
					Image:         WebserverImage,
					LivenessProbe: &containerLivenessProbeConfigC2,
				},
			}, podNodeName)
			ginkgo.By(fmt.Sprintf("Assert pod annotation %v", alpha1.AnnotationNativeContainerProbeContext))

			ginkgo.By("Wait NodePodProbe update completion")
			pod2 := pods[0]
			gomega.Eventually(func() *appsv1alpha1.NodePodProbe {
				gotNodePodProbe, err := tester.GetNodePodProbe(pod2.Spec.NodeName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return gotNodePodProbe
			}, 70*time.Second, time.Second).ShouldNot(gomega.BeNil())

			ginkgo.By("Check the generated nodePodProbe config")
			expectNodePodProbeSpec = append(expectNodePodProbeSpec, appsv1alpha1.PodProbe{
				Name:      pod2.Name,
				Namespace: pod2.Namespace,
				Probes: []appsv1alpha1.ContainerProbe{
					{
						ContainerName: cNameC2,
						Name:          fmt.Sprintf("%v-%v", pod2.Name, cNameC2),
						Probe: appsv1alpha1.ContainerProbeSpec{
							Probe: v1.Probe{
								ProbeHandler: v1.ProbeHandler{
									Exec: &v1.ExecAction{
										Command: []string{"/bin/sh", "-c", "/liveness.sh"},
									},
								},
								InitialDelaySeconds: 2000,
								TimeoutSeconds:      5,
								PeriodSeconds:       200,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
						},
					},
				},
				UID: fmt.Sprintf("%v", pod2.UID),
			})
			gomega.Eventually(func() string {
				gotNodePodProbe, err := tester.GetNodePodProbe(pod2.Spec.NodeName)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				podProbe := gotNodePodProbe.Spec.PodProbes
				return util.DumpJSON(podProbe)
			}, 70*time.Second, time.Second).Should(gomega.Equal(util.DumpJSON(expectNodePodProbeSpec)))
		})
	})
})
