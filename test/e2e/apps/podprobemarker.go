/*
Copyright 2022 The Kruise Authors.

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
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/controller/podprobemarker"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = SIGDescribe("PodProbeMarker", func() {
	f := framework.NewDefaultFramework("podprobemarkers")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.PodProbeMarkerTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewPodProbeMarkerTester(c, kc)
	})

	framework.KruiseDescribe("PodProbeMarker functionality", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
		})

		ginkgo.It("pod probe marker test1", func() {
			nodeList, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if len(nodeList.Items) == 0 {
				ginkgo.By("pod probe markers list nodeList is zero")
				return
			}
			nppList, err := kc.AppsV1alpha1().NodePodProbes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(nppList.Items).To(gomega.HaveLen(len(nodeList.Items)))

			// create statefulset
			sts := tester.NewBaseStatefulSet(ns)
			ginkgo.By(fmt.Sprintf("Create statefulset(%s/%s)", sts.Namespace, sts.Name))
			tester.CreateStatefulSet(sts)

			// create pod probe marker
			ppmList := tester.NewPodProbeMarker(ns)
			ppm1, ppm2 := &ppmList[0], &ppmList[1]
			_, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Create(context.TODO(), ppm1, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 10)

			// check finalizer
			ppm1, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Get(context.TODO(), ppm1.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(controllerutil.ContainsFinalizer(ppm1, podprobemarker.PodProbeMarkerFinalizer)).To(gomega.BeTrue())

			pods, err := tester.ListActivePods(ns)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			validPods := sets.NewString()
			for _, pod := range pods {
				validPods.Insert(string(pod.UID))
				npp, err := kc.AppsV1alpha1().NodePodProbes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				var podProbe *appsv1alpha1.PodProbe
				for i := range npp.Spec.PodProbes {
					obj := &npp.Spec.PodProbes[i]
					if obj.UID == string(pod.UID) {
						podProbe = obj
						break
					}
				}
				gomega.Expect(podProbe).NotTo(gomega.BeNil())
				gomega.Expect(pod.Labels["nginx"]).To(gomega.Equal("healthy"))
				condition := util.GetCondition(pod, "game.kruise.io/healthy")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionTrue)))
				condition = util.GetCondition(pod, "game.kruise.io/check")
				gomega.Expect(condition).To(gomega.BeNil())
			}
			nppList, err = kc.AppsV1alpha1().NodePodProbes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, npp := range nppList.Items {
				for _, podProbe := range npp.Spec.PodProbes {
					gomega.Expect(validPods.Has(podProbe.UID)).To(gomega.BeTrue())
				}
			}
			// create other pod probe marker
			_, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Create(context.TODO(), ppm2, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 10)

			pods, err = tester.ListActivePods(ns)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			for _, pod := range pods {
				// healthy probe
				gomega.Expect(pod.Labels["nginx"]).To(gomega.Equal("healthy"))
				condition := util.GetCondition(pod, "game.kruise.io/healthy")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionTrue)))
				// check probe
				gomega.Expect(pod.Annotations["controller.kubernetes.io/pod-deletion-cost"]).To(gomega.Equal("10"))
				condition = util.GetCondition(pod, "game.kruise.io/check")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionTrue)))
			}

			// update failed probe
			ppm1, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Get(context.TODO(), ppm1.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ppm1.Spec.Probes[0].Probe.Exec = &v1.ExecAction{
				Command: []string{"/bin/sh", "-c", "failed /"},
			}
			_, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Update(context.TODO(), ppm1, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ppm2, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Get(context.TODO(), ppm2.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ppm2.Spec.Probes[0].Probe.Exec = &v1.ExecAction{
				Command: []string{"/bin/sh", "-c", "failed -ef"},
			}
			_, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Update(context.TODO(), ppm2, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 10)

			pods, err = tester.ListActivePods(ns)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			for _, pod := range pods {
				// healthy probe
				gomega.Expect(pod.Labels["nginx"]).To(gomega.Equal(""))
				condition := util.GetCondition(pod, "game.kruise.io/healthy")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionFalse)))
				// check probe
				gomega.Expect(pod.Annotations["controller.kubernetes.io/pod-deletion-cost"]).To(gomega.Equal("-10"))
				condition = util.GetCondition(pod, "game.kruise.io/check")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionFalse)))
			}

			// update success probe
			ppm1, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Get(context.TODO(), ppm1.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ppm1.Spec.Probes[0].Probe.Exec = &v1.ExecAction{
				Command: []string{"/bin/sh", "-c", "ls /"},
			}
			_, err = kc.AppsV1alpha1().PodProbeMarkers(ns).Update(context.TODO(), ppm1, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// scale down
			sts, err = c.AppsV1().StatefulSets(ns).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sts.Spec.Replicas = utilpointer.Int32Ptr(1)
			_, err = c.AppsV1().StatefulSets(ns).Update(context.TODO(), sts, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 10)

			pods, err = tester.ListActivePods(ns)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			validPods = sets.NewString()
			for _, pod := range pods {
				validPods.Insert(string(pod.UID))
				// healthy probe
				gomega.Expect(pod.Labels["nginx"]).To(gomega.Equal("healthy"))
				condition := util.GetCondition(pod, "game.kruise.io/healthy")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionTrue)))
				// check probe
				gomega.Expect(pod.Annotations["controller.kubernetes.io/pod-deletion-cost"]).To(gomega.Equal("-10"))
				condition = util.GetCondition(pod, "game.kruise.io/check")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionFalse)))
			}
			nppList, err = kc.AppsV1alpha1().NodePodProbes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, npp := range nppList.Items {
				for _, podProbe := range npp.Spec.PodProbes {
					gomega.Expect(validPods.Has(podProbe.UID)).To(gomega.BeTrue())
				}
			}

			// scale up
			sts, err = c.AppsV1().StatefulSets(ns).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sts.Spec.Replicas = utilpointer.Int32Ptr(2)
			_, err = c.AppsV1().StatefulSets(ns).Update(context.TODO(), sts, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			tester.WaitForStatefulSetRunning(sts)
			time.Sleep(time.Second * 10)

			pods, err = tester.ListActivePods(ns)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*sts.Spec.Replicas)))
			validPods = sets.NewString()
			for _, pod := range pods {
				validPods.Insert(string(pod.UID))
				// healthy probe
				gomega.Expect(pod.Labels["nginx"]).To(gomega.Equal("healthy"))
				condition := util.GetCondition(pod, "game.kruise.io/healthy")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionTrue)))
				// check probe
				gomega.Expect(pod.Annotations["controller.kubernetes.io/pod-deletion-cost"]).To(gomega.Equal("-10"))
				condition = util.GetCondition(pod, "game.kruise.io/check")
				gomega.Expect(condition).NotTo(gomega.BeNil())
				gomega.Expect(string(condition.Status)).To(gomega.Equal(string(v1.ConditionFalse)))
			}
			nppList, err = kc.AppsV1alpha1().NodePodProbes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, npp := range nppList.Items {
				for _, podProbe := range npp.Spec.PodProbes {
					gomega.Expect(validPods.Has(podProbe.UID)).To(gomega.BeTrue())
				}
			}

			// delete podProbeMarker
			for _, ppm := range ppmList {
				err = kc.AppsV1alpha1().PodProbeMarkers(ns).Delete(context.TODO(), ppm.Name, metav1.DeleteOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
			time.Sleep(time.Second * 3)
			nppList, err = kc.AppsV1alpha1().NodePodProbes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, npp := range nppList.Items {
				gomega.Expect(npp.Spec.PodProbes).To(gomega.HaveLen(0))
			}
		})
	})
})
