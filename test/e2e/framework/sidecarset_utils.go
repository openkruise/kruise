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
	"context"
	"fmt"
	"time"

	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"

	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	imageutils "k8s.io/kubernetes/test/utils/image"
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

func (s *SidecarSetTester) NewBaseSidecarSet(ns string) *appsv1alpha1.SidecarSet {
	return &appsv1alpha1.SidecarSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SidecarSet",
			APIVersion: "apps.kruise.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{

			Name: fmt.Sprintf("test-sidecarset-%s", ns),
			Labels: map[string]string{
				"app": "sidecar",
			},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			InitContainers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:    "init-sidecar",
						Command: []string{"/bin/sh", "-c", "sleep 1"},
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
					},
				},
			},
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:            "nginx-sidecar",
						Image:           imageutils.GetE2EImage(imageutils.Nginx),
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"tail", "-f", "/dev/null"},
					},
					PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
					ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
						Type: appsv1alpha1.ShareVolumePolicyEnabled,
					},
				},
				{
					Container: corev1.Container{
						Name:    "busybox-sidecar",
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"/bin/sh", "-c", "sleep 10000000"},
					},
					PodInjectPolicy: appsv1alpha1.AfterAppContainerType,
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "sidecarset"},
			},
			Namespace: ns,
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
			Replicas: ptr.To(int32(1)),
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

func (s *SidecarSetTester) CreateSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) (*appsv1alpha1.SidecarSet, error) {
	Logf("create sidecarSet(%s)", sidecarSet.Name)
	_, err := s.kc.AppsV1alpha1().SidecarSets().Create(context.TODO(), sidecarSet, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForSidecarSetCreated(sidecarSet)
	return s.kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
}

func (s *SidecarSetTester) UpdateSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) {
	Logf("update sidecarSet(%s)", sidecarSet.Name)
	sidecarSetClone, _ := s.kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		sidecarSetClone.Spec = sidecarSet.Spec
		sidecarSetClone.Annotations = sidecarSet.Annotations
		sidecarSetClone.Labels = sidecarSet.Labels
		_, updateErr := s.kc.AppsV1alpha1().SidecarSets().Update(context.TODO(), sidecarSetClone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		sidecarSetClone, _ = s.kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *SidecarSetTester) UpdateDeployment(obj *apps.Deployment) {
	objClone, _ := s.c.AppsV1().Deployments(obj.Namespace).Get(context.TODO(), obj.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		objClone.Spec = obj.Spec
		_, updateErr := s.c.AppsV1().Deployments(obj.Namespace).Update(context.TODO(), objClone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		objClone, _ = s.c.AppsV1().Deployments(obj.Namespace).Get(context.TODO(), obj.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForDeploymentRunning(obj)
}

func (s *SidecarSetTester) UpdatePod(pod *corev1.Pod) {
	Logf("update pod(%s/%s)", pod.Namespace, pod.Name)
	podClone := pod.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		podClone.Annotations = pod.Annotations
		podClone.Labels = pod.Labels
		podClone.Spec = pod.Spec
		_, updateErr := s.c.CoreV1().Pods(podClone.Namespace).Update(context.TODO(), podClone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		podClone, _ = s.c.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *SidecarSetTester) WaitForSidecarSetUpgradeComplete(sidecarSet *appsv1alpha1.SidecarSet, exceptStatus *appsv1alpha1.SidecarSetStatus) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := s.kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Status.MatchedPods == exceptStatus.MatchedPods &&
				inner.Status.UpdatedPods == exceptStatus.UpdatedPods &&
				inner.Status.UpdatedReadyPods == exceptStatus.UpdatedReadyPods &&
				inner.Status.ReadyPods == exceptStatus.ReadyPods &&
				inner.Generation == inner.Status.ObservedGeneration {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		inner, _ := s.kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
		Failf("Failed waiting for sidecarSet to upgrade complete: %v status(%v)", pollErr, inner.Status)
	}
}

func (s *SidecarSetTester) CreateDeployment(deployment *apps.Deployment) {
	Logf("create deployment(%s/%s)", deployment.Namespace, deployment.Name)
	_, err := s.c.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForDeploymentRunning(deployment)
}

func (s *SidecarSetTester) DeleteSidecarSets(ns string) {
	sidecarSetList, err := s.kc.AppsV1alpha1().SidecarSets().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List sidecarSets failed: %s", err.Error())
		return
	}

	for _, sidecarSet := range sidecarSetList.Items {
		if sidecarSet.Namespace != ns {
			continue
		}
		s.DeleteSidecarSet(&sidecarSet)
	}
}

func (s *SidecarSetTester) DeleteSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) {
	err := s.kc.AppsV1alpha1().SidecarSets().Delete(context.TODO(), sidecarSet.Name, metav1.DeleteOptions{})
	if err != nil {
		Logf("delete sidecarSet(%s) failed: %s", sidecarSet.Name, err.Error())
	}
	s.WaitForSidecarSetDeleted(sidecarSet)
}

func (s *SidecarSetTester) DeleteDeployments(namespace string) {
	deploymentList, err := s.c.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List Deployments failed: %s", err.Error())
		return
	}

	for _, deployment := range deploymentList.Items {
		s.DeleteDeployment(&deployment)
	}
}

func (s *SidecarSetTester) DeleteDeployment(deployment *apps.Deployment) {
	err := s.c.AppsV1().Deployments(deployment.Namespace).Delete(context.TODO(), deployment.Name, metav1.DeleteOptions{})
	if err != nil {
		Logf("delete deployment(%s/%s) failed: %s", deployment.Namespace, deployment.Name, err.Error())
		return
	}
	s.WaitForDeploymentDeleted(deployment)
}

func (s *SidecarSetTester) WaitForSidecarSetCreated(sidecarSet *appsv1alpha1.SidecarSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := s.kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
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
			inner, err := s.c.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			if inner.Status.ObservedGeneration == inner.Generation && *inner.Spec.Replicas == inner.Status.UpdatedReplicas &&
				*inner.Spec.Replicas == inner.Status.ReadyReplicas && *inner.Spec.Replicas == inner.Status.Replicas {
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
			_, err := s.c.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
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
			_, err := s.kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
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

func (s *SidecarSetTester) GetSelectorPods(namespace string, selector *metav1.LabelSelector) ([]*corev1.Pod, error) {
	faster, err := util.ValidatedLabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	podList, err := s.c.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: faster.String()})
	if err != nil {
		return nil, err
	}
	pods := make([]*corev1.Pod, 0)
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp.IsZero() {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

func (s *SidecarSetTester) NewBaseCloneSet(namespace string) *appsv1alpha1.CloneSet {
	return &appsv1alpha1.CloneSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneSet",
			APIVersion: appsv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "busybox",
			Namespace: namespace,
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: ptr.To(int32(2)),
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
							Image:   imageutils.GetE2EImage(imageutils.BusyBox),
							Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Type: appsv1alpha1.RecreateCloneSetUpdateStrategyType,
				MaxUnavailable: &intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "100%",
				},
				MaxSurge: &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 0,
				},
			},
		},
	}
}

