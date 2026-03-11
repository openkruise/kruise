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

package v1alpha1

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	imageutils "k8s.io/kubernetes/test/utils/image"

	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework/common"
)

type DeploymentTester struct {
	c  clientset.Interface
	ns string
}

func NewDeploymentTester(c clientset.Interface, ns string) *DeploymentTester {
	return &DeploymentTester{
		c:  c,
		ns: ns,
	}
}

func (t *DeploymentTester) NewDeployment(name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.ns,
			Name:      name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"owner": name}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"owner": name},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "nginx",
							Image: imageutils.GetE2EImage(imageutils.Nginx),
							Env: []v1.EnvVar{
								{Name: "test", Value: "foo"},
							},
						},
					},
				},
			},
		},
	}
}

func (t *DeploymentTester) CreateDeployment(dp *appsv1.Deployment) (*appsv1.Deployment, error) {
	return t.c.AppsV1().Deployments(t.ns).Create(context.TODO(), dp, metav1.CreateOptions{})
}

func (t *DeploymentTester) GetDeployment(name string) (*appsv1.Deployment, error) {
	return t.c.AppsV1().Deployments(t.ns).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *DeploymentTester) GetSelectorPods(namespace string, selector *metav1.LabelSelector) ([]v1.Pod, error) {
	faster, err := util.ValidatedLabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	podList, err := t.c.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: faster.String()})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

// WaitForDeploymentRunning waits for the Deployment to have all replicas ready.
// It polls every second for up to 5 minutes.
func (t *DeploymentTester) WaitForDeploymentRunning(dp *appsv1.Deployment) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, time.Minute*5, true,
		func(ctx context.Context) (bool, error) {
			inner, err := t.c.AppsV1().Deployments(dp.Namespace).Get(context.TODO(), dp.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if inner.Status.ObservedGeneration >= dp.Generation &&
				inner.Status.ReadyReplicas == *dp.Spec.Replicas &&
				inner.Status.Replicas == *dp.Spec.Replicas &&
				inner.Status.UpdatedReplicas == *dp.Spec.Replicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		// Get detailed pod info for debugging
		pods, _ := t.GetSelectorPods(dp.Namespace, dp.Spec.Selector)
		common.Failf("Failed waiting for Deployment %s/%s to be running: %v\nDeployment: %s\nPods: %s",
			dp.Namespace, dp.Name, pollErr, util.DumpJSON(dp), util.DumpJSON(pods))
	}
}
