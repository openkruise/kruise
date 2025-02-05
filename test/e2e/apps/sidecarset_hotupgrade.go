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

package apps

import (
	"fmt"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	utilpointer "k8s.io/utils/pointer"
)

var _ = SIGDescribe("SidecarSet", func() {
	f := framework.NewDefaultFramework("sidecarset")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.SidecarSetTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewSidecarSetTester(c, kc)
	})

	framework.KruiseDescribe("SidecarSet HotUpgrade functionality [SidecarSetHotUpgrade]", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all SidecarSet in cluster")
			tester.DeleteSidecarSets(ns)
			tester.DeleteDeployments(ns)
		})

		framework.ConformanceIt("sidecarSet inject pod hot upgrade sidecar container", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			sidecarSetIn.Spec.Containers[0].UpgradeStrategy = appsv1alpha1.SidecarContainerUpgradeStrategy{
				UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
				HotUpgradeEmptyImage: BusyboxImage,
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
			// check pod sidecarSet version in annotations
			gomega.Expect(pods[0].Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation("nginx-sidecar-1")]).To(gomega.Equal("1"))
			gomega.Expect(pods[0].Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation("nginx-sidecar-1")]).To(gomega.Equal("0"))
			gomega.Expect(pods[0].Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation("nginx-sidecar-2")]).To(gomega.Equal("0"))
			gomega.Expect(pods[0].Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation("nginx-sidecar-2")]).To(gomega.Equal("1"))

			// except sidecar container -> image
			exceptContainer := map[string]string{
				"nginx-sidecar-1": NginxImage,
				"nginx-sidecar-2": BusyboxImage,
			}
			for sidecar, image := range exceptContainer {
				sidecarContainer := util.GetContainer(sidecar, pods[0])
				gomega.Expect(sidecarContainer).ShouldNot(gomega.BeNil())
				gomega.Expect(sidecarContainer.Image).To(gomega.Equal(image))
			}
			ginkgo.By("sidecarSet inject pod hot upgrade sidecar container done")
		})

		framework.ConformanceIt("sidecarSet upgrade hot sidecar container image", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			sidecarSetIn.Spec.Containers[0].Image = NginxImage
			sidecarSetIn.Spec.Containers[0].UpgradeStrategy = appsv1alpha1.SidecarContainerUpgradeStrategy{
				UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
				HotUpgradeEmptyImage: BusyboxImage,
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(1)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// check pod image and annotations
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			podIn := pods[0]
			workSidecarContainer := util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn)["nginx-sidecar"], podIn)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal("nginx-sidecar-1"))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(NginxImage))
			_, emptyContainer := sidecarcontrol.GetPodHotUpgradeContainers("nginx-sidecar", podIn)
			emptySidecarContainer := util.GetContainer(emptyContainer, podIn)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal("nginx-sidecar-2"))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(BusyboxImage))
			// check pod sidecarSet version in annotations
			gomega.Expect(podIn.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation("nginx-sidecar-1")]).To(gomega.Equal("1"))
			gomega.Expect(podIn.Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation("nginx-sidecar-1")]).To(gomega.Equal("0"))
			gomega.Expect(podIn.Annotations[sidecarcontrol.GetPodSidecarSetVersionAnnotation("nginx-sidecar-2")]).To(gomega.Equal("0"))
			gomega.Expect(podIn.Annotations[sidecarcontrol.GetPodSidecarSetVersionAltAnnotation("nginx-sidecar-2")]).To(gomega.Equal("1"))

			// update sidecarSet sidecar container
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) sidecar container image to new nginx", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Image = NewNginxImage
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
			workSidecarContainer = util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn)["nginx-sidecar"], podIn)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal("nginx-sidecar-2"))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(NewNginxImage))
			_, emptyContainer = sidecarcontrol.GetPodHotUpgradeContainers("nginx-sidecar", podIn)
			emptySidecarContainer = util.GetContainer(emptyContainer, podIn)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal("nginx-sidecar-1"))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(BusyboxImage))

			//update sidecarSet sidecar container again
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) sidecar container image to nginx", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Image = NginxImage
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
			workSidecarContainer = util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn)["nginx-sidecar"], podIn)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal("nginx-sidecar-1"))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(NginxImage))
			_, emptyContainer = sidecarcontrol.GetPodHotUpgradeContainers("nginx-sidecar", podIn)
			emptySidecarContainer = util.GetContainer(emptyContainer, podIn)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal("nginx-sidecar-2"))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(BusyboxImage))

			//update sidecarSet sidecar container others parameters, then don't upgrade
			//sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
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

		framework.ConformanceIt("sidecarSet upgrade hot sidecar container failed image", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			sidecarSetIn.Spec.Containers[0].Image = NginxImage
			sidecarSetIn.Spec.Containers[0].UpgradeStrategy = appsv1alpha1.SidecarContainerUpgradeStrategy{
				UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
				HotUpgradeEmptyImage: BusyboxImage,
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(2)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// check pod image and annotations
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			podIn := pods[0]
			workSidecarContainer := util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn)["nginx-sidecar"], podIn)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal("nginx-sidecar-1"))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(NginxImage))
			_, emptyContainer := sidecarcontrol.GetPodHotUpgradeContainers("nginx-sidecar", podIn)
			emptySidecarContainer := util.GetContainer(emptyContainer, podIn)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal("nginx-sidecar-2"))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(BusyboxImage))

			ginkgo.By(fmt.Sprintf("Update SidecarSet(%s) with failed image", sidecarSetIn.Name))
			// update sidecarSet sidecar container failed image
			sidecarSetIn.Spec.Containers[0].Image = InvalidImage
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
			//update sidecarSet sidecar container again, and success image
			sidecarSetIn.Spec.Containers[0].Image = NewNginxImage
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
			workSidecarContainer = util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn1)["nginx-sidecar"], podIn1)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal("nginx-sidecar-2"))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(NewNginxImage))
			_, emptyContainer = sidecarcontrol.GetPodHotUpgradeContainers("nginx-sidecar", podIn1)
			emptySidecarContainer = util.GetContainer(emptyContainer, podIn1)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal("nginx-sidecar-1"))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(BusyboxImage))
			// pod[1]
			podIn2 := pods[1]
			workSidecarContainer = util.GetContainer(sidecarcontrol.GetPodHotUpgradeInfoInAnnotations(podIn2)["nginx-sidecar"], podIn2)
			gomega.Expect(workSidecarContainer.Name).To(gomega.Equal("nginx-sidecar-2"))
			gomega.Expect(workSidecarContainer.Image).To(gomega.Equal(NewNginxImage))
			_, emptyContainer = sidecarcontrol.GetPodHotUpgradeContainers("nginx-sidecar", podIn2)
			emptySidecarContainer = util.GetContainer(emptyContainer, podIn2)
			gomega.Expect(emptySidecarContainer.Name).To(gomega.Equal("nginx-sidecar-1"))
			gomega.Expect(emptySidecarContainer.Image).To(gomega.Equal(BusyboxImage))

			ginkgo.By("sidecarSet upgrade hot sidecar container failed image done")
		})
	})
})
