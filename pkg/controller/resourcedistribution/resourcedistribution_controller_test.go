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

	// init
	targets, _, _ := prepareNamespaces(reconcileHandler.Client, distributor)
	resourceVersion := hashResource(distributor.Spec.Resource)

	// check for resource version after reconciling
	for namespace := range targets {
		resource := &corev1.Secret{}
		// check whether resource exists
		if err := reconcileHandler.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: "test-secret-1"}, resource); err != nil {
			t.Fatalf("failed to getting resource(%s.%s) from fake client, err %v", namespace, "test-secret-1", err)
		}
		// check resource source and version
		if distributor.Name != resource.Annotations[utils.SourceResourceDistributionOfResource] ||
			resourceVersion != resource.Annotations[utils.ResourceHashCodeAnnotation] {
			t.Fatalf("failed to sync resource(%s) for namespace (%s) from %s, \nstdVersion:%x\nnowVersion:%x", resource.Name, namespace, resource.Annotations[utils.SourceResourceDistributionOfResource], resourceVersion, resource.Annotations[utils.ResourceHashCodeAnnotation])
		}
	}

	// check for deleting resource
	resource := &corev1.Secret{}
	if err := reconcileHandler.Client.Get(context.TODO(), types.NamespacedName{Namespace: "ns-5", Name: "test-secret-1"}, resource); err == nil || !errors.IsNotFound(err) {
		t.Fatalf("failed to delete resource from fake client, err %v", err)
	}
}

func TestGetNamespaces(t *testing.T) {
	distributor := buildResourceDistributionWithSecret()
	makeClientEnvironment(distributor)

	isInTargets, allNamespaces, err := prepareNamespaces(reconcileHandler.Client, distributor)
	if err != nil {
		t.Fatalf("failed to test allNamespaces function, err %v", err)
	}
	if len(isInTargets) != 3 {
		t.Fatalf("the number of expected target namespace is %d, but got %d", 3, len(isInTargets))
	}
	if len(allNamespaces) != 5 {
		t.Fatalf("the number of expected all namespace is %d, but got %d", 5, len(allNamespaces))
	}
}

func buildResourceDistributionWithSecret() *appsv1alpha1.ResourceDistribution {
	const resourceJson = `{
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
	raw := runtime.RawExtension{Raw: []byte(resourceJson)}
	return buildResourceDistribution(raw)
}

func buildResourceDistribution(raw runtime.RawExtension) *appsv1alpha1.ResourceDistribution {
	return &appsv1alpha1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "test-resource-distribution"},
		Spec: appsv1alpha1.ResourceDistributionSpec{
			Resource: raw,
			Targets: appsv1alpha1.ResourceDistributionTargets{
				ExcludedNamespaces: []appsv1alpha1.ResourceDistributionNamespace{
					{
						Name: "ns-4",
					},
				},
				IncludedNamespaces: []appsv1alpha1.ResourceDistributionNamespace{
					{
						Name: "ns-1",
					},
					{
						Name: "ns-2",
					},
					{
						Name: "ns-3",
					},
					{
						Name: "ns-5",
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
					"group":       "one",
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
				},
			},
		},
		&corev1.Secret{ // for delete
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret-1",
				Namespace: "ns-5",
				Annotations: map[string]string{
					utils.SourceResourceDistributionOfResource: "test-resource-distribution",
				},
			},
		},
	}
}

func makeClientEnvironment(addition ...runtime.Object) {
	env := append(makeEnvironment(), addition...)
	fakeClient := fake.NewFakeClientWithScheme(scheme, env...)
	reconcileHandler.Client = fakeClient
}
