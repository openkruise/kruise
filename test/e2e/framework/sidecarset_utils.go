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

package framework

import (
	utilpointer "k8s.io/utils/pointer"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"

	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

type SidecarSetTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
}

func NewSidecarSetTester(c clientset.Interface, kc kruiseclientset.Interface) *SidecarSetTester {
	return &SidecarSetTester{
		c:  c,
		kc: kc,
	}
}

func (s *SidecarSetTester) NewBaseSidecarSet() *appsv1alpha1.SidecarSet {
	return &appsv1alpha1.SidecarSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SidecarSet",
			APIVersion: "apps.kruise.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sidecarset",
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			InitContainers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:    "init-sidecar",
						Command: []string{"/bin/sh", "-c", "sleep 1"},
						Image:   "busybox:latest",
					},
				},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:    "nginx-sidecar",
						Image:   "nginx:latest",
						Command: []string{"tail", "-f", "/dev/null"},
					},
					PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyEnabled,
					},
				},
				{
					Container: corev1.Container{
						Name:    "busybox-sidecar",
						Image:   "busybox:latest",
						Command: []string{"/bin/sh", "-c", "sleep 10000000"},
					},
					PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "sidecarset"},
			},
		},
	}
}

func (s *SidecarSetTester) NewBaseDeployment(namespace string) *apps.Deployment {
	return &apps.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployment-test",
			Namespace: namespace,
		},
		Spec: apps.DeploymentSpec{
			Replicas: utilpointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "sidecarset",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "sidecarset",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "main",
							Image:   "busybox:latest",
							Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
				},
			},
		},
	}
}

func (s *SidecarSetTester) CreateSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) *appsv1alpha1.SidecarSet {
	Logf("create sidecarSet(%s)", sidecarSet.Name)
	_, err := s.kc.AppsV1alpha1().SidecarSets().Create(sidecarSet)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForSidecarSetCreated(sidecarSet)
	sidecarSet, _ = s.kc.AppsV1alpha1().SidecarSets().Get(sidecarSet.Name, metav1.GetOptions{})
	return sidecarSet
}

func (s *SidecarSetTester) UpdateSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) {
	Logf("update sidecarSet(%s)", sidecarSet.Name)
	sidecarSetClone := sidecarSet.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		sidecarSetClone.Spec = sidecarSet.Spec
		_, updateErr := s.kc.AppsV1alpha1().SidecarSets().Update(sidecarSetClone)
		if updateErr == nil {
			return nil
		}
		sidecarSetClone, _ = s.kc.AppsV1alpha1().SidecarSets().Get(sidecarSetClone.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *SidecarSetTester) UpdatePod(pod *corev1.Pod) {
	Logf("update pod(%s.%s)", pod.Namespace, pod.Name)
	podClone := pod.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		podClone.ObjectMeta = pod.ObjectMeta
		podClone.Spec = pod.Spec
		_, updateErr := s.c.CoreV1().Pods(podClone.Namespace).Update(podClone)
		if updateErr == nil {
			return nil
		}
		podClone, _ = s.c.CoreV1().Pods(podClone.Namespace).Update(podClone)
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *SidecarSetTester) WaitForSidecarSetUpgradeComplete(sidecarSet *appsv1alpha1.SidecarSet, exceptStatus *appsv1alpha1.SidecarSetStatus) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := s.kc.AppsV1alpha1().SidecarSets().Get(sidecarSet.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Status.MatchedPods == exceptStatus.MatchedPods &&
				inner.Status.UpdatedPods == exceptStatus.UpdatedPods &&
				inner.Status.UpdatedReadyPods == exceptStatus.UpdatedReadyPods &&
				inner.Status.ReadyPods == exceptStatus.ReadyPods {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for sidecarSet to upgrade complete: %v", pollErr)
	}
}

func (s *SidecarSetTester) CreateDeployment(deployment *apps.Deployment) {
	Logf("create deployment(%s.%s)", deployment.Namespace, deployment.Name)
	_, err := s.c.AppsV1().Deployments(deployment.Namespace).Create(deployment)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForDeploymentRunning(deployment)
}

func (s *SidecarSetTester) DeleteSidecarSets() {
	sidecarSetList, err := s.kc.AppsV1alpha1().SidecarSets().List(metav1.ListOptions{})
	if err != nil {
		Logf("List sidecarSets failed: %s", err.Error())
		return
	}

	for _, sidecarSet := range sidecarSetList.Items {
		s.DeleteSidecarSet(&sidecarSet)
	}
}

func (s *SidecarSetTester) DeleteSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) {
	err := s.kc.AppsV1alpha1().SidecarSets().Delete(sidecarSet.Name, &metav1.DeleteOptions{})
	if err != nil {
		Logf("delete sidecarSet(%s) failed: %s", sidecarSet.Name, err.Error())
	}
	s.WaitForSidecarSetDeleted(sidecarSet)
}

func (s *SidecarSetTester) DeleteDeployments(namespace string) {
	deploymentList, err := s.c.AppsV1().Deployments(namespace).List(metav1.ListOptions{})
	if err != nil {
		Logf("List Deployments failed: %s", err.Error())
		return
	}

	for _, deployment := range deploymentList.Items {
		s.DeleteDeployment(&deployment)
	}
}

func (s *SidecarSetTester) DeleteDeployment(deployment *apps.Deployment) {
	err := s.c.AppsV1().Deployments(deployment.Namespace).Delete(deployment.Name, &metav1.DeleteOptions{})
	if err != nil {
		Logf("delete deployment(%s.%s) failed: %s", deployment.Namespace, deployment.Name, err.Error())
		return
	}
	s.WaitForDeploymentDeleted(deployment)
}

func (s *SidecarSetTester) WaitForSidecarSetCreated(sidecarSet *appsv1alpha1.SidecarSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := s.kc.AppsV1alpha1().SidecarSets().Get(sidecarSet.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return true, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for sidecarSet to enter running: %v", pollErr)
	}
}

func (s *SidecarSetTester) WaitForDeploymentRunning(deployment *apps.Deployment) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := s.c.AppsV1().Deployments(deployment.Namespace).Get(deployment.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			if *inner.Spec.Replicas == inner.Status.ReadyReplicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for deployment to enter running: %v", pollErr)
	}
}

func (s *SidecarSetTester) WaitForDeploymentDeleted(deployment *apps.Deployment) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := s.c.AppsV1().Deployments(deployment.Namespace).Get(deployment.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for deployment to enter Deleted: %v", pollErr)
	}
}

func (s *SidecarSetTester) WaitForSidecarSetDeleted(sidecarSet *appsv1alpha1.SidecarSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := s.kc.AppsV1alpha1().SidecarSets().Get(sidecarSet.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for SidecarSet to enter Deleted: %v", pollErr)
	}
}

func (s *SidecarSetTester) GetSelectorPods(namespace string, selector *metav1.LabelSelector) ([]corev1.Pod, error) {
	faster, err := util.GetFastLabelSelector(selector)
	if err != nil {
		return nil, err
	}
	podList, err := s.c.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: faster.String()})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}
