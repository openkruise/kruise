/*
Copyright 2022 The Kruise Authors.

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
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	utilpointer "k8s.io/utils/pointer"
)

type PersistentPodStateTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
}

func NewPersistentPodStateTester(c clientset.Interface, kc kruiseclientset.Interface) *PersistentPodStateTester {
	return &PersistentPodStateTester{
		c:  c,
		kc: kc,
	}
}

func (s *PersistentPodStateTester) NewBaseStatefulset(namespace string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "statefulset",
			Namespace:   namespace,
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: utilpointer.Int32Ptr(5),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "staticip",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "staticip",
					},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "main",
							Image:   "busybox:1.34",
							Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
					TerminationGracePeriodSeconds: utilpointer.Int64Ptr(5),
				},
			},
		},
	}
}

func (s *PersistentPodStateTester) WaitForStatefulsetRunning(sts *appsv1.StatefulSet) {
	pollErr := wait.PollImmediate(time.Second, time.Minute*5,
		func() (bool, error) {
			inner, err := s.c.AppsV1().StatefulSets(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			if inner.Generation != inner.Status.ObservedGeneration {
				return false, nil
			}
			if *inner.Spec.Replicas == inner.Status.ReadyReplicas && *inner.Spec.Replicas == inner.Status.UpdatedReplicas &&
				*inner.Spec.Replicas == inner.Status.Replicas {
				return true, nil
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for statefulset(%s/%s) to enter running: %v", sts.Namespace, sts.Name, pollErr)
	}
}

func (s *PersistentPodStateTester) CreateStatefulset(sts *appsv1.StatefulSet) {
	Logf("create StatefulSet(%s/%s)", sts.Namespace, sts.Name)
	_, err := s.c.AppsV1().StatefulSets(sts.Namespace).Create(context.TODO(), sts, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForStatefulsetRunning(sts)
}

func (s *PersistentPodStateTester) ListPodsInKruiseSts(sts *appsv1.StatefulSet) ([]*corev1.Pod, error) {
	podList, err := s.c.CoreV1().Pods(sts.Namespace).List(context.TODO(), metav1.ListOptions{})
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

func (s *PersistentPodStateTester) UpdateStatefulset(sts *appsv1.StatefulSet) {
	Logf("update statefulset(%s/%s)", sts.Namespace, sts.Name)
	stsClone, _ := s.c.AppsV1().StatefulSets(sts.Namespace).Get(context.TODO(), sts.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		stsClone.Spec = sts.Spec
		stsClone.Annotations = sts.Annotations
		stsClone.Labels = sts.Labels
		_, updateErr := s.c.AppsV1().StatefulSets(stsClone.Namespace).Update(context.TODO(), stsClone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		stsClone, _ = s.c.AppsV1().StatefulSets(stsClone.Namespace).Get(context.TODO(), stsClone.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	time.Sleep(time.Second)
	s.WaitForStatefulsetRunning(sts)
}
