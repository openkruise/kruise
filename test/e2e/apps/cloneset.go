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
	"encoding/json"
	"fmt"
	"sort"
	"time"

	utilpointer "k8s.io/utils/pointer"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
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

		framework.ConformanceIt("in-place update two container images with priorities successfully", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			cs.Spec.Template.Spec.Containers = append(cs.Spec.Template.Spec.Containers, v1.Container{
				Name:      "redis",
				Image:     RedisImage,
				Command:   []string{"sleep", "999"},
				Env:       []v1.EnvVar{{Name: appspub.ContainerLaunchPriorityEnvName, Value: "10"}},
				Lifecycle: &v1.Lifecycle{PostStart: &v1.Handler{Exec: &v1.ExecAction{Command: []string{"sleep", "10"}}}},
			})
			cs.Spec.Template.Spec.TerminationGracePeriodSeconds = utilpointer.Int64(3)
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

			ginkgo.By("Update images of nginx and redis")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Template.Spec.Containers[0].Image = NewNginxImage
				cs.Spec.Template.Spec.Containers[1].Image = imageutils.GetE2EImage(imageutils.BusyBox)
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

			ginkgo.By("Verify two containers have all updated in-place")
			pods, err = tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))

			pod := pods[0]
			nginxContainerStatus := util.GetContainerStatus("nginx", pod)
			redisContainerStatus := util.GetContainerStatus("redis", pod)
			gomega.Expect(nginxContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
			gomega.Expect(redisContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))

			ginkgo.By("Verify nginx should be stopped after new redis has started 10s")
			gomega.Expect(nginxContainerStatus.LastTerminationState.Terminated.FinishedAt.After(redisContainerStatus.State.Running.StartedAt.Time.Add(time.Second*10))).
				Should(gomega.Equal(true), fmt.Sprintf("nginx finish at %v is not after redis start %v + 10s",
					nginxContainerStatus.LastTerminationState.Terminated.FinishedAt,
					redisContainerStatus.State.Running.StartedAt))

			ginkgo.By("Verify in-place update state in two batches")
			inPlaceUpdateState := appspub.InPlaceUpdateState{}
			gomega.Expect(pod.Annotations[appspub.InPlaceUpdateStateKey]).ShouldNot(gomega.BeEmpty())
			err = json.Unmarshal([]byte(pod.Annotations[appspub.InPlaceUpdateStateKey]), &inPlaceUpdateState)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(inPlaceUpdateState.ContainerBatchesRecord)).Should(gomega.Equal(2))
			gomega.Expect(inPlaceUpdateState.ContainerBatchesRecord[0].Containers).Should(gomega.Equal([]string{"redis"}))
			gomega.Expect(inPlaceUpdateState.ContainerBatchesRecord[1].Containers).Should(gomega.Equal([]string{"nginx"}))
		})

		framework.ConformanceIt("in-place update two container images with priorities, should not update the next when the previous one failed", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			cs.Spec.Template.Spec.Containers = append(cs.Spec.Template.Spec.Containers, v1.Container{
				Name:      "redis",
				Image:     RedisImage,
				Env:       []v1.EnvVar{{Name: appspub.ContainerLaunchPriorityEnvName, Value: "10"}},
				Lifecycle: &v1.Lifecycle{PostStart: &v1.Handler{Exec: &v1.ExecAction{Command: []string{"sleep", "10"}}}},
			})
			cs.Spec.Template.Spec.TerminationGracePeriodSeconds = utilpointer.Int64(3)
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

			ginkgo.By("Update images of nginx and redis")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Template.Spec.Containers[0].Image = NewNginxImage
				cs.Spec.Template.Spec.Containers[1].Image = imageutils.GetE2EImage(imageutils.BusyBox)
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for CloneSet generation consistent")
			gomega.Eventually(func() bool {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Generation == cs.Status.ObservedGeneration
			}, 10*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Wait for redis failed to start")
			var pod *v1.Pod
			gomega.Eventually(func() *v1.ContainerStateTerminated {
				pods, err = tester.ListPodsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(len(pods)).Should(gomega.Equal(1))
				pod = pods[0]
				redisContainerStatus := util.GetContainerStatus("redis", pod)
				return redisContainerStatus.LastTerminationState.Terminated
			}, 60*time.Second, time.Second).ShouldNot(gomega.BeNil())

			gomega.Eventually(func() *v1.ContainerStateWaiting {
				pods, err = tester.ListPodsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(len(pods)).Should(gomega.Equal(1))
				pod = pods[0]
				redisContainerStatus := util.GetContainerStatus("redis", pod)
				return redisContainerStatus.State.Waiting
			}, 60*time.Second, time.Second).ShouldNot(gomega.BeNil())

			nginxContainerStatus := util.GetContainerStatus("nginx", pod)
			gomega.Expect(nginxContainerStatus.RestartCount).Should(gomega.Equal(int32(0)))

			ginkgo.By("Verify in-place update state only one batch and remain next")
			inPlaceUpdateState := appspub.InPlaceUpdateState{}
			gomega.Expect(pod.Annotations[appspub.InPlaceUpdateStateKey]).ShouldNot(gomega.BeEmpty())
			err = json.Unmarshal([]byte(pod.Annotations[appspub.InPlaceUpdateStateKey]), &inPlaceUpdateState)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(inPlaceUpdateState.ContainerBatchesRecord)).Should(gomega.Equal(1))
			gomega.Expect(inPlaceUpdateState.ContainerBatchesRecord[0].Containers).Should(gomega.Equal([]string{"redis"}))
			gomega.Expect(inPlaceUpdateState.NextContainerImages).Should(gomega.Equal(map[string]string{"nginx": NewNginxImage}))
		})

		// This can't be Conformance yet.
		ginkgo.It("in-place update two container image and env from metadata with priorities", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			cs.Spec.Template.Annotations = map[string]string{"config": "foo"}
			cs.Spec.Template.Spec.Containers = append(cs.Spec.Template.Spec.Containers, v1.Container{
				Name:  "redis",
				Image: RedisImage,
				Env: []v1.EnvVar{
					{Name: appspub.ContainerLaunchPriorityEnvName, Value: "10"},
					{Name: "CONFIG", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.annotations['config']"}}},
				},
				Lifecycle: &v1.Lifecycle{PostStart: &v1.Handler{Exec: &v1.ExecAction{Command: []string{"sleep", "10"}}}},
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

			ginkgo.By("Update nginx image and config annotation")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Template.Spec.Containers[0].Image = NewNginxImage
				cs.Spec.Template.Annotations["config"] = "bar"
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

			ginkgo.By("Verify two containers have all updated in-place")
			pods, err = tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(1))

			pod := pods[0]
			nginxContainerStatus := util.GetContainerStatus("nginx", pod)
			redisContainerStatus := util.GetContainerStatus("redis", pod)
			gomega.Expect(nginxContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))
			gomega.Expect(redisContainerStatus.RestartCount).Should(gomega.Equal(int32(1)))

			ginkgo.By("Verify nginx should be stopped after new redis has started")
			gomega.Expect(nginxContainerStatus.LastTerminationState.Terminated.FinishedAt.After(redisContainerStatus.State.Running.StartedAt.Time.Add(time.Second*10))).
				Should(gomega.Equal(true), fmt.Sprintf("nginx finish at %v is not after redis start %v + 10s",
					nginxContainerStatus.LastTerminationState.Terminated.FinishedAt,
					redisContainerStatus.State.Running.StartedAt))

			ginkgo.By("Verify in-place update state in two batches")
			inPlaceUpdateState := appspub.InPlaceUpdateState{}
			gomega.Expect(pod.Annotations[appspub.InPlaceUpdateStateKey]).ShouldNot(gomega.BeEmpty())
			err = json.Unmarshal([]byte(pod.Annotations[appspub.InPlaceUpdateStateKey]), &inPlaceUpdateState)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(inPlaceUpdateState.ContainerBatchesRecord)).Should(gomega.Equal(2))
			gomega.Expect(inPlaceUpdateState.ContainerBatchesRecord[0].Containers).Should(gomega.Equal([]string{"redis"}))
			gomega.Expect(inPlaceUpdateState.ContainerBatchesRecord[1].Containers).Should(gomega.Equal([]string{"nginx"}))
		})

		ginkgo.It(`CloneSet partition="99%", replicas=4, make sure one pod is upgraded`, func() {
			updateStrategy := appsv1alpha1.CloneSetUpdateStrategy{
				Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
				MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
				Partition:      &intstr.IntOrString{Type: intstr.String, StrVal: "99%"},
			}
			cs := tester.NewCloneSet("clone-"+randStr, 4, updateStrategy)
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
			}, 3*time.Second, time.Second).Should(gomega.Equal(int32(4)))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(4)))

			pods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(4))

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

			ginkgo.By("Wait for one pods updated")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.UpdatedReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			time.Sleep(10 * time.Second)
			ginkgo.By("Wait for one pods updated, check again after 10s")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.UpdatedReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Wait for one pods updated and ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.UpdatedReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			ginkgo.By("Check expectedPartitionReplicas filed")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ExpectedUpdatedReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))
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
