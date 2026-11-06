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
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"
)

var (
	scheme           *runtime.Scheme
	reconcileHandler = &ReconcileResourceDistribution{}
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

func TestDoReconcile(t *testing.T) {
	distributor := buildResourceDistributionWithSecret()
	makeClientEnvironment(distributor)

	if _, err := reconcileHandler.doReconcile(distributor); err != nil {
		t.Fatalf("failed to test doReconcile, err %v", err)
	}

	// 1. check distributor.status
	mice := &appsv1beta1.ResourceDistribution{}
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

func TestDoReconcileSyncsResourceMetadata(t *testing.T) {
	// The template also sets an annotation the controller overwrites, which must
	// not count as a difference on later reconciles.
	newRaw := func(env, note string) runtime.RawExtension {
		return runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{
			"apiVersion": "v1",
			"data": {"test": "test"},
			"kind": "Secret",
			"metadata": {
				"name": "test-secret-1",
				"labels": {"env": %q},
				"annotations": {"note": %q, %q: "from-template"}
			},
			"type": "Opaque"
		}`, env, note, utils.SourceResourceDistributionOfResource))}
	}
	reconcileAndGet := func(distributor *appsv1beta1.ResourceDistribution) *corev1.Secret {
		t.Helper()
		if _, err := reconcileHandler.doReconcile(distributor); err != nil {
			t.Fatalf("failed to test doReconcile, err %v", err)
		}
		secret := &corev1.Secret{}
		if err := reconcileHandler.Client.Get(context.TODO(), types.NamespacedName{Namespace: "ns-1", Name: "test-secret-1"}, secret); err != nil {
			t.Fatalf("failed to get resource from fake client, err %v", err)
		}
		return secret
	}

	distributor := buildResourceDistribution(newRaw("prod", "first"))
	makeClientEnvironment(distributor)

	secret := reconcileAndGet(distributor)
	if secret.Labels["env"] != "prod" {
		t.Fatalf("expected label env=prod after the first reconcile, got %v", secret.Labels)
	}

	// metadata set by someone else on the distributed resource must survive
	secret.Labels["other"] = "kept"
	if err := reconcileHandler.Client.Update(context.TODO(), secret); err != nil {
		t.Fatalf("failed to update resource, err %v", err)
	}

	// change only the metadata of the template
	distributor.Spec.Resource = newRaw("staging", "second")
	secret = reconcileAndGet(distributor)
	if secret.Labels["env"] != "staging" || secret.Annotations["note"] != "second" {
		t.Fatalf("expected the new template metadata, got labels %v annotations %v", secret.Labels, secret.Annotations)
	}
	if secret.Labels["other"] != "kept" {
		t.Fatalf("expected label other=kept to survive, got %v", secret.Labels)
	}
	if got, want := secret.Annotations[utils.ResourceHashCodeAnnotation], hashResource(distributor.Spec.Resource); got != want {
		t.Fatalf("expected hash annotation %s, got %s", want, got)
	}
	if got := secret.Annotations[utils.SourceResourceDistributionOfResource]; got != distributor.Name {
		t.Fatalf("expected source annotation %s, got %s", distributor.Name, got)
	}

	// nothing changed, so another reconcile must not write the resource again
	if got := reconcileAndGet(distributor); got.ResourceVersion != secret.ResourceVersion {
		t.Fatalf("expected no update without a change, resourceVersion went from %s to %s", secret.ResourceVersion, got.ResourceVersion)
	}
}

func buildResourceDistributionWithSecret() *appsv1beta1.ResourceDistribution {
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

func buildResourceDistribution(raw runtime.RawExtension) *appsv1beta1.ResourceDistribution {
	return &appsv1beta1.ResourceDistribution{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1beta1.GroupVersion.String(),
			Kind:       "ResourceDistribution",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-resource-distribution",
		},
		Spec: appsv1beta1.ResourceDistributionSpec{
			Resource: raw,
			Targets: appsv1beta1.ResourceDistributionTargets{
				ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{
						{
							Name: "ns-4",
						},
					},
				},
				IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{
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
				NamespaceSelector: metav1.LabelSelector{
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
		WithStatusSubresource(&appsv1beta1.ResourceDistribution{}).Build()
	reconcileHandler.Client = fakeClient
}
