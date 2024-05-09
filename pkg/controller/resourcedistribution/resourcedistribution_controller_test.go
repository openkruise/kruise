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

package resourcedistribution

import (
	"context"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	scheme           *runtime.Scheme
	reconcileHandler = &ReconcileResourceDistribution{}
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

func TestDoReconcile(t *testing.T) {
	distributor := buildResourceDistributionWithSecret()
	makeClientEnvironment(distributor)

	if _, err := reconcileHandler.doReconcile(distributor); err != nil {
		t.Fatalf("failed to test doReconcile, err %v", err)
	}

	// 1. check distributor.status
	mice := &appsv1alpha1.ResourceDistribution{}
	reconcileHandler.Client.Get(context.TODO(), types.NamespacedName{Name: distributor.Name}, mice)
	if len(mice.Status.Conditions) < NumberOfConditionTypes {
		t.Fatalf("unexpected .status.conditions size, expected %d actual %d", NumberOfConditionTypes, len(mice.Status.Conditions))
	}

	// 2. check listNamespacesForDistributor function
	matched, unmatched, err := listNamespacesForDistributor(reconcileHandler.Client, &distributor.Spec.Targets)
	if err != nil {
		t.Fatalf("failed to list namespaces for distributor, err %v", err)
	}
	if len(matched) != 4 {
		t.Fatalf("expected number of matched namespaces is %v, while the actual is %v\n", 4, len(matched))
	}
	if len(unmatched) != 1 {
		t.Fatalf("expected number of unmatched namespaces is %v, while the actual is %v\n", 1, len(unmatched))
	}

	// 3. check resource version after reconciling
	resourceVersion := hashResource(distributor.Spec.Resource)
	for _, namespace := range matched {
		resource := &corev1.Secret{}
		// check whether resource exists
		if err := reconcileHandler.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: "test-secret-1"}, resource); err != nil {
			t.Fatalf("failed to get resource(%s/%s) from fake client, err %v", namespace, "test-secret-1", err)
		}
		// check resource source and version
		if !isControlledByDistributor(resource, distributor) {
			t.Fatalf("failed to sync resource(%s) for namespace (%s), owner set error", resource.Name, namespace)
		}
		if resourceVersion != resource.Annotations[utils.ResourceHashCodeAnnotation] {
			t.Fatalf("failed to sync resource(%s) for namespace (%s) from %s, \nexpected version:%x \nactual version:%x", resource.Name, namespace, resource.Annotations[utils.SourceResourceDistributionOfResource], resourceVersion, resource.Annotations[utils.ResourceHashCodeAnnotation])
		}
	}

	// 4. check resource deletion for unmatched namespaces after reconciling
	for _, namespace := range unmatched {
		resource := &corev1.Secret{}
		if err := reconcileHandler.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: "test-secret-1"}, resource); err == nil || !errors.IsNotFound(err) {
			t.Fatalf("failed to delete resource from fake client, err %v", err)
		}
	}
}

func buildResourceDistributionWithSecret() *appsv1alpha1.ResourceDistribution {
	const resourceJSON = `{
		"apiVersion": "v1",
		"data": {
			"test": "test"
		},
		"kind": "Secret",
		"metadata": {
			"name": "test-secret-1"
		},
		"type": "Opaque"
	}`
	raw := runtime.RawExtension{Raw: []byte(resourceJSON)}
	return buildResourceDistribution(raw)
}

func buildResourceDistribution(raw runtime.RawExtension) *appsv1alpha1.ResourceDistribution {
	return &appsv1alpha1.ResourceDistribution{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1alpha1.GroupVersion.String(),
			Kind:       "ResourceDistribution",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-resource-distribution",
		},
		Spec: appsv1alpha1.ResourceDistributionSpec{
			Resource: raw,
			Targets: appsv1alpha1.ResourceDistributionTargets{
				ExcludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
					List: []appsv1alpha1.ResourceDistributionNamespace{
						{
							Name: "ns-4",
						},
					},
				},
				IncludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
					List: []appsv1alpha1.ResourceDistributionNamespace{
						{
							Name: "ns-1",
						},
						{
							Name: "ns-3",
						},
						{
							Name: "ns-5",
						},
					},
				},
				NamespaceLabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"group": "one",
					},
				},
			},
		},
	}
}

func makeEnvironment() []runtime.Object {
	distributor := buildResourceDistributionWithSecret()
	return []runtime.Object{
		&corev1.Namespace{ // for create
			ObjectMeta: metav1.ObjectMeta{
				Name: "ns-1",
				Labels: map[string]string{
					"group":       "one",
					"environment": "develop",
				},
			},
		},
		&corev1.Namespace{ // for create
			ObjectMeta: metav1.ObjectMeta{
				Name: "ns-2",
				Labels: map[string]string{
					"group":       "one",
					"environment": "develop",
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ns-3",
				Labels: map[string]string{
					"group":       "two",
					"environment": "develop",
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ns-4",
				Labels: map[string]string{
					"group":       "one",
					"environment": "test",
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ns-5",
				Labels: map[string]string{
					"group":       "two",
					"environment": "test",
				},
			},
		},
		&corev1.Secret{ // for update
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret-1",
				Namespace: "ns-1",
				Annotations: map[string]string{
					utils.SourceResourceDistributionOfResource: "test-resource-distribution",
					utils.ResourceHashCodeAnnotation:           hashResource(distributor.Spec.Resource),
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(distributor, distributor.GroupVersionKind()),
				},
			},
		},
		&corev1.Secret{ // for delete
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret-1",
				Namespace: "ns-4",
				Annotations: map[string]string{
					utils.SourceResourceDistributionOfResource: "test-resource-distribution",
					utils.ResourceHashCodeAnnotation:           hashResource(distributor.Spec.Resource),
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(distributor, distributor.GroupVersionKind()),
				},
			},
		},
	}
}

func makeClientEnvironment(addition ...runtime.Object) {
	env := append(makeEnvironment(), addition...)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(env...).
		WithStatusSubresource(&appsv1alpha1.ResourceDistribution{}).Build()
	reconcileHandler.Client = fakeClient
}
