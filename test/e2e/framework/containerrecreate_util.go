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
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type ContainerRecreateTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewContainerRecreateTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *ContainerRecreateTester {
	return &ContainerRecreateTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

func (t *ContainerRecreateTester) CreateTestCloneSetAndGetPods(randStr string, replicas int32, containers []v1.Container) (pods []*v1.Pod) {
	set := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: t.ns, Name: "clone-foo-" + randStr},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"rand": randStr}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"rand": randStr},
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
	if _, err = t.kc.AppsV1alpha1().CloneSets(t.ns).Create(context.TODO(), set, metav1.CreateOptions{}); err != nil {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	// Wait for 60s
	gomega.Eventually(func() int32 {
		set, err = t.kc.AppsV1alpha1().CloneSets(t.ns).Get(context.TODO(), set.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return set.Status.ReadyReplicas
	}, 120*time.Second, 3*time.Second).Should(gomega.Equal(replicas))

	podList, err := t.c.CoreV1().Pods(t.ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "rand=" + randStr})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	for i := range podList.Items {
		p := &podList.Items[i]
		klog.Infof("Pod(%s/%s/%s) status(%s)", p.Namespace, p.Name, p.UID, util.DumpJSON(p.Status))
		pods = append(pods, p)
	}
	return
}

func (t *ContainerRecreateTester) CleanAllTestResources() error {
	if err := t.kc.AppsV1alpha1().ContainerRecreateRequests(t.ns).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{}); err != nil {
		return err
	}
	if err := t.kc.AppsV1alpha1().CloneSets(t.ns).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{}); err != nil {
		return err
	}
	return nil
}

func (t *ContainerRecreateTester) CreateCRR(crr *appsv1alpha1.ContainerRecreateRequest) (*appsv1alpha1.ContainerRecreateRequest, error) {
	return t.kc.AppsV1alpha1().ContainerRecreateRequests(crr.Namespace).Create(context.TODO(), crr, metav1.CreateOptions{})
}

func (t *ContainerRecreateTester) GetCRR(name string) (*appsv1alpha1.ContainerRecreateRequest, error) {
	return t.kc.AppsV1alpha1().ContainerRecreateRequests(t.ns).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *ContainerRecreateTester) GetPod(name string) (*v1.Pod, error) {
	return t.c.CoreV1().Pods(t.ns).Get(context.TODO(), name, metav1.GetOptions{})
}
