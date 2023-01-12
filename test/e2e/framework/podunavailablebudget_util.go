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

package framework

import (
	"context"
	"fmt"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"

	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	imageutils "k8s.io/kubernetes/test/utils/image"
	utilpointer "k8s.io/utils/pointer"
)

type PodUnavailableBudgetTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
}

func NewPodUnavailableBudgetTester(c clientset.Interface, kc kruiseclientset.Interface) *PodUnavailableBudgetTester {
	return &PodUnavailableBudgetTester{
		c:  c,
		kc: kc,
	}
}

func (t *PodUnavailableBudgetTester) NewBasePub(namespace string) *policyv1alpha1.PodUnavailableBudget {
	return &policyv1alpha1.PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyv1alpha1.GroupVersion.String(),
			Kind:       "PodUnavailableBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "webserver-pub",
		},
		Spec: policyv1alpha1.PodUnavailableBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"pub-controller": "true",
				},
			},
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 1,
			},
		},
	}
}

func (s *PodUnavailableBudgetTester) NewBaseDeployment(namespace string) *apps.Deployment {
	return &apps.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webserver",
			Namespace: namespace,
		},
		Spec: apps.DeploymentSpec{
			Replicas: utilpointer.Int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":            "webserver",
					"pub-controller": "true",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":            "webserver",
						"pub-controller": "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "main",
							Image:           imageutils.GetE2EImage(imageutils.Httpd),
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
			},
			Strategy: apps.DeploymentStrategy{
				Type: apps.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &apps.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{
						Type:   intstr.String,
						StrVal: "50%",
					},
					MaxSurge: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 0,
					},
				},
			},
		},
	}
}

func (s *PodUnavailableBudgetTester) NewBaseCloneSet(namespace string) *appsv1alpha1.CloneSet {
	return &appsv1alpha1.CloneSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneSet",
			APIVersion: appsv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webserver",
			Namespace: namespace,
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: utilpointer.Int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":            "webserver",
					"pub-controller": "true",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":            "webserver",
						"pub-controller": "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "main",
							Image:           imageutils.GetE2EImage(imageutils.Httpd),
							ImagePullPolicy: corev1.PullIfNotPresent,
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

func (t *PodUnavailableBudgetTester) CreatePub(pub *policyv1alpha1.PodUnavailableBudget) *policyv1alpha1.PodUnavailableBudget {
	Logf("create PodUnavailableBudget(%s/%s)", pub.Namespace, pub.Name)
	_, err := t.kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Create(context.TODO(), pub, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	t.WaitForPubCreated(pub)
	pub, _ = t.kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
	return pub
}

func (t *PodUnavailableBudgetTester) CreateDeployment(deployment *apps.Deployment) {
	Logf("create deployment(%s/%s)", deployment.Namespace, deployment.Name)
	_, err := t.c.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	t.WaitForDeploymentReadyAndRunning(deployment)
	Logf("create deployment(%s/%s) done", deployment.Namespace, deployment.Name)
}

func (t *PodUnavailableBudgetTester) CreateCloneSet(cloneset *appsv1alpha1.CloneSet) *appsv1alpha1.CloneSet {
	Logf("create CloneSet(%s/%s)", cloneset.Namespace, cloneset.Name)
	_, err := t.kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Create(context.TODO(), cloneset, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	t.WaitForCloneSetRunning(cloneset)
	Logf("create cloneset(%s/%s) done", cloneset.Namespace, cloneset.Name)
	cloneset, _ = t.kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Get(context.TODO(), cloneset.Name, metav1.GetOptions{})
	return cloneset
}

func (t *PodUnavailableBudgetTester) WaitForPubCreated(pub *policyv1alpha1.PodUnavailableBudget) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := t.kc.PolicyV1alpha1().PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return true, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for PodUnavailableBudget to enter running: %v", pollErr)
	}
}

func (t *PodUnavailableBudgetTester) WaitForCloneSetRunning(cloneset *appsv1alpha1.CloneSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Get(context.TODO(), cloneset.Name, metav1.GetOptions{})
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

func (t *PodUnavailableBudgetTester) WaitForDeploymentReadyAndRunning(deployment *apps.Deployment) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := t.c.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			count := *inner.Spec.Replicas
			if inner.Generation == inner.Status.ObservedGeneration && inner.Status.UpdatedReplicas == count &&
				count == inner.Status.ReadyReplicas && count == inner.Status.Replicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for deployment to enter running: %v", pollErr)
	}
}

