/*
Copyright 2020 The Kruise Authors.

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
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	"github.com/openkruise/kruise/test/e2e/framework/v1alpha1"
)

var _ = ginkgo.Describe("SidecarSet", ginkgo.Label("SidecarSet", "workload"), func() {
	f := v1alpha1.NewDefaultFramework("sidecarset")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *v1alpha1.SidecarSetTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = v1alpha1.NewSidecarSetTester(c, kc)
	})

	ginkgo.Context("SidecarSet HotUpgrade functionality [SidecarSetHotUpgrade]", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentSpecReport().Failed() {
				common.DumpDebugInfo(c, ns)
			}
			common.Logf("Deleting all SidecarSet in cluster")
			tester.DeleteSidecarSets(ns)
			tester.DeleteDeployments(ns)
		})

		ginkgo.It("sidecarSet inject pod hot upgrade sidecar container", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			// Get the dynamically generated container name
			sidecarContainerName := sidecarSetIn.Spec.Containers[0].Name
			sidecarSetIn.Spec.Containers[0].UpgradeStrategy = appsv1alpha1.SidecarContainerUpgradeStrategy{
				UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
				HotUpgradeEmptyImage: common.BusyboxImage,
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// get pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods[0].Spec.Containers).To(gomega.HaveLen(len(sidecarSetIn.Spec.Containers) + len(deploymentIn.Spec.Template.Spec.Containers) + 1))
			// check pod sidecarSet version in annotations (hot upgrade containers have -1 and -2 suffixes)
			hotUpgradeContainer1 := sidecarContainerName + "-1"
			hotUpgradeContainer2 := sidecarContainerName + "-2"
			gomega.Expect(pods[0].Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(hotUpgradeContainer1)]).To(gomega.Equal("1"))
			gomega.Expect(pods[0].Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(hotUpgradeContainer1)]).To(gomega.Equal("0"))
			gomega.Expect(pods[0].Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(hotUpgradeContainer2)]).To(gomega.Equal("0"))
			gomega.Expect(pods[0].Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(hotUpgradeContainer2)]).To(gomega.Equal("1"))

			// except sidecar container -> image
			exceptContainer := map[string]string{
				hotUpgradeContainer1: common.NginxImage,
				hotUpgradeContainer2: common.BusyboxImage,
			}
			for sidecar, image := range exceptContainer {
				sidecarContainer := util.GetContainer(sidecar, pods[0])
				gomega.Expect(sidecarContainer).ShouldNot(gomega.BeNil())
				gomega.Expect(sidecarContainer.Image).To(gomega.Equal(image))
			}
			ginkgo.By("sidecarSet inject pod hot upgrade sidecar container done")
		})

		ginkgo.It("sidecarSet upgrade hot sidecar container image", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			// Get the dynamically generated container name
			sidecarContainerName := sidecarSetIn.Spec.Containers[0].Name
			hotUpgradeContainer1 := sidecarContainerName + "-1"
			hotUpgradeContainer2 := sidecarContainerName + "-2"
			sidecarSetIn.Spec.Containers[0].Image = common.NginxImage
			sidecarSetIn.Spec.Containers[0].UpgradeStrategy = appsv1alpha1.SidecarContainerUpgradeStrategy{
				UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
				HotUpgradeEmptyImage: common.BusyboxImage,
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To[int32](1)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// check pod image and annotations
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			podIn := pods[0]
			workSidecarContainer := util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn)[sidecarContainerName], podIn)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer1))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(common.NginxImage))
			_, emptyContainer := sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainerName, podIn)
			emptySidecarContainer := util.GetContainer(emptyContainer, podIn)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer2))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(common.BusyboxImage))
			// check pod sidecarSet version in annotations
			gomega.Expect(podIn.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(hotUpgradeContainer1)]).To(gomega.Equal("1"))
			gomega.Expect(podIn.Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(hotUpgradeContainer1)]).To(gomega.Equal("0"))
			gomega.Expect(podIn.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation(hotUpgradeContainer2)]).To(gomega.Equal("0"))
			gomega.Expect(podIn.Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation(hotUpgradeContainer2)]).To(gomega.Equal("1"))

			// update sidecarSet sidecar container
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) sidecar container image to new nginx", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Image = common.NewNginxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      1,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        1,
			}
			time.Sleep(time.Second * 5)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// check pod image and annotations
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			podIn = pods[0]
			workSidecarContainer = util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn)[sidecarContainerName], podIn)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer2))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(common.NewNginxImage))
			_, emptyContainer = sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainerName, podIn)
			emptySidecarContainer = util.GetContainer(emptyContainer, podIn)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer1))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(common.BusyboxImage))

			// update sidecarSet sidecar container again
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) sidecar container image to nginx", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Image = common.NginxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      1,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        1,
			}
			time.Sleep(time.Second * 5)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// check pod image and annotations
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			podIn = pods[0]
			workSidecarContainer = util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn)[sidecarContainerName], podIn)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer1))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(common.NginxImage))
			_, emptyContainer = sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainerName, podIn)
			emptySidecarContainer = util.GetContainer(emptyContainer, podIn)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer2))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(common.BusyboxImage))

			// update sidecarSet sidecar container others parameters, then don't upgrade
			// sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) others parameters, then don't upgrade", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "PROXY",
					Value: "127.0.0.1",
				},
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      1,
				UpdatedPods:      0,
				UpdatedReadyPods: 0,
				ReadyPods:        1,
			}
			time.Sleep(time.Second * 30)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			ginkgo.By("sidecarSet upgrade hot sidecar container image done")
		})

		ginkgo.It("sidecarSet upgrade hot sidecar container failed image", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			// Get the dynamically generated container name
			sidecarContainerName := sidecarSetIn.Spec.Containers[0].Name
			hotUpgradeContainer1 := sidecarContainerName + "-1"
			hotUpgradeContainer2 := sidecarContainerName + "-2"
			sidecarSetIn.Spec.Containers[0].Image = common.NginxImage
			sidecarSetIn.Spec.Containers[0].UpgradeStrategy = appsv1alpha1.SidecarContainerUpgradeStrategy{
				UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
				HotUpgradeEmptyImage: common.BusyboxImage,
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To[int32](2)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// check pod image and annotations
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			podIn := pods[0]
			workSidecarContainer := util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn)[sidecarContainerName], podIn)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer1))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(common.NginxImage))
			_, emptyContainer := sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainerName, podIn)
			emptySidecarContainer := util.GetContainer(emptyContainer, podIn)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer2))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(common.BusyboxImage))

			ginkgo.By(fmt.Sprintf("Update SidecarSet(%s) with failed image", sidecarSetIn.Name))
			// update sidecarSet sidecar container failed image
			sidecarSetIn.Spec.Containers[0].Image = common.InvalidImage
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 0,
				ReadyPods:        1,
			}
			time.Sleep(time.Second * 30)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			ginkgo.By(fmt.Sprintf("Update SidecarSet(%s) with success image", sidecarSetIn.Name))
			// update sidecarSet sidecar container again, and success image
			sidecarSetIn.Spec.Containers[0].Image = common.NewNginxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			time.Sleep(time.Minute * 1)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// check pod image and annotations
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			// pod[0]
			podIn1 := pods[0]
			workSidecarContainer = util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn1)[sidecarContainerName], podIn1)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer2))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(common.NewNginxImage))
			_, emptyContainer = sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainerName, podIn1)
			emptySidecarContainer = util.GetContainer(emptyContainer, podIn1)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer1))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(common.BusyboxImage))
			// pod[1]
			podIn2 := pods[1]
			workSidecarContainer = util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn2)[sidecarContainerName], podIn2)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer2))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(common.NewNginxImage))
			_, emptyContainer = sidecarcontrol.GetPodHotUpgradeContainers(sidecarContainerName, podIn2)
			emptySidecarContainer = util.GetContainer(emptyContainer, podIn2)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal(hotUpgradeContainer1))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(common.BusyboxImage))

			ginkgo.By("sidecarSet upgrade hot sidecar container failed image done")
		})
	})
})
