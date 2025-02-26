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
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/configuration"
	"github.com/openkruise/kruise/test/e2e/framework"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller/history"
	utilpointer "k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
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

	framework.KruiseDescribe("SidecarSet Injecting functionality [SidecarSetInject]", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all SidecarSet in cluster")
			tester.DeleteSidecarSets(ns)
			tester.DeleteDeployments(ns)
		})
		framework.ConformanceIt("pods don't have matched sidecarSet", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			// sidecarSet no matched pods
			sidecarSet.Spec.Selector.MatchLabels["app"] = "nomatched"
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			_, _ = tester.CreateSidecarSet(sidecarSet)
			time.Sleep(time.Second)

			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// get pods
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deployment.Spec.Replicas)))
			pod := pods[0]
			gomega.Expect(pod.Spec.Containers).To(gomega.HaveLen(len(deployment.Spec.Template.Spec.Containers)))
			ginkgo.By("test no matched sidecarSet done")
		})

		framework.ConformanceIt("sidecarset with volumes.downwardAPI", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			sidecarSet.Spec.Volumes = []corev1.Volume{
				{
					Name: "podinfo",
					VolumeSource: corev1.VolumeSource{
						DownwardAPI: &corev1.DownwardAPIVolumeSource{
							Items: []corev1.DownwardAPIVolumeFile{
								{
									Path: "labels",
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.labels",
									},
								},
								{
									Path: "annotations",
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.annotations",
									},
								},
							},
						},
					},
				},
			}
			sidecarSet.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
				{
					Name:      "podinfo",
					MountPath: "/etc/podinfo",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s with volumes.downwardAPI", sidecarSet.Name))
			_, _ = tester.CreateSidecarSet(sidecarSet)
		})

		framework.ConformanceIt("sidecarSet inject pod sidecar container", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			_, _ = tester.CreateSidecarSet(sidecarSet)
			time.Sleep(time.Second)

			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// get pods
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			pod := pods[0]
			gomega.Expect(pod.Spec.Containers).To(gomega.HaveLen(len(deployment.Spec.Template.Spec.Containers) + len(sidecarSet.Spec.Containers)))
			gomega.Expect(pod.Spec.InitContainers).To(gomega.HaveLen(len(deployment.Spec.Template.Spec.InitContainers) + len(sidecarSet.Spec.InitContainers)))
			exceptContainers := []string{"nginx-sidecar", "main", "busybox-sidecar"}
			for i, except := range exceptContainers {
				gomega.Expect(except).To(gomega.Equal(pod.Spec.Containers[i].Name))
			}
			ginkgo.By("sidecarSet inject pod sidecar container done")
		})

		framework.ConformanceIt("sidecarSet inject pod sidecar container volumeMounts", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			// create deployment
			deployment := tester.NewBaseDeployment(ns)

			cases := []struct {
				name               string
				getDeployment      func() *apps.Deployment
				getSidecarSets     func() *appsv1alpha1.SidecarSet
				exceptVolumeMounts []string
				exceptEnvs         []string
				exceptVolumes      []string
			}{
				{
					name: "append normal volumeMounts",
					getDeployment: func() *apps.Deployment {
						deployIn := deployment.DeepCopy()
						deployIn.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
							{
								Name:      "main-volume",
								MountPath: "/main-volume",
							},
						}
						deployIn.Spec.Template.Spec.Volumes = []corev1.Volume{
							{
								Name: "main-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						}
						return deployIn
					},
					getSidecarSets: func() *appsv1alpha1.SidecarSet {
						sidecarSetIn := sidecarSet.DeepCopy()
						sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
						sidecarSetIn.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
							{
								Name:      "nginx-volume",
								MountPath: "/nginx-volume",
							},
						}
						sidecarSetIn.Spec.Volumes = []corev1.Volume{
							{
								Name: "nginx-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						}
						return sidecarSetIn
					},
					exceptVolumeMounts: []string{"/main-volume", "/nginx-volume"},
					exceptVolumes:      []string{"main-volume", "nginx-volume"},
				},
			}

			for _, cs := range cases {
				ginkgo.By(cs.name)
				sidecarSetIn := cs.getSidecarSets()
				ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
				_, _ = tester.CreateSidecarSet(sidecarSetIn)
				time.Sleep(time.Second)

				deploymentIn := cs.getDeployment()
				ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
				tester.CreateDeployment(deploymentIn)
				// get pods
				pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				// volume
				for _, volume := range cs.exceptVolumes {
					object := util.GetPodVolume(pods[0], volume)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
				// volumeMounts
				sidecarContainer := &pods[0].Spec.Containers[0]
				for _, volumeMount := range cs.exceptVolumeMounts {
					object := util.GetContainerVolumeMount(sidecarContainer, volumeMount)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
				// envs
				for _, env := range cs.exceptEnvs {
					object := util.GetContainerEnvVar(sidecarContainer, env)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
			}
			ginkgo.By("sidecarSet inject pod sidecar container volumeMounts done")
		})

		framework.ConformanceIt("sidecarSet inject pod sidecar container volumeMounts, SubPathExpr with expanded subpath", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			// create deployment
			deployment := tester.NewBaseDeployment(ns)

			cases := []struct {
				name               string
				getDeployment      func() *apps.Deployment
				getSidecarSets     func() *appsv1alpha1.SidecarSet
				exceptVolumeMounts []string
				exceptEnvs         []string
				exceptVolumes      []string
			}{
				{
					name: "append volumeMounts SubPathExpr, volumes with expanded subpath",
					getDeployment: func() *apps.Deployment {
						deployIn := deployment.DeepCopy()
						deployIn.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
							{
								Name:        "main-volume",
								MountPath:   "/main-volume",
								SubPathExpr: "foo/$(POD_NAME)/$(OD_NAME)/conf",
							},
						}
						deployIn.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
							{
								Name:  "POD_NAME",
								Value: "bar",
							},
							{
								Name:  "OD_NAME",
								Value: "od_name",
							},
						}
						deployIn.Spec.Template.Spec.Volumes = []corev1.Volume{
							{
								Name: "main-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						}
						return deployIn
					},
					getSidecarSets: func() *appsv1alpha1.SidecarSet {
						sidecarSetIn := sidecarSet.DeepCopy()
						sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
						sidecarSetIn.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
							{
								Name:      "nginx-volume",
								MountPath: "/nginx-volume",
							},
						}
						sidecarSetIn.Spec.Volumes = []corev1.Volume{
							{
								Name: "nginx-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						}
						return sidecarSetIn
					},
					exceptVolumeMounts: []string{"/main-volume", "/nginx-volume"},
					exceptVolumes:      []string{"main-volume", "nginx-volume"},
					exceptEnvs:         []string{"POD_NAME", "OD_NAME"},
				},
			}

			for _, cs := range cases {
				ginkgo.By(cs.name)
				sidecarSetIn := cs.getSidecarSets()
				ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
				_, _ = tester.CreateSidecarSet(sidecarSetIn)
				time.Sleep(time.Second)

				deploymentIn := cs.getDeployment()
				ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
				tester.CreateDeployment(deploymentIn)
				// get pods
				pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				// volume
				for _, volume := range cs.exceptVolumes {
					object := util.GetPodVolume(pods[0], volume)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
				// volumeMounts
				sidecarContainer := &pods[0].Spec.Containers[0]
				for _, volumeMount := range cs.exceptVolumeMounts {
					object := util.GetContainerVolumeMount(sidecarContainer, volumeMount)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
				// envs
				for _, env := range cs.exceptEnvs {
					object := util.GetContainerEnvVar(sidecarContainer, env)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
			}
			ginkgo.By("sidecarSet inject pod sidecar container volumeMounts, SubPathExpr with expanded subpath done")
		})

		framework.ConformanceIt("sidecarSet inject pod sidecar container transfer Envs", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			sidecarSetIn.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "OD_NAME",
					Value: "sidecar_name",
				},
				{
					Name:  "SidecarName",
					Value: "nginx-sidecar",
				},
			}
			sidecarSetIn.Spec.Containers[0].TransferEnv = []appsv1alpha1.TransferEnvVar{
				{
					SourceContainerName: "main",
					EnvName:             "POD_NAME",
				},
				{
					SourceContainerName: "main",
					EnvName:             "OD_NAME",
				},
				{
					SourceContainerName: "main",
					EnvName:             "PROXY_IP",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			_, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "POD_NAME",
					Value: "bar",
				},
				{
					Name:  "OD_NAME",
					Value: "od_name",
				},
				{
					Name:  "PROXY_IP",
					Value: "127.0.0.1",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// get pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			podIn := pods[0]
			gomega.Expect(podIn.Spec.Containers).To(gomega.HaveLen(2))
			// except envs
			exceptEnvs := map[string]string{
				"POD_NAME":    "bar",
				"OD_NAME":     "sidecar_name",
				"PROXY_IP":    "127.0.0.1",
				"SidecarName": "nginx-sidecar",
			}
			sidecarContainer := &podIn.Spec.Containers[0]
			// envs
			for key, value := range exceptEnvs {
				object := util.GetContainerEnvValue(sidecarContainer, key)
				gomega.Expect(object).To(gomega.Equal(value))
			}
			ginkgo.By("sidecarSet inject pod sidecar container transfer Envs done")
		})

		framework.ConformanceIt("sidecarSet inject pod sidecar container transfer Envs with downward API by metadata.labels", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			sidecarSetIn.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "OD_NAME",
					Value: "sidecar_name",
				},
				{
					Name:  "SidecarName",
					Value: "nginx-sidecar",
				},
			}
			sidecarSetIn.Spec.Containers[0].TransferEnv = []appsv1alpha1.TransferEnvVar{
				{
					SourceContainerNameFrom: &appsv1alpha1.SourceContainerNameSource{
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.labels['biz']",
						},
					},
					EnvNames: []string{
						"POD_NAME",
						"OD_NAME",
						"PROXY_IP",
					},
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			_, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			podLabels := deploymentIn.Spec.Template.ObjectMeta.Labels
			podLabels["biz"] = "main"
			deploymentIn.Spec.Template.ObjectMeta.Labels = podLabels
			deploymentIn.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "POD_NAME",
					Value: "bar",
				},
				{
					Name:  "OD_NAME",
					Value: "od_name",
				},
				{
					Name:  "PROXY_IP",
					Value: "127.0.0.1",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// get pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			podIn := pods[0]
			gomega.Expect(podIn.Spec.Containers).To(gomega.HaveLen(2))
			// except envs
			exceptEnvs := map[string]string{
				"POD_NAME":    "bar",
				"OD_NAME":     "sidecar_name",
				"PROXY_IP":    "127.0.0.1",
				"SidecarName": "nginx-sidecar",
			}
			sidecarContainer := &podIn.Spec.Containers[0]
			// envs
			for key, value := range exceptEnvs {
				object := util.GetContainerEnvValue(sidecarContainer, key)
				gomega.Expect(object).To(gomega.Equal(value))
			}
			ginkgo.By("sidecarSet inject pod sidecar container transfer Envs with downward API by metadata.labels done")
		})

		framework.ConformanceIt("sidecarSet inject pod sidecar container transfer Envs with downward API by metadata.annotations", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			sidecarSetIn.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "OD_NAME",
					Value: "sidecar_name",
				},
				{
					Name:  "SidecarName",
					Value: "nginx-sidecar",
				},
			}
			sidecarSetIn.Spec.Containers[0].TransferEnv = []appsv1alpha1.TransferEnvVar{
				{
					SourceContainerNameFrom: &appsv1alpha1.SourceContainerNameSource{
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.annotations['biz']",
						},
					},
					EnvNames: []string{
						"POD_NAME",
						"OD_NAME",
						"PROXY_IP",
					},
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			_, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Template.ObjectMeta.Annotations = map[string]string{
				"biz": "main",
			}
			deploymentIn.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "POD_NAME",
					Value: "bar",
				},
				{
					Name:  "OD_NAME",
					Value: "od_name",
				},
				{
					Name:  "PROXY_IP",
					Value: "127.0.0.1",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// get pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			podIn := pods[0]
			gomega.Expect(podIn.Spec.Containers).To(gomega.HaveLen(2))
			// except envs
			exceptEnvs := map[string]string{
				"POD_NAME":    "bar",
				"OD_NAME":     "sidecar_name",
				"PROXY_IP":    "127.0.0.1",
				"SidecarName": "nginx-sidecar",
			}
			sidecarContainer := &podIn.Spec.Containers[0]
			// envs
			for key, value := range exceptEnvs {
				object := util.GetContainerEnvValue(sidecarContainer, key)
				gomega.Expect(object).To(gomega.Equal(value))
			}
			ginkgo.By("sidecarSet inject pod sidecar container transfer Envs with downward API by metadata.annotations done")
		})

		// currently skip
		// todo
		/*framework.ConformanceIt("sidecarSet inject initContainer with restartPolicy=Always", func() {
			always := corev1.ContainerRestartPolicyAlways
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			sidecarSet.Spec.Containers = nil
			sidecarSet.Spec.InitContainers = nil
			obj1 := sidecarSet.DeepCopy()
			obj1.Name = "sidecarset-1"
			obj1.Spec.InitContainers = []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:          "init-1",
						Command:       []string{"/bin/sh", "-c", "sleep 1000000"},
						Image:         "busybox:latest",
						RestartPolicy: &always,
					},
				},
			}
			ginkgo.By("Creating SidecarSet failed")
			_, err := kc.AppsV1alpha1().SidecarSets().Create(context.TODO(), obj1, metav1.CreateOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())
			obj1.Spec.UpdateStrategy.Type = appsv1alpha1.NotUpdateSidecarSetStrategyType
			obj2 := sidecarSet.DeepCopy()
			obj2.Spec.UpdateStrategy.Type = appsv1alpha1.NotUpdateSidecarSetStrategyType
			obj2.Name = "sidecarset-2"
			obj2.Spec.InitContainers = []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:          "hot-init",
						Image:         "openkruise/hotupgrade-sample:sidecarv1",
						RestartPolicy: &always,
						Lifecycle: &corev1.Lifecycle{
							PostStart: &corev1.LifecycleHandler{
								Exec: &corev1.ExecAction{
									Command: []string{"/bin/sh", "/migrate.sh"},
								},
							},
						},
					},
					UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
						UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
						HotUpgradeEmptyImage: "openkruise/hotupgrade-sample:empty",
					},
				},
			}
			ginkgo.By("Creating SidecarSet success")
			_, err = tester.CreateSidecarSet(obj1)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			_, err = tester.CreateSidecarSet(obj2)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Template.ObjectMeta.Annotations = map[string]string{
				"biz": "main",
			}
			deploymentIn.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "POD_NAME",
					Value: "bar",
				},
				{
					Name:  "OD_NAME",
					Value: "od_name",
				},
				{
					Name:  "PROXY_IP",
					Value: "127.0.0.1",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// get pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			podIn := pods[0]
			gomega.Expect(podIn.Spec.InitContainers).To(gomega.HaveLen(3))
			ginkgo.By("sidecarSet inject pod sidecar container transfer Envs with downward API by metadata.annotations done")
		})*/
	})

	framework.KruiseDescribe("SidecarSet Upgrade functionality [SidecarSetUpgrade]", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all SidecarSet in cluster")
			tester.DeleteSidecarSets(ns)
			tester.DeleteDeployments(ns)
		})

		framework.ConformanceIt("sidecarSet patch pod metadata", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			sidecarSetIn.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
				{
					PatchPolicy: appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy,
					Annotations: map[string]string{
						"key": `{"nginx-sidecar":1}`,
					},
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			// create cm whitelist
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: util.GetKruiseNamespace(),
					Name:      configuration.KruiseConfigurationName,
				},
				Data: map[string]string{
					configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"annotationKeyExprs":["key"],"selector":{"matchLabels":{"app":"sidecar"}}}]}`,
				},
			}
			_, err := c.CoreV1().ConfigMaps(cm.Namespace).Create(context.TODO(), cm, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second)
			defer func(maps v1core.ConfigMapInterface, ctx context.Context, name string, opts metav1.DeleteOptions) {
				_ = maps.Delete(ctx, name, opts)
			}(c.CoreV1().ConfigMaps(cm.Namespace), context.TODO(), cm.Name, metav1.DeleteOptions{})

			// create sidecarSet again
			sidecarSetIn, err = tester.CreateSidecarSet(sidecarSetIn)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			time.Sleep(time.Second * 3)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(2))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// check pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Spec.Containers[0].Image).Should(gomega.Equal(sidecarSetIn.Spec.Containers[0].Image))
				gomega.Expect(pod.Annotations["key"]).Should(gomega.Equal(`{"nginx-sidecar":1}`))
			}

			// only modify annotations, and do not trigger in-place update
			sidecarSetIn.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
				{
					PatchPolicy: appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy,
					Annotations: map[string]string{
						"key": `{"nginx-sidecar":2}`,
					},
				},
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 3)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// get pods
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Spec.Containers[0].Image).Should(gomega.Equal(sidecarSetIn.Spec.Containers[0].Image))
				gomega.Expect(pod.Annotations["key"]).Should(gomega.Equal(`{"nginx-sidecar":1}`))
			}

			// upgrade sidecar container image version
			sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 3)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// get pods
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Spec.Containers[0].Image).Should(gomega.Equal(BusyboxImage))
				gomega.Expect(pod.Annotations["key"]).Should(gomega.Equal(`{"nginx-sidecar":2}`))
			}
			ginkgo.By("sidecarSet update pod annotations done")
		})

		framework.ConformanceIt("sidecarSet upgrade cold sidecar container image only", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
				MaxUnavailable: &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 2,
				},
			}
			sidecarSetIn.Spec.NamespaceSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{"inject": "sidecar"},
			}
			sidecarSetIn.Spec.Namespace = ""
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s %s", sidecarSetIn.Name, util.DumpJSON(sidecarSetIn)))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(2))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			// check pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Spec.Containers).Should(gomega.HaveLen(1))
			}

			// update ns
			nsObj, err := c.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			nsObj.Labels["inject"] = "sidecar"
			_, err = c.CoreV1().Namespaces().Update(context.TODO(), nsObj, metav1.UpdateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// delete pods
			for _, pod := range pods {
				err = c.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
			time.Sleep(time.Second * 5)

			// check pods
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Spec.Containers).Should(gomega.HaveLen(2))
				gomega.Expect(pod.Spec.Containers[0].Image).Should(gomega.Equal(sidecarSetIn.Spec.Containers[0].Image))
			}

			// update sidecarSet sidecar container
			sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// get pods
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Spec.Containers[0].Image).Should(gomega.Equal(BusyboxImage))
				_, sidecarSetUpgradable := podutil.GetPodCondition(&pod.Status, sidecarcontrol.SidecarSetUpgradable)
				gomega.Expect(sidecarSetUpgradable.Status).Should(gomega.Equal(corev1.ConditionTrue))
			}
			ginkgo.By("sidecarSet upgrade cold sidecar container image done")
		})

		framework.ConformanceIt("sidecarSet upgrade cold sidecar container failed image, and only update one pod", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(2))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			sidecarSetIn, err := kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			//check pod sidecar upgrade spec annotations
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				origin := sets.String{}
				for _, sidecar := range sidecarSetIn.Spec.InitContainers {
					if sidecarcontrol.IsSidecarContainer(sidecar.Container) {
						origin.Insert(sidecar.Name)
					}
				}
				for _, sidecar := range sidecarSetIn.Spec.Containers {
					origin.Insert(sidecar.Name)
				}
				// SidecarSetHashAnnotation = "kruise.io/sidecarset-hash"
				upgradeSpec1 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashAnnotation, pod)
				gomega.Expect(upgradeSpec1.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec1.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetRevision(sidecarSetIn)))
				target1 := sets.NewString(upgradeSpec1.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target1.List())).To(gomega.Equal(true))
				// SidecarSetHashWithoutImageAnnotation = "kruise.io/sidecarset-hash-without-image"
				upgradeSpec2 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashWithoutImageAnnotation, pod)
				gomega.Expect(upgradeSpec2.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec2.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetWithoutImageRevision(sidecarSetIn)))
				target2 := sets.NewString(upgradeSpec2.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target2.List())).To(gomega.Equal(true))
			}

			// update sidecarSet sidecar container failed image
			sidecarSetIn.Spec.Containers[0].Image = InvalidImage
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 0,
				ReadyPods:        1,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// update sidecarSet sidecar container success image
			sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			sidecarSetIn, err = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			//check pod sidecar upgrade spec annotations
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				origin := sets.String{}
				for _, sidecar := range sidecarSetIn.Spec.InitContainers {
					if sidecarcontrol.IsSidecarContainer(sidecar.Container) {
						origin.Insert(sidecar.Name)
					}
				}
				for _, sidecar := range sidecarSetIn.Spec.Containers {
					origin.Insert(sidecar.Name)
				}
				// SidecarSetHashAnnotation = "kruise.io/sidecarset-hash"
				upgradeSpec1 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashAnnotation, pod)
				gomega.Expect(upgradeSpec1.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec1.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetRevision(sidecarSetIn)))
				target1 := sets.NewString(upgradeSpec1.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target1.List())).To(gomega.Equal(true))
				// SidecarSetHashWithoutImageAnnotation = "kruise.io/sidecarset-hash-without-image"
				upgradeSpec2 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashWithoutImageAnnotation, pod)
				gomega.Expect(upgradeSpec2.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec2.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetWithoutImageRevision(sidecarSetIn)))
				target2 := sets.NewString(upgradeSpec2.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target2.List())).To(gomega.Equal(true))
			}

			ginkgo.By("sidecarSet upgrade cold sidecar container failed image, and only update one pod done")
		})

		framework.ConformanceIt("sidecarSet upgrade sidecar container (more than image field), no pod should be updated", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.InitContainers = nil
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", util.DumpJSON(sidecarSetIn)))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(1))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			sidecarSetIn, err := kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			//check pod sidecar upgrade spec annotations
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				origin := sets.String{}
				for _, sidecar := range sidecarSetIn.Spec.InitContainers {
					if sidecarcontrol.IsSidecarContainer(sidecar.Container) {
						origin.Insert(sidecar.Name)
					}
				}
				for _, sidecar := range sidecarSetIn.Spec.Containers {
					origin.Insert(sidecar.Name)
				}
				// SidecarSetHashAnnotation = "kruise.io/sidecarset-hash"
				upgradeSpec1 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashAnnotation, pod)
				gomega.Expect(upgradeSpec1.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec1.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetRevision(sidecarSetIn)))
				target1 := sets.NewString(upgradeSpec1.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target1.List())).To(gomega.Equal(true))
				// SidecarSetHashWithoutImageAnnotation = "kruise.io/sidecarset-hash-without-image"
				upgradeSpec2 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashWithoutImageAnnotation, pod)
				gomega.Expect(upgradeSpec2.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec2.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetWithoutImageRevision(sidecarSetIn)))
				target2 := sets.NewString(upgradeSpec2.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target2.List())).To(gomega.Equal(true))
			}

			// modify sidecarSet sidecar field out of image
			sidecarSetIn.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "version",
					Value: "v2",
				},
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      1,
				UpdatedPods:      0,
				UpdatedReadyPods: 0,
				ReadyPods:        1,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// check all the pods' condition
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				_, condition := podutil.GetPodCondition(&pod.Status, sidecarcontrol.SidecarSetUpgradable)
				gomega.Expect(condition.Status).Should(gomega.Equal(corev1.ConditionFalse))
			}

			// scale deployment replicas=2
			deploymentIn.Spec.Replicas = ptr.To(int32(2))
			tester.UpdateDeployment(deploymentIn)
			time.Sleep(time.Second)

			// update sidecarSet image
			sidecarSetIn.Spec.Containers[0].Image = NewNginxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 3)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			ginkgo.By("sidecarSet upgrade sidecar container (more than image field), no pod should be updated done")
		})

		framework.ConformanceIt("multi sidecarSet upgrade sidecar container, check pod condition", func() {
			// create sidecarSet 1
			sidecarSetIn1 := tester.NewBaseSidecarSet(ns)
			sidecarSetIn1.Name = "test-sidecarset-1"
			sidecarSetIn1.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn1.Spec.Containers = sidecarSetIn1.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn1.Name))
			sidecarSetIn1, _ = tester.CreateSidecarSet(sidecarSetIn1)
			time.Sleep(time.Second)

			// create sidecarSet 2
			sidecarSetIn2 := tester.NewBaseSidecarSet(ns)
			sidecarSetIn2.Name = "test-sidecarset-2"
			sidecarSetIn2.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn2.Spec.InitContainers = []appsv1alpha1.SidecarContainer{}
			sidecarSetIn2.Spec.Containers = sidecarSetIn2.Spec.Containers[1:2]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn2.Name))
			sidecarSetIn2, _ = tester.CreateSidecarSet(sidecarSetIn2)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(2))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			sidecarSetIn1, err := kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn1.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			sidecarSetIn2, err = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn2.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// modify sidecarSet1 field out of image, should not update pod
			sidecarSetIn1.Spec.Containers[0].Command = []string{"sleep", "1000"}
			tester.UpdateSidecarSet(sidecarSetIn1)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      0,
				UpdatedReadyPods: 0,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn1, except)

			// modify sidecarSet2, only change image
			sidecarSetIn2.Spec.Containers[0].Image = NginxImage
			tester.UpdateSidecarSet(sidecarSetIn2)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn2, except)

			// check all the pods' condition, due to sidecarSet1 is not updated, so the condition should be false
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				_, condition := podutil.GetPodCondition(&pod.Status, sidecarcontrol.SidecarSetUpgradable)
				gomega.Expect(condition.Status).Should(gomega.Equal(corev1.ConditionFalse))
			}

			// then update sidecarSet1, all the pods should be updated
			sidecarSetIn1.Spec.Containers[0].Image = NewNginxImage
			sidecarSetIn1.Spec.Containers[0].Command = []string{"tail", "-f", "/dev/null"}
			tester.UpdateSidecarSet(sidecarSetIn1)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn1, except)

			// all the sidecarset is updated, so the condition should be true
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				_, condition := podutil.GetPodCondition(&pod.Status, sidecarcontrol.SidecarSetUpgradable)
				gomega.Expect(condition.Status).Should(gomega.Equal(corev1.ConditionTrue))
			}

			ginkgo.By("multi sidecarSet upgrade sidecar container, check pod condition done")
		})

		framework.ConformanceIt("sidecarSet upgrade cold sidecar container image, and paused", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(2))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// update sidecarSet sidecar container
			sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 5)
			// paused
			sidecarSetIn.Spec.UpdateStrategy.Paused = true
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// paused = false, continue update pods
			sidecarSetIn.Spec.UpdateStrategy.Paused = false
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			ginkgo.By("sidecarSet upgrade cold sidecar container image, and paused done")
		})

		framework.ConformanceIt("sidecarSet upgrade cold sidecar container image, and selector", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(2))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// update pod[0] labels[canary.release] = true
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			canaryPod := pods[0]
			canaryPod.Labels["canary.release"] = "true"
			tester.UpdatePod(canaryPod)
			time.Sleep(time.Second)
			// update sidecarSet sidecar container
			sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
			// update sidecarSet selector
			sidecarSetIn.Spec.UpdateStrategy.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"canary.release": "true",
				},
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 5)
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        2,
			}
			time.Sleep(time.Minute)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// check pod image
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			for _, pod := range pods {
				if _, ok := pod.Labels["canary.release"]; ok {
					sidecarContainer := pod.Spec.Containers[0]
					gomega.Expect(sidecarContainer.Image).To(gomega.Equal(BusyboxImage))
				} else {
					sidecarContainer := pod.Spec.Containers[0]
					gomega.Expect(sidecarContainer.Image).To(gomega.Equal(NginxImage))
				}
			}

			// update sidecarSet selector == nil, and update all pods
			sidecarSetIn.Spec.UpdateStrategy.Selector = nil
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 5)
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			time.Sleep(time.Minute)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			ginkgo.By("sidecarSet upgrade cold sidecar container image, and selector done")
		})

		framework.ConformanceIt("sidecarSet upgrade cold sidecar container image, and partition", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(2))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// update sidecarSet sidecar container
			sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
			// update sidecarSet selector
			sidecarSetIn.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "50%",
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// update sidecarSet partition, update all pods
			sidecarSetIn.Spec.UpdateStrategy.Partition = nil
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			ginkgo.By("sidecarSet upgrade cold sidecar container image, and partition done")
		})

		framework.ConformanceIt("sidecarSet upgrade cold sidecar container image, and maxUnavailable", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(4))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			// update sidecarSet sidecar container
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) failed image", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Image = InvalidImage
			// update sidecarSet selector
			sidecarSetIn.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "50%",
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      4,
				UpdatedPods:      2,
				UpdatedReadyPods: 0,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// update sidecarSet sidecar container
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) success image", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Image = BusyboxImage
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      4,
				UpdatedPods:      4,
				UpdatedReadyPods: 4,
				ReadyPods:        4,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			ginkgo.By("sidecarSet upgrade cold sidecar container image, and maxUnavailable done")
		})

		framework.ConformanceIt("sidecarSet update init sidecar container, and don't upgrade", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			gomega.Expect(sidecarSetIn.Spec.InitContainers[0].PodInjectPolicy).To(gomega.Equal(appsv1alpha1.AfterAppContainerType))
			gomega.Expect(sidecarSetIn.Spec.Containers[0].PodInjectPolicy).To(gomega.Equal(appsv1alpha1.BeforeAppContainerType))
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(1))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s/%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			// check sidecarSet
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      1,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        1,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			sidecarSetIn, _ = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			hash1 := sidecarSetIn.Annotations[sidecarcontrol.SidecarSetHashAnnotation]

			// update sidecarSet sidecar container
			sidecarSetIn.Spec.InitContainers[0].Image = InvalidImage
			tester.UpdateSidecarSet(sidecarSetIn)
			ginkgo.By("update sidecarset init container image, and sidecarSet hash not changed")
			time.Sleep(time.Second * 5)
			sidecarSetIn, _ = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			hash2 := sidecarSetIn.Annotations[sidecarcontrol.SidecarSetHashAnnotation]
			// hash not changed
			gomega.Expect(hash1).To(gomega.Equal(hash2))
			ginkgo.By("sidecarSet upgrade init sidecar container, and don't upgrade done")
		})

		framework.ConformanceIt("sidecarSet history revision checker", func() {
			// check function
			revisionChecker := func(s *appsv1alpha1.SidecarSet, expectedCount int, expectedOrder []int64) {
				list := tester.ListControllerRevisions(s)
				// check the number of revisions
				gomega.Expect(list).To(gomega.HaveLen(expectedCount))
				for _, revision := range list {
					// check fields of revision
					mice := make(map[string]interface{})
					err := json.Unmarshal(revision.Data.Raw, &mice)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					spec := mice["spec"].(map[string]interface{})
					_, ok1 := spec["volumes"]
					_, ok2 := spec["containers"]
					_, ok3 := spec["initContainers"]
					_, ok4 := spec["imagePullSecrets"]
					gomega.Expect(ok1 && ok2 && ok3 && ok4).To(gomega.BeTrue())
				}
				if expectedOrder == nil {
					return
				}
				gomega.Expect(list).To(gomega.HaveLen(len(expectedOrder)))
				history.SortControllerRevisions(list)
				for i := range list {
					gomega.Expect(list[i].Revision).To(gomega.Equal(expectedOrder[i]))
				}
			}

			waitingForSidecarSetReconcile := func(name string) {
				gomega.Eventually(func() bool {
					sidecarSet, err := kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), name, metav1.GetOptions{})
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return sidecarSet.Status.ObservedGeneration == sidecarSet.Generation
				}, 10*time.Second, time.Second).Should(gomega.BeTrue())
			}

			ginkgo.By("check after sidecarset creating...")
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.SetName("e2e-test-for-history-revisions")
			sidecarSetIn.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "secret-1"}}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			waitingForSidecarSetReconcile(sidecarSetIn.Name)
			revisionChecker(sidecarSetIn, 1, nil)

			// update sidecarSet and stored revisions
			ginkgo.By("check after sidecarset updating 15 times...")
			for i := 2; i <= 15; i++ {
				sidecarSetIn.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: fmt.Sprintf("secret-%d", i)}}
				tester.UpdateSidecarSet(sidecarSetIn)
				waitingForSidecarSetReconcile(sidecarSetIn.Name)
			}
			// expected order after update
			expectedOrder := []int64{6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
			revisionChecker(sidecarSetIn, 10, expectedOrder)

			ginkgo.By("check after sidecarset updating by using old revision...")
			sidecarSetIn.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: fmt.Sprintf("secret-%d", 12)}}
			tester.UpdateSidecarSet(sidecarSetIn)
			waitingForSidecarSetReconcile(sidecarSetIn.Name)
			expectedOrder = []int64{6, 7, 8, 9, 10, 11, 13, 14, 15, 16}
			revisionChecker(sidecarSetIn, 10, expectedOrder)
			ginkgo.By("sidecarSet history revision check done")
		})

		framework.ConformanceIt("sidecarSet history revision data checker", func() {
			// check function
			revisionChecker := func(list []*apps.ControllerRevision) {
				gomega.Expect(list).To(gomega.HaveLen(2))
				history.SortControllerRevisions(list)
				patchPodMetadata := map[string][]appsv1alpha1.SidecarSetPatchPodMetadata{}
				for _, revision := range list {
					mice := make(map[string]interface{})
					err := json.Unmarshal(revision.Data.Raw, &mice)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					spec := mice["spec"].(map[string]interface{})
					_, ok1 := spec["volumes"]
					_, ok2 := spec["containers"]
					_, ok3 := spec["initContainers"]
					_, ok4 := spec["imagePullSecrets"]
					_, ok5 := spec["patchPodMetadata"]
					gomega.Expect(ok1 && ok2 && ok3 && ok4 && ok5).To(gomega.BeTrue())
					b, _ := json.Marshal(spec["patchPodMetadata"])
					var patch []appsv1alpha1.SidecarSetPatchPodMetadata
					gomega.Expect(json.Unmarshal(b, &patch)).NotTo(gomega.HaveOccurred())
					patchPodMetadata[revision.Name] = patch
				}
				gomega.Expect(patchPodMetadata[list[0].Name][0].Annotations["sidecarset.kruise.io/test"] == "version-1").Should(gomega.BeTrue())
				gomega.Expect(patchPodMetadata[list[1].Name][0].Annotations["sidecarset.kruise.io/test"] == "version-2").Should(gomega.BeTrue())
			}

			waitingForSidecarSetReconcile := func(name string) {
				gomega.Eventually(func() bool {
					sidecarSet, err := kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), name, metav1.GetOptions{})
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					return sidecarSet.Status.ObservedGeneration == sidecarSet.Generation
				}, 10*time.Second, time.Second).Should(gomega.BeTrue())
			}

			ginkgo.By("check after sidecarset creating...")
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.SetName("e2e-test-for-history-revision-data")
			sidecarSetIn.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "secret-1"}}
			sidecarSetIn.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
				{
					PatchPolicy: appsv1alpha1.SidecarSetRetainPatchPolicy,
					Annotations: map[string]string{
						"sidecarset.kruise.io/test": "version-1",
					},
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			waitingForSidecarSetReconcile(sidecarSetIn.Name)

			ginkgo.By(fmt.Sprintf("Updating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
				{
					PatchPolicy: appsv1alpha1.SidecarSetRetainPatchPolicy,
					Annotations: map[string]string{
						"sidecarset.kruise.io/test": "version-2",
					},
				},
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			waitingForSidecarSetReconcile(sidecarSetIn.Name)
			list := tester.ListControllerRevisions(sidecarSetIn)
			revisionChecker(list)
			ginkgo.By("sidecarSet history revision data check done")
		})

		framework.ConformanceIt("sidecarSet InjectionStrategy.Revision checker", func() {
			// create sidecarSet
			nginxName := func(tag string) string {
				return fmt.Sprintf("nginx:%s", tag)
			}
			tags := []string{
				"latest", "1.21.1", "1.21", "1.20.1", "1.20", "1.19.10",
			}
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			if sidecarSetIn.Labels == nil {
				sidecarSetIn.Labels = map[string]string{
					appsv1alpha1.SidecarSetCustomVersionLabel: "0",
				}
			}
			sidecarSetIn.SetName("e2e-test-for-injection-strategy-revision")
			sidecarSetIn.Spec.UpdateStrategy.Paused = true
			sidecarSetIn.Spec.Containers[0].Image = nginxName(tags[0])
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)
			for i := 1; i < 6; i++ {
				// update sidecarSet and stored revisions
				sidecarSetIn.Spec.Containers[0].Image = nginxName(tags[i])
				sidecarSetIn.Labels[appsv1alpha1.SidecarSetCustomVersionLabel] = strconv.Itoa(i)
				tester.UpdateSidecarSet(sidecarSetIn)
				gomega.Eventually(func() int {
					rv := tester.ListControllerRevisions(sidecarSetIn)
					return len(rv)
				}, 5*time.Second, time.Second).Should(gomega.Equal(i + 1))
			}

			// pick a history revision to inject
			pick := 3
			list := tester.ListControllerRevisions(sidecarSetIn)
			gomega.Expect(list).To(gomega.HaveLen(6))
			history.SortControllerRevisions(list)
			sidecarSetIn.Spec.InjectionStrategy.Revision = &appsv1alpha1.SidecarSetInjectRevision{
				CustomVersion: utilpointer.String(strconv.Itoa(pick)),
				Policy:        appsv1alpha1.AlwaysSidecarSetInjectRevisionPolicy,
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = ptr.To(int32(1))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			// check sidecarSet revision
			pods, err := tester.GetSelectorPods(ns, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			gomega.Expect(pods[0].Spec.Containers[0].Image).To(gomega.Equal(nginxName(tags[pick])))

			// check pod sidecarSetHash
			gomega.Expect(len(pods[0].Annotations[sidecarcontrol.SidecarSetHashAnnotation]) > 0).To(gomega.BeTrue())
			hash := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
			err = json.Unmarshal([]byte(pods[0].Annotations[sidecarcontrol.SidecarSetHashAnnotation]), &hash)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(hash[sidecarSetIn.Name].SidecarSetControllerRevision).To(gomega.Equal(list[pick].Name))

			// check again after sidecarSet upgrade
			sidecarSetIn.Spec.UpdateStrategy.Paused = false
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      1,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        1,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			pods, err = tester.GetSelectorPods(ns, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			gomega.Expect(pods[0].Spec.Containers[0].Image).To(gomega.Equal(nginxName(tags[5])))
			gomega.Expect(len(pods[0].Annotations[sidecarcontrol.SidecarSetHashAnnotation]) > 0).To(gomega.BeTrue())
			hash = make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
			err = json.Unmarshal([]byte(pods[0].Annotations[sidecarcontrol.SidecarSetHashAnnotation]), &hash)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(hash[sidecarSetIn.Name].SidecarSetControllerRevision).To(gomega.Equal(list[5].Name))
			ginkgo.By("sidecarSet InjectionStrategy.Revision check done")
		})

		framework.ConformanceIt("sidecarSet inject pod sidecar during canary upgrades", func() {
			// create sidecarSet
			nginxName := func(tag string) string {
				return fmt.Sprintf("nginx:%s", tag)
			}
			stableTag, canaryTag := "1.20.1", "1.21.1"
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Labels = map[string]string{
				appsv1alpha1.SidecarSetCustomVersionLabel: "0",
			}
			sidecarSetIn.SetName("e2e-test-for-canary-upgrade")
			sidecarSetIn.Spec.Containers[0].Image = nginxName(stableTag)
			sidecarSetIn.Spec.UpdateStrategy.Type = appsv1alpha1.RollingUpdateSidecarSetStrategyType
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn, _ = tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentStable, deploymentCanary := tester.NewBaseDeployment(ns), tester.NewBaseDeployment(ns)
			deploymentStable.Name += "-stable"
			deploymentStable.Spec.Replicas = ptr.To(int32(1))
			deploymentStable.Spec.Template.Labels["version"] = "stable"
			deploymentStable.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent
			deploymentStable.Spec.Template.Spec.TerminationGracePeriodSeconds = ptr.To(int64(0))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentStable.Namespace, deploymentStable.Name))
			deploymentCanary.Name += "-canary"
			deploymentCanary.Spec.Replicas = ptr.To(int32(1))
			deploymentCanary.Spec.Template.Labels["version"] = "canary"
			deploymentCanary.Spec.Template.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent
			deploymentCanary.Spec.Template.Spec.TerminationGracePeriodSeconds = ptr.To(int64(0))
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentCanary.Namespace, deploymentCanary.Name))
			tester.CreateDeployment(deploymentStable)
			tester.CreateDeployment(deploymentCanary)
			tester.WaitForDeploymentRunning(deploymentStable)
			tester.WaitForDeploymentRunning(deploymentCanary)

			calculateSidecarImages := func(pods []*corev1.Pod) (stableNum, canaryNum int) {
				for _, pod := range pods {
					for _, container := range pod.Spec.Containers {
						if container.Image == nginxName(stableTag) {
							stableNum++
						}
						if container.Image == nginxName(canaryTag) {
							canaryNum++
						}
					}
				}
				return
			}
			var stableNum, canaryNum int

			// check sidecar original revisions
			podStable, err := tester.GetSelectorPods(ns, &metav1.LabelSelector{MatchLabels: deploymentStable.Spec.Template.Labels})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(podStable).To(gomega.HaveLen(1))
			stableNum, canaryNum = calculateSidecarImages(podStable)
			gomega.Expect(stableNum).To(gomega.Equal(1))
			gomega.Expect(canaryNum).To(gomega.Equal(0))
			podCanary, err := tester.GetSelectorPods(ns, &metav1.LabelSelector{MatchLabels: deploymentCanary.Spec.Template.Labels})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(podCanary).To(gomega.HaveLen(1))
			stableNum, canaryNum = calculateSidecarImages(podCanary)
			gomega.Expect(stableNum).To(gomega.Equal(1))
			gomega.Expect(canaryNum).To(gomega.Equal(0))
			ginkgo.By(fmt.Sprintf("All pods are injected with a stable sidecar"))

			// canary update sidecarSet
			sidecarSetIn.Spec.Containers[0].Image = nginxName(canaryTag)
			sidecarSetIn.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "SOME_ENV", Value: "SOME_VAL"}} // make in-place upgrade impossible
			sidecarSetIn.Spec.UpdateStrategy.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{"version": "canary"},
			}
			sidecarSetIn.Spec.InjectionStrategy.Revision = &appsv1alpha1.SidecarSetInjectRevision{
				CustomVersion: ptr.To("0"),
				Policy:        appsv1alpha1.PartialSidecarSetInjectRevisionPolicy,
			}
			sidecarSetIn.Labels = map[string]string{
				appsv1alpha1.SidecarSetCustomVersionLabel: "1",
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			expect := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      0,
				UpdatedReadyPods: 0,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, expect)
			ginkgo.By(fmt.Sprintf("Sidecarset Updated"))

			// scale up deployments
			deploymentStable.Spec.Replicas = ptr.To(int32(2))
			deploymentCanary.Spec.Replicas = ptr.To(int32(2))
			tester.UpdateDeployment(deploymentStable)
			tester.UpdateDeployment(deploymentCanary)
			tester.WaitForDeploymentRunning(deploymentStable)
			tester.WaitForDeploymentRunning(deploymentCanary)

			// check sidecar versions
			podStable, err = tester.GetSelectorPods(ns, &metav1.LabelSelector{MatchLabels: deploymentStable.Spec.Template.Labels})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(podStable).To(gomega.HaveLen(2))
			stableNum, canaryNum = calculateSidecarImages(podStable)
			gomega.Expect(stableNum).To(gomega.Equal(2))
			gomega.Expect(canaryNum).To(gomega.Equal(0))
			podCanary, err = tester.GetSelectorPods(ns, &metav1.LabelSelector{MatchLabels: deploymentCanary.Spec.Template.Labels})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(podCanary).To(gomega.HaveLen(2))
			stableNum, canaryNum = calculateSidecarImages(podCanary)
			gomega.Expect(stableNum).To(gomega.Equal(1))
			gomega.Expect(canaryNum).To(gomega.Equal(1))
			ginkgo.By(fmt.Sprintf("All pods are injected with a suitable sidecar"))
		})
	})
})