func (t *PodUnavailableBudgetTester) WaitForCloneSetMinReadyAndRunning(cloneSets []*appsv1alpha1.CloneSet, minReady int32) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*10,
		func() (bool, error) {
			var readyReplicas int32 = 0
			completed := 0
			for _, cloneSet := range cloneSets {
				inner, err := t.kc.AppsV1alpha1().CloneSets(cloneSet.Namespace).Get(context.TODO(), cloneSet.Name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				readyReplicas += inner.Status.ReadyReplicas
				count := *inner.Spec.Replicas
				if inner.Generation == inner.Status.ObservedGeneration && inner.Status.UpdatedReplicas == count &&
					count == inner.Status.ReadyReplicas && count == inner.Status.Replicas {
					completed++
				}
			}

			if readyReplicas < minReady {
				return false, fmt.Errorf("deployment ReadyReplicas(%d) < except(%d)", readyReplicas, minReady)
			}
			if completed == len(cloneSets) {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for cloneSet to enter running: %v", pollErr)
	}
}

func (t *PodUnavailableBudgetTester) DeletePubs(namespace string) {
	pubList, err := t.kc.PolicyV1alpha1().PodUnavailableBudgets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List sidecarSets failed: %s", err.Error())
		return
	}

	for _, pub := range pubList.Items {
		err := t.kc.PolicyV1alpha1().PodUnavailableBudgets(namespace).Delete(context.TODO(), pub.Name, metav1.DeleteOptions{})
		if err != nil {
			Logf("delete PodUnavailableBudget(%s/%s) failed: %s", pub.Namespace, pub.Name, err.Error())
		}
	}
}

func (t *PodUnavailableBudgetTester) DeleteDeployments(namespace string) {
	deploymentList, err := t.c.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List Deployments failed: %s", err.Error())
		return
	}

	for _, deployment := range deploymentList.Items {
		err := t.c.AppsV1().Deployments(namespace).Delete(context.TODO(), deployment.Name, metav1.DeleteOptions{})
		if err != nil {
			Logf("delete Deployment(%s/%s) failed: %s", deployment.Namespace, deployment.Name, err.Error())
			continue
		}
		t.WaitForDeploymentDeleted(&deployment)
	}
}

func (t *PodUnavailableBudgetTester) DeleteCloneSets(namespace string) {
	objectList, err := t.kc.AppsV1alpha1().CloneSets(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List CloneSets failed: %s", err.Error())
		return
	}

	for _, object := range objectList.Items {
		err := t.kc.AppsV1alpha1().CloneSets(namespace).Delete(context.TODO(), object.Name, metav1.DeleteOptions{})
		if err != nil {
			Logf("delete CloneSet(%s/%s) failed: %s", object.Namespace, object.Name, err.Error())
			continue
		}
		t.WaitForCloneSetDeleted(&object)
	}
}

func (t *PodUnavailableBudgetTester) WaitForDeploymentDeleted(deployment *apps.Deployment) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := t.c.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
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

func (t *PodUnavailableBudgetTester) WaitForCloneSetDeleted(cloneset *appsv1alpha1.CloneSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := t.kc.AppsV1alpha1().CloneSets(cloneset.Namespace).Get(context.TODO(), cloneset.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for cloneset to enter Deleted: %v", pollErr)
	}
}

func (s *SidecarSetTester) WaitForSidecarSetMinReadyAndUpgrade(sidecarSet *appsv1alpha1.SidecarSet, exceptStatus *appsv1alpha1.SidecarSetStatus, minReady int32) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := s.kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSet.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if minReady > 0 && inner.Status.ReadyPods < minReady {
				return false, fmt.Errorf("sidecarSet(%s) ReadyReplicas(%d) < except(%d)", sidecarSet.Name, inner.Status.ReadyPods, minReady)
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
