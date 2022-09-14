/*
Copyright 2020 The Kruise Authors.

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
	"strings"
	"time"

	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

type ResourceDistributionTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
}

func NewResourceDistributionTester(c clientset.Interface, kc kruiseclientset.Interface) *ResourceDistributionTester {
	return &ResourceDistributionTester{
		c:  c,
		kc: kc,
	}
}

func (s *ResourceDistributionTester) NewBaseResourceDistribution(nsPrefix string) *appsv1alpha1.ResourceDistribution {
	const resourceJSON = `{
    "apiVersion": "v1",
    "data": {
        ".dockerconfigjson": "eyJhdXRocyI6eyJodHRwczovL2luZGV4LmRvY2tlci5pby92Mi8iOnsidXNlcm5hbWUiOiJtaW5jaG91IiwicGFzc3dvcmQiOiJtaW5nemhvdS5zd3giLCJlbWFpbCI6InZlYy5nLnN1bkBnbWFpbC5jb20iLCJhdXRoIjoiYldsdVkyaHZkVHB0YVc1bmVtaHZkUzV6ZDNnPSJ9fX0="
    },
    "kind": "Secret",
    "metadata": {
        "name": "resourcedistribution-e2e-test-secret"
    },
    "type": "Opaque"
}`
	return &appsv1alpha1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsPrefix,
		},
		Spec: appsv1alpha1.ResourceDistributionSpec{
			Resource: runtime.RawExtension{
				Raw: []byte(resourceJSON),
			},
			Targets: appsv1alpha1.ResourceDistributionTargets{
				ExcludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
					List: []appsv1alpha1.ResourceDistributionNamespace{
						{
							Name: nsPrefix + "-4",
						},
					},
				},
				IncludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
					List: []appsv1alpha1.ResourceDistributionNamespace{
						{
							Name: nsPrefix + "-1",
						},
						{
							Name: nsPrefix + "-2",
						},
						{
							Name: nsPrefix + "-3",
						},
						{
							Name: nsPrefix + "-5",
						},
					},
				},
				NamespaceLabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"e2e-rd-group": "one",
					},
				},
			},
		},
	}
}

func (s *ResourceDistributionTester) NewBaseNamespace(namespace string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
}

func (s *ResourceDistributionTester) CreateResourceDistribution(resourceDistribution *appsv1alpha1.ResourceDistribution) *appsv1alpha1.ResourceDistribution {
	Logf("create ResourceDistribution(%s)", resourceDistribution.Name)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		_, err := s.kc.AppsV1alpha1().ResourceDistributions().Create(context.TODO(), resourceDistribution, metav1.CreateOptions{})
		return err
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForResourceDistributionCreated(resourceDistribution.Name, time.Minute)
	resourceDistribution, _ = s.kc.AppsV1alpha1().ResourceDistributions().Get(context.TODO(), resourceDistribution.Name, metav1.GetOptions{})
	return resourceDistribution
}

func (s *ResourceDistributionTester) UpdateResourceDistribution(resourceDistribution *appsv1alpha1.ResourceDistribution) {
	Logf("update ResourceDistribution(%s)", resourceDistribution.Name)
	resourceDistributionClone, _ := s.kc.AppsV1alpha1().ResourceDistributions().Get(context.TODO(), resourceDistribution.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		resourceDistributionClone.Spec = resourceDistribution.Spec
		resourceDistributionClone.Annotations = resourceDistribution.Annotations
		resourceDistributionClone.Labels = resourceDistribution.Labels
		_, updateErr := s.kc.AppsV1alpha1().ResourceDistributions().Update(context.TODO(), resourceDistributionClone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		resourceDistributionClone, _ = s.kc.AppsV1alpha1().ResourceDistributions().Get(context.TODO(), resourceDistributionClone.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *ResourceDistributionTester) UpdateNamespace(namespace *corev1.Namespace) {
	Logf("update namespace(%s)", namespace.Name)
	namespaceClone := namespace.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		namespaceClone.Annotations = namespace.Annotations
		namespaceClone.Labels = namespace.Labels
		namespaceClone.Spec = namespace.Spec
		_, updateErr := s.c.CoreV1().Namespaces().Update(context.TODO(), namespaceClone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		namespaceClone, _ = s.c.CoreV1().Namespaces().Get(context.TODO(), namespace.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *ResourceDistributionTester) CreateNamespaces(namespaces ...*corev1.Namespace) {
	for _, namespace := range namespaces {
		if _, err := s.c.CoreV1().Namespaces().Get(context.TODO(), namespace.Name, metav1.GetOptions{}); err == nil || !errors.IsNotFound(err) {
			continue
		}
		Logf("create namespace(%s)", namespace.Name)
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			_, err := s.c.CoreV1().Namespaces().Create(context.TODO(), namespace, metav1.CreateOptions{})
			return err
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		s.WaitForNamespaceCreated(namespace)
	}
}

func (s *ResourceDistributionTester) CreateSecretResources(secrets ...*corev1.Secret) {
	for _, secret := range secrets {
		Logf("create secrets(%s/%s)", secret.Namespace, secret.Name)
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			_, err := s.c.CoreV1().Secrets(secret.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
			return err
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		s.WaitForSecretCreated(secret.Namespace, secret.Name, time.Minute)
	}
}

func (s *ResourceDistributionTester) GetSecret(namespace, name string, mustExistAssertion bool) (*corev1.Secret, error) {
	if mustExistAssertion {
		s.WaitForSecretCreated(namespace, name, 2*time.Minute)
	} else {
		time.Sleep(3 * time.Second)
	}
	secret, err := s.c.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func (s *ResourceDistributionTester) UpdateSecret(secret *corev1.Secret) error {
	Logf("update secret(%s/%s)", secret.Namespace, secret.Name)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		secretClone, getErr := s.c.CoreV1().Secrets(secret.Namespace).Get(context.TODO(), secret.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		metadata := &secretClone.ObjectMeta
		secretClone = secret.DeepCopy()
		secretClone.ObjectMeta = *metadata
		_, updateErr := s.c.CoreV1().Secrets(secret.Namespace).Update(context.TODO(), secretClone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		return updateErr
	})
}

func (s *ResourceDistributionTester) DeleteSecret(namespace, name string) error {
	return s.c.CoreV1().Secrets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (s *ResourceDistributionTester) GetResourceDistribution(name string, mustExistAssertion bool) (*appsv1alpha1.ResourceDistribution, error) {
	if mustExistAssertion {
		s.WaitForResourceDistributionCreated(name, 2*time.Minute)
	} else {
		s.WaitForResourceDistributionCreated(name, time.Minute)
	}
	resourceDistribution, err := s.kc.AppsV1alpha1().ResourceDistributions().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return resourceDistribution, nil
}

func (s *ResourceDistributionTester) DeleteResourceDistributions(nsPrefix string) {
	resourceDistributionList, err := s.kc.AppsV1alpha1().ResourceDistributions().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List ResourceDistribution failed: %s", err.Error())
		return
	}

	for _, resourceDistribution := range resourceDistributionList.Items {
		if strings.HasPrefix(resourceDistribution.Name, nsPrefix) {
			s.DeleteResourceDistribution(&resourceDistribution)
		}
	}
}

func (s *ResourceDistributionTester) DeleteNamespaces(nsPrefix string) {
	namespaces, err := s.c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logf("List ResourceDistribution failed: %s", err.Error())
		return
	}

	for _, namespace := range namespaces.Items {
		if strings.HasPrefix(namespace.Name, nsPrefix) {
			Logf("delete namespace %s", namespace.Name)
			s.DeleteNamespace(&namespace)
		}
	}
}

func (s *ResourceDistributionTester) DeleteNamespace(namespace *corev1.Namespace) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := s.c.CoreV1().Namespaces().Delete(context.TODO(), namespace.Name, metav1.DeleteOptions{})
		return err
	})
	if err != nil {
		Logf("delete namespace(%s) failed: %s", namespace.Name, err.Error())
	}
	s.WaitForNamespaceDeleted(namespace)
}

func (s *ResourceDistributionTester) DeleteResourceDistribution(resourceDistribution *appsv1alpha1.ResourceDistribution) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := s.kc.AppsV1alpha1().ResourceDistributions().Delete(context.TODO(), resourceDistribution.Name, metav1.DeleteOptions{})
		return err
	})
	if err != nil {
		Logf("delete ResourceDistribution(%s) failed: %s", resourceDistribution.Name, err.Error())
	}
	s.WaitForResourceDistributionDeleted(resourceDistribution)
}

func (s *ResourceDistributionTester) WaitForResourceDistributionCreated(name string, timeout time.Duration) {
	pollErr := wait.PollImmediate(time.Second, timeout,
		func() (bool, error) {
			_, err := s.kc.AppsV1alpha1().ResourceDistributions().Get(context.TODO(), name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return true, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for ResourceDistribution to enter running: %v", pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForNamespaceCreated(namespace *corev1.Namespace) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := s.c.CoreV1().Namespaces().Get(context.TODO(), namespace.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return true, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for namespace to enter created: %v", pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForSecretCreated(namespace, name string, timeout time.Duration) {
	pollErr := wait.PollImmediate(time.Second, timeout,
		func() (bool, error) {
			_, err := s.c.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
			Logf("wait for secret(%s) err %v", namespace, err)
			if err != nil && errors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, err
			}
			return true, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for secret(%s/%s) to enter created: %v", namespace, name, pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForNamespaceDeleted(namespace *corev1.Namespace) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := s.c.CoreV1().Namespaces().Get(context.TODO(), namespace.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for namespace to enter Deleted: %v", pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForResourceDistributionDeleted(resourceDistribution *appsv1alpha1.ResourceDistribution) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := s.kc.AppsV1alpha1().ResourceDistributions().Get(context.TODO(), resourceDistribution.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for ResourceDistribution to enter Deleted: %v", pollErr)
	}
}

func (s *ResourceDistributionTester) GetNamespaceForDistributor(targets *appsv1alpha1.ResourceDistributionTargets) (matchedNamespaces, unmatchedNamespaces sets.String, err error) {
	matchedNamespaces = sets.NewString()
	unmatchedNamespaces = sets.NewString()

	namespacesList, err := s.c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	for _, namespace := range namespacesList.Items {
		unmatchedNamespaces.Insert(namespace.Name)
	}

	if targets.AllNamespaces {
		// 2. select all namespaces via targets.AllNamespace
		for _, namespace := range namespacesList.Items {
			matchedNamespaces.Insert(namespace.Name)
		}
	} else if len(targets.NamespaceLabelSelector.MatchLabels) != 0 || len(targets.NamespaceLabelSelector.MatchExpressions) != 0 {
		// 1. select the namespaces via targets.NamespaceLabelSelector
		selectors, err := util.ValidatedLabelSelectorAsSelector(&targets.NamespaceLabelSelector)
		if err != nil {
			return nil, nil, err
		}
		namespaces, err := s.c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{LabelSelector: selectors.String()})
		if err != nil {
			return nil, nil, err
		}
		for _, namespace := range namespaces.Items {
			matchedNamespaces.Insert(namespace.Name)
		}
	}

	// 3. select the namespaces via targets.IncludedNamespaces
	for _, namespace := range targets.IncludedNamespaces.List {
		matchedNamespaces.Insert(namespace.Name)
	}

	// 4. exclude the namespaces via target.ExcludedNamespaces
	for _, namespace := range targets.ExcludedNamespaces.List {
		matchedNamespaces.Delete(namespace.Name)
	}

	// 5. remove matched namespaces from unmatched namespace set
	for matched := range matchedNamespaces {
		unmatchedNamespaces.Delete(matched)
	}

	return
}
