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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	imageutils "k8s.io/kubernetes/test/utils/image"
	utilpointer "k8s.io/utils/pointer"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework"
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

		framework.ConformanceIt("specific delete a Pod, when scalingExcludePreparingDelete is disabled", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 3, appsv1alpha1.CloneSetUpdateStrategy{})
			cs.Spec.Template.Labels["lifecycle-hook"] = "true"
			cs.Spec.Lifecycle = &appspub.Lifecycle{
				PreDelete: &appspub.LifecycleHook{
					LabelsHandler: map[string]string{"lifecycle-hook": "true"},
				},
			}
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

			oldPods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(oldPods).To(gomega.HaveLen(int(cs.Status.Replicas)))

			specifiedPodName := oldPods[0].Name
			ginkgo.By(fmt.Sprintf("Patch Pod %s with specified-delete label", specifiedPodName))
			patchBody := []byte(fmt.Sprintf(`{"metadata":{"labels":{"%s":"true"}}}`, appsv1alpha1.SpecifiedDeleteKey))
			_, err = c.CoreV1().Pods(cs.Namespace).Patch(context.TODO(), specifiedPodName, types.StrategicMergePatchType, patchBody, metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait specified pod becoming PreparingDelete")
			gomega.Eventually(func() appspub.LifecycleStateType {
				pod, err := c.CoreV1().Pods(cs.Namespace).Get(context.TODO(), specifiedPodName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return appspub.LifecycleStateType(pod.Labels[appspub.LifecycleStateKey])
			}, 10*time.Second, time.Second).Should(gomega.Equal(appspub.LifecycleStatePreparingDelete))

			ginkgo.By("Should not scale up")
			gomega.Consistently(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(3)))

			newPods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(util.GetPodNames(newPods).List()).Should(gomega.Equal(util.GetPodNames(newPods).List()))

			ginkgo.By("Remove lifecycle hook label and wait it to be deleted")
			patchBody = []byte(`{"metadata":{"labels":{"lifecycle-hook":null}}}`)
			_, err = c.CoreV1().Pods(cs.Namespace).Patch(context.TODO(), specifiedPodName, types.StrategicMergePatchType, patchBody, metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() *v1.Pod {
				pod, err := c.CoreV1().Pods(cs.Namespace).Get(context.TODO(), specifiedPodName, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				}
				return pod
			}, 30*time.Second, time.Second).Should(gomega.BeNil())

			ginkgo.By("Wait new Pod created and two old Pods should be still running")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, 3*time.Second).Should(gomega.Equal(int32(3)))

			newPods, err = tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(newPods).To(gomega.HaveLen(int(cs.Status.Replicas)))

			keepOldPods := util.GetPodNames(newPods).Intersection(util.GetPodNames(oldPods)).List()
			gomega.Expect(keepOldPods).To(gomega.HaveLen(2))
		})

		framework.ConformanceIt("specific scale down with lifecycle and then scale up, when scalingExcludePreparingDelete is enabled", func() {
			cs := tester.NewCloneSet("clone-"+randStr, 3, appsv1alpha1.CloneSetUpdateStrategy{})
			cs.Labels = map[string]string{appsv1alpha1.CloneSetScalingExcludePreparingDeleteKey: "true"}
			cs.Spec.Template.Labels["lifecycle-hook"] = "true"
			cs.Spec.Lifecycle = &appspub.Lifecycle{
				PreDelete: &appspub.LifecycleHook{
					LabelsHandler: map[string]string{"lifecycle-hook": "true"},
				},
			}
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

			oldPods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(oldPods).To(gomega.HaveLen(int(cs.Status.Replicas)))

			specifiedPodName := oldPods[0].Name
			ginkgo.By(fmt.Sprintf("Scale down replicas=2 with specified Pod %s", specifiedPodName))
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Replicas = utilpointer.Int32(2)
				cs.Spec.ScaleStrategy.PodsToDelete = []string{specifiedPodName}
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait specified pod becoming PreparingDelete")
			gomega.Eventually(func() appspub.LifecycleStateType {
				pod, err := c.CoreV1().Pods(cs.Namespace).Get(context.TODO(), specifiedPodName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return appspub.LifecycleStateType(pod.Labels[appspub.LifecycleStateKey])
			}, 10*time.Second, time.Second).Should(gomega.Equal(appspub.LifecycleStatePreparingDelete))

			cs, err = tester.GetCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Status.Replicas).To(gomega.Equal(int32(3)))

			ginkgo.By("Scale up to 3 again and wait status.replicas to be 4")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Replicas = utilpointer.Int32(3)
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 10*time.Second, 3*time.Second).Should(gomega.Equal(int32(4)))

			ginkgo.By("Remove lifecycle hook label and wait it to be deleted")
			patchBody := []byte(`{"metadata":{"labels":{"lifecycle-hook":null}}}`)
			_, err = c.CoreV1().Pods(cs.Namespace).Patch(context.TODO(), specifiedPodName, types.StrategicMergePatchType, patchBody, metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() *v1.Pod {
				pod, err := c.CoreV1().Pods(cs.Namespace).Get(context.TODO(), specifiedPodName, metav1.GetOptions{})
				if err != nil {
					if errors.IsNotFound(err) {
						return nil
					}
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				}
				return pod
			}, 30*time.Second, time.Second).Should(gomega.BeNil())

			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.Replicas
			}, 3*time.Second, 3*time.Second).Should(gomega.Equal(int32(3)))

			newPods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(newPods).To(gomega.HaveLen(int(cs.Status.Replicas)))

			keepOldPods := util.GetPodNames(newPods).Intersection(util.GetPodNames(oldPods)).List()
			gomega.Expect(keepOldPods).To(gomega.HaveLen(2))
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
				Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{Exec: &v1.ExecAction{Command: []string{"sleep", "10"}}}},
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
				Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{Exec: &v1.ExecAction{Command: []string{"sleep", "10"}}}},
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
				Lifecycle: &v1.Lifecycle{PostStart: &v1.LifecycleHandler{Exec: &v1.ExecAction{Command: []string{"sleep", "10"}}}},
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

		ginkgo.It(`CloneSet Update with DisablePVCReuse=true`, func() {
			updateStrategy := appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.RecreateCloneSetUpdateStrategyType}
			cs := tester.NewCloneSet("clone-"+randStr, 4, updateStrategy)
			imageConfig := imageutils.GetConfig(imageutils.Nginx)
			imageConfig.SetRegistry("docker.io/library")
			imageConfig.SetVersion("alpine")
			cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
			cs.Spec.VolumeClaimTemplates = []v1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data-vol1"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.VolumeResourceRequirements{
							Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data-vol2"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.VolumeResourceRequirements{
							Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
						},
					},
				},
			}
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.RecreateCloneSetUpdateStrategyType))

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
			instanceIds := sets.NewString()
			for _, pod := range pods {
				instanceIds.Insert(pod.Labels[appsv1alpha1.CloneSetInstanceID])
			}
			pvcs, err := tester.ListPVCForCloneSet()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pvcs)).Should(gomega.Equal(8))
			pvcIds := sets.NewString()
			for _, pvc := range pvcs {
				gomega.Expect(instanceIds.Has(pvc.Labels[appsv1alpha1.CloneSetInstanceID])).Should(gomega.BeTrue())
				pvcIds.Insert(pvc.Name)
				ref := metav1.GetControllerOf(pvc)
				gomega.Expect(ref).NotTo(gomega.BeNil())
				gomega.Expect(ref.Kind).To(gomega.Equal("CloneSet"))
			}

			ginkgo.By("delete pod, and reused pvc")
			for _, pod := range pods {
				err = c.CoreV1().Pods(ns).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
			time.Sleep(time.Second * 3)
			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(4)))
			// check pvc reused
			pvcs, err = tester.ListPVCForCloneSet()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pvcs)).Should(gomega.Equal(8))
			for _, pvc := range pvcs {
				gomega.Expect(instanceIds.Has(pvc.Labels[appsv1alpha1.CloneSetInstanceID])).Should(gomega.BeTrue())
				gomega.Expect(pvcIds.Has(pvc.Name)).To(gomega.Equal(true))
				ref := metav1.GetControllerOf(pvc)
				gomega.Expect(ref).NotTo(gomega.BeNil())
				gomega.Expect(ref.Kind).To(gomega.Equal("CloneSet"))
			}

			// update cloneSet again, and DisablePVCReuse=true
			ginkgo.By("Update cloneSet DisablePVCReuse=true")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.ScaleStrategy.DisablePVCReuse = true
			})
			time.Sleep(time.Second)
			ginkgo.By("delete pod, and reused pvc")
			for _, pod := range pods {
				err = c.CoreV1().Pods(ns).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
			time.Sleep(time.Second * 3)
			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(4)))

			// check pvc un-reused
			pods, err = tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(4))
			instanceIds = sets.NewString()
			for _, pod := range pods {
				instanceIds.Insert(pod.Labels[appsv1alpha1.CloneSetInstanceID])
			}
			pvcs, err = tester.ListPVCForCloneSet()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pvcs)).Should(gomega.Equal(8))
			for _, pvc := range pvcs {
				gomega.Expect(instanceIds.Has(pvc.Labels[appsv1alpha1.CloneSetInstanceID])).Should(gomega.BeTrue())
				gomega.Expect(pvcIds.Has(pvc.Name)).To(gomega.Equal(false))
				ref := metav1.GetControllerOf(pvc)
				gomega.Expect(ref).NotTo(gomega.BeNil())
				gomega.Expect(ref.Kind).To(gomega.Equal("CloneSet"))
			}
		})

		ginkgo.It(`CloneSet regard preparing-update pod as update when scaling`, func() {
			const updateHookLabel = "preparing-update-hook"
			updateStrategy := appsv1alpha1.CloneSetUpdateStrategy{
				Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
				MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
				Partition:      &intstr.IntOrString{Type: intstr.String, StrVal: "99%"},
			}
			cs := tester.NewCloneSet("clone-"+randStr, 2, updateStrategy)
			imageConfig := imageutils.GetConfig(imageutils.Nginx)
			imageConfig.SetRegistry("docker.io/library")
			imageConfig.SetVersion("alpine")
			cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
			lifecycleHooks := appspub.Lifecycle{
				InPlaceUpdate: &appspub.LifecycleHook{
					LabelsHandler: map[string]string{
						updateHookLabel: "true",
					},
				},
			}
			cs.Spec.Lifecycle = &lifecycleHooks
			cs.Spec.Template.Labels[updateHookLabel] = "true"
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(2)))

			pods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(2))

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

			groupPodsByRevision := func(pods []*v1.Pod) (updated, current []*v1.Pod, preUpdateIndex []int) {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				for index, pod := range pods {
					if pod.DeletionTimestamp != nil {
						continue
					}
					if utils.EqualToRevisionHash("", pod, cs.Status.UpdateRevision) {
						updated = append(updated, pod)
					} else {
						current = append(current, pod)
					}
					if pod.Labels[appspub.LifecycleStateKey] == string(appspub.LifecycleStatePreparingUpdate) {
						preUpdateIndex = append(preUpdateIndex, index)
					}
				}
				return
			}

			ginkgo.By("group currentPod and updatePod")
			gomega.Eventually(func() int {
				pods, err = tester.ListPodsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(pods)
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(2))

			updated, current, preUpdateIndex := groupPodsByRevision(pods)
			gomega.Expect(len(current)).Should(gomega.Equal(2))
			gomega.Expect(len(updated)).Should(gomega.Equal(0))
			gomega.Expect(len(preUpdateIndex)).Should(gomega.Equal(1))

			ginkgo.By("rebuild one old version pod")
			if current[0].Labels[appspub.LifecycleStateKey] != string(appspub.LifecycleStatePreparingUpdate) {
				err = tester.DeletePod(current[0].Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			} else {
				err = tester.DeletePod(current[1].Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
			time.Sleep(3 * time.Second)

			ginkgo.By("check rebuilt pod, it should be current revision")
			gomega.Eventually(func() int {
				pods, err = tester.ListPodsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(pods)
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(2))
			updated, current, preUpdateIndex = groupPodsByRevision(pods)
			gomega.Expect(len(current)).Should(gomega.Equal(2))
			gomega.Expect(len(updated)).Should(gomega.Equal(0))
			gomega.Expect(len(preUpdateIndex)).Should(gomega.Equal(1))

			ginkgo.By("scale up cloneSet to 3 replicas")
			tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Replicas = utilpointer.Int32(3)
			})

			ginkgo.By("check scaled pod, it should be current revision")
			gomega.Eventually(func() int {
				pods, err = tester.ListPodsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return len(pods)
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(3))
			updated, current, preUpdateIndex = groupPodsByRevision(pods)
			gomega.Expect(len(current)).Should(gomega.Equal(3))
			gomega.Expect(len(updated)).Should(gomega.Equal(0))
			gomega.Expect(len(preUpdateIndex)).Should(gomega.Equal(1))

			ginkgo.By("update one pod to update revision")
			f.PodClient().Update(pods[preUpdateIndex[0]].Name, func(pod *v1.Pod) {
				delete(pod.Labels, updateHookLabel)
			})
			gomega.Eventually(func() bool {
				pods, err = tester.ListPodsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				updated, current, preUpdateIndex = groupPodsByRevision(pods)
				return len(updated) == 1 && len(current) == 2 && len(preUpdateIndex) == 0
			}, 120*time.Second, 3*time.Second).Should(gomega.BeTrue())

			ginkgo.By("updating cloneSet partition to nil")
			tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.UpdateStrategy.Partition = nil
				cs.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{Type: intstr.String, StrVal: "100%"}
			})
			gomega.Eventually(func() bool {
				pods, err = tester.ListPodsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				updated, current, preUpdateIndex = groupPodsByRevision(pods)
				return len(updated) == 1 && len(current) == 2 && len(preUpdateIndex) == 2
			}, 120*time.Hour, 3*time.Second).Should(gomega.BeTrue())

			for _, p := range current {
				f.PodClient().Update(p.Name, func(pod *v1.Pod) {
					delete(pod.Labels, updateHookLabel)
				})
			}
			gomega.Eventually(func() bool {
				pods, err = tester.ListPodsForCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				updated, current, preUpdateIndex = groupPodsByRevision(pods)
				return len(updated) == 3 && len(current) == 0 && len(preUpdateIndex) == 0
			}, 120*time.Second, 3*time.Second).Should(gomega.BeTrue())
		})

		ginkgo.It(`CloneSet Update with VolumeClaimTemplate changes`, func() {
			testUpdateVolumeClaimTemplates(tester, randStr, c)
		})

		ginkgo.It(`change resource and qos -> succeed to recreate`, func() {
			testChangePodQOS(tester, randStr, c)
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

func testChangePodQOS(tester *framework.CloneSetTester, randStr string, c clientset.Interface) {
	cs := tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
	cs.Spec.Template.Spec.Containers[0].Image = NginxImage
	cs.Spec.Template.ObjectMeta.Labels["test-env"] = "foo"
	cs.Spec.Template.Spec.Containers[0].Env = append(cs.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
		Name:      "TEST_ENV",
		ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['test-env']"}},
	})
	cs, err := tester.CreateCloneSet(cs)
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

	ginkgo.By("Update resource and qos")
	err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
		cs.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse("1"),
				v1.ResourceMemory: resource.MustParse("1Gi"),
			},
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse("1"),
				v1.ResourceMemory: resource.MustParse("1Gi"),
			},
		}
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

	ginkgo.By("Verify the podID changed")
	pods, err = tester.ListPodsForCloneSet(cs.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(len(pods)).Should(gomega.Equal(1))
	newPodUID := pods[0].UID

	gomega.Expect(oldPodUID).ShouldNot(gomega.Equal(newPodUID))
}

