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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework/common"
)

type BroadcastJobTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewBroadcastJobTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *BroadcastJobTester {
	return &BroadcastJobTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

func (t *BroadcastJobTester) CreateBroadcastJob(job *appsv1alpha1.BroadcastJob) (*appsv1alpha1.BroadcastJob, error) {
	job, err := t.kc.AppsV1alpha1().BroadcastJobs(t.ns).Create(context.TODO(), job, metav1.CreateOptions{})
	t.WaitForBroadcastJobCreated(job)
	return job, err
}

func (t *BroadcastJobTester) GetBroadcastJob(name string) (*appsv1alpha1.BroadcastJob, error) {
	return t.kc.AppsV1alpha1().BroadcastJobs(t.ns).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *BroadcastJobTester) GetPodsOfJob(job *appsv1alpha1.BroadcastJob) (pods []*v1.Pod, err error) {
	podList, err := t.c.CoreV1().Pods(t.ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		controllerRef := metav1.GetControllerOf(pod)
		if controllerRef != nil && controllerRef.UID == job.UID {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}

func (t *BroadcastJobTester) WaitForBroadcastJobCreated(job *appsv1alpha1.BroadcastJob) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, time.Minute, true,
		func(ctx context.Context) (bool, error) {
			_, err := t.kc.AppsV1alpha1().BroadcastJobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return true, nil
		})
	if pollErr != nil {
		common.Failf("Failed waiting for BroadcastJob to enter running: %v", pollErr)
	}
}

// WaitForBroadcastJobDesired waits for the BroadcastJob to have its Desired status set to the expected count.
// It polls every second for up to 30 seconds.
func (t *BroadcastJobTester) WaitForBroadcastJobDesired(job *appsv1alpha1.BroadcastJob, expectedDesired int32) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, 30*time.Second, true,
		func(ctx context.Context) (bool, error) {
			inner, err := t.kc.AppsV1alpha1().BroadcastJobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			common.Logf("BroadcastJob %s/%s: Desired=%d, expected=%d",
				job.Namespace, job.Name, inner.Status.Desired, expectedDesired)
			return inner.Status.Desired == expectedDesired, nil
		})
	if pollErr != nil {
		job, _ := t.GetBroadcastJob(job.Name)
		common.Failf("Failed waiting for BroadcastJob %s/%s to have desired=%d: %v\nBroadcastJob status: %+v",
			job.Namespace, job.Name, expectedDesired, pollErr, job.Status)
	}
}

// WaitForBroadcastJobSucceeded waits for all pods of the BroadcastJob to succeed.
// It polls every second for up to 3 minutes.
func (t *BroadcastJobTester) WaitForBroadcastJobSucceeded(job *appsv1alpha1.BroadcastJob, expectedSucceeded int32) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, 3*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			inner, err := t.kc.AppsV1alpha1().BroadcastJobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			common.Logf("BroadcastJob %s/%s: Succeeded=%d, expected=%d, Active=%d, Failed=%d",
				job.Namespace, job.Name, inner.Status.Succeeded, expectedSucceeded, inner.Status.Active, inner.Status.Failed)
			return inner.Status.Succeeded == expectedSucceeded, nil
		})
	if pollErr != nil {
		job, _ := t.GetBroadcastJob(job.Name)
		pods, _ := t.GetPodsOfJob(job)
		common.Failf("Failed waiting for BroadcastJob %s/%s to have %d succeeded: %v\nBroadcastJob status: %+v\nPods: %+v",
			job.Namespace, job.Name, expectedSucceeded, pollErr, job.Status, pods)
	}
}
