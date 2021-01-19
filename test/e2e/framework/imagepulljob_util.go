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
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
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

func (tester *ImagePullJobTester) CreateJob(job *appsv1alpha1.ImagePullJob) error {
	_, err := tester.kc.AppsV1alpha1().ImagePullJobs(job.Namespace).Create(job)
	return err
}

func (tester *ImagePullJobTester) DeleteJob(job *appsv1alpha1.ImagePullJob) error {
	return tester.kc.AppsV1alpha1().ImagePullJobs(job.Namespace).Delete(job.Name, &metav1.DeleteOptions{})
}

func (tester *ImagePullJobTester) DeleteAllJobs(ns string) error {
	return tester.kc.AppsV1alpha1().ImagePullJobs(ns).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{})
}

func (tester *ImagePullJobTester) GetJob(job *appsv1alpha1.ImagePullJob) (*appsv1alpha1.ImagePullJob, error) {
	return tester.kc.AppsV1alpha1().ImagePullJobs(job.Namespace).Get(job.Name, metav1.GetOptions{})
}

func (tester *ImagePullJobTester) ListJobs(ns string) (*appsv1alpha1.ImagePullJobList, error) {
	return tester.kc.AppsV1alpha1().ImagePullJobs(ns).List(metav1.ListOptions{})
}
