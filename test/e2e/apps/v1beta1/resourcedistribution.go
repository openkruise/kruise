/*
Copyright 2026 The Kruise Authors.

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
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	framework "github.com/openkruise/kruise/test/e2e/framework/v1beta1"
)

var _ = ginkgo.Describe("ResourceDistribution", ginkgo.Serial, ginkgo.Label("ResourceDistribution", "v1beta1"), func() {
	f := framework.NewDefaultFramework("resourcedistribution-v1beta1")
	var c clientset.Interface
	var kc kruiseclientset.Interface

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
	})

	ginkgo.It("should distribute resources with the beta api", func() {
		prefix := fmt.Sprintf("rd-beta-%d", time.Now().UnixNano())
		namespaces := []*corev1.Namespace{
			{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-2"}},
			{ObjectMeta: metav1.ObjectMeta{Name: prefix + "-3"}},
		}
		for _, namespace := range namespaces {
			_, err := c.CoreV1().Namespaces().Create(context.TODO(), namespace, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
		defer cleanupNamespaces(c, prefix)

		rd := &appsv1beta1.ResourceDistribution{
			ObjectMeta: metav1.ObjectMeta{Name: prefix},
			Spec: appsv1beta1.ResourceDistributionSpec{
				Resource: runtime.RawExtension{
					Raw: []byte(`{
						"apiVersion":"v1",
						"kind":"Secret",
						"metadata":{"name":"resourcedistribution-e2e-beta-secret"},
						"type":"Opaque",
						"data":{"test":"dGVzdA=="}
					}`),
				},
				Targets: appsv1beta1.ResourceDistributionTargets{
					IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
						List: []appsv1beta1.ResourceDistributionNamespace{
							{Name: prefix + "-1"},
							{Name: prefix + "-2"},
						},
					},
					ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
						List: []appsv1beta1.ResourceDistributionNamespace{
							{Name: prefix + "-3"},
						},
					},
				},
			},
		}
		rd, err := kc.AppsV1beta1().ResourceDistributions().Create(context.TODO(), rd, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer func() {
			_ = kc.AppsV1beta1().ResourceDistributions().Delete(context.TODO(), rd.Name, metav1.DeleteOptions{})
		}()

		gomega.Eventually(func(g gomega.Gomega) {
			current, err := kc.AppsV1beta1().ResourceDistributions().Get(context.TODO(), rd.Name, metav1.GetOptions{})
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(current.Status.Succeeded).To(gomega.Equal(int32(2)))
			g.Expect(current.Status.Desired).To(gomega.Equal(int32(2)))
		}, 2*time.Minute, time.Second).Should(gomega.Succeed())

		for _, namespace := range []string{prefix + "-1", prefix + "-2"} {
			gomega.Eventually(func() error {
				secret, err := c.CoreV1().Secrets(namespace).Get(context.TODO(), "resourcedistribution-e2e-beta-secret", metav1.GetOptions{})
				if err != nil {
					return err
				}
				if secret.Annotations[validating.SourceResourceDistributionOfResource] != rd.Name {
					return fmt.Errorf("secret %s/%s missing source annotation", namespace, secret.Name)
				}
				return nil
			}, 2*time.Minute, time.Second).Should(gomega.Succeed())
		}

		_, err = c.CoreV1().Secrets(prefix+"-3").Get(context.TODO(), "resourcedistribution-e2e-beta-secret", metav1.GetOptions{})
		gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
	})
})

func cleanupNamespaces(c clientset.Interface, prefix string) {
	namespaces, err := c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		common.Logf("list namespaces failed: %v", err)
		return
	}
	for _, namespace := range namespaces.Items {
		if len(namespace.Name) >= len(prefix) && namespace.Name[:len(prefix)] == prefix {
			_ = c.CoreV1().Namespaces().Delete(context.TODO(), namespace.Name, metav1.DeleteOptions{})
		}
	}
}
