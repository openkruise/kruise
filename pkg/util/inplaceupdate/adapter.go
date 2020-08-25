/*
Copyright 2019 The Kruise Authors.

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

package inplaceupdate

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type adapter interface {
	getPod(namespace, name string) (*v1.Pod, error)
	updatePod(pod *v1.Pod) error
	updatePodStatus(pod *v1.Pod) error
}

type adapterRuntimeClient struct {
	client.Client
}

func (c *adapterRuntimeClient) getPod(namespace, name string) (*v1.Pod, error) {
	pod := &v1.Pod{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, pod)
	return pod, err
}

func (c *adapterRuntimeClient) updatePod(pod *v1.Pod) error {
	return c.Update(context.TODO(), pod)
}

func (c *adapterRuntimeClient) updatePodStatus(pod *v1.Pod) error {
	return c.Status().Update(context.TODO(), pod)
}

type adapterTypedClient struct {
	client clientset.Interface
}

func (c *adapterTypedClient) getPod(namespace, name string) (*v1.Pod, error) {
	return c.client.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
}

func (c *adapterTypedClient) updatePod(pod *v1.Pod) error {
	_, err := c.client.CoreV1().Pods(pod.Namespace).Update(pod)
	return err
}

func (c *adapterTypedClient) updatePodStatus(pod *v1.Pod) error {
	_, err := c.client.CoreV1().Pods(pod.Namespace).UpdateStatus(pod)
	return err
}

type adapterInformer struct {
	podInformer coreinformers.PodInformer
}

func (c *adapterInformer) getPod(namespace, name string) (*v1.Pod, error) {
	pod, err := c.podInformer.Lister().Pods(namespace).Get(name)
	if err == nil {
		return pod.DeepCopy(), nil
	}
	return nil, err
}

func (c *adapterInformer) updatePod(pod *v1.Pod) error {
	return c.podInformer.Informer().GetIndexer().Update(pod)
}

func (c *adapterInformer) updatePodStatus(pod *v1.Pod) error {
	return c.podInformer.Informer().GetIndexer().Update(pod)
}
