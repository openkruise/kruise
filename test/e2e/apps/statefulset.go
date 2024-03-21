/*
Copyright 2019 The Kruise Authors.
Copyright 2014 The Kubernetes Authors.

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
	"regexp"
	"strconv"
	"strings"
	"time"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	watchtools "k8s.io/client-go/tools/watch"
	imageutils "k8s.io/kubernetes/test/utils/image"
	"k8s.io/utils/pointer"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
)

const (
	zookeeperManifestPath   = "test/e2e/testing-manifests/statefulset/zookeeper"
	mysqlGaleraManifestPath = "test/e2e/testing-manifests/statefulset/mysql-galera"
	redisManifestPath       = "test/e2e/testing-manifests/statefulset/redis"
	cockroachDBManifestPath = "test/e2e/testing-manifests/statefulset/cockroachdb"
	// We don't restart MySQL cluster regardless of restartCluster, since MySQL doesn't handle restart well
	restartCluster = true

	// Timeout for reads from databases running on stateful pods.
	readTimeout = 60 * time.Second
)

// GCE Quota requirements: 3 pds, one per stateful pod manifest declared above.
// GCE Api requirements: nodes and master need storage r/w permissions.
var _ = SIGDescribe("StatefulSet", func() {
	f := framework.NewDefaultFramework("statefulset")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var serverMinorVersion int

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		if v, err := c.Discovery().ServerVersion(); err != nil {
			framework.Logf("Failed to discovery server version: %v", err)
		} else {
			if serverMinorVersion, err = strconv.Atoi(v.Minor); err != nil {
				framework.Logf("Failed to convert server version %+v: %v", v, err)
			}
		}
	})

	framework.KruiseDescribe("Basic StatefulSet functionality [StatefulSetBasic]", func() {
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var statefulPodMounts, podMounts []v1.VolumeMount
		var ss *appsv1beta1.StatefulSet

		ginkgo.BeforeEach(func() {
			statefulPodMounts = []v1.VolumeMount{{Name: "datadir", MountPath: "/data/"}}
			podMounts = []v1.VolumeMount{{Name: "home", MountPath: "/home"}}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 2, statefulPodMounts, podMounts, labels)

			ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
			headlessService := framework.CreateServiceSpec(headlessSvcName, "", true, labels)
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), headlessService, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})

		// This can't be Conformance yet because it depends on a default
		// StorageClass and a dynamic provisioner.
		ginkgo.It("should provide basic identity", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			sst := framework.NewStatefulSetTester(c, kc)
			sst.PauseNewPods(ss)

			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Saturating stateful set " + ss.Name)
			sst.Saturate(ss)

			ginkgo.By("Verifying statefulset mounted data directory is usable")
			framework.ExpectNoError(sst.CheckMount(ss, "/data"))

			ginkgo.By("Verifying statefulset provides a stable hostname for each pod")
			framework.ExpectNoError(sst.CheckHostname(ss))

			ginkgo.By("Verifying statefulset set proper service name")
			framework.ExpectNoError(sst.CheckServiceName(ss, headlessSvcName))

			cmd := "echo $(hostname) | dd of=/data/hostname conv=fsync"
			ginkgo.By("Running " + cmd + " in all stateful pods")
			framework.ExpectNoError(sst.ExecInStatefulPods(ss, cmd))

			ginkgo.By("Restarting statefulset " + ss.Name)
			sst.Restart(ss)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)

			ginkgo.By("Verifying statefulset mounted data directory is usable")
			framework.ExpectNoError(sst.CheckMount(ss, "/data"))

			cmd = "if [ \"$(cat /data/hostname)\" = \"$(hostname)\" ]; then exit 0; else exit 1; fi"
			ginkgo.By("Running " + cmd + " in all stateful pods")
			framework.ExpectNoError(sst.ExecInStatefulPods(ss, cmd))
		})

		// This can't be Conformance yet because it depends on a default
		// StorageClass and a dynamic provisioner.
		ginkgo.It("should adopt matching orphans and release non-matching pods", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 1
			sst := framework.NewStatefulSetTester(c, kc)
			sst.PauseNewPods(ss)

			// Replace ss with the one returned from Create() so it has the UID.
			// Save Kind since it won't be populated in the returned ss.
			kind := ss.Kind
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ss.Kind = kind

			ginkgo.By("Saturating stateful set " + ss.Name)
			sst.Saturate(ss)
			pods := sst.GetPodList(ss)
			gomega.Expect(pods.Items).To(gomega.HaveLen(int(*ss.Spec.Replicas)))

			ginkgo.By("Checking that stateful set pods are created with ControllerRef")
			pod := pods.Items[0]
			controllerRef := metav1.GetControllerOf(&pod)
			gomega.Expect(controllerRef).ToNot(gomega.BeNil())
			gomega.Expect(controllerRef.Kind).To(gomega.Equal(ss.Kind))
			gomega.Expect(controllerRef.Name).To(gomega.Equal(ss.Name))
			gomega.Expect(controllerRef.UID).To(gomega.Equal(ss.UID))

			ginkgo.By("Orphaning one of the stateful set's pods")
			f.PodClient().Update(pod.Name, func(pod *v1.Pod) {
				pod.OwnerReferences = nil
			})

			ginkgo.By("Checking that the stateful set readopts the pod")
			gomega.Expect(framework.WaitForPodCondition(c, pod.Namespace, pod.Name, "adopted", framework.StatefulSetTimeout,
				func(pod *v1.Pod) (bool, error) {
					controllerRef := metav1.GetControllerOf(pod)
					if controllerRef == nil {
						return false, nil
					}
					if controllerRef.Kind != ss.Kind || controllerRef.Name != ss.Name || controllerRef.UID != ss.UID {
						return false, fmt.Errorf("pod has wrong controllerRef: %v", controllerRef)
					}
					return true, nil
				},
			)).To(gomega.Succeed(), "wait for pod %q to be readopted", pod.Name)

			ginkgo.By("Removing the labels from one of the stateful set's pods")
			prevLabels := pod.Labels
			f.PodClient().Update(pod.Name, func(pod *v1.Pod) {
				pod.Labels = nil
			})

			ginkgo.By("Checking that the stateful set releases the pod")
			gomega.Expect(framework.WaitForPodCondition(c, pod.Namespace, pod.Name, "released", framework.StatefulSetTimeout,
				func(pod *v1.Pod) (bool, error) {
					controllerRef := metav1.GetControllerOf(pod)
					if controllerRef != nil {
						return false, nil
					}
					return true, nil
				},
			)).To(gomega.Succeed(), "wait for pod %q to be released", pod.Name)

			// If we don't do this, the test leaks the Pod and PVC.
			ginkgo.By("Readding labels to the stateful set's pod")
			f.PodClient().Update(pod.Name, func(pod *v1.Pod) {
				pod.Labels = prevLabels
			})

			ginkgo.By("Checking that the stateful set readopts the pod")
			gomega.Expect(framework.WaitForPodCondition(c, pod.Namespace, pod.Name, "adopted", framework.StatefulSetTimeout,
				func(pod *v1.Pod) (bool, error) {
					controllerRef := metav1.GetControllerOf(pod)
					if controllerRef == nil {
						return false, nil
					}
					if controllerRef.Kind != ss.Kind || controllerRef.Name != ss.Name || controllerRef.UID != ss.UID {
						return false, fmt.Errorf("pod has wrong controllerRef: %v", controllerRef)
					}
					return true, nil
				},
			)).To(gomega.Succeed(), "wait for pod %q to be readopted", pod.Name)
		})

		// This can't be Conformance yet because it depends on a default
		// StorageClass and a dynamic provisioner.
		ginkgo.It("should not deadlock when a pod's predecessor fails", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 2
			sst := framework.NewStatefulSetTester(c, kc)
			sst.PauseNewPods(ss)

			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			time.Sleep(time.Minute)
			sst.WaitForRunning(1, 0, ss)

			ginkgo.By("Resuming stateful pod at index 0.")
			sst.ResumeNextPod(ss)

			ginkgo.By("Waiting for stateful pod at index 1 to enter running.")
			sst.WaitForRunning(2, 1, ss)

			// Now we have 1 healthy and 1 unhealthy stateful pod. Deleting the healthy stateful pod should *not*
			// create a new stateful pod till the remaining stateful pod becomes healthy, which won't happen till
			// we set the healthy bit.

			ginkgo.By("Deleting healthy stateful pod at index 0.")
			sst.DeleteStatefulPodAtIndex(0, ss)

			ginkgo.By("Confirming stateful pod at index 0 is recreated.")
			sst.WaitForRunning(2, 1, ss)

			ginkgo.By("Resuming stateful pod at index 1.")
			sst.ResumeNextPod(ss)

			ginkgo.By("Confirming all stateful pods in statefulset are created.")
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
		})

		// This can't be Conformance yet because it depends on a default
		// StorageClass and a dynamic provisioner.
		ginkgo.It("should perform rolling updates and roll backs of template modifications with PVCs", func() {
			ginkgo.By("Creating a new StatefulSet with PVCs")
			*(ss.Spec.Replicas) = 3
			rollbackTest(c, kc, ns, ss)
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Rolling Update
			Description: StatefulSet MUST support the RollingUpdate strategy to automatically replace Pods one at a time when the Pod template changes. The StatefulSet's status MUST indicate the CurrentRevision and UpdateRevision. If the template is changed to match a prior revision, StatefulSet MUST detect this as a rollback instead of creating a new revision. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("should perform rolling updates and roll backs of template modifications", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet("ss2", ns, headlessSvcName, 3, nil, nil, labels)
			rollbackTest(c, kc, ns, ss)
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Rolling Update with Partition
			Description: StatefulSet's RollingUpdate strategy MUST support the Partition parameter for canaries and phased rollouts. If a Pod is deleted while a rolling update is in progress, StatefulSet MUST restore the Pod without violating the Partition. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("should perform canary updates and phased rolling updates of template modifications", func() {
			ginkgo.By("Creating a new StaefulSet")
			ss := framework.NewStatefulSet("ss2", ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
					return &appsv1beta1.RollingUpdateStatefulSetStrategy{
						Partition: func() *int32 {
							i := int32(3)
							return &i
						}()}
				}(),
			}
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to currentRevision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}
			newImage := NewNginxImage
			oldImage := ss.Spec.Template.Spec.Containers[0].Image

			ginkgo.By(fmt.Sprintf("Updating stateful set template: update image from %s to %s", oldImage, newImage))
			gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")

			ginkgo.By("Not applying an update when the partition is greater than the number of replicas")
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
					fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Spec.Containers[0].Image,
						oldImage))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Performing a canary update")
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
					return &appsv1beta1.RollingUpdateStatefulSetStrategy{
						Partition: func() *int32 {
							i := int32(2)
							return &i
						}()}
				}(),
			}
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
					Type: apps.RollingUpdateStatefulSetStrategyType,
					RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
						return &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: func() *int32 {
								i := int32(2)
								return &i
							}()}
					}(),
				}
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ss, pods = sst.WaitForPartitionedRollingUpdate(ss)
			for i := range pods.Items {
				if i < int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							oldImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							currentRevision))
				} else {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to new image  %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							newImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to new revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							updateRevision))
				}
			}

			ginkgo.By("Restoring Pods to the correct revision when they are deleted")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				if i < int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							oldImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							currentRevision))
				} else {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to new image  %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							newImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to new revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							updateRevision))
				}
			}

			ginkgo.By("Performing a phased rolling update")
			for i := int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) - 1; i >= 0; i-- {
				ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
					update.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
							j := int32(i)
							return &appsv1beta1.RollingUpdateStatefulSetStrategy{
								Partition: &j,
							}
						}(),
					}
				})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				ss, pods = sst.WaitForPartitionedRollingUpdate(ss)
				for i := range pods.Items {
					if i < int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) {
						gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
							fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
								pods.Items[i].Namespace,
								pods.Items[i].Name,
								pods.Items[i].Spec.Containers[0].Image,
								oldImage))
						gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
							fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
								pods.Items[i].Namespace,
								pods.Items[i].Name,
								pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
								currentRevision))
					} else {
						gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
							fmt.Sprintf("Pod %s/%s has image %s not equal to new image  %s",
								pods.Items[i].Namespace,
								pods.Items[i].Name,
								pods.Items[i].Spec.Containers[0].Image,
								newImage))
						gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
							fmt.Sprintf("Pod %s/%s has revision %s not equal to new revision %s",
								pods.Items[i].Namespace,
								pods.Items[i].Name,
								pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
								updateRevision))
					}
				}
			}
			gomega.Expect(ss.Status.CurrentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s current revision %s does not equal update revision %s on update completion",
					ss.Namespace,
					ss.Name,
					ss.Status.CurrentRevision,
					updateRevision))

		})

		// Do not mark this as Conformance.
		// The legacy OnDelete strategy only exists for backward compatibility with pre-v1 APIs.
		ginkgo.It("should implement legacy replacement when the update strategy is OnDelete", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet("ss2", ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.OnDeleteStatefulSetStrategyType,
			}
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Restoring Pods to the current revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}
			newImage := NewNginxImage
			oldImage := ss.Spec.Template.Spec.Containers[0].Image

			ginkgo.By(fmt.Sprintf("Updating stateful set template: update image from %s to %s", oldImage, newImage))
			gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")

			ginkgo.By("Recreating Pods at the new revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
					fmt.Sprintf("Pod %s/%s has image %s not equal to new image %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Spec.Containers[0].Image,
						newImage))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
					fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						updateRevision))
			}
		})

		/*
			Testname: AdvancedStatefulSet, InPlaceUpdate
			Description: StatefulSet MUST in-place update pods for pod inplace update strategy. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("should in-place update pods when the pod update strategy is InPlace", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
					PodUpdatePolicy:       appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
					InPlaceUpdateStrategy: &appspub.InPlaceUpdateStrategy{GracePeriodSeconds: 10},
				},
			}
			ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Restoring Pods to the current revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}
			newImage := NewNginxImage
			oldImage := ss.Spec.Template.Spec.Containers[0].Image

			ginkgo.By(fmt.Sprintf("Updating stateful set template: update image from %s to %s", oldImage, newImage))
			gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
			var partition int32 = 3
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				if update.Spec.UpdateStrategy.RollingUpdate == nil {
					update.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{}
				}
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				partition = 0
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("InPlace update Pods at the new revision")
			sst.WaitForPodUpdatedAndRunning(ss, pods.Items[0].Name, currentRevision)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
					fmt.Sprintf("Pod %s/%s has image %s not equal to new image %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Spec.Containers[0].Image,
						newImage))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
					fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						updateRevision))
			}
		})

		framework.ConformanceIt("should in-place update env from label", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
					PodUpdatePolicy:       appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
					InPlaceUpdateStrategy: &appspub.InPlaceUpdateStrategy{GracePeriodSeconds: 10},
				},
			}
			ss.Spec.Template.ObjectMeta.Labels = map[string]string{"test-env": "foo"}
			for k, v := range labels {
				ss.Spec.Template.ObjectMeta.Labels[k] = v
			}
			ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
				Name:      "TEST_ENV",
				ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['test-env']"}},
			})
			ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Restoring Pods to the current revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Updating stateful set template: update label for env")
			var partition int32 = 3
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.ObjectMeta.Labels["test-env"] = "bar"
				if update.Spec.UpdateStrategy.RollingUpdate == nil {
					update.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{}
				}
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				partition = 0
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("InPlace update Pods at the new revision")
			sst.WaitForPodUpdatedAndRunning(ss, pods.Items[0].Name, currentRevision)
			sst.WaitForRunningAndReady(3, ss)

			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Status.ContainerStatuses[0].RestartCount).To(gomega.Equal(int32(1)))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision))
			}
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Scaling
			Description: StatefulSet MUST create Pods in ascending order by ordinal index when scaling up, and delete Pods in descending order when scaling down. Scaling up or down MUST pause if any Pods belonging to the StatefulSet are unhealthy. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("Scaling should happen in predictable order and halt if any stateful pod is unhealthy", func() {
			psLabels := klabels.Set(labels)
			ginkgo.By("Initializing watcher for selector " + psLabels.String())
			watcher, err := f.ClientSet.CoreV1().Pods(ns).Watch(context.TODO(), metav1.ListOptions{
				LabelSelector: psLabels.AsSelector().String(),
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating stateful set " + ssName + " in namespace " + ns)
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 1, nil, nil, psLabels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss, err = kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Waiting until all stateful set " + ssName + " replicas will be running in namespace " + ns)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)

			ginkgo.By("Confirming that stateful set scale up will halt with unhealthy stateful pod")
			sst.BreakHTTPProbe(ss)
			sst.WaitForRunningAndNotReady(*ss.Spec.Replicas, ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			sst.UpdateReplicas(ss, 3)
			sst.ConfirmStatefulPodCount(1, ss, 10*time.Second, true)

			ginkgo.By("Scaling up stateful set " + ssName + " to 3 replicas and waiting until all of them will be running in namespace " + ns)
			sst.RestoreHTTPProbe(ss)
			sst.WaitForRunningAndReady(3, ss)

			ginkgo.By("Verifying that stateful set " + ssName + " was scaled up in order")
			expectedOrder := []string{ssName + "-0", ssName + "-1", ssName + "-2"}
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), framework.StatefulSetTimeout)
			defer cancel()
			_, err = watchtools.UntilWithoutRetry(ctx, watcher, func(event watch.Event) (bool, error) {
				if event.Type != watch.Added {
					return false, nil
				}
				pod := event.Object.(*v1.Pod)
				if pod.Name == expectedOrder[0] {
					expectedOrder = expectedOrder[1:]
				}
				return len(expectedOrder) == 0, nil

			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Scale down will halt with unhealthy stateful pod")
			watcher, err = f.ClientSet.CoreV1().Pods(ns).Watch(context.TODO(), metav1.ListOptions{
				LabelSelector: psLabels.AsSelector().String(),
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			sst.BreakHTTPProbe(ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			sst.WaitForRunningAndNotReady(3, ss)
			sst.UpdateReplicas(ss, 0)
			sst.ConfirmStatefulPodCount(3, ss, 10*time.Second, true)

			ginkgo.By("Scaling down stateful set " + ssName + " to 0 replicas and waiting until none of pods will run in namespace" + ns)
			sst.RestoreHTTPProbe(ss)
			sst.Scale(ss, 0)

			ginkgo.By("Verifying that stateful set " + ssName + " was scaled down in reverse order")
			expectedOrder = []string{ssName + "-2", ssName + "-1", ssName + "-0"}
			ctx, cancel = watchtools.ContextWithOptionalTimeout(context.Background(), framework.StatefulSetTimeout)
			defer cancel()
			_, err = watchtools.UntilWithoutRetry(ctx, watcher, func(event watch.Event) (bool, error) {
				if event.Type != watch.Deleted {
					return false, nil
				}
				pod := event.Object.(*v1.Pod)
				if pod.Name == expectedOrder[0] {
					expectedOrder = expectedOrder[1:]
				}
				return len(expectedOrder) == 0, nil

			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Burst Scaling
			Description: StatefulSet MUST support the Parallel PodManagementPolicy for burst scaling. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("Burst scaling should run to completion even with unhealthy pods", func() {
			psLabels := klabels.Set(labels)

			ginkgo.By("Creating stateful set " + ssName + " in namespace " + ns)
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 1, nil, nil, psLabels)
			ss.Spec.PodManagementPolicy = apps.ParallelPodManagement
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Waiting until all stateful set " + ssName + " replicas will be running in namespace " + ns)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)

			ginkgo.By("Confirming that stateful set scale up will not halt with unhealthy stateful pod")
			sst.BreakHTTPProbe(ss)
			sst.WaitForRunningAndNotReady(*ss.Spec.Replicas, ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			sst.UpdateReplicas(ss, 3)
			sst.ConfirmStatefulPodCount(3, ss, 10*time.Second, false)

			ginkgo.By("Scaling up stateful set " + ssName + " to 3 replicas and waiting until all of them will be running in namespace " + ns)
			sst.RestoreHTTPProbe(ss)
			sst.WaitForRunningAndReady(3, ss)

			ginkgo.By("Scale down will not halt with unhealthy stateful pod")
			sst.BreakHTTPProbe(ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			sst.WaitForRunningAndNotReady(3, ss)
			sst.UpdateReplicas(ss, 0)
			sst.ConfirmStatefulPodCount(0, ss, 10*time.Second, false)

			ginkgo.By("Scaling down stateful set " + ssName + " to 0 replicas and waiting until none of pods will run in namespace" + ns)
			sst.RestoreHTTPProbe(ss)
			sst.Scale(ss, 0)
			sst.WaitForStatusReplicas(ss, 0)
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Recreate Failed Pod
			Description: StatefulSet MUST delete and recreate Pods it owns that go into a Failed state, such as when they are rejected or evicted by a Node. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("Should recreate evicted statefulset", func() {
			podName := "test-pod"
			statefulPodName := ssName + "-0"
			ginkgo.By("Looking for a node to schedule stateful set and pod")
			nodes := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
			node := nodes.Items[0]

			ginkgo.By("Creating pod with conflicting port in namespace " + f.Namespace.Name)
			conflictingPort := v1.ContainerPort{HostPort: 21017, ContainerPort: 21017, Name: "conflict"}
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "nginx",
							Image: imageutils.GetE2EImage(imageutils.Nginx),
							Ports: []v1.ContainerPort{conflictingPort},
						},
					},
					NodeName: node.Name,
				},
			}
			pod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Creating statefulset with conflicting port in namespace " + f.Namespace.Name)
			ss := framework.NewStatefulSet(ssName, f.Namespace.Name, headlessSvcName, 1, nil, nil, labels)
			statefulPodContainer := &ss.Spec.Template.Spec.Containers[0]
			statefulPodContainer.Ports = append(statefulPodContainer.Ports, conflictingPort)
			ss.Spec.Template.Spec.NodeName = node.Name
			_, err = kc.AppsV1beta1().StatefulSets(f.Namespace.Name).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Waiting until pod " + podName + " will start running in namespace " + f.Namespace.Name)
			if err := f.WaitForPodRunning(podName); err != nil {
				framework.Failf("Pod %v did not start running: %v", podName, err)
			}

			var initialStatefulPodUID types.UID
			ginkgo.By("Waiting until stateful pod " + statefulPodName + " will be recreated and deleted at least once in namespace " + f.Namespace.Name)
			w, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Watch(context.TODO(), metav1.SingleObject(metav1.ObjectMeta{Name: statefulPodName}))
			framework.ExpectNoError(err)
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), framework.StatefulPodTimeout)
			defer cancel()
			// we need to get UID from pod in any state and wait until stateful set controller will remove pod at least once
			_, err = watchtools.UntilWithoutRetry(ctx, w, func(event watch.Event) (bool, error) {
				pod := event.Object.(*v1.Pod)
				switch event.Type {
				case watch.Deleted:
					framework.Logf("Observed delete event for stateful pod %v in namespace %v", pod.Name, pod.Namespace)
					if initialStatefulPodUID == "" {
						return false, nil
					}
					return true, nil
				}
				framework.Logf("Observed stateful pod in namespace: %v, name: %v, uid: %v, status phase: %v. Waiting for statefulset controller to delete.",
					pod.Namespace, pod.Name, pod.UID, pod.Status.Phase)
				initialStatefulPodUID = pod.UID
				return false, nil
			})
			if err != nil {
				framework.Failf("Pod %v expected to be re-created at least once", statefulPodName)
			}

			ginkgo.By("Removing pod with conflicting port in namespace " + f.Namespace.Name)
			err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, *metav1.NewDeleteOptions(0))
			framework.ExpectNoError(err)

			ginkgo.By("Waiting when stateful pod " + statefulPodName + " will be recreated in namespace " + f.Namespace.Name + " and will be in running state")
			// we may catch delete event, that's why we are waiting for running phase like this, and not with watchtools.UntilWithoutRetry
			gomega.Eventually(func() error {
				statefulPod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Get(context.TODO(), statefulPodName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if statefulPod.Status.Phase != v1.PodRunning {
					return fmt.Errorf("Pod %v is not in running phase: %v", statefulPod.Name, statefulPod.Status.Phase)
				} else if statefulPod.UID == initialStatefulPodUID {
					return fmt.Errorf("Pod %v wasn't recreated: %v == %v", statefulPod.Name, statefulPod.UID, initialStatefulPodUID)
				}
				return nil
			}, framework.StatefulPodTimeout, 2*time.Second).Should(gomega.BeNil())
		})

		/*
			Release : v1.16
			Testname: StatefulSet resource Replica scaling
			Description: Create a StatefulSet resource.
			Newly created StatefulSet resource MUST have a scale of one.
			Bring the scale of the StatefulSet resource up to two. StatefulSet scale MUST be at two replicas.
		*/
		framework.ConformanceIt("should have a working scale subresource", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 1, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)

			ginkgo.By("getting scale subresource")
			scale, err := kc.AppsV1beta1().StatefulSets(ns).GetScale(context.TODO(), ssName, metav1.GetOptions{})
			if err != nil {
				framework.Failf("Failed to get scale subresource: %v", err)
			}
			framework.ExpectEqual(scale.Spec.Replicas, int32(1))
			framework.ExpectEqual(scale.Status.Replicas, int32(1))

			ginkgo.By("updating a scale subresource")
			if serverMinorVersion >= 18 {
				scale.ResourceVersion = "" // indicate the scale update should be unconditional
			}
			scale.Spec.Replicas = 2
			scaleResult, err := kc.AppsV1beta1().StatefulSets(ns).UpdateScale(context.TODO(), ssName, scale, metav1.UpdateOptions{})
			if err != nil {
				framework.Failf("Failed to put scale subresource: %v", err)
			}
			framework.ExpectEqual(scaleResult.Spec.Replicas, int32(2))

			ginkgo.By("verifying the statefulset Spec.Replicas was modified")
			ss, err = kc.AppsV1beta1().StatefulSets(ns).Get(context.TODO(), ssName, metav1.GetOptions{})
			if err != nil {
				framework.Failf("Failed to get statefulset resource: %v", err)
			}
			framework.ExpectEqual(*(ss.Spec.Replicas), int32(2))
		})

		/*
			Testname: StatefulSet, ScaleStrategy
			Description: StatefulSet resource MUST support the MaxUnavailable ScaleStrategy for scaling.
			It only affects when create new pod, terminating pod and unavailable pod at the Parallel PodManagementPolicy.
		*/
		framework.ConformanceIt("Should can update pods when the statefulset scale strategy is set", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			maxUnavailable := intstr.FromInt(2)
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, nil, nil, labels)
			ss.Spec.Template.Spec.Containers[0].Name = "busybox"
			ss.Spec.Template.Spec.Containers[0].Image = BusyboxImage
			ss.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "3600"}
			ss.Spec.PodManagementPolicy = apps.ParallelPodManagement
			ss.Spec.Template.Spec.RestartPolicy = v1.RestartPolicyAlways
			ss.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{
				MinReadySeconds: pointer.Int32(3),
				PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
			}
			ss.Spec.ScaleStrategy = &appsv1beta1.StatefulSetScaleStrategy{MaxUnavailable: &maxUnavailable}
			ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			sst := framework.NewStatefulSetTester(c, kc)
			// sst.SetHTTPProbe(ss)
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)

			ginkgo.By("Scaling up stateful set " + ssName + " to 10 replicas and check create new pod equal MaxUnavailable")
			sst.UpdateReplicas(ss, 10)
			sst.ConfirmStatefulPodCount(5, ss, time.Second, false)
			sst.WaitForRunningAndReady(10, ss)

			ginkgo.By("Confirming that stateful can update all pods to be unhealthy")
			maxUnavailable = intstr.FromString("100%")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.ScaleStrategy.MaxUnavailable = &maxUnavailable
				update.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &intstr.IntOrString{Type: intstr.String, StrVal: "100%"}
				update.Spec.Template.Spec.Containers[0].Command = []string{}
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndNotReady(10, ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			ss = sst.WaitForStatus(ss)

			ginkgo.By("Confirming that stateful can update all pods if any stateful pod is unhealthy")

			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Labels["test-update"] = "yes"
				update.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "180"}
			})
			sst.WaitForRunningAndReady(10, ss)
			sst.WaitForStatusReadyReplicas(ss, 10)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var pods *v1.PodList
			sst.WaitForState(ss, func(set *appsv1beta1.StatefulSet, pl *v1.PodList) (bool, error) {
				ss = set
				pods = pl
				sst.SortStatefulPods(pods)
				for i := range pods.Items {
					if pods.Items[i].Labels[apps.StatefulSetRevisionLabel] != set.Status.UpdateRevision {
						framework.Logf("Waiting for Pod %s/%s to have revision %s update revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							set.Status.UpdateRevision,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel])
						return false, nil
					}
				}
				return true, nil
			})

			ginkgo.By("Confirming Pods were updated successful")
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels["test-update"]).To(gomega.Equal("yes"))
			}
		})
	})

	//ginkgo.Describe("Deploy clustered applications [Feature:StatefulSet] [Slow]", func() {
	//	var appTester *clusterAppTester
	//
	//	ginkgo.BeforeEach(func() {
	//		appTester = &clusterAppTester{client: c, ns: ns}
	//	})
	//
	//	ginkgo.AfterEach(func() {
	//		if ginkgo.CurrentGinkgoTestDescription().Failed {
	//			framework.DumpDebugInfo(c, ns)
	//		}
	//		framework.Logf("Deleting all statefulset in ns %v", ns)
	//		e2estatefulset.DeleteAllStatefulSets(c, ns)
	//	})
	//
	//	// Do not mark this as Conformance.
	//	// StatefulSet Conformance should not be dependent on specific applications.
	//	ginkgo.It("should creating a working zookeeper cluster", func() {
	//		e2epv.SkipIfNoDefaultStorageClass(c)
	//		appTester.statefulPod = &zookeeperTester{client: c}
	//		appTester.run()
	//	})
	//
	//	// Do not mark this as Conformance.
	//	// StatefulSet Conformance should not be dependent on specific applications.
	//	ginkgo.It("should creating a working redis cluster", func() {
	//		e2epv.SkipIfNoDefaultStorageClass(c)
	//		appTester.statefulPod = &redisTester{client: c}
	//		appTester.run()
	//	})
	//
	//	// Do not mark this as Conformance.
	//	// StatefulSet Conformance should not be dependent on specific applications.
	//	ginkgo.It("should creating a working mysql cluster", func() {
	//		e2epv.SkipIfNoDefaultStorageClass(c)
	//		appTester.statefulPod = &mysqlGaleraTester{client: c}
	//		appTester.run()
	//	})
	//
	//	// Do not mark this as Conformance.
	//	// StatefulSet Conformance should not be dependent on specific applications.
	//	ginkgo.It("should creating a working CockroachDB cluster", func() {
	//		e2epv.SkipIfNoDefaultStorageClass(c)
	//		appTester.statefulPod = &cockroachDBTester{client: c}
	//		appTester.run()
	//	})
	//})
	//
	//// Make sure minReadySeconds is honored
	//// Don't mark it as conformance yet
	//ginkgo.It("MinReadySeconds should be honored when enabled", func() {
	//	ssName := "test-ss"
	//	headlessSvcName := "test"
	//	// Define StatefulSet Labels
	//	ssPodLabels := map[string]string{
	//		"name": "sample-pod",
	//		"pod":  WebserverImageName,
	//	}
	//	ss := e2estatefulset.NewStatefulSet(ssName, ns, headlessSvcName, 1, nil, nil, ssPodLabels)
	//	setHTTPProbe(ss)
	//	ss, err := c.AppsV1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
	//	framework.ExpectNoError(err)
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 1)
	//})
	//
	//ginkgo.It("AvailableReplicas should get updated accordingly when MinReadySeconds is enabled", func() {
	//	ssName := "test-ss"
	//	headlessSvcName := "test"
	//	// Define StatefulSet Labels
	//	ssPodLabels := map[string]string{
	//		"name": "sample-pod",
	//		"pod":  WebserverImageName,
	//	}
	//	ss := e2estatefulset.NewStatefulSet(ssName, ns, headlessSvcName, 2, nil, nil, ssPodLabels)
	//	ss.Spec.MinReadySeconds = 30
	//	setHTTPProbe(ss)
	//	ss, err := c.AppsV1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
	//	framework.ExpectNoError(err)
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 0)
	//	// let's check that the availableReplicas have still not updated
	//	time.Sleep(5 * time.Second)
	//	ss, err = c.AppsV1().StatefulSets(ns).Get(context.TODO(), ss.Name, metav1.GetOptions{})
	//	framework.ExpectNoError(err)
	//	if ss.Status.AvailableReplicas != 0 {
	//		framework.Failf("invalid number of availableReplicas: expected=%v received=%v", 0, ss.Status.AvailableReplicas)
	//	}
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 2)
	//
	//	ss, err = updateStatefulSetWithRetries(c, ns, ss.Name, func(update *appsv1.StatefulSet) {
	//		update.Spec.MinReadySeconds = 3600
	//	})
	//	framework.ExpectNoError(err)
	//	// We don't expect replicas to be updated till 1 hour, so the availableReplicas should be 0
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 0)
	//
	//	ss, err = updateStatefulSetWithRetries(c, ns, ss.Name, func(update *appsv1.StatefulSet) {
	//		update.Spec.MinReadySeconds = 0
	//	})
	//	framework.ExpectNoError(err)
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 2)
	//
	//	ginkgo.By("check availableReplicas are shown in status")
	//	out, err := framework.RunKubectl(ns, "get", "statefulset", ss.Name, "-o=yaml")
	//	framework.ExpectNoError(err)
	//	if !strings.Contains(out, "availableReplicas: 2") {
	//		framework.Failf("invalid number of availableReplicas: expected=%v received=%v", 2, out)
	//	}
	//})

	ginkgo.Describe("Non-retain StatefulSetPersistentVolumeClaimPolicy [Feature:StatefulSetAutoDeletePVC]", func() {
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var statefulPodMounts, podMounts []v1.VolumeMount
		var ss *appsv1beta1.StatefulSet

		ginkgo.BeforeEach(func() {
			statefulPodMounts = []v1.VolumeMount{{Name: "datadir", MountPath: "/data/"}}
			podMounts = []v1.VolumeMount{{Name: "home", MountPath: "/home"}}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 2, statefulPodMounts, podMounts, labels)

			ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
			headlessService := framework.CreateServiceSpec(headlessSvcName, "", true, labels)
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), headlessService, metav1.CreateOptions{})
			framework.ExpectNoError(err)
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})

		ginkgo.It("should delete PVCs with a WhenDeleted policy", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist with their owner refs")
			err = verifyStatefulSetPVCsExistWithOwnerRefs(c, kc, ss, []int{0, 1, 2}, true, false)
			framework.ExpectNoError(err)

			ginkgo.By("Deleting stateful set " + ss.Name)
			err = kc.AppsV1beta1().StatefulSets(ns).Delete(context.TODO(), ss.Name, metav1.DeleteOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Verifying PVCs deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{})
			framework.ExpectNoError(err)
		})

		ginkgo.It("should delete PVCs with a OnScaledown policy", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenScaled: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0, 1, 2})
			framework.ExpectNoError(err)

			ginkgo.By("Scaling stateful set " + ss.Name + " to one replica")
			ss, err = framework.NewStatefulSetTester(c, kc).Scale(ss, 1)
			framework.ExpectNoError(err)

			ginkgo.By("Verifying all but one PVC deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0})
			framework.ExpectNoError(err)
		})

		ginkgo.It("should delete PVCs with a OnScaledown policy and reserveOrdinals=[0,1]", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenScaled: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			ss.Spec.ReserveOrdinals = []int{0, 1}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist")
			err = verifyStatefulSetPVCsExist(c, ss, []int{2, 3, 4})
			framework.ExpectNoError(err)

			ginkgo.By("Scaling stateful set " + ss.Name + " to one replica")
			ss, err = framework.NewStatefulSetTester(c, kc).Scale(ss, 1)
			framework.ExpectNoError(err)

			ginkgo.By("Verifying all but one PVC deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{2})
			framework.ExpectNoError(err)
		})

		ginkgo.It("should delete PVCs after adopting pod (WhenDeleted)", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist with their owner refs")
			err = verifyStatefulSetPVCsExistWithOwnerRefs(c, kc, ss, []int{0, 1, 2}, true, false)
			framework.ExpectNoError(err)

			ginkgo.By("Orphaning the 3rd pod")
			patch, err := json.Marshal(metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{},
			})
			framework.ExpectNoError(err, "Could not Marshal JSON for patch payload")
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), fmt.Sprintf("%s-2", ss.Name), types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{}, "")
			framework.ExpectNoError(err, "Could not patch payload")

			ginkgo.By("Deleting stateful set " + ss.Name)
			err = kc.AppsV1beta1().StatefulSets(ns).Delete(context.TODO(), ss.Name, metav1.DeleteOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Verifying PVCs deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{})
			framework.ExpectNoError(err)
		})

		ginkgo.It("should delete PVCs after adopting pod (WhenScaled) [Feature:StatefulSetAutoDeletePVC]", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenScaled: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0, 1, 2})
			framework.ExpectNoError(err)

			ginkgo.By("Orphaning the 3rd pod")
			patch, err := json.Marshal(metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{},
			})
			framework.ExpectNoError(err, "Could not Marshal JSON for patch payload")
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), fmt.Sprintf("%s-2", ss.Name), types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{}, "")
			framework.ExpectNoError(err, "Could not patch payload")

			ginkgo.By("Scaling stateful set " + ss.Name + " to one replica")
			ss, err = framework.NewStatefulSetTester(c, kc).Scale(ss, 1)
			framework.ExpectNoError(err)

			ginkgo.By("Verifying all but one PVC deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0})
			framework.ExpectNoError(err)
		})
	})
})

