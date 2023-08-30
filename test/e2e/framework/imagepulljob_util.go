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

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

type ImagePullJobTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
}

func NewImagePullJobTester(c clientset.Interface, kc kruiseclientset.Interface) *ImagePullJobTester {
	return &ImagePullJobTester{
		c:  c,
		kc: kc,
	}
}

func (tester *ImagePullJobTester) CreateJob(job *appsv1beta1.ImagePullJob) error {
	_, err := tester.kc.AppsV1beta1().ImagePullJobs(job.Namespace).Create(context.TODO(), job, metav1.CreateOptions{})
	return err
}

func (tester *ImagePullJobTester) DeleteJob(job *appsv1beta1.ImagePullJob) error {
	return tester.kc.AppsV1beta1().ImagePullJobs(job.Namespace).Delete(context.TODO(), job.Name, metav1.DeleteOptions{})
}

func (tester *ImagePullJobTester) DeleteAllJobs(ns string) error {
	return tester.kc.AppsV1beta1().ImagePullJobs(ns).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{})
}

func (tester *ImagePullJobTester) GetJob(job *appsv1beta1.ImagePullJob) (*appsv1beta1.ImagePullJob, error) {
	return tester.kc.AppsV1beta1().ImagePullJobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
}

func (tester *ImagePullJobTester) ListJobs(ns string) (*appsv1beta1.ImagePullJobList, error) {
	return tester.kc.AppsV1beta1().ImagePullJobs(ns).List(context.TODO(), metav1.ListOptions{})
}
