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
	"sort"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

var _ = SIGDescribe("CloneSet", func() {
	f := framework.NewDefaultFramework("clonesets")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.CloneSetTester
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewCloneSetTester(c, kc, ns)
		randStr = rand.String(10)
	})

	framework.KruiseDescribe("CloneSet Scaling", func() {
		var err error

		framework.ConformanceIt("scales in normal cases", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 3, appsv1alpha1.CloneSetUpdateStrategy{})
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.RecreateCloneSetUpdateStrategyType))
			gomega.Expect(cs.Spec.UpdateStrategy.MaxUnavailable).To(gomega.Equal(func() *intstr.IntOrString { i := intstr.FromString("20%"); return &i }()))

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(3)))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(3)))
		})

		framework.ConformanceIt("scales with minReadySeconds and scaleStrategy", func() {
			const replicas int32 = 4
			const scaleMaxUnavailable int32 = 1
			cs := tester.NewCloneSet("clone-"+randStr, replicas, appsv1alpha1.CloneSetUpdateStrategy{})
			cs.Spec.MinReadySeconds = 10
			cs.Spec.Template.Spec.Containers[0].ImagePullPolicy = "IfNotPresent"
			cs.Spec.ScaleStrategy.MaxUnavailable = &intstr.IntOrString{Type: intstr.Int, IntVal: scaleMaxUnavailable}
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 120*time.Second, time.Second).Should(gomega.Equal(replicas))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(replicas))

			ginkgo.By("check create time of pods")
			pods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sort.Slice(pods, func(i, j int) bool {
				return pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp)
			})
			allowFluctuation := 2 * time.Second
			for i := 1; i < int(replicas); i++ {
				lastPodCondition := podutil.GetPodReadyCondition(pods[i-1].Status)
				gomega.Expect(pods[i].CreationTimestamp.Sub(lastPodCondition.LastTransitionTime.Time) >= time.Duration(cs.Spec.MinReadySeconds)*time.Second).To(gomega.BeTrue())
				gomega.Expect(pods[i].CreationTimestamp.Sub(lastPodCondition.LastTransitionTime.Time) <= time.Duration(cs.Spec.MinReadySeconds)*time.Second+allowFluctuation).To(gomega.BeTrue())
			}
		})

		framework.ConformanceIt("pods should be ready when paused=true", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 3, appsv1alpha1.CloneSetUpdateStrategy{
				Type:   appsv1alpha1.RecreateCloneSetUpdateStrategyType,
				Paused: true,
			})
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(3)))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(3)))
		})
	})

	framework.KruiseDescribe("CloneSet Updating", func() {
		var err error

		// This can't be Conformance yet.
		ginkgo.It("in-place update images with the same imageID", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			imageConfig := imageutils.GetConfig(imageutils.Nginx)
			imageConfig.SetRegistry("docker.io/library")
			imageConfig.SetVersion("alpine")
			cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			pods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))
			oldPodUID := pods[0].UID
			oldContainerStatus := pods[0].Status.ContainerStatuses[0]

			ginkgo.By("Update image to nginx mainline-alpine")
			imageConfig.SetVersion("mainline-alpine")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				if cs.Annotations == nil {
					cs.Annotations = map[string]string{}
				}
				cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for CloneSet generation consistent")
			gomega.Eventually(func() bool {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Generation == cs.Status.ObservedGeneration
			}, 10*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Wait for all pods updated and ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.UpdatedReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Verify the containerID changed and imageID not changed")
			pods, err = tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))
			newPodUID := pods[0].UID
			newContainerStatus := pods[0].Status.ContainerStatuses[0]

			gomega.Expect(oldPodUID).Should(gomega.Equal(newPodUID))
			gomega.Expect(newContainerStatus.ContainerID).NotTo(gomega.Equal(oldContainerStatus.ContainerID))
			gomega.Expect(newContainerStatus.ImageID).Should(gomega.Equal(oldContainerStatus.ImageID))
		})

		// This can't be Conformance yet.
		ginkgo.It("in-place update both image and env from label", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			cs.Spec.Template.Spec.Containers[0].Image = NginxImage
			cs.Spec.Template.ObjectMeta.Labels["test-env"] = "foo"
			cs.Spec.Template.Spec.Containers[0].Env = append(cs.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
				Name:      "TEST_ENV",
				ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['test-env']"}},
			})
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			pods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))
			oldPodUID := pods[0].UID
			oldContainerStatus := pods[0].Status.ContainerStatuses[0]

			ginkgo.By("Update test-env label")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				if cs.Annotations == nil {
					cs.Annotations = map[string]string{}
				}
				cs.Spec.Template.ObjectMeta.Labels["test-env"] = "bar"
				cs.Spec.Template.Spec.Containers[0].Image = NewNginxImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for CloneSet generation consistent")
			gomega.Eventually(func() bool {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Generation == cs.Status.ObservedGeneration
			}, 10*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Wait for all pods updated and ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.UpdatedReadyReplicas
			}, 180*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Verify the containerID changed and restartCount should be 1")
			pods, err = tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))
			newPodUID := pods[0].UID
			newContainerStatus := pods[0].Status.ContainerStatuses[0]

			gomega.Expect(oldPodUID).Should(gomega.Equal(newPodUID))
			gomega.Expect(newContainerStatus.ContainerID).NotTo(gomega.Equal(oldContainerStatus.ContainerID))
			gomega.Expect(newContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
		})
	})

	framework.KruiseDescribe("CloneSet pre-download images", func() {
		var err error

		framework.ConformanceIt("pre-download for new image", func() {
			partition := intstr.FromInt(1)
			cs := tester.NewCloneSet("clone-"+randStr, 5, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType, Partition: &partition})
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))
			gomega.Expect(cs.Spec.UpdateStrategy.MaxUnavailable).To(gomega.Equal(func() *intstr.IntOrString { i := intstr.FromString("20%"); return &i }()))

			ginkgo.By("Wait for replicas satisfied")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(5)))

			ginkgo.By("Update image to new nginx")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				if cs.Annotations == nil {
					cs.Annotations = map[string]string{}
				}
				cs.Annotations[appsv1alpha1.ImagePreDownloadParallelismKey] = "2"
				cs.Spec.Template.Spec.Containers[0].Image = NewNginxImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Should get the ImagePullJob")
			var job *appsv1alpha1.ImagePullJob
			gomega.Eventually(func() int {
				jobs, err := tester.ListImagePullJobsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if len(jobs) > 0 {
					job = jobs[0]
				}
				return len(jobs)
			}, 3*time.Second, time.Second).Should(gomega.Equal(1))

			ginkgo.By("Check the ImagePullJob spec and status")
			gomega.Expect(job.Spec.Image).To(gomega.Equal(NewNginxImage))
			gomega.Expect(job.Spec.Parallelism.IntValue()).To(gomega.Equal(2))
		})
	})
})
