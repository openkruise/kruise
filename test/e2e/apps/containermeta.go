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
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = SIGDescribe("ContainerMeta", func() {
	f := framework.NewDefaultFramework("containermeta")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.CloneSetTester
	var randStr string
	var nodeTester *framework.NodeTester
	var nodes []*v1.Node
	var err error
	var replicas int32

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewCloneSetTester(c, kc, ns)
		randStr = rand.String(10)
		nodeTester = framework.NewNodeTester(c)
		nodes, err = nodeTester.ListRealNodesWithFake(nil)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		replicas = int32(len(nodes))
	})

	framework.KruiseDescribe("In-place update env from metadata", func() {
		var err error

		// This can't be Conformance yet.
		ginkgo.It("should recreate container when annotations for env changed", func() {
			ginkgo.By(fmt.Sprintf("Create a CloneSet with replicas=%d", replicas))
			cs := tester.NewCloneSet("clone-"+randStr, replicas, appsv1alpha1.CloneSetUpdateStrategy{Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType})
			if cs.Spec.Template.ObjectMeta.Annotations == nil {
				cs.Spec.Template.ObjectMeta.Annotations = map[string]string{}
			}
			cs.Spec.Template.ObjectMeta.Annotations["test-env"] = "foo"
			cs.Spec.Template.Spec.Containers[0].Env = append(cs.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
				Name:      "TEST_ENV",
				ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.annotations['test-env']"}},
			})
			// For heterogeneous scenario like edge cluster, I want to deploy a Pod for each Node to verify that the functionality works
			cs.Spec.Template.Spec.TopologySpreadConstraints = []v1.TopologySpreadConstraint{
				{
					LabelSelector:     cs.Spec.Selector,
					MaxSkew:           1,
					TopologyKey:       "kubernetes.io/hostname",
					WhenUnsatisfiable: v1.ScheduleAnyway,
				},
			}
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for all pods ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(replicas))

			pods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods)).Should(gomega.Equal(int(replicas)))

			ginkgo.By("Patch pods[0] annotation to bar")
			mergePatch := []byte(`{"metadata":{"annotations":{"test-env":"bar"}}}`)
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), pods[0].Name, types.StrategicMergePatchType, mergePatch, metav1.PatchOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check pods[0] container not to be recreate")
			var pod0 *v1.Pod
			gomega.Eventually(func() int32 {
				pod0, err = c.CoreV1().Pods(ns).Get(context.TODO(), pods[0].Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return pod0.Status.ContainerStatuses[0].RestartCount
			}, 5*time.Second, time.Second).Should(gomega.Equal(int32(0)))
			gomega.Expect(pod0.Status.ContainerStatuses[0].ContainerID).Should(gomega.Equal(pods[0].Status.ContainerStatuses[0].ContainerID))

			ginkgo.By("Update CloneSet template annotation to bar")
			err = tester.UpdateCloneSet(cs.Name, func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Template.ObjectMeta.Annotations["test-env"] = "bar"
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for all pods updated and ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if cs.Status.ObservedGeneration != cs.Generation {
					return 0
				}
				return cs.Status.UpdatedReadyReplicas
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(replicas))
			ginkgo.By("Check all pods restarted")
			newPods, err := tester.ListPodsForCloneSet(cs.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for i := range newPods {
				gomega.Expect(newPods[i].Status.ContainerStatuses[0].RestartCount).Should(gomega.Equal(int32(1)), fmt.Sprintf("new pod status: %s", util.DumpJSON(newPods[i].Status)))
				gomega.Expect(newPods[i].Status.ContainerStatuses[0].ContainerID).NotTo(gomega.Equal(pods[i].Status.ContainerStatuses[0].ContainerID))
			}
		})
	})
})
