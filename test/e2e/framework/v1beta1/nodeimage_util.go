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
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	AppsV1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/util"
)

const (
	// Allow fake NodeImage with no Node related, just for tests
	FakeNodeImageLabelKey = "apps.kruise.io/fake-nodeimage"
)

type NodeImageTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
}

func NewNodeImageTester(c clientset.Interface, kc kruiseclientset.Interface) *NodeImageTester {
	return &NodeImageTester{
		c:  c,
		kc: kc,
	}
}

func (tester *NodeImageTester) CreateFakeNodeImageIfNotPresent() error {
	_, err := tester.kc.AppsV1beta1().NodeImages().Get(context.TODO(), "fake-nodeimage", metav1.GetOptions{})
	if err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	fakeObj := AppsV1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-nodeimage", Labels: map[string]string{FakeNodeImageLabelKey: "true"}},
	}
	_, err = tester.kc.AppsV1beta1().NodeImages().Create(context.TODO(), &fakeObj, metav1.CreateOptions{})
	return err
}

func (tester *NodeImageTester) DeleteFakeNodeImage() error {
	err := tester.kc.AppsV1beta1().NodeImages().Delete(context.TODO(), "fake-nodeimage", metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return nil
	}
	return err
}

func (tester *NodeImageTester) ListNodeImages() (*AppsV1beta1.NodeImageList, error) {
	return tester.kc.AppsV1beta1().NodeImages().List(context.TODO(), metav1.ListOptions{})
}

func (tester *NodeImageTester) ExpectNodes() ([]*v1.Node, error) {
	nodeList, err := tester.c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(nodeList.Items) == 0 {
		return nil, fmt.Errorf("no nodes found")
	}
	var nodes []*v1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		// ignore vk
		if node.Labels["type"] == "virtual-kubelet" {
			continue
		}
		nodes = append(nodes, &nodeList.Items[i])
	}
	nodeImageList, err := tester.ListNodeImages()
	if err != nil {
		return nil, err
	}
	if len(nodes) != len(nodeImageList.Items)-1 {
		return nil, fmt.Errorf("unexpected nodes number %d and nodeimages number %d, nodes: %v, nodeimages: %v",
			len(nodes), len(nodeImageList.Items), util.DumpJSON(nodes), util.DumpJSON(nodeImageList))
	}
	return nodes, nil
}

func (tester *NodeImageTester) GetNodeImage(name string) (*AppsV1beta1.NodeImage, error) {
	return tester.kc.AppsV1beta1().NodeImages().Get(context.TODO(), name, metav1.GetOptions{})
}

func (tester *NodeImageTester) IsImageInSpec(image, nodeName string, secrets []string) (bool, error) {
	nodeImage, err := tester.GetNodeImage(nodeName)
	if err != nil {
		return false, err
	}

	imageName, imageTag, err := daemonutil.NormalizeImageRefToNameTag(image)
	if err != nil {
		return false, err
	}
	imageSpec, ok := nodeImage.Spec.Images[imageName]
	if !ok {
		return false, nil
	}

	newSecrets := make([]AppsV1beta1.ReferenceObject, 0)
	sort.Strings(secrets)
	for _, name := range secrets {
		obj := AppsV1beta1.ReferenceObject{
			Namespace: util.GetKruiseDaemonConfigNamespace(),
			Name:      name,
		}
		newSecrets = append(newSecrets, obj)
	}

	for _, tagSpec := range imageSpec.Tags {
		if tagSpec.Tag == imageTag && util.IsJSONObjectEqual(newSecrets, tagSpec.PullSecrets) {
			return true, nil
		}
	}
	return false, nil
}
