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

package validating

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var (
	testScheme *runtime.Scheme
	handler    = &ResourceDistributionCreateUpdateHandler{}
)

func init() {
	testScheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(testScheme))
	utilruntime.Must(corev1.AddToScheme(testScheme))
}

func TestResourceDistributionCreateValidation(t *testing.T) {
	// build rd objects
	rdSecret := buildResourceDistributionWithSecret()
	rdConfigMap := buildResourceDistributionWithConfigMap()

	makeEnvironment()

	// test successful cases of secret and configmap distributor
	errs := handler.validateResourceDistribution(rdSecret, nil)
	errs = append(errs, handler.validateResourceDistribution(rdConfigMap, nil)...)
	if len(errs) != 0 {
		t.Fatalf("failed to validate the creating case, err: %v", errs)
	}
}

func TestResourceDistributionCreateValidationWithEmptyResource(t *testing.T) {
	// build rd object with empty
	rdSecret := buildResourceDistributionWithSecret()
	rdSecret.Spec.Resource = runtime.RawExtension{}

	makeEnvironment()

	// test failed case with empty resource
	errs := handler.validateResourceDistribution(rdSecret, nil)
	if len(errs) != 1 {
		t.Fatalf("failed to validate the empty resource case, err: %v", errs)
	}
}

func TestResourceDistributionCreateValidationWithWrongResource(t *testing.T) {
	// build rd object with empty
	rdSecret := buildResourceDistributionWithSecret()
	rdSecret.Spec.Resource = runtime.RawExtension{
		Raw: []byte("something is wrong....!#@$#@$%@^^&#%$&"),
	}

	makeEnvironment()

	// test failed case with empty resource
	errs := handler.validateResourceDistribution(rdSecret, nil)
	if len(errs) != 1 {
		t.Fatalf("failed to validate the wrong resource case, err: %v", errs)
	}
}

func TestResourceDistributionUpdateValidation(t *testing.T) {
	// build rd objects
	oldRD := buildResourceDistributionWithSecret()
	newRD := oldRD.DeepCopy()
	newRD.Spec.Targets.IncludedNamespaces.List = append(newRD.Spec.Targets.IncludedNamespaces.List, appsv1alpha1.ResourceDistributionNamespace{Name: "ns-4"})

	makeEnvironment()

	// test successful updating case
	errs := handler.validateResourceDistribution(newRD, oldRD)
	if len(errs) != 0 {
		t.Fatalf("failed to validate the updating case, err: %v", errs)
	}
}

func TestValidateResourceDistributionTargets(t *testing.T) {
	targets := &appsv1alpha1.ResourceDistributionTargets{
		// error 1
		ExcludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
			List: []appsv1alpha1.ResourceDistributionNamespace{
				{Name: "ns-1"},
			},
		},
		// error 2
		IncludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
			List: []appsv1alpha1.ResourceDistributionNamespace{
				{Name: "ns-1"}, {Name: "ns-2"}, {Name: "kube-system"}, {Name: ""},
			},
		},
		// error 3
		NamespaceLabelSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"$#%$%": "#@$@#$",
			},
		},
	}
	errs := handler.validateResourceDistributionSpecTargets(targets, field.NewPath("targets"))
	if len(errs) != 3 {
		t.Fatalf("failed to validate the spec.targets, err: %v", errs)
	}
}

func TestResourceDistributionUpdateConflict(t *testing.T) {
	// resource details
	const changedResourceJSON = `{
		"apiVersion": "v1",
		"data": {
			"test": "MWYyZDFlMmU2N2Rm"
		},
		"kind": "Secret",
		"metadata": {
			"name": "test-secret-2"
		},
		"type": "Opaque"
	}`
	// env details
	conflictingResource := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret-1",
			Namespace: "ns-2",
		},
	}

	// build rd objects
	oldRD := buildResourceDistributionWithSecret()
	newRD := oldRD.DeepCopy()
	newRD.Spec.Resource.Raw = []byte(changedResourceJSON)

	makeEnvironment(oldRD, conflictingResource)

	// test failed case under the conflict between new and old resources
	errs := handler.validateResourceDistribution(newRD, oldRD)
	if len(errs) != 1 {
		t.Fatalf("failed to validate the conflict of updating case, err: %v", errs)
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
	return buildResourceDistribution(resourceJSON)
}

func buildResourceDistributionWithConfigMap() *appsv1alpha1.ResourceDistribution {
	const resourceJSON = `{
		"apiVersion": "v1",
		"data": {
			"game.properties": "enemy.types=aliens,monsters\nplayer.maximum-lives=5\n",
			"player_initial_lives": "3",
			"ui_properties_file_name": "user-interface.properties",
			"user-interface.properties": "color.good=purple\ncolor.bad=yellow\nallow.textmode=true\n"
		},
		"kind": "ConfigMap",
		"metadata": {
			"name": "game-demo"
		}
	}`
	return buildResourceDistribution(resourceJSON)
}

func buildResourceDistribution(resourceYaml string) *appsv1alpha1.ResourceDistribution {
	return &appsv1alpha1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "test-resource-distribution"},
		Spec: appsv1alpha1.ResourceDistributionSpec{
			Resource: runtime.RawExtension{
				Raw: []byte(resourceYaml),
			},
			Targets: appsv1alpha1.ResourceDistributionTargets{
				ExcludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
					List: []appsv1alpha1.ResourceDistributionNamespace{
						{
							Name: "ns-3",
						},
					},
				},
				IncludedNamespaces: appsv1alpha1.ResourceDistributionTargetNamespaces{
					List: []appsv1alpha1.ResourceDistributionNamespace{
						{
							Name: "ns-1",
						},
						{
							Name: "ns-2",
						},
					},
				},
				NamespaceLabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"environment": "develop",
					},
				},
			},
		},
	}
}

func makeEnvironment(addition ...client.Object) {
	env := []client.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ns-1",
				Labels: map[string]string{
					"group":       "one",
					"environment": "develop",
				},
			},
		},
		&corev1.Namespace{
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
					"group":       "two",
					"environment": "test",
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret-1",
				Namespace: "ns-1",
				Annotations: map[string]string{
					SourceResourceDistributionOfResource: "test-resource-distribution",
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret-2",
				Namespace: "ns-3",
			},
		},
	}
	env = append(env, addition...)
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(env...).Build()
	handler.Client = fakeClient
}
