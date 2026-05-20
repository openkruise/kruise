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
	"strings"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework/common"
)

// ResourceDistributionTester is a helper for ResourceDistribution v1beta1 E2E tests.
type ResourceDistributionTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
}

func NewResourceDistributionTester(c clientset.Interface, kc kruiseclientset.Interface) *ResourceDistributionTester {
	return &ResourceDistributionTester{c: c, kc: kc}
}

const baseResourceJSON = `{
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

// NewBaseResourceDistribution returns a canonical v1beta1 ResourceDistribution for testing.
func (s *ResourceDistributionTester) NewBaseResourceDistribution(nsPrefix string) *appsv1beta1.ResourceDistribution {
	return &appsv1beta1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: nsPrefix},
		Spec: appsv1beta1.ResourceDistributionSpec{
			Resource: runtime.RawExtension{Raw: []byte(baseResourceJSON)},
			Targets: appsv1beta1.ResourceDistributionTargets{
				ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{
						{Name: nsPrefix + "-4"},
					},
				},
				IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{
						{Name: nsPrefix + "-1"},
						{Name: nsPrefix + "-2"},
						{Name: nsPrefix + "-3"},
						{Name: nsPrefix + "-5"},
					},
				},
				NamespaceSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"e2e-rd-group": "one"},
				},
			},
		},
	}
}

func (s *ResourceDistributionTester) NewBaseNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func (s *ResourceDistributionTester) CreateResourceDistribution(rd *appsv1beta1.ResourceDistribution) *appsv1beta1.ResourceDistribution {
	common.Logf("create ResourceDistribution(%s)", rd.Name)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		_, err := s.kc.AppsV1beta1().ResourceDistributions().Create(context.TODO(), rd, metav1.CreateOptions{})
		return err
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.WaitForResourceDistributionCreated(rd.Name, time.Minute)
	rd, _ = s.kc.AppsV1beta1().ResourceDistributions().Get(context.TODO(), rd.Name, metav1.GetOptions{})
	return rd
}

func (s *ResourceDistributionTester) UpdateResourceDistribution(rd *appsv1beta1.ResourceDistribution) {
	common.Logf("update ResourceDistribution(%s)", rd.Name)
	clone, _ := s.kc.AppsV1beta1().ResourceDistributions().Get(context.TODO(), rd.Name, metav1.GetOptions{})
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone.Spec = rd.Spec
		clone.Annotations = rd.Annotations
		clone.Labels = rd.Labels
		_, updateErr := s.kc.AppsV1beta1().ResourceDistributions().Update(context.TODO(), clone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		clone, _ = s.kc.AppsV1beta1().ResourceDistributions().Get(context.TODO(), clone.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *ResourceDistributionTester) UpdateNamespace(ns *corev1.Namespace) {
	common.Logf("update namespace(%s)", ns.Name)
	clone := ns.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone.Annotations = ns.Annotations
		clone.Labels = ns.Labels
		clone.Spec = ns.Spec
		_, updateErr := s.c.CoreV1().Namespaces().Update(context.TODO(), clone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		clone, _ = s.c.CoreV1().Namespaces().Get(context.TODO(), ns.Name, metav1.GetOptions{})
		return updateErr
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (s *ResourceDistributionTester) CreateNamespaces(namespaces ...*corev1.Namespace) {
	for _, ns := range namespaces {
		if _, err := s.c.CoreV1().Namespaces().Get(context.TODO(), ns.Name, metav1.GetOptions{}); err == nil || !errors.IsNotFound(err) {
			continue
		}
		common.Logf("create namespace(%s)", ns.Name)
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			_, err := s.c.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			return err
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		s.WaitForNamespaceCreated(ns)
	}
}

func (s *ResourceDistributionTester) GetSecret(namespace, name string, mustExist bool) (*corev1.Secret, error) {
	if mustExist {
		s.WaitForSecretCreated(namespace, name, 2*time.Minute)
	} else {
		time.Sleep(3 * time.Second)
	}
	return s.PollSecret(namespace, name)
}

// PollSecret returns the current secret without blocking; for use inside gomega.Eventually.
func (s *ResourceDistributionTester) PollSecret(namespace, name string) (*corev1.Secret, error) {
	secret, err := s.c.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func (s *ResourceDistributionTester) GetResourceDistribution(name string, mustExist bool) (*appsv1beta1.ResourceDistribution, error) {
	timeout := time.Minute
	if mustExist {
		timeout = 2 * time.Minute
	}
	s.WaitForResourceDistributionCreated(name, timeout)
	rd, err := s.kc.AppsV1beta1().ResourceDistributions().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return rd, nil
}

func (s *ResourceDistributionTester) DeleteResourceDistributions(nsPrefix string) {
	list, err := s.kc.AppsV1beta1().ResourceDistributions().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		common.Logf("List ResourceDistribution failed: %s", err.Error())
		return
	}
	for i := range list.Items {
		if strings.HasPrefix(list.Items[i].Name, nsPrefix) {
			s.DeleteResourceDistribution(&list.Items[i])
		}
	}
}

func (s *ResourceDistributionTester) DeleteNamespaces(nsPrefix string) {
	namespaces, err := s.c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		common.Logf("List namespaces failed: %s", err.Error())
		return
	}
	for i := range namespaces.Items {
		if strings.HasPrefix(namespaces.Items[i].Name, nsPrefix) {
			common.Logf("delete namespace %s", namespaces.Items[i].Name)
			s.DeleteNamespace(&namespaces.Items[i])
		}
	}
}

func (s *ResourceDistributionTester) DeleteNamespace(ns *corev1.Namespace) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return s.c.CoreV1().Namespaces().Delete(context.TODO(), ns.Name, metav1.DeleteOptions{})
	})
	if err != nil {
		common.Logf("delete namespace(%s) failed: %s", ns.Name, err.Error())
	}
	s.WaitForNamespaceDeleted(ns)
}

func (s *ResourceDistributionTester) DeleteResourceDistribution(rd *appsv1beta1.ResourceDistribution) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return s.kc.AppsV1beta1().ResourceDistributions().Delete(context.TODO(), rd.Name, metav1.DeleteOptions{})
	})
	if err != nil {
		common.Logf("delete ResourceDistribution(%s) failed: %s", rd.Name, err.Error())
	}
	s.WaitForResourceDistributionDeleted(rd)
}

func (s *ResourceDistributionTester) WaitForResourceDistributionCreated(name string, timeout time.Duration) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, timeout, false,
		func(ctx context.Context) (bool, error) {
			_, err := s.kc.AppsV1beta1().ResourceDistributions().Get(ctx, name, metav1.GetOptions{})
			return err == nil, nil
		})
	if pollErr != nil {
		common.Failf("Failed waiting for ResourceDistribution to be created: %v", pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForNamespaceCreated(ns *corev1.Namespace) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, time.Minute, false,
		func(ctx context.Context) (bool, error) {
			_, err := s.c.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
			return err == nil, nil
		})
	if pollErr != nil {
		common.Failf("Failed waiting for namespace %s to be created: %v", ns.Name, pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForSecretCreated(namespace, name string, timeout time.Duration) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, timeout, false,
		func(ctx context.Context) (bool, error) {
			_, err := s.c.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
			common.Logf("wait for secret(%s/%s) err %v", namespace, name, err)
			if errors.IsNotFound(err) {
				return false, nil
			}
			return err == nil, err
		})
	if pollErr != nil {
		common.Failf("Failed waiting for secret(%s/%s) to be created: %v", namespace, name, pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForSecretDeleted(namespace, name string, timeout time.Duration) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, timeout, false,
		func(ctx context.Context) (bool, error) {
			_, err := s.c.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})
	if pollErr != nil {
		common.Failf("Failed waiting for secret(%s/%s) to be deleted: %v", namespace, name, pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForNamespaceDeleted(ns *corev1.Namespace) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, time.Minute, false,
		func(ctx context.Context) (bool, error) {
			_, err := s.c.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})
	if pollErr != nil {
		common.Failf("Failed waiting for namespace %s to be deleted: %v", ns.Name, pollErr)
	}
}

func (s *ResourceDistributionTester) WaitForResourceDistributionDeleted(rd *appsv1beta1.ResourceDistribution) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, time.Minute, false,
		func(ctx context.Context) (bool, error) {
			_, err := s.kc.AppsV1beta1().ResourceDistributions().Get(ctx, rd.Name, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})
	if pollErr != nil {
		common.Failf("Failed waiting for ResourceDistribution %s to be deleted: %v", rd.Name, pollErr)
	}
}

func (s *ResourceDistributionTester) UpdateSecret(secret *corev1.Secret) error {
	common.Logf("update secret(%s/%s)", secret.Namespace, secret.Name)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := s.c.CoreV1().Secrets(secret.Namespace).Get(context.TODO(), secret.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		meta := clone.ObjectMeta
		clone = secret.DeepCopy()
		clone.ObjectMeta = meta
		_, err = s.c.CoreV1().Secrets(secret.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
		return err
	})
}

func (s *ResourceDistributionTester) DeleteSecret(namespace, name string) error {
	return s.c.CoreV1().Secrets(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

// GetNamespaceForDistributor returns the namespaces matched/unmatched by the given targets.
func (s *ResourceDistributionTester) GetNamespaceForDistributor(targets *appsv1beta1.ResourceDistributionTargets) (matched, unmatched sets.String, err error) {
	matched = sets.NewString()
	unmatched = sets.NewString()

	nsList, err := s.c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	for _, ns := range nsList.Items {
		unmatched.Insert(ns.Name)
	}

	if targets.AllNamespaces {
		for _, ns := range nsList.Items {
			if ns.Name == "kube-system" || ns.Name == "kube-public" {
				continue
			}
			matched.Insert(ns.Name)
		}
	} else {
		sel := targets.NamespaceSelector
		if len(sel.MatchLabels) != 0 || len(sel.MatchExpressions) != 0 {
			selector, err := util.ValidatedLabelSelectorAsSelector(&sel)
			if err != nil {
				return nil, nil, err
			}
			nsByLabel, err := s.c.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
			if err != nil {
				return nil, nil, err
			}
			for _, ns := range nsByLabel.Items {
				matched.Insert(ns.Name)
			}
		}
	}

	for _, ns := range targets.IncludedNamespaces.List {
		matched.Insert(ns.Name)
	}
	for _, ns := range targets.ExcludedNamespaces.List {
		matched.Delete(ns.Name)
	}
	for m := range matched {
		unmatched.Delete(m)
	}
	return
}

// WaitForDistributionStatus polls until the RD status reflects the expected
// Desired and Succeeded counts.
func (s *ResourceDistributionTester) WaitForDistributionStatus(name string, desired, succeeded int32, timeout time.Duration) {
	pollErr := wait.PollUntilContextTimeout(context.TODO(), time.Second, timeout, false,
		func(ctx context.Context) (bool, error) {
			rd, err := s.kc.AppsV1beta1().ResourceDistributions().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			common.Logf("ResourceDistribution %s status: desired=%d succeeded=%d (want desired=%d succeeded=%d)",
				name, rd.Status.Desired, rd.Status.Succeeded, desired, succeeded)
			return rd.Status.Desired == desired && rd.Status.Succeeded == succeeded, nil
		})
	if pollErr != nil {
		common.Failf("ResourceDistribution %s did not reach desired=%d succeeded=%d: %v", name, desired, succeeded, pollErr)
	}
}
