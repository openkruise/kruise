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

package v1beta1

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	AppsV1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/controller/imagepulljob"
	"github.com/openkruise/kruise/pkg/util"
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

func (tester *ImagePullJobTester) CreateJob(job *AppsV1beta1.ImagePullJob) error {
	_, err := tester.kc.AppsV1beta1().ImagePullJobs(job.Namespace).Create(context.TODO(), job, metav1.CreateOptions{})
	return err
}

func (tester *ImagePullJobTester) DeleteJob(job *AppsV1beta1.ImagePullJob) error {
	return tester.kc.AppsV1beta1().ImagePullJobs(job.Namespace).Delete(context.TODO(), job.Name, metav1.DeleteOptions{})
}

func (tester *ImagePullJobTester) DeleteAllJobs(ns string) error {
	return tester.kc.AppsV1beta1().ImagePullJobs(ns).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{})
}

func (tester *ImagePullJobTester) GetJob(job *AppsV1beta1.ImagePullJob) (*AppsV1beta1.ImagePullJob, error) {
	return tester.kc.AppsV1beta1().ImagePullJobs(job.Namespace).Get(context.TODO(), job.Name, metav1.GetOptions{})
}

func (tester *ImagePullJobTester) ListJobs(ns string) (*AppsV1beta1.ImagePullJobList, error) {
	return tester.kc.AppsV1beta1().ImagePullJobs(ns).List(context.TODO(), metav1.ListOptions{})
}

func (tester *ImagePullJobTester) CreateSecret(secret *v1.Secret) (*v1.Secret, error) {
	return tester.c.CoreV1().Secrets(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
}

func (tester *ImagePullJobTester) UpdateSecret(secret *v1.Secret) (*v1.Secret, error) {
	namespace, name := secret.GetNamespace(), secret.GetName()
	var err error
	var newSecret *v1.Secret
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		newSecret, err = tester.c.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		newSecret.Data = secret.Data
		newSecret, err = tester.c.CoreV1().Secrets(namespace).Update(context.TODO(), newSecret, metav1.UpdateOptions{})
		return err
	})
	return newSecret, err
}

func (tester *ImagePullJobTester) ListSyncedSecrets(source *v1.Secret) ([]v1.Secret, error) {
	options := metav1.ListOptions{}
	lister, err := tester.c.CoreV1().Secrets(util.GetKruiseDaemonConfigNamespace()).List(context.TODO(), options)
	if err != nil {
		return nil, err
	}
	var secrets []v1.Secret
	for i := range lister.Items {
		obj := lister.Items[i]
		ref := AppsV1beta1.ParseReferenceObject(obj.Annotations[imagepulljob.SecretAnnotationSourceSecretKey])
		if ref.Namespace == source.Namespace && ref.Name == source.Name {
			secrets = append(secrets, obj)
		}
	}
	return secrets, nil
}
