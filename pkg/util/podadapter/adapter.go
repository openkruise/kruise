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

package podadapter

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Adapter interface {
	GetPod(namespace, name string) (*v1.Pod, error)
	UpdatePod(pod *v1.Pod) (*v1.Pod, error)
	UpdatePodStatus(pod *v1.Pod) error
}

type AdapterWithPatch interface {
	Adapter
	PatchPod(pod *v1.Pod, patch client.Patch) (*v1.Pod, error)
}

type AdapterRuntimeClient struct {
	client.Client
}

func (c *AdapterRuntimeClient) GetPod(namespace, name string) (*v1.Pod, error) {
	pod := &v1.Pod{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, pod)
	return pod, err
}

func (c *AdapterRuntimeClient) UpdatePod(pod *v1.Pod) (*v1.Pod, error) {
	return pod, c.Update(context.TODO(), pod)
}

func (c *AdapterRuntimeClient) UpdatePodStatus(pod *v1.Pod) error {
	return c.Status().Update(context.TODO(), pod)
}

func (c *AdapterRuntimeClient) PatchPod(pod *v1.Pod, patch client.Patch) (*v1.Pod, error) {
	return pod, c.Patch(context.TODO(), pod, patch)
}

type AdapterTypedClient struct {
	Client clientset.Interface
}

func (c *AdapterTypedClient) GetPod(namespace, name string) (*v1.Pod, error) {
	return c.Client.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c *AdapterTypedClient) UpdatePod(pod *v1.Pod) (*v1.Pod, error) {
	return c.Client.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
}

func (c *AdapterTypedClient) UpdatePodStatus(pod *v1.Pod) error {
	_, err := c.Client.CoreV1().Pods(pod.Namespace).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
	return err
}

func (c *AdapterTypedClient) PatchPod(pod *v1.Pod, patch client.Patch) (*v1.Pod, error) {
	patchData, err := patch.Data(pod)
	if err != nil {
		return nil, err
	}
	return c.Client.CoreV1().Pods(pod.Namespace).Patch(context.TODO(), pod.Name, patch.Type(), patchData, metav1.PatchOptions{})
}

type AdapterInformer struct {
	PodInformer coreinformers.PodInformer
}

func (c *AdapterInformer) GetPod(namespace, name string) (*v1.Pod, error) {
	pod, err := c.PodInformer.Lister().Pods(namespace).Get(name)
	if err == nil {
		return pod.DeepCopy(), nil
	}
	return nil, err
}

func (c *AdapterInformer) UpdatePod(pod *v1.Pod) (*v1.Pod, error) {
	return pod, c.PodInformer.Informer().GetIndexer().Update(pod)
}

func (c *AdapterInformer) UpdatePodStatus(pod *v1.Pod) error {
	return c.PodInformer.Informer().GetIndexer().Update(pod)
}