func kubectlExecWithRetries(args ...string) (out string) {
	var err error
	for i := 0; i < 3; i++ {
		if out, err = framework.RunKubectl(args...); err == nil {
			return
		}
		framework.Logf("Retrying %v:\nerror %v\nstdout %v", args, err, out)
	}
	framework.Failf("Failed to execute \"%v\" with retries: %v", args, err)
	return
}

type statefulPodTester interface {
	deploy(ns string) *appsv1beta1.StatefulSet
	write(statefulPodIndex int, kv map[string]string)
	read(statefulPodIndex int, key string) string
	name() string
}

//type clusterAppTester struct {
//	ns          string
//	statefulPod statefulPodTester
//	tester      *framework.StatefulSetTester
//}
//
//func (c *clusterAppTester) run() {
//	ginkgo.By("Deploying " + c.statefulPod.name())
//	ss := c.statefulPod.deploy(c.ns)
//
//	ginkgo.By("Creating foo:bar in member with index 0")
//	c.statefulPod.write(0, map[string]string{"foo": "bar"})
//
//	switch c.statefulPod.(type) {
//	case *mysqlGaleraTester:
//		// Don't restart MySQL cluster since it doesn't handle restarts well
//	default:
//		if restartCluster {
//			ginkgo.By("Restarting stateful set " + ss.Name)
//			c.tester.Restart(ss)
//			c.tester.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
//		}
//	}
//
//	ginkgo.By("Reading value under foo from member with index 2")
//	if err := pollReadWithTimeout(c.statefulPod, 2, "foo", "bar"); err != nil {
//		framework.Failf("%v", err)
//	}
//}
//
//type zookeeperTester struct {
//	ss     *appsv1beta1.StatefulSet
//	tester *framework.StatefulSetTester
//}
//
//func (z *zookeeperTester) name() string {
//	return "zookeeper"
//}
//
//func (z *zookeeperTester) deploy(ns string) *appsv1beta1.StatefulSet {
//	z.ss = z.tester.CreateStatefulSet(zookeeperManifestPath, ns)
//	return z.ss
//}
//
//func (z *zookeeperTester) write(statefulPodIndex int, kv map[string]string) {
//	name := fmt.Sprintf("%v-%d", z.ss.Name, statefulPodIndex)
//	ns := fmt.Sprintf("--namespace=%v", z.ss.Namespace)
//	for k, v := range kv {
//		cmd := fmt.Sprintf("/opt/zookeeper/bin/zkCli.sh create /%v %v", k, v)
//		framework.Logf(framework.RunKubectlOrDie("exec", ns, name, "--", "/bin/sh", "-c", cmd))
//	}
//}
//
//func (z *zookeeperTester) read(statefulPodIndex int, key string) string {
//	name := fmt.Sprintf("%v-%d", z.ss.Name, statefulPodIndex)
//	ns := fmt.Sprintf("--namespace=%v", z.ss.Namespace)
//	cmd := fmt.Sprintf("/opt/zookeeper/bin/zkCli.sh get /%v", key)
//	return lastLine(framework.RunKubectlOrDie("exec", ns, name, "--", "/bin/sh", "-c", cmd))
//}
//
//type mysqlGaleraTester struct {
//	ss     *appsv1beta1.StatefulSet
//	tester *framework.StatefulSetTester
//}
//
//func (m *mysqlGaleraTester) name() string {
//	return "mysql: galera"
//}
//
//func (m *mysqlGaleraTester) mysqlExec(cmd, ns, podName string) string {
//	cmd = fmt.Sprintf("/usr/bin/mysql -u root -B -e '%v'", cmd)
//	// TODO: Find a readiness probe for mysql that guarantees writes will
//	// succeed and ditch retries. Current probe only reads, so there's a window
//	// for a race.
//	return kubectlExecWithRetries(fmt.Sprintf("--namespace=%v", ns), "exec", podName, "--", "/bin/sh", "-c", cmd)
//}
//
//func (m *mysqlGaleraTester) deploy(ns string) *appsv1beta1.StatefulSet {
//	m.ss = m.tester.CreateStatefulSet(mysqlGaleraManifestPath, ns)
//
//	framework.Logf("Deployed statefulset %v, initializing database", m.ss.Name)
//	for _, cmd := range []string{
//		"create database statefulset;",
//		"use statefulset; create table foo (k varchar(20), v varchar(20));",
//	} {
//		framework.Logf(m.mysqlExec(cmd, ns, fmt.Sprintf("%v-0", m.ss.Name)))
//	}
//	return m.ss
//}
//
//func (m *mysqlGaleraTester) write(statefulPodIndex int, kv map[string]string) {
//	name := fmt.Sprintf("%v-%d", m.ss.Name, statefulPodIndex)
//	for k, v := range kv {
//		cmd := fmt.Sprintf("use statefulset; insert into foo (k, v) values (\"%v\", \"%v\");", k, v)
//		framework.Logf(m.mysqlExec(cmd, m.ss.Namespace, name))
//	}
//}
//
//func (m *mysqlGaleraTester) read(statefulPodIndex int, key string) string {
//	name := fmt.Sprintf("%v-%d", m.ss.Name, statefulPodIndex)
//	return lastLine(m.mysqlExec(fmt.Sprintf("use statefulset; select v from foo where k=\"%v\";", key), m.ss.Namespace, name))
//}
//
//type redisTester struct {
//	ss     *appsv1beta1.StatefulSet
//	tester *framework.StatefulSetTester
//}
//
//func (m *redisTester) name() string {
//	return "redis: master/slave"
//}
//
//func (m *redisTester) redisExec(cmd, ns, podName string) string {
//	cmd = fmt.Sprintf("/opt/redis/redis-cli -h %v %v", podName, cmd)
//	return framework.RunKubectlOrDie(fmt.Sprintf("--namespace=%v", ns), "exec", podName, "--", "/bin/sh", "-c", cmd)
//}
//
//func (m *redisTester) deploy(ns string) *appsv1beta1.StatefulSet {
//	m.ss = m.tester.CreateStatefulSet(redisManifestPath, ns)
//	return m.ss
//}
//
//func (m *redisTester) write(statefulPodIndex int, kv map[string]string) {
//	name := fmt.Sprintf("%v-%d", m.ss.Name, statefulPodIndex)
//	for k, v := range kv {
//		framework.Logf(m.redisExec(fmt.Sprintf("SET %v %v", k, v), m.ss.Namespace, name))
//	}
//}
//
//func (m *redisTester) read(statefulPodIndex int, key string) string {
//	name := fmt.Sprintf("%v-%d", m.ss.Name, statefulPodIndex)
//	return lastLine(m.redisExec(fmt.Sprintf("GET %v", key), m.ss.Namespace, name))
//}
//
//type cockroachDBTester struct {
//	ss     *appsv1beta1.StatefulSet
//	tester *framework.StatefulSetTester
//}
//
//func (c *cockroachDBTester) name() string {
//	return "CockroachDB"
//}
//
//func (c *cockroachDBTester) cockroachDBExec(cmd, ns, podName string) string {
//	cmd = fmt.Sprintf("/cockroach/cockroach sql --insecure --host %s.cockroachdb -e \"%v\"", podName, cmd)
//	return framework.RunKubectlOrDie(fmt.Sprintf("--namespace=%v", ns), "exec", podName, "--", "/bin/sh", "-c", cmd)
//}
//
//func (c *cockroachDBTester) deploy(ns string) *appsv1beta1.StatefulSet {
//	c.ss = c.tester.CreateStatefulSet(cockroachDBManifestPath, ns)
//	framework.Logf("Deployed statefulset %v, initializing database", c.ss.Name)
//	for _, cmd := range []string{
//		"CREATE DATABASE IF NOT EXISTS foo;",
//		"CREATE TABLE IF NOT EXISTS foo.bar (k STRING PRIMARY KEY, v STRING);",
//	} {
//		framework.Logf(c.cockroachDBExec(cmd, ns, fmt.Sprintf("%v-0", c.ss.Name)))
//	}
//	return c.ss
//}
//
//func (c *cockroachDBTester) write(statefulPodIndex int, kv map[string]string) {
//	name := fmt.Sprintf("%v-%d", c.ss.Name, statefulPodIndex)
//	for k, v := range kv {
//		cmd := fmt.Sprintf("UPSERT INTO foo.bar VALUES ('%v', '%v');", k, v)
//		framework.Logf(c.cockroachDBExec(cmd, c.ss.Namespace, name))
//	}
//}
//func (c *cockroachDBTester) read(statefulPodIndex int, key string) string {
//	name := fmt.Sprintf("%v-%d", c.ss.Name, statefulPodIndex)
//	return lastLine(c.cockroachDBExec(fmt.Sprintf("SELECT v FROM foo.bar WHERE k='%v';", key), c.ss.Namespace, name))
//}

