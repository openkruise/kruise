package v1alpha1

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/configmapset"
	kruiseframework "github.com/openkruise/kruise/test/e2e/framework/v1alpha1"
)

var _ = ginkgo.Describe("ConfigMapSet", func() {
	f := kruiseframework.NewDefaultFramework("configmapset")
	var tester *kruiseframework.ConfigMapSetTester
	var namespace string

	ginkgo.BeforeEach(func() {
		tester = kruiseframework.NewConfigMapSetTester(f.KruiseClientSet, f.ClientSet)
		namespace = f.Namespace.Name
	})

	ginkgo.AfterEach(func() {
		// Clean up is handled by framework
	})

	ginkgo.It("should be able to create, update and delete a ConfigMapSet with simple pod updates", func() {
		cmsName := "test-cms-basic"
		podName := "test-pod-basic"

		ginkgo.By(fmt.Sprintf("1. Create ConfigMapSet %s", cmsName))
		cms := &appsv1alpha1.ConfigMapSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmsName,
				Namespace: namespace,
			},
			Spec: appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "cms-test"},
				},
				Data: map[string]string{
					"config.yaml": "key: v1",
				},
				CustomVersion: "v1",
				Containers: []appsv1alpha1.ConfigMapSetContainer{
					{
						Name:      "main",
						MountPath: "/etc/config",
					},
				},
				EffectPolicy: &appsv1alpha1.EffectPolicy{
					Type: appsv1alpha1.EffectPolicyTypeHotUpdate,
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			},
		}

		tester.CreateConfigMapSet(cms)

		ginkgo.By(fmt.Sprintf("2. Create a Pod %s matching the ConfigMapSet", podName))
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: namespace,
				Labels: map[string]string{
					"app": "cms-test",
				},
				Annotations: map[string]string{
					configmapset.GetConfigMapSetEnabledKey(): "true",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "main",
						Image:   "nginx:alpine",
						Command: []string{"sleep", "3600"},
					},
				},
			},
		}
		tester.CreatePod(pod)

		// Wait for pod to be running and injected
		tester.WaitForPodAnnotation(namespace, podName, configmapset.GetConfigMapSetCurrentCustomVersionKey(cmsName), "v1")

		ginkgo.By(fmt.Sprintf("3. Update ConfigMapSet %s data", cmsName))
		cms = tester.GetConfigMapSet(namespace, cmsName)
		cms.Spec.Data["config.yaml"] = "key: v2"
		cms.Spec.CustomVersion = "v2"
		tester.UpdateConfigMapSet(cms)

		// Wait for pod annotation to be updated
		tester.WaitForPodAnnotation(namespace, podName, configmapset.GetConfigMapSetCurrentCustomVersionKey(cmsName), "v2")

		ginkgo.By(fmt.Sprintf("4. Check ConfigMapSet %s status", cmsName))
		tester.WaitForConfigMapSetStatus(namespace, cmsName, func(c *appsv1alpha1.ConfigMapSet) bool {
			return c.Status.CurrentCustomVersion == "v2" && c.Status.UpdatedReplicas == 1
		})

		ginkgo.By(fmt.Sprintf("5. Delete Pod %s", podName))
		tester.DeletePod(namespace, podName)
	})

	ginkgo.It("should respect partition during updates", func() {
		cmsName := "test-cms-partition"

		ginkgo.By(fmt.Sprintf("1. Create ConfigMapSet %s with 50%% partition", cmsName))
		cms := &appsv1alpha1.ConfigMapSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmsName,
				Namespace: namespace,
			},
			Spec: appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "cms-partition-test"},
				},
				Data: map[string]string{
					"config.yaml": "key: v1",
				},
				CustomVersion: "v1",
				EffectPolicy: &appsv1alpha1.EffectPolicy{
					Type: appsv1alpha1.EffectPolicyTypeHotUpdate,
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &intstr.IntOrString{Type: intstr.Int, IntVal: 1}, // Leave 1 old pod
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				},
			},
		}

		tester.CreateConfigMapSet(cms)

		ginkgo.By("2. Create 2 Pods matching the ConfigMapSet")
		for i := 0; i < 2; i++ {
			podName := fmt.Sprintf("test-pod-partition-%d", i)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespace,
					Labels: map[string]string{
						"app": "cms-partition-test",
					},
					Annotations: map[string]string{
						configmapset.GetConfigMapSetEnabledKey(): "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "main",
							Image:   "nginx:alpine",
							Command: []string{"sleep", "3600"},
						},
					},
				},
			}
			tester.CreatePod(pod)
		}

		// Wait for both pods to be v1
		for i := 0; i < 2; i++ {
			podName := fmt.Sprintf("test-pod-partition-%d", i)
			tester.WaitForPodAnnotation(namespace, podName, configmapset.GetConfigMapSetCurrentCustomVersionKey(cmsName), "v1")
		}

		ginkgo.By(fmt.Sprintf("3. Update ConfigMapSet %s data", cmsName))
		cms = tester.GetConfigMapSet(namespace, cmsName)
		cms.Spec.Data["config.yaml"] = "key: v2"
		cms.Spec.CustomVersion = "v2"
		tester.UpdateConfigMapSet(cms)

		// With Partition=1 and Replicas=2, exactly 1 pod should be updated
		ginkgo.By("4. Check that only 1 Pod is updated")
		tester.WaitForConfigMapSetStatus(namespace, cmsName, func(c *appsv1alpha1.ConfigMapSet) bool {
			return c.Status.UpdatedReplicas == 1 && c.Status.Replicas == 2
		})

		// Wait a bit to ensure the other one is not updated
		time.Sleep(3 * time.Second)

		updatedCount := 0
		for i := 0; i < 2; i++ {
			podName := fmt.Sprintf("test-pod-partition-%d", i)
			pod := tester.GetPod(namespace, podName)
			if pod.Annotations[configmapset.GetConfigMapSetCurrentCustomVersionKey(cmsName)] == "v2" {
				updatedCount++
			}
		}

		gomega.Expect(updatedCount).To(gomega.Equal(1))
	})

	ginkgo.It("should always inject current revision for newly created pods during rolling update", func() {
		cmsName := "test-cms-fallback"

		ginkgo.By(fmt.Sprintf("1. Create ConfigMapSet %s with 50%% partition", cmsName))
		cms := &appsv1alpha1.ConfigMapSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmsName,
				Namespace: namespace,
			},
			Spec: appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "cms-fallback-test"},
				},
				Data: map[string]string{
					"config.yaml": "key: v1",
				},
				CustomVersion: "v1",
				EffectPolicy: &appsv1alpha1.EffectPolicy{
					Type: appsv1alpha1.EffectPolicyTypeHotUpdate,
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			},
		}

		tester.CreateConfigMapSet(cms)

		ginkgo.By("2. Create 1 Pod matching the ConfigMapSet")
		podName1 := "test-pod-fallback-1"
		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName1,
				Namespace: namespace,
				Labels: map[string]string{
					"app": "cms-fallback-test",
				},
				Annotations: map[string]string{
					configmapset.GetConfigMapSetEnabledKey(): "true",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "main",
						Image:   "nginx:alpine",
						Command: []string{"sleep", "3600"},
					},
				},
			},
		}
		tester.CreatePod(pod1)
		tester.WaitForPodAnnotation(namespace, podName1, configmapset.GetConfigMapSetCurrentCustomVersionKey(cmsName), "v1")

		ginkgo.By(fmt.Sprintf("3. Update ConfigMapSet %s to v2", cmsName))
		cms = tester.GetConfigMapSet(namespace, cmsName)
		cms.Spec.Data["config.yaml"] = "key: v2"
		cms.Spec.CustomVersion = "v2"
		tester.UpdateConfigMapSet(cms)

		// Wait for the first pod to be updated to v2 (since partition is 1, but we only have 1 pod right now, it might update or wait depending on calculation, actually expectedUpdate = 1 - 1 = 0, so it will NOT update yet!)
		// Wait, partition=1, replicas=1. expectedUpdate = 1 - 1 = 0.
		// Let's create the second pod.

		ginkgo.By("4. Create 2nd Pod concurrently")
		podName2 := "test-pod-fallback-2"
		pod2 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName2,
				Namespace: namespace,
				Labels: map[string]string{
					"app": "cms-fallback-test",
				},
				Annotations: map[string]string{
					configmapset.GetConfigMapSetEnabledKey(): "true",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "main",
						Image:   "nginx:alpine",
						Command: []string{"sleep", "3600"},
					},
				},
			},
		}
		tester.CreatePod(pod2)

		// The webhook should inject "v1" (CurrentRevision) into the new pod, NOT v2.
		// Even though it's during a rollout.
		ginkgo.By("5. Check that new pod is initially injected with v1 by webhook")
		pod2 = tester.GetPod(namespace, podName2)
		gomega.Expect(pod2.Annotations[configmapset.GetConfigMapSetCurrentCustomVersionKey(cmsName)]).To(gomega.Equal("v1"))

		// Now we have 2 pods. Partition is 1. Expected update is 2 - 1 = 1.
		// The controller will eventually pick one of them to update to v2.
		ginkgo.By("6. Wait for controller to update exactly 1 pod to v2")
		tester.WaitForConfigMapSetStatus(namespace, cmsName, func(c *appsv1alpha1.ConfigMapSet) bool {
			return c.Status.UpdatedReplicas == 1 && c.Status.Replicas == 2
		})
	})

	ginkgo.It("should reject deletion if pods are still using it via webhook", func() {
		cmsName := "test-cms-delete"
		podName := "test-pod-delete"

		ginkgo.By(fmt.Sprintf("1. Create ConfigMapSet %s", cmsName))
		cms := &appsv1alpha1.ConfigMapSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmsName,
				Namespace: namespace,
			},
			Spec: appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "cms-delete-test"},
				},
				Data: map[string]string{
					"config.yaml": "key: v1",
				},
			},
		}

		tester.CreateConfigMapSet(cms)

		ginkgo.By(fmt.Sprintf("2. Create a Pod %s matching the ConfigMapSet", podName))
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: namespace,
				Labels: map[string]string{
					"app": "cms-delete-test",
				},
				Annotations: map[string]string{
					configmapset.GetConfigMapSetEnabledKey(): "true",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "main",
						Image:   "nginx:alpine",
						Command: []string{"sleep", "3600"},
					},
				},
			},
		}
		tester.CreatePod(pod)

		// Wait for pod to be running
		tester.WaitForPodAnnotation(namespace, podName, configmapset.GetConfigMapSetCurrentRevisionKey(cmsName), "") // just wait for it to be processed

		ginkgo.By(fmt.Sprintf("3. Try to delete ConfigMapSet %s, should fail", cmsName))
		cmsToDelete := tester.GetConfigMapSet(namespace, cmsName)
		err := tester.Kc.AppsV1alpha1().ConfigMapSets(cmsToDelete.Namespace).Delete(context.TODO(), cmsToDelete.Name, metav1.DeleteOptions{})
		gomega.Expect(err).To(gomega.HaveOccurred(), "Expected deletion to fail due to webhook validation")
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("is still used configmapSet"))

		ginkgo.By(fmt.Sprintf("4. Delete Pod %s", podName))
		tester.DeletePod(namespace, podName)

		// Wait for pod to disappear
		err = wait.PollImmediate(time.Second, time.Minute, func() (bool, error) {
			_, err := tester.K8sClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
			if err != nil {
				return true, nil // Gone
			}
			return false, nil
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("5. Try to delete ConfigMapSet %s again, should succeed", cmsName))
		tester.DeleteConfigMapSet(namespace, cmsName)
	})
})
