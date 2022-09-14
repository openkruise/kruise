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
	"time"

	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
)

type EphemeralJobTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewEphemeralJobTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *EphemeralJobTester {
	return &EphemeralJobTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

func (s *EphemeralJobTester) DeleteEphemeralJobs(ns string) {
	ejobLists, err := s.kc.AppsV1alpha1().EphemeralJobs(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List sidecarSets failed: %s", err.Error())
		return
	}

	for _, ejob := range ejobLists.Items {
		s.DeleteEphemeralJob(&ejob)
	}
}

func (t *EphemeralJobTester) DeleteEphemeralJob(job *appsv1alpha1.EphemeralJob) {
	err := t.kc.AppsV1alpha1().EphemeralJobs(t.ns).Delete(context.TODO(), job.Name, metav1.DeleteOptions{})
	if err != nil {
		Logf("delete ephemeraljob(%s) failed: %s", job.Name, err.Error())
	}
	t.WaitForEphemeralJobDeleted(job)
}

func (t *EphemeralJobTester) GetEphemeralJob(name string) (*appsv1alpha1.EphemeralJob, error) {
	return t.kc.AppsV1alpha1().EphemeralJobs(t.ns).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *EphemeralJobTester) GetPodsByEjob(name string) ([]*v1.Pod, error) {
	podList, err := t.c.CoreV1().Pods(t.ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var res []*v1.Pod
	for _, p := range podList.Items {
		res = append(res, &p)
	}
	return res, nil
}

func (t *EphemeralJobTester) WaitForEphemeralJobCreated(job *appsv1alpha1.EphemeralJob) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := t.kc.AppsV1alpha1().EphemeralJobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return true, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for EphemeralJob to enter running: %v", pollErr)
	}
}

func (t *EphemeralJobTester) WaitForEphemeralJobDeleted(job *appsv1alpha1.EphemeralJob) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := t.kc.AppsV1alpha1().EphemeralJobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for EphemeralJob to enter Deleted: %v", pollErr)
	}
}

func (t *EphemeralJobTester) CreateTestDeployment(randStr string, replicas int32, containers []v1.Container) (pods []*v1.Pod) {
	deployment := &apps.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.ns,
			Name:      "foo-" + randStr,
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"rand": randStr}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"rand": randStr,
						"run":  "nginx",
					},
				},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
								{
									Weight:          100,
									PodAffinityTerm: v1.PodAffinityTerm{TopologyKey: v1.LabelHostname, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"rand": randStr}}},
								},
							},
						},
					},
					Containers: containers,
				},
			},
		},
	}

	var err error
	Logf("create deployment(%s/%s)", deployment.Namespace, deployment.Name)
	_, err = t.c.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	t.WaitForDeploymentRunning(deployment)

	// Wait for 60s
	gomega.Eventually(func() int32 {
		deploy, err := t.c.AppsV1().Deployments(t.ns).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return deploy.Status.ReadyReplicas
	}, 120*time.Second, 3*time.Second).Should(gomega.Equal(replicas))

	return
}

func (t *EphemeralJobTester) CreateTestEphemeralJob(randStr string, replicas, Parallelism int32, selector metav1.LabelSelector, containers []v1.EphemeralContainer) *appsv1alpha1.EphemeralJob {
	job := &appsv1alpha1.EphemeralJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: t.ns, Name: "job-" + randStr},
		Spec: appsv1alpha1.EphemeralJobSpec{
			Selector:    &selector,
			Replicas:    &replicas,
			Parallelism: &Parallelism,
			Template: appsv1alpha1.EphemeralContainerTemplateSpec{
				EphemeralContainers: containers,
			},
		},
	}

	Logf("create ephemeral job(%s/%s)", job.Namespace, job.Name)
	job, _ = t.kc.AppsV1alpha1().EphemeralJobs(t.ns).Create(context.TODO(), job, metav1.CreateOptions{})
	t.WaitForEphemeralJobCreated(job)

	job, _ = t.kc.AppsV1alpha1().EphemeralJobs(t.ns).Get(context.TODO(), "job-"+randStr, metav1.GetOptions{})
	return job
}

func (t *EphemeralJobTester) CreateEphemeralJob(job *appsv1alpha1.EphemeralJob) *appsv1alpha1.EphemeralJob {
	job.Namespace = t.ns
	Logf("create ephemeral job(%s/%s)", job.Namespace, job.Name)
	job, _ = t.kc.AppsV1alpha1().EphemeralJobs(t.ns).Create(context.TODO(), job, metav1.CreateOptions{})
	t.WaitForEphemeralJobCreated(job)
	return job
}

func (t *EphemeralJobTester) CheckEphemeralJobExist(job *appsv1alpha1.EphemeralJob) bool {
	_, err := t.kc.AppsV1alpha1().EphemeralJobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
	return !errors.IsNotFound(err)
}

func (s *EphemeralJobTester) WaitForDeploymentRunning(deployment *apps.Deployment) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := s.c.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
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

func (s *EphemeralJobTester) DeleteDeployments(namespace string) {
	deploymentList, err := s.c.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List Deployments failed: %s", err.Error())
		return
	}

	for _, deployment := range deploymentList.Items {
		s.DeleteDeployment(&deployment)
	}
}

func (s *EphemeralJobTester) DeleteDeployment(deployment *apps.Deployment) {
	err := s.c.AppsV1().Deployments(deployment.Namespace).Delete(context.TODO(), deployment.Name, metav1.DeleteOptions{})
	if err != nil {
		Logf("delete deployment(%s/%s) failed: %s", deployment.Namespace, deployment.Name, err.Error())
		return
	}
	s.WaitForDeploymentDeleted(deployment)
}

func (s *EphemeralJobTester) WaitForDeploymentDeleted(deployment *apps.Deployment) {
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