func lastLine(out string) string {
	outLines := strings.Split(strings.Trim(out, "\n"), "\n")
	return outLines[len(outLines)-1]
}

func pollReadWithTimeout(statefulPod statefulPodTester, statefulPodNumber int, key, expectedVal string) error {
	err := wait.PollImmediate(time.Second, readTimeout, func() (bool, error) {
		val := statefulPod.read(statefulPodNumber, key)
		if val == "" {
			return false, nil
		} else if val != expectedVal {
			return false, fmt.Errorf("expected value %v, found %v", expectedVal, val)
		}
		return true, nil
	})

	if err == wait.ErrWaitTimeout {
		return fmt.Errorf("timed out when trying to read value for key %v from stateful pod %d", key, statefulPodNumber)
	}
	return err
}

// This function is used by two tests to test StatefulSet rollbacks: one using
// PVCs and one using no storage.
func rollbackTest(c clientset.Interface, kc kruiseclientset.Interface, ns string, ss *appsv1beta1.StatefulSet) {
	sst := framework.NewStatefulSetTester(c, kc)
	sst.SetHTTPProbe(ss)
	ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
	ss = sst.WaitForStatus(ss)
	currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
	gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
		fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
			ss.Namespace, ss.Name, updateRevision, currentRevision))
	pods := sst.GetPodList(ss)
	for i := range pods.Items {
		gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
			fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
				currentRevision))
	}
	sst.SortStatefulPods(pods)
	err = sst.BreakPodHTTPProbe(ss, &pods.Items[1])
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ss, pods = sst.WaitForPodNotReady(ss, pods.Items[1].Name)
	newImage := NewNginxImage
	oldImage := ss.Spec.Template.Spec.Containers[0].Image

	ginkgo.By(fmt.Sprintf("Updating StatefulSet template: update image from %s to %s", oldImage, newImage))
	gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
	ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
		update.Spec.Template.Spec.Containers[0].Image = newImage
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("Creating a new revision")
	ss = sst.WaitForStatus(ss)
	currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
	gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
		"Current revision should not equal update revision during rolling update")

	ginkgo.By("Updating Pods in reverse ordinal order")
	pods = sst.GetPodList(ss)
	sst.SortStatefulPods(pods)
	err = sst.RestorePodHTTPProbe(ss, &pods.Items[1])
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ss, pods = sst.WaitForPodReady(ss, pods.Items[1].Name)
	ss, pods = sst.WaitForRollingUpdate(ss)
	gomega.Expect(ss.Status.CurrentRevision).To(gomega.Equal(updateRevision),
		fmt.Sprintf("StatefulSet %s/%s current revision %s does not equal update revision %s on update completion",
			ss.Namespace,
			ss.Name,
			ss.Status.CurrentRevision,
			updateRevision))
	for i := range pods.Items {
		gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
			fmt.Sprintf(" Pod %s/%s has image %s not have new image %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Spec.Containers[0].Image,
				newImage))
		gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
			fmt.Sprintf("Pod %s/%s revision %s is not equal to update revision %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
				updateRevision))
	}

	ginkgo.By("Rolling back to a previous revision")
	err = sst.BreakPodHTTPProbe(ss, &pods.Items[1])
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ss, pods = sst.WaitForPodNotReady(ss, pods.Items[1].Name)
	priorRevision := currentRevision
	ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
		update.Spec.Template.Spec.Containers[0].Image = oldImage
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ss = sst.WaitForStatus(ss)
	currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
	gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
		"Current revision should not equal update revision during roll back")
	gomega.Expect(priorRevision).To(gomega.Equal(updateRevision),
		"Prior revision should equal update revision during roll back")

	ginkgo.By("Rolling back update in reverse ordinal order")
	pods = sst.GetPodList(ss)
	sst.SortStatefulPods(pods)
	sst.RestorePodHTTPProbe(ss, &pods.Items[1])
	ss, pods = sst.WaitForPodReady(ss, pods.Items[1].Name)
	ss, pods = sst.WaitForRollingUpdate(ss)
	gomega.Expect(ss.Status.CurrentRevision).To(gomega.Equal(priorRevision),
		fmt.Sprintf("StatefulSet %s/%s current revision %s does not equal prior revision %s on rollback completion",
			ss.Namespace,
			ss.Name,
			ss.Status.CurrentRevision,
			updateRevision))

	for i := range pods.Items {
		gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
			fmt.Sprintf("Pod %s/%s has image %s not equal to previous image %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Spec.Containers[0].Image,
				oldImage))
		gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(priorRevision),
			fmt.Sprintf("Pod %s/%s revision %s is not equal to prior revision %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
				priorRevision))
	}
}

