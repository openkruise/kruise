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
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	priorityName    = "KRUISE_CONTAINER_PRIORITY"
	priorityBarrier = "KRUISE_CONTAINER_BARRIER"
)

var _ = SIGDescribe("containerpriority", func() {
	f := framework.NewDefaultFramework("containerpriority")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.CloneSetTester
	var randStr string
	var cs *appsv1alpha1.CloneSet

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewCloneSetTester(c, kc, ns)
		randStr = rand.String(10)
	})

	framework.KruiseDescribe("start a pod with different container priorities", func() {
		var err error

		ginkgo.It("container priority in normal case", func() {
			cs = tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{})
			cs.Spec.Template.Spec.Containers = append(cs.Spec.Template.Spec.Containers, v1.Container{
				Name:  "nginx2",
				Image: "nginx:1.21",
				Env: []v1.EnvVar{
					{Name: priorityName, Value: "10"},
					{Name: "NGINX_PORT", Value: "81"},
				},
			})
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for the pod ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			pods, err := tester.GetSelectorPods(cs.Namespace, cs.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods[0].Spec.Containers[0].Env[1].Name).To(gomega.Equal(priorityBarrier))
			gomega.Expect(*pods[0].Spec.Containers[0].Env[1].ValueFrom.ConfigMapKeyRef).To(gomega.Equal(v1.ConfigMapKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: pods[0].Name + "-barrier"},
				Key:                  "p_0",
			}))
			gomega.Expect(pods[0].Spec.Containers[1].Env[2].Name).To(gomega.Equal(priorityBarrier))
			gomega.Expect(*pods[0].Spec.Containers[1].Env[2].ValueFrom.ConfigMapKeyRef).To(gomega.Equal(v1.ConfigMapKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: pods[0].Name + "-barrier"},
				Key:                  "p_10",
			}))
			var containerStatus1 v1.ContainerStatus
			var containerStatus2 v1.ContainerStatus
			for _, container := range pods[0].Status.ContainerStatuses {
				if container.Name == pods[0].Spec.Containers[0].Name {
					containerStatus1 = container
				} else {
					containerStatus2 = container
				}

			}
			earlierThan := containerStatus1.State.Running.StartedAt.Time.After(containerStatus2.State.Running.StartedAt.Time)
			gomega.Expect(earlierThan).To(gomega.Equal(true))
		})

		ginkgo.It("run with no container priority", func() {
			cs = tester.NewCloneSet("clone-"+randStr, 1, appsv1alpha1.CloneSetUpdateStrategy{})
			cs.Spec.Template.Spec.Containers = append(cs.Spec.Template.Spec.Containers, v1.Container{
				Name:  "nginx2",
				Image: "nginx:1.21",
				Env: []v1.EnvVar{
					{Name: "NGINX_PORT", Value: "81"},
				},
			})
			cs, err = tester.CreateCloneSet(cs)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Wait for the pod ready")
			gomega.Eventually(func() int32 {
				cs, err = tester.GetCloneSet(cs.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return cs.Status.ReadyReplicas
			}, 120*time.Second, 3*time.Second).Should(gomega.Equal(int32(1)))

			pods, err := tester.GetSelectorPods(cs.Namespace, cs.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(len(pods[0].Spec.Containers[0].Env)).To(gomega.Equal(1))
			gomega.Expect(len(pods[0].Spec.Containers[1].Env)).To(gomega.Equal(1))
			var containerStatus1 v1.ContainerStatus
			var containerStatus2 v1.ContainerStatus
			for _, container := range pods[0].Status.ContainerStatuses {
				if container.Name == pods[0].Spec.Containers[0].Name {
					containerStatus1 = container
				} else {
					containerStatus2 = container
				}

			}
			earlierThan := containerStatus1.State.Running.StartedAt.Time.Before(containerStatus2.State.Running.StartedAt.Time)
			gomega.Expect(earlierThan).To(gomega.Equal(true))
		})
	})

})
