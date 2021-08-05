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
	"fmt"

	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
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
	_, err := tester.kc.AppsV1alpha1().NodeImages().Get(context.TODO(), "fake-nodeimage", metav1.GetOptions{})
	if err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	fakeObj := appsv1alpha1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-nodeimage", Labels: map[string]string{FakeNodeImageLabelKey: "true"}},
	}
	_, err = tester.kc.AppsV1alpha1().NodeImages().Create(context.TODO(), &fakeObj, metav1.CreateOptions{})
	return err
}

func (tester *NodeImageTester) DeleteFakeNodeImage() error {
	err := tester.kc.AppsV1alpha1().NodeImages().Delete(context.TODO(), "fake-nodeimage", metav1.DeleteOptions{})
	if err != nil && errors.IsNotFound(err) {
		return nil
	}
	return err
}

func (tester *NodeImageTester) ListNodeImages() (*appsv1alpha1.NodeImageList, error) {
	return tester.kc.AppsV1alpha1().NodeImages().List(context.TODO(), metav1.ListOptions{})
}

func (tester *NodeImageTester) ExpectNodes() ([]*v1.Node, error) {
	nodeList, err := tester.c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(nodeList.Items) == 0 {
		return nil, fmt.Errorf("no nodes found")
	}
	nodeImageList, err := tester.ListNodeImages()
	if err != nil {
		return nil, err
	}
	if len(nodeList.Items) != len(nodeImageList.Items)-1 {
		return nil, fmt.Errorf("unexpected nodes number %d and nodeimages number %d, nodes: %v, nodeimages: %v",
			len(nodeList.Items), len(nodeImageList.Items), util.DumpJSON(nodeList), util.DumpJSON(nodeImageList))
	}
	var nodes []*v1.Node
	for i := range nodeList.Items {
		nodes = append(nodes, &nodeList.Items[i])
	}
	return nodes, nil
}

func (tester *NodeImageTester) GetNodeImage(name string) (*appsv1alpha1.NodeImage, error) {
	return tester.kc.AppsV1alpha1().NodeImages().Get(context.TODO(), name, metav1.GetOptions{})
}

func (tester *NodeImageTester) IsImageInSpec(image, nodeName string) (bool, error) {
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
	for _, tagSpec := range imageSpec.Tags {
		if tagSpec.Tag == imageTag {
			return true, nil
		}
	}
	return false, nil
}