// verifyStatefulSetPVCsExist confirms that exactly the PVCs for ss with the specified ids exist. This polls until the situation occurs, an error happens, or until timeout (in the latter case an error is also returned). Beware that this cannot tell if a PVC will be deleted at some point in the future, so if used to confirm that no PVCs are deleted, the caller should wait for some event giving the PVCs a reasonable chance to be deleted, before calling this function.
func verifyStatefulSetPVCsExist(c clientset.Interface, ss *appsv1beta1.StatefulSet, claimIds []int) error {
	idSet := map[int]struct{}{}
	for _, id := range claimIds {
		idSet[id] = struct{}{}
	}
	return wait.PollImmediate(framework.StatefulSetPoll, framework.StatefulSetTimeout, func() (bool, error) {
		pvcList, err := c.CoreV1().PersistentVolumeClaims(ss.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: klabels.Everything().String()})
		if err != nil {
			framework.Logf("WARNING: Failed to list pvcs for verification, retrying: %v", err)
			return false, nil
		}
		for _, claim := range ss.Spec.VolumeClaimTemplates {
			pvcNameRE := regexp.MustCompile(fmt.Sprintf("^%s-%s-([0-9]+)$", claim.Name, ss.Name))
			seenPVCs := map[int]struct{}{}
			for _, pvc := range pvcList.Items {
				matches := pvcNameRE.FindStringSubmatch(pvc.Name)
				if len(matches) != 2 {
					continue
				}
				ordinal, err := strconv.ParseInt(matches[1], 10, 32)
				if err != nil {
					framework.Logf("ERROR: bad pvc name %s (%v)", pvc.Name, err)
					return false, err
				}
				if _, found := idSet[int(ordinal)]; !found {
					return false, nil // Retry until the PVCs are consistent.
				} else {
					seenPVCs[int(ordinal)] = struct{}{}
				}
			}
			if len(seenPVCs) != len(idSet) {
				framework.Logf("Found %d of %d PVCs", len(seenPVCs), len(idSet))
				return false, nil // Retry until the PVCs are consistent.
			}
		}
		return true, nil
	})
}

