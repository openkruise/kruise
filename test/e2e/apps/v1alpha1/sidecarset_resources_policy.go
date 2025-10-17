/*
Copyright 2025 The Kruise Authors.

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
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	apps2 "github.com/openkruise/kruise/test/e2e/framework/common"
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

	ginkgo.Context("SidecarSet ResourcesPolicy functionality [SidecarSetResourcesPolicy]", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentSpecReport().Failed() {
				apps2.DumpDebugInfo(c, ns)
			}
			apps2.Logf("Deleting all SidecarSet and Deployments in namespace %s", ns)
			tester.DeleteSidecarSets(ns)
			tester.DeleteDeployments(ns)
		})

		// User Story 1: Specific container name matching
		ginkgo.It("Story 1 - ResourcesPolicy with specific container name regex", func() {
			apps2.Logf("Testing User Story 1: Specific container name matching")

			// Create SidecarSet with ResourcesPolicy targeting specific container
			sidecarSet := &appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset-story1",
				},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story1"},
					},
					Namespace: ns,
					Containers: []appsv1alpha1.SidecarContainer{
						{
							Container: corev1.Container{
								Name:    "sidecar1",
								Image:   "busybox:latest",
								Command: []string{"/bin/sh", "-c", "sleep 10000000"},
							},
							ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
								TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
								TargetContainersNameRegex: "^large-engine-v4$", // Only match large-engine-v4
								ResourceExpr: appsv1alpha1.ResourceExpr{
									Limits: &appsv1alpha1.ResourceExprLimits{
										CPU:    "max(cpu*50%, 50m)",
										Memory: "200Mi",
									},
									Requests: &appsv1alpha1.ResourceExprRequests{
										CPU:    "max(cpu*50%, 50m)",
										Memory: "100Mi",
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			_, err := tester.CreateSidecarSet(sidecarSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Create Deployment with two containers: large.engine.v4 and large.engine.v8
			deployment := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment-story1",
					Namespace: ns,
				},
				Spec: apps.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story1"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "story1"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "large-engine-v4",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("50m"),
											corev1.ResourceMemory: resource.MustParse("100Mi"),
										},
									},
								},
								{
									Name:  "large-engine-v8",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("50m"),
											corev1.ResourceMemory: resource.MustParse("100Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating Deployment %s/%s", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// Wait for pods to be ready
			ginkgo.By("Waiting for pods to be ready")
			gomega.Eventually(func() bool {
				pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
				if err != nil || len(pods) != 1 {
					return false
				}
				for _, pod := range pods {
					if pod.Status.Phase != corev1.PodRunning {
						return false
					}
				}
				return true
			}, 60*time.Second, 3*time.Second).Should(gomega.BeTrue())

			// Get pods and verify sidecar resources
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			pod := pods[0]

			ginkgo.By("Verifying sidecar container resources")
			// Find sidecar1 container
			var sidecarContainer *corev1.Container
			for i := range pod.Spec.Containers {
				if pod.Spec.Containers[i].Name == "sidecar1" {
					sidecarContainer = &pod.Spec.Containers[i]
					break
				}
			}
			gomega.Expect(sidecarContainer).NotTo(gomega.BeNil(), "sidecar1 container not found")

			// Expected: max(sum(200m) * 50%, 50m) = 100m for CPU limit
			expectedCPULimit := resource.MustParse("100m")
			expectedMemoryLimit := resource.MustParse("200Mi")
			expectedCPURequest := resource.MustParse("50m") // max(sum(50m) * 50%, 50m) = 50m
			expectedMemoryRequest := resource.MustParse("100Mi")

			gomega.Expect(sidecarContainer.Resources.Limits).NotTo(gomega.BeNil())
			gomega.Expect(sidecarContainer.Resources.Limits.Cpu().Cmp(expectedCPULimit)).To(gomega.Equal(0),
				fmt.Sprintf("Expected CPU limit %s, got %s", expectedCPULimit.String(), sidecarContainer.Resources.Limits.Cpu().String()))
			gomega.Expect(sidecarContainer.Resources.Limits.Memory().Cmp(expectedMemoryLimit)).To(gomega.Equal(0),
				fmt.Sprintf("Expected Memory limit %s, got %s", expectedMemoryLimit.String(), sidecarContainer.Resources.Limits.Memory().String()))

			gomega.Expect(sidecarContainer.Resources.Requests).NotTo(gomega.BeNil())
			gomega.Expect(sidecarContainer.Resources.Requests.Cpu().Cmp(expectedCPURequest)).To(gomega.Equal(0),
				fmt.Sprintf("Expected CPU request %s, got %s", expectedCPURequest.String(), sidecarContainer.Resources.Requests.Cpu().String()))
			gomega.Expect(sidecarContainer.Resources.Requests.Memory().Cmp(expectedMemoryRequest)).To(gomega.Equal(0),
				fmt.Sprintf("Expected Memory request %s, got %s", expectedMemoryRequest.String(), sidecarContainer.Resources.Requests.Memory().String()))

			apps2.Logf("User Story 1 test passed: sidecar resources correctly calculated based on large-engine-v4 only")
		})

		// User Story 2: Sum mode aggregation
		ginkgo.It("Story 2 - ResourcesPolicy with sum mode", func() {
			apps2.Logf("Testing User Story 2: Sum mode aggregation")

			// Create SidecarSet with sum mode
			sidecarSet := &appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset-story2",
				},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story2"},
					},
					Namespace: ns,
					Containers: []appsv1alpha1.SidecarContainer{
						{
							Container: corev1.Container{
								Name:    "sidecar1",
								Image:   "busybox:latest",
								Command: []string{"/bin/sh", "-c", "sleep 10000000"},
							},
							ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
								TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
								TargetContainersNameRegex: "^large-engine-v.*$",
								ResourceExpr: appsv1alpha1.ResourceExpr{
									Limits: &appsv1alpha1.ResourceExprLimits{
										CPU:    "max(cpu*50%, 50m)",
										Memory: "200Mi",
									},
									Requests: &appsv1alpha1.ResourceExprRequests{
										CPU:    "max(cpu*50%, 50m)",
										Memory: "100Mi",
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			_, err := tester.CreateSidecarSet(sidecarSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Create Deployment with two large.engine containers
			deployment := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment-story2",
					Namespace: ns,
				},
				Spec: apps.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story2"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "story2"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "large-engine-v4",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("50m"),
											corev1.ResourceMemory: resource.MustParse("100Mi"),
										},
									},
								},
								{
									Name:  "large-engine-v8",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("400m"),
											corev1.ResourceMemory: resource.MustParse("400Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating Deployment %s/%s", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// Wait for pods to be ready
			ginkgo.By("Waiting for pods to be ready")
			gomega.Eventually(func() bool {
				pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
				if err != nil || len(pods) != 1 {
					return false
				}
				for _, pod := range pods {
					if pod.Status.Phase != corev1.PodRunning {
						return false
					}
				}
				return true
			}, 60*time.Second, 3*time.Second).Should(gomega.BeTrue())

			// Get pods and verify sidecar resources
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			pod := pods[0]

			ginkgo.By("Verifying sidecar container resources with sum mode")
			var sidecarContainer *corev1.Container
			for i := range pod.Spec.Containers {
				if pod.Spec.Containers[i].Name == "sidecar1" {
					sidecarContainer = &pod.Spec.Containers[i]
					break
				}
			}
			gomega.Expect(sidecarContainer).NotTo(gomega.BeNil())

			// Expected: max((200m + 400m) * 50%, 50m) = 300m for CPU limit
			expectedCPULimit := resource.MustParse("300m")
			expectedMemoryLimit := resource.MustParse("200Mi")
			expectedCPURequest := resource.MustParse("75m") // max((50m + 100m) * 50%, 50m) = 75m
			expectedMemoryRequest := resource.MustParse("100Mi")

			gomega.Expect(sidecarContainer.Resources.Limits.Cpu().Cmp(expectedCPULimit)).To(gomega.Equal(0),
				fmt.Sprintf("Expected CPU limit %s, got %s", expectedCPULimit.String(), sidecarContainer.Resources.Limits.Cpu().String()))
			gomega.Expect(sidecarContainer.Resources.Limits.Memory().Cmp(expectedMemoryLimit)).To(gomega.Equal(0))

			gomega.Expect(sidecarContainer.Resources.Requests.Cpu().Cmp(expectedCPURequest)).To(gomega.Equal(0),
				fmt.Sprintf("Expected CPU request %s, got %s", expectedCPURequest.String(), sidecarContainer.Resources.Requests.Cpu().String()))
			gomega.Expect(sidecarContainer.Resources.Requests.Memory().Cmp(expectedMemoryRequest)).To(gomega.Equal(0))

			apps2.Logf("User Story 2 test passed: sum mode correctly aggregates resources")
		})

		// User Story 3: Max mode aggregation
		ginkgo.It("Story 3 - ResourcesPolicy with max mode", func() {
			apps2.Logf("Testing User Story 3: Max mode aggregation")

			// Create SidecarSet with max mode
			sidecarSet := &appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset-story3",
				},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story3"},
					},
					Namespace: ns,
					Containers: []appsv1alpha1.SidecarContainer{
						{
							Container: corev1.Container{
								Name:    "sidecar1",
								Image:   "busybox:latest",
								Command: []string{"/bin/sh", "-c", "sleep 10000000"},
							},
							ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
								TargetContainerMode:       appsv1alpha1.TargetContainerModeMax,
								TargetContainersNameRegex: "^large-engine-v.*$",
								ResourceExpr: appsv1alpha1.ResourceExpr{
									Limits: &appsv1alpha1.ResourceExprLimits{
										CPU:    "max(cpu*50%, 50m)",
										Memory: "200Mi",
									},
									Requests: &appsv1alpha1.ResourceExprRequests{
										CPU:    "max(cpu*50%, 50m)",
										Memory: "100Mi",
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			_, err := tester.CreateSidecarSet(sidecarSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Create Deployment
			deployment := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment-story3",
					Namespace: ns,
				},
				Spec: apps.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story3"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "story3"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "large-engine-v4",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("50m"),
											corev1.ResourceMemory: resource.MustParse("100Mi"),
										},
									},
								},
								{
									Name:  "large-engine-v8",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("400m"),
											corev1.ResourceMemory: resource.MustParse("400Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating Deployment %s/%s", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// Wait for pods to be ready
			ginkgo.By("Waiting for pods to be ready")
			gomega.Eventually(func() bool {
				pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
				if err != nil || len(pods) != 1 {
					return false
				}
				for _, pod := range pods {
					if pod.Status.Phase != corev1.PodRunning {
						return false
					}
				}
				return true
			}, 60*time.Second, 3*time.Second).Should(gomega.BeTrue())

			// Verify sidecar resources
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			pod := pods[0]

			ginkgo.By("Verifying sidecar container resources with max mode")
			var sidecarContainer *corev1.Container
			for i := range pod.Spec.Containers {
				if pod.Spec.Containers[i].Name == "sidecar1" {
					sidecarContainer = &pod.Spec.Containers[i]
					break
				}
			}
			gomega.Expect(sidecarContainer).NotTo(gomega.BeNil())

			// Expected: max(max(200m, 400m) * 50%, 50m) = 200m for CPU limit
			expectedCPULimit := resource.MustParse("200m")
			expectedMemoryLimit := resource.MustParse("200Mi")
			expectedCPURequest := resource.MustParse("50m") // max(max(50m, 100m) * 50%, 50m) = 50m
			expectedMemoryRequest := resource.MustParse("100Mi")

			gomega.Expect(sidecarContainer.Resources.Limits.Cpu().Cmp(expectedCPULimit)).To(gomega.Equal(0),
				fmt.Sprintf("Expected CPU limit %s, got %s", expectedCPULimit.String(), sidecarContainer.Resources.Limits.Cpu().String()))
			gomega.Expect(sidecarContainer.Resources.Limits.Memory().Cmp(expectedMemoryLimit)).To(gomega.Equal(0))

			gomega.Expect(sidecarContainer.Resources.Requests.Cpu().Cmp(expectedCPURequest)).To(gomega.Equal(0),
				fmt.Sprintf("Expected CPU request %s, got %s", expectedCPURequest.String(), sidecarContainer.Resources.Requests.Cpu().String()))
			gomega.Expect(sidecarContainer.Resources.Requests.Memory().Cmp(expectedMemoryRequest)).To(gomega.Equal(0))

			apps2.Logf("User Story 3 test passed: max mode correctly aggregates resources")
		})

		// User Story 4: Unlimited resources handling
		ginkgo.It("Story 4 - ResourcesPolicy with unlimited resources", func() {
			apps2.Logf("Testing User Story 4: Unlimited resources handling")

			// Create SidecarSet
			sidecarSet := &appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset-story4",
				},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story4"},
					},
					Namespace: ns,
					Containers: []appsv1alpha1.SidecarContainer{
						{
							Container: corev1.Container{
								Name:    "sidecar1",
								Image:   "busybox:latest",
								Command: []string{"/bin/sh", "-c", "sleep 10000000"},
							},
							ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
								TargetContainerMode:       appsv1alpha1.TargetContainerModeMax,
								TargetContainersNameRegex: "^large-engine-v.*$",
								ResourceExpr: appsv1alpha1.ResourceExpr{
									Limits: &appsv1alpha1.ResourceExprLimits{
										CPU:    "max(cpu*50%, 50m)",
										Memory: "200Mi",
									},
									Requests: &appsv1alpha1.ResourceExprRequests{
										CPU:    "max(cpu*50%, 50m)",
										Memory: "100Mi",
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			_, err := tester.CreateSidecarSet(sidecarSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Create Deployment where large.engine.v8 has no CPU limit (unlimited)
			deployment := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment-story4",
					Namespace: ns,
				},
				Spec: apps.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story4"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "story4"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "large-engine-v4",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("50m"),
											corev1.ResourceMemory: resource.MustParse("100Mi"),
										},
									},
								},
								{
									Name:  "large-engine-v8",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											// No CPU limit - unlimited
											corev1.ResourceMemory: resource.MustParse("400Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating Deployment %s/%s", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// Wait for pods to be ready
			ginkgo.By("Waiting for pods to be ready")
			gomega.Eventually(func() bool {
				pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
				if err != nil || len(pods) != 1 {
					return false
				}
				for _, pod := range pods {
					if pod.Status.Phase != corev1.PodRunning {
						return false
					}
				}
				return true
			}, 60*time.Second, 3*time.Second).Should(gomega.BeTrue())

			// Verify sidecar resources
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			pod := pods[0]

			ginkgo.By("Verifying sidecar container resources with unlimited handling")
			var sidecarContainer *corev1.Container
			for i := range pod.Spec.Containers {
				if pod.Spec.Containers[i].Name == "sidecar1" {
					sidecarContainer = &pod.Spec.Containers[i]
					break
				}
			}
			gomega.Expect(sidecarContainer).NotTo(gomega.BeNil())

			// Expected: CPU limit should NOT be set (unlimited), because v8 has no CPU limit
			// Memory limit should be set: 200Mi
			// CPU request should be set: max(max(50m, 100m) * 50%, 50m) = 50m
			// Memory request should be set: 100Mi
			_, hasCPULimit := sidecarContainer.Resources.Limits[corev1.ResourceCPU]
			gomega.Expect(hasCPULimit).To(gomega.BeFalse(), "CPU limit should NOT be set (unlimited)")

			expectedMemoryLimit := resource.MustParse("200Mi")
			gomega.Expect(sidecarContainer.Resources.Limits.Memory().Cmp(expectedMemoryLimit)).To(gomega.Equal(0),
				fmt.Sprintf("Expected Memory limit %s, got %s", expectedMemoryLimit.String(), sidecarContainer.Resources.Limits.Memory().String()))

			expectedCPURequest := resource.MustParse("50m")
			expectedMemoryRequest := resource.MustParse("100Mi")
			gomega.Expect(sidecarContainer.Resources.Requests.Cpu().Cmp(expectedCPURequest)).To(gomega.Equal(0))
			gomega.Expect(sidecarContainer.Resources.Requests.Memory().Cmp(expectedMemoryRequest)).To(gomega.Equal(0))

			apps2.Logf("User Story 4 test passed: unlimited resources correctly handled")
		})

		// User Story 5: Complex linear expression
		ginkgo.It("Story 5 - ResourcesPolicy with complex linear expression", func() {
			apps2.Logf("Testing User Story 5: Complex linear expression")

			// Create SidecarSet with complex expression
			// Expression: 0.5*cpu - 0.3*max(0, cpu-4) + 0.3*max(0, cpu-8)
			// Simplified for testing: Use cpu*40% for simplicity
			sidecarSet := &appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset-story5",
				},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story5"},
					},
					Namespace: ns,
					Containers: []appsv1alpha1.SidecarContainer{
						{
							Container: corev1.Container{
								Name:    "sidecar1",
								Image:   "busybox:latest",
								Command: []string{"/bin/sh", "-c", "sleep 10000000"},
							},
							ResourcesPolicy: &appsv1alpha1.ResourcesPolicy{
								TargetContainerMode:       appsv1alpha1.TargetContainerModeSum,
								TargetContainersNameRegex: ".*",
								ResourceExpr: appsv1alpha1.ResourceExpr{
									Limits: &appsv1alpha1.ResourceExprLimits{
										// Complex expression with arithmetic operations
										CPU:    "cpu*50% + 100m",
										Memory: "max(memory*20% + 100Mi, 200Mi)",
									},
									Requests: &appsv1alpha1.ResourceExprRequests{
										CPU:    "cpu*30% + 50m",
										Memory: "memory*15% + 50Mi",
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			_, err := tester.CreateSidecarSet(sidecarSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Create Deployment
			deployment := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment-story5",
					Namespace: ns,
				},
				Spec: apps.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "story5"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "story5"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app1",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("400Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("200Mi"),
										},
									},
								},
								{
									Name:  "app2",
									Image: "nginx:1.14.2",
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("400m"),
											corev1.ResourceMemory: resource.MustParse("600Mi"),
										},
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("300Mi"),
										},
									},
								},
							},
						},
					},
				},
			}

			ginkgo.By(fmt.Sprintf("Creating Deployment %s/%s", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// Wait for pods to be ready
			ginkgo.By("Waiting for pods to be ready")
			gomega.Eventually(func() bool {
				pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
				if err != nil || len(pods) != 1 {
					return false
				}
				for _, pod := range pods {
					if pod.Status.Phase != corev1.PodRunning {
						return false
					}
				}
				return true
			}, 60*time.Second, 3*time.Second).Should(gomega.BeTrue())

			// Verify sidecar resources
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			pod := pods[0]

			ginkgo.By("Verifying sidecar container resources with complex expression")
			var sidecarContainer *corev1.Container
			for i := range pod.Spec.Containers {
				if pod.Spec.Containers[i].Name == "sidecar1" {
					sidecarContainer = &pod.Spec.Containers[i]
					break
				}
			}
			gomega.Expect(sidecarContainer).NotTo(gomega.BeNil())

			// Expected calculations:
			// CPU limit: (200m + 400m) * 50% + 100m = 300m + 100m = 400m
			// Memory limit: max((400Mi + 600Mi) * 20% + 100Mi, 200Mi) = max(200Mi + 100Mi, 200Mi) = 300Mi
			// CPU request: (100m + 200m) * 30% + 50m = 90m + 50m = 140m
			// Memory request: (200Mi + 300Mi) * 15% + 50Mi = 75Mi + 50Mi = 125Mi
			expectedCPULimit := resource.MustParse("400m")
			expectedMemoryLimit := resource.MustParse("300Mi")
			expectedCPURequest := resource.MustParse("140m")
			expectedMemoryRequest := resource.MustParse("125Mi")

			gomega.Expect(sidecarContainer.Resources.Limits.Cpu().Cmp(expectedCPULimit)).To(gomega.Equal(0),
				fmt.Sprintf("Expected CPU limit %s, got %s", expectedCPULimit.String(), sidecarContainer.Resources.Limits.Cpu().String()))
			gomega.Expect(sidecarContainer.Resources.Limits.Memory().Cmp(expectedMemoryLimit)).To(gomega.Equal(0),
				fmt.Sprintf("Expected Memory limit %s, got %s", expectedMemoryLimit.String(), sidecarContainer.Resources.Limits.Memory().String()))

			gomega.Expect(sidecarContainer.Resources.Requests.Cpu().Cmp(expectedCPURequest)).To(gomega.Equal(0),
				fmt.Sprintf("Expected CPU request %s, got %s", expectedCPURequest.String(), sidecarContainer.Resources.Requests.Cpu().String()))
			gomega.Expect(sidecarContainer.Resources.Requests.Memory().Cmp(expectedMemoryRequest)).To(gomega.Equal(0),
				fmt.Sprintf("Expected Memory request %s, got %s", expectedMemoryRequest.String(), sidecarContainer.Resources.Requests.Memory().String()))

			apps2.Logf("User Story 5 test passed: complex linear expression correctly evaluated")
		})
	})
})