func (s *SidecarSetTester) CreateCloneSet(cloneset *appsv1alpha1.CloneSet) *appsv1alpha1.CloneSet {
	Logf("create CloneSet(%s/%s)", cloneset.Namespace, cloneset.Name)
	_, err := s.kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Create(context.TODO(), cloneset, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForCloneSetRunning(cloneset)
	Logf("create cloneset(%s/%s) done", cloneset.Namespace, cloneset.Name)
	cloneset, _ = s.kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Get(context.TODO(), cloneset.Name, metav1.GetOptions{})
	return cloneset
}

func (s *SidecarSetTester) WaitForCloneSetRunning(cloneset *appsv1alpha1.CloneSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := s.kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Get(context.TODO(), cloneset.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if *inner.Spec.Replicas == inner.Status.ReadyReplicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for cloneset to enter running: %v", pollErr)
	}
}

func (s *SidecarSetTester) ListControllerRevisions(sidecarSet *appsv1alpha1.SidecarSet) []*apps.ControllerRevision {
	selector, err := util.ValidatedLabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{
		sidecarcontrol.SidecarSetKindName: sidecarSet.Name,
	}})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	revisionList, err := s.c.AppsV1().ControllerRevisions(webhookutil.GetNamespace()).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	revisions := make([]*apps.ControllerRevision, len(revisionList.Items))
	for i := range revisionList.Items {
		revisions[i] = &revisionList.Items[i]
	}
	return revisions
}