// verifyStatefulSetPVCsExistWithOwnerRefs works as verifyStatefulSetPVCsExist, but also waits for the ownerRefs to match.
func verifyStatefulSetPVCsExistWithOwnerRefs(c clientset.Interface, kc kruiseclientset.Interface, ss *appsv1beta1.StatefulSet, claimIndices []int, wantSetRef, wantPodRef bool) error {
	indexSet := map[int]struct{}{}
	for _, id := range claimIndices {
		indexSet[id] = struct{}{}
	}
	set, _ := kc.AppsV1beta1().StatefulSets(ss.Namespace).Get(context.TODO(), ss.Name, metav1.GetOptions{})
	setUID := set.GetUID()
	if setUID == "" {
		framework.Failf("Statefulset %s missing UID", ss.Name)
	}
	return wait.PollImmediate(framework.StatefulSetPoll, framework.StatefulSetTimeout, func() (bool, error) {
		pvcList, err := c.CoreV1().PersistentVolumeClaims(ss.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: klabels.Everything().String()})
		if err != nil {
			framework.Logf("WARNING: Failed to list pvcs for verification, retrying: %v", err)
			return false, nil
		}
		for _, claim := range ss.Spec.VolumeClaimTemplates {
			pvcNameRE := regexp.MustCompile(fmt.Sprintf("^%s-%s-([0-9]+)$", claim.Name, ss.Name))
			seenPVCs := map[int]struct{}{}
			for _, pvc := range pvcList.Items {
				matches := pvcNameRE.FindStringSubmatch(pvc.Name)
				if len(matches) != 2 {
					continue
				}
				ordinal, err := strconv.ParseInt(matches[1], 10, 32)
				if err != nil {
					framework.Logf("ERROR: bad pvc name %s (%v)", pvc.Name, err)
					return false, err
				}
				if _, found := indexSet[int(ordinal)]; !found {
					framework.Logf("Unexpected, retrying")
					return false, nil // Retry until the PVCs are consistent.
				}
				var foundSetRef, foundPodRef bool
				for _, ref := range pvc.GetOwnerReferences() {
					if ref.Kind == "StatefulSet" && ref.UID == setUID {
						foundSetRef = true
					}
					if ref.Kind == "Pod" {
						podName := fmt.Sprintf("%s-%d", ss.Name, ordinal)
						pod, err := c.CoreV1().Pods(ss.Namespace).Get(context.TODO(), podName, metav1.GetOptions{})
						if err != nil {
							framework.Logf("Pod %s not found, retrying (%v)", podName, err)
							return false, nil
						}
						podUID := pod.GetUID()
						if podUID == "" {
							framework.Failf("Pod %s is missing UID", pod.Name)
						}
						if ref.UID == podUID {
							foundPodRef = true
						}
					}
				}
				if foundSetRef == wantSetRef && foundPodRef == wantPodRef {
					seenPVCs[int(ordinal)] = struct{}{}
				}
			}
			if len(seenPVCs) != len(indexSet) {
				framework.Logf("Only %d PVCs, retrying", len(seenPVCs))
				return false, nil // Retry until the PVCs are consistent.
			}
		}
		return true, nil
	})
}