func checkPVCsDoRecreate(numsOfPVCs int, recreate bool) func(instanceIds, newInstanceIds, pvcIds sets.String, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) {
	return func(instanceIds, newInstanceIds, pvcIds sets.String, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) {
		gomega.Expect(len(pvcs)).Should(gomega.Equal(numsOfPVCs))
		for _, pvc := range pvcs {
			id := pvc.Labels[appsv1alpha1.CloneSetInstanceID]
			gomega.Expect(newInstanceIds.Has(id)).To(gomega.Equal(true))
			gomega.Expect(instanceIds.Has(id)).To(gomega.Equal(!recreate))
			gomega.Expect(pvcIds.Has(pvc.Name)).To(gomega.Equal(!recreate))
		}
	}
}

func checkPodsDoRecreate(numsOfPods int, recreate bool) func(instanceIds, newInstanceIds, pvcIds sets.String, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) {
	return func(instanceIds, newInstanceIds, pvcIds sets.String, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) {
		gomega.Expect(len(pods)).Should(gomega.Equal(numsOfPods))
		for _, pod := range pods {
			gomega.Expect(instanceIds.Has(pod.Labels[appsv1alpha1.CloneSetInstanceID])).To(gomega.Equal(!recreate))
		}
	}
}

func changeCloneSetAndWaitReady(tester *framework.CloneSetTester, cs *appsv1alpha1.CloneSet,
	fn func(cs *appsv1alpha1.CloneSet), instanceIds, pvcIds sets.String,
	checkFns ...func(instanceIds, newInstanceIds, pvcIds sets.String, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim)) (sets.String, sets.String) {
	err := tester.UpdateCloneSet(cs.Name, fn)

	replica := *cs.Spec.Replicas
	ginkgo.By("Wait for replicas satisfied")
	gomega.Eventually(func() int32 {
		cs, err = tester.GetCloneSet(cs.Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return cs.Status.Replicas
	}, 3*time.Second, time.Second).Should(gomega.Equal(replica))
	time.Sleep(time.Second * 3)

	ginkgo.By("Wait for all pods ready")
	gomega.Eventually(func() int32 {
		cs, err = tester.GetCloneSet(cs.Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		if cs.Status.ObservedGeneration != cs.Generation {
			return -1
		}
		return cs.Status.UpdatedReadyReplicas
	}, 120*time.Second, 3*time.Second).Should(gomega.Equal(*cs.Spec.Replicas))

	pods, err := tester.ListPodsForCloneSet(cs.Name)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(int32(len(pods))).Should(gomega.Equal(replica))
	newInstanceIds := sets.NewString()
	for _, pod := range pods {
		newInstanceIds.Insert(pod.Labels[appsv1alpha1.CloneSetInstanceID])
	}
	pvcs, err := tester.ListPVCForCloneSet()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	if len(instanceIds) > 0 && len(pvcIds) > 0 {
		for _, checkFn := range checkFns {
			checkFn(instanceIds, newInstanceIds, pvcIds, pods, pvcs)
		}
	}

	// record new pvcIds
	newPvcIds := sets.NewString()
	for _, pvc := range pvcs {
		gomega.Expect(newInstanceIds.Has(pvc.Labels[appsv1alpha1.CloneSetInstanceID])).Should(gomega.BeTrue())
		newPvcIds.Insert(pvc.Name)
	}

	return newInstanceIds, newPvcIds
}

func testUpdateVolumeClaimTemplates(tester *framework.CloneSetTester, randStr string, c clientset.Interface) {
	updateStrategy := appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.RecreateCloneSetUpdateStrategyType}
	var replicas int = 4
	cs := tester.NewCloneSet("clone-"+randStr, int32(replicas), updateStrategy)
	imageConfig := imageutils.GetConfig(imageutils.Nginx)
	imageConfig.SetRegistry("docker.io/library")
	imageConfig.SetVersion("alpine")
	cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
	cs.Spec.VolumeClaimTemplates = []v1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "data-vol1"},
			Spec: v1.PersistentVolumeClaimSpec{
				AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				Resources: v1.VolumeResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "data-vol2"},
			Spec: v1.PersistentVolumeClaimSpec{
				AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				Resources: v1.VolumeResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
	}
	cs, err := tester.CreateCloneSet(cs)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(cs.Spec.UpdateStrategy.Type).To(gomega.Equal(appsv1alpha1.RecreateCloneSetUpdateStrategyType))

	instanceIds, pvcIds := changeCloneSetAndWaitReady(tester, cs, func(cs *appsv1alpha1.CloneSet) {}, nil, nil)

	numsOfPVCs := replicas * 2
	checkPVCSize1 := func(instanceIds, newInstanceIds, pvcIds sets.String, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) {
		gomega.Expect(len(pvcs)).Should(gomega.Equal(numsOfPVCs))
		for _, pvc := range pvcs {
			id := pvc.Labels[appsv1alpha1.CloneSetInstanceID]
			gomega.Expect(newInstanceIds.Has(id)).To(gomega.Equal(true))
			req := "1Gi"
			if strings.Contains(pvc.Name, "data-vol1") {
				req = "2Gi"
			}
			gomega.Expect(pvc.Spec.Resources.Requests.Storage().String()).To(gomega.Equal(req))
		}
	}
	// update cloneSet image + vct size
	ginkgo.By("Update cloneSet image and volumeClaimTemplates")
	updateImageAndVCTFn := func(cs *appsv1alpha1.CloneSet) {
		imageConfig = imageutils.GetConfig(imageutils.NginxNew)
		cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
		cs.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests = v1.ResourceList{v1.ResourceStorage: resource.MustParse("2Gi")}
		cs.Spec.UpdateStrategy = appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType}
	}
	instanceIds, pvcIds = changeCloneSetAndWaitReady(tester, cs, updateImageAndVCTFn,
		instanceIds, pvcIds, checkPodsDoRecreate(replicas, true),
		checkPVCsDoRecreate(replicas*2, true), checkPVCSize1)

	// update cloneSet only size
	ginkgo.By("Update cloneSet only volumeClaimTemplates")
	updateVCTOnly := func(cs *appsv1alpha1.CloneSet) {
		cs.Spec.VolumeClaimTemplates[1].Spec.Resources.Requests = v1.ResourceList{v1.ResourceStorage: resource.MustParse("2Gi")}
		cs.Spec.UpdateStrategy = appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType}
	}
	instanceIds, pvcIds = changeCloneSetAndWaitReady(tester, cs, updateVCTOnly,
		instanceIds, pvcIds, checkPodsDoRecreate(replicas, false),
		checkPVCsDoRecreate(replicas*2, false), checkPVCSize1)

	checkPVCSize2 := func(instanceIds, newInstanceIds, pvcIds sets.String, pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) {
		gomega.Expect(len(pvcs)).Should(gomega.Equal(numsOfPVCs))
		for _, pvc := range pvcs {
			id := pvc.Labels[appsv1alpha1.CloneSetInstanceID]
			gomega.Expect(newInstanceIds.Has(id)).To(gomega.Equal(true))
			req := "2Gi"
			gomega.Expect(pvc.Spec.Resources.Requests.Storage().String()).To(gomega.Equal(req))
		}
	}

	// update cloneSet image
	ginkgo.By("Update cloneSet image and vct size changed in previous step")
	updateImageOnly := func(cs *appsv1alpha1.CloneSet) {
		imageConfig = imageutils.GetConfig(imageutils.Redis)
		cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
		cs.Spec.UpdateStrategy = appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType}
	}
	instanceIds, pvcIds = changeCloneSetAndWaitReady(tester, cs, updateImageOnly,
		instanceIds, pvcIds, checkPodsDoRecreate(replicas, true),
		checkPVCsDoRecreate(replicas*2, true), checkPVCSize2)

	// inplace-only update strategy with image change
	inplaceOnlyWithImage := func(cs *appsv1alpha1.CloneSet) {
		imageConfig = imageutils.GetConfig(imageutils.Httpd)
		cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
		cs.Spec.UpdateStrategy = appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType}
	}
	instanceIds, pvcIds = changeCloneSetAndWaitReady(tester, cs, inplaceOnlyWithImage,
		instanceIds, pvcIds, checkPodsDoRecreate(replicas, false),
		checkPVCsDoRecreate(replicas*2, false), checkPVCSize2)

	// inplace-only update strategy with image and vct changes -> in-place update with pvc no changes
	inplaceOnlyWithImageAndVCT := func(cs *appsv1alpha1.CloneSet) {
		imageConfig = imageutils.GetConfig(imageutils.Redis)
		cs.Spec.Template.Spec.Containers[0].Image = imageConfig.GetE2EImage()
		cs.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests = v1.ResourceList{v1.ResourceStorage: resource.MustParse("3Gi")}
		cs.Spec.VolumeClaimTemplates[1].Spec.Resources.Requests = v1.ResourceList{v1.ResourceStorage: resource.MustParse("3Gi")}
		cs.Spec.UpdateStrategy = appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType}
	}
	instanceIds, pvcIds = changeCloneSetAndWaitReady(tester, cs, inplaceOnlyWithImageAndVCT,
		instanceIds, pvcIds, checkPodsDoRecreate(replicas, false),
		checkPVCsDoRecreate(replicas*2, false), checkPVCSize2)
}
