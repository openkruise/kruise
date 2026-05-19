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

package validating

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

var (
	testScheme *runtime.Scheme
	handler    = &ResourceDistributionCreateUpdateHandler{}
)

func init() {
	testScheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(testScheme))
	utilruntime.Must(appsv1beta1.AddToScheme(testScheme))
	utilruntime.Must(corev1.AddToScheme(testScheme))
}

func TestResourceDistributionCreateValidation(t *testing.T) {
	rdSecret := buildResourceDistributionWithSecret()
	rdConfigMap := buildResourceDistributionWithConfigMap()

	makeEnvironment()

	errs := handler.validateResourceDistribution(rdSecret, nil)
	errs = append(errs, handler.validateResourceDistribution(rdConfigMap, nil)...)
	if len(errs) != 0 {
		t.Fatalf("failed to validate the creating case, err: %v", errs)
	}
}

func TestResourceDistributionCreateValidationWithEmptyResource(t *testing.T) {
	rd := buildResourceDistributionWithSecret()
	rd.Spec.Resource = runtime.RawExtension{}

	makeEnvironment()

	errs := handler.validateResourceDistribution(rd, nil)
	if len(errs) != 1 {
		t.Fatalf("failed to validate the empty resource case, err: %v", errs)
	}
}

func TestResourceDistributionCreateValidationWithWrongResource(t *testing.T) {
	rd := buildResourceDistributionWithSecret()
	rd.Spec.Resource = runtime.RawExtension{
		Raw: []byte("something is wrong....!#@$#@$%@^^&#%$&"),
	}

	makeEnvironment()

	errs := handler.validateResourceDistribution(rd, nil)
	if len(errs) != 1 {
		t.Fatalf("failed to validate the wrong resource case, err: %v", errs)
	}
}

func TestResourceDistributionUpdateValidation(t *testing.T) {
	oldRD := buildResourceDistributionWithSecret()
	newRD := oldRD.DeepCopy()
	newRD.Spec.Targets.IncludedNamespaces.List = append(newRD.Spec.Targets.IncludedNamespaces.List,
		appsv1beta1.ResourceDistributionNamespace{Name: "ns-4"})

	makeEnvironment()

	errs := handler.validateResourceDistribution(newRD, oldRD)
	if len(errs) != 0 {
		t.Fatalf("failed to validate the updating case, err: %v", errs)
	}
}

func TestValidateResourceDistributionSpecSuccessCases(t *testing.T) {
	makeEnvironment()

	successCases := map[string]*appsv1beta1.ResourceDistribution{
		"includedNamespaces only": buildResourceDistribution(`{
			"apiVersion":"v1","kind":"Secret",
			"metadata":{"name":"s1"},"type":"Opaque","data":{"k":"dg=="}
		}`),
		"allNamespaces flag": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistribution(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1"},"data":{}}`)
			rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{AllNamespaces: true}
			return rd
		}(),
		"namespaceSelector only": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistribution(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm2"},"data":{}}`)
			rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{
				NamespaceSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
			}
			return rd
		}(),
		"includedNamespaces with excludedNamespaces": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistribution(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm3"},"data":{}}`)
			rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{
				IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-1"}, {Name: "ns-2"}},
				},
				ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-3"}},
				},
			}
			return rd
		}(),
		"allNamespaces with excludedNamespaces": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistribution(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm4"},"data":{}}`)
			rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{
				AllNamespaces: true,
				ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-3"}},
				},
			}
			return rd
		}(),
		"allNamespaces with namespaceSelector": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistribution(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm5"},"data":{}}`)
			rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{
				AllNamespaces: true,
				NamespaceSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "dev"},
				},
			}
			return rd
		}(),
		"empty targets (no error — same as v1alpha1)": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistribution(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm6"},"data":{}}`)
			rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{}
			return rd
		}(),
		"metadata.namespace in resource is ignored": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistribution(`{
				"apiVersion":"v1","kind":"ConfigMap",
				"metadata":{"name":"cm7","namespace":"should-be-ignored"},
				"data":{}
			}`)
			return rd
		}(),
	}

	for name, rd := range successCases {
		t.Run(name, func(t *testing.T) {
			errs := handler.validateResourceDistribution(rd, nil)
			if len(errs) != 0 {
				t.Errorf("expected success, got: %v", errs)
			}
		})
	}
}

func TestValidateResourceDistributionSpecErrorCases(t *testing.T) {
	makeEnvironment()

	errorCases := map[string]*appsv1beta1.ResourceDistribution{
		"empty resource": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistributionWithSecret()
			rd.Spec.Resource = runtime.RawExtension{}
			return rd
		}(),
		"garbage resource": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistributionWithSecret()
			rd.Spec.Resource = runtime.RawExtension{Raw: []byte("not json!!")}
			return rd
		}(),
		"invalid namespace name in includedNamespaces": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistributionWithSecret()
			rd.Spec.Targets.IncludedNamespaces.List = []appsv1beta1.ResourceDistributionNamespace{{Name: "INVALID"}}
			return rd
		}(),
		"empty namespace name in includedNamespaces": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistributionWithSecret()
			rd.Spec.Targets.IncludedNamespaces.List = []appsv1beta1.ResourceDistributionNamespace{{Name: ""}}
			return rd
		}(),
		"invalid namespace name in excludedNamespaces": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistributionWithSecret()
			rd.Spec.Targets.ExcludedNamespaces.List = []appsv1beta1.ResourceDistributionNamespace{{Name: "INVALID"}}
			return rd
		}(),
		"same namespace in included and excluded": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistributionWithSecret()
			rd.Spec.Targets.IncludedNamespaces.List = []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-1"}}
			rd.Spec.Targets.ExcludedNamespaces.List = []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-1"}}
			return rd
		}(),
		"invalid namespaceSelector label key": func() *appsv1beta1.ResourceDistribution {
			rd := buildResourceDistributionWithSecret()
			rd.Spec.Targets.NamespaceSelector = metav1.LabelSelector{
				MatchLabels: map[string]string{"$#%invalid": "value"},
			}
			return rd
		}(),
	}

	for name, rd := range errorCases {
		t.Run(name, func(t *testing.T) {
			errs := handler.validateResourceDistribution(rd, nil)
			if len(errs) == 0 {
				t.Errorf("expected failure for %q but got none", name)
			}
		})
	}
}

// TestValidateResourceDistributionUpdateSuccessCases tests update-specific paths.
func TestValidateResourceDistributionUpdateSuccessCases(t *testing.T) {
	makeEnvironment()

	successCases := map[string]struct{ oldRD, newRD *appsv1beta1.ResourceDistribution }{
		"add a namespace to includedNamespaces": {
			oldRD: buildResourceDistributionWithSecret(),
			newRD: func() *appsv1beta1.ResourceDistribution {
				rd := buildResourceDistributionWithSecret()
				rd.Spec.Targets.IncludedNamespaces.List = append(
					rd.Spec.Targets.IncludedNamespaces.List,
					appsv1beta1.ResourceDistributionNamespace{Name: "ns-4"},
				)
				return rd
			}(),
		},
		"switch from includedNamespaces to allNamespaces": {
			oldRD: buildResourceDistributionWithSecret(),
			newRD: func() *appsv1beta1.ResourceDistribution {
				rd := buildResourceDistributionWithSecret()
				rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{AllNamespaces: true}
				return rd
			}(),
		},
		"switch from includedNamespaces to namespaceSelector": {
			oldRD: buildResourceDistributionWithSecret(),
			newRD: func() *appsv1beta1.ResourceDistribution {
				rd := buildResourceDistributionWithSecret()
				rd.Spec.Targets = appsv1beta1.ResourceDistributionTargets{
					NamespaceSelector: metav1.LabelSelector{MatchLabels: map[string]string{"env": "dev"}},
				}
				return rd
			}(),
		},
	}

	for name, tc := range successCases {
		t.Run(name, func(t *testing.T) {
			errs := handler.validateResourceDistribution(tc.newRD, tc.oldRD)
			if len(errs) != 0 {
				t.Errorf("expected success, got: %v", errs)
			}
		})
	}
}

// TestValidateResourceDistributionUpdateErrorCases tests update-specific rejections.
func TestValidateResourceDistributionUpdateErrorCases(t *testing.T) {
	makeEnvironment()

	oldRD := buildResourceDistributionWithSecret()

	errorCases := map[string]*appsv1beta1.ResourceDistribution{
		"change resource name (immutable)": func() *appsv1beta1.ResourceDistribution {
			rd := oldRD.DeepCopy()
			rd.Spec.Resource = runtime.RawExtension{Raw: []byte(`{
				"apiVersion":"v1","kind":"Secret",
				"metadata":{"name":"changed-name"},"type":"Opaque","data":{}
			}`)}
			return rd
		}(),
		"change resource kind (immutable)": func() *appsv1beta1.ResourceDistribution {
			rd := oldRD.DeepCopy()
			rd.Spec.Resource = runtime.RawExtension{Raw: []byte(`{
				"apiVersion":"v1","kind":"ConfigMap",
				"metadata":{"name":"test-secret-1"},"data":{}
			}`)}
			return rd
		}(),
	}

	for name, newRD := range errorCases {
		t.Run(name, func(t *testing.T) {
			errs := handler.validateResourceDistribution(newRD, oldRD)
			if len(errs) == 0 {
				t.Errorf("expected failure for %q but got none", name)
			}
		})
	}
}

func TestValidateResourceDistributionTargets(t *testing.T) {
	targets := appsv1beta1.ResourceDistributionTargets{
		ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
			List: []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-1"}},
		},
		IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
			List: []appsv1beta1.ResourceDistributionNamespace{
				{Name: "ns-1"}, {Name: "ns-2"}, {Name: "kube-system"}, {Name: ""},
			},
		},
		NamespaceSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{"$#%$%": "#@$@#$"},
		},
	}
	errs := validateResourceDistributionTargets(targets, field.NewPath("targets"))
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors, got %d: %v", len(errs), errs)
	}
}

func TestResourceDistributionV1beta1IgnoresEmbeddedNamespace(t *testing.T) {
	rd := buildResourceDistributionWithSecret()
	rd.Spec.Resource.Raw = []byte(`{
		"apiVersion":"v1",
		"kind":"ConfigMap",
		"metadata":{"name":"game-demo","namespace":"should-be-ignored"},
		"data":{"k":"v"}
	}`)

	makeEnvironment()

	errs := handler.validateResourceDistribution(rd, nil)
	require.Len(t, errs, 0)
}

func TestResourceDistributionUpdateConflict(t *testing.T) {
	const changedResourceJSON = `{
		"apiVersion": "v1",
		"data": {"test": "MWYyZDFlMmU2N2Rm"},
		"kind": "Secret",
		"metadata": {"name": "test-secret-2"},
		"type": "Opaque"
	}`
	conflictingResource := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret-1", Namespace: "ns-2"},
	}

	oldRD := buildResourceDistributionWithSecret()
	newRD := oldRD.DeepCopy()
	newRD.Spec.Resource.Raw = []byte(changedResourceJSON)

	makeEnvironment(oldRD, conflictingResource)

	errs := handler.validateResourceDistribution(newRD, oldRD)
	if len(errs) != 1 {
		t.Fatalf("failed to validate the conflict of updating case, err: %v", errs)
	}
}

// buildResourceDistributionWithSecret returns a v1beta1 ResourceDistribution with a Secret.
func buildResourceDistributionWithSecret() *appsv1beta1.ResourceDistribution {
	const resourceJSON = `{
		"apiVersion": "v1",
		"data": {"test": "test"},
		"kind": "Secret",
		"metadata": {"name": "test-secret-1"},
		"type": "Opaque"
	}`
	return buildResourceDistribution(resourceJSON)
}

// buildResourceDistributionWithConfigMap returns a v1beta1 ResourceDistribution with a ConfigMap.
func buildResourceDistributionWithConfigMap() *appsv1beta1.ResourceDistribution {
	const resourceJSON = `{
		"apiVersion": "v1",
		"data": {
			"game.properties": "enemy.types=aliens,monsters\nplayer.maximum-lives=5\n",
			"player_initial_lives": "3",
			"ui_properties_file_name": "user-interface.properties",
			"user-interface.properties": "color.good=purple\ncolor.bad=yellow\nallow.textmode=true\n"
		},
		"kind": "ConfigMap",
		"metadata": {"name": "game-demo"}
	}`
	return buildResourceDistribution(resourceJSON)
}

// buildResourceDistribution constructs a v1beta1 ResourceDistribution from raw resource JSON.
func buildResourceDistribution(resourceJSON string) *appsv1beta1.ResourceDistribution {
	return &appsv1beta1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "test-resource-distribution"},
		Spec: appsv1beta1.ResourceDistributionSpec{
			Resource: runtime.RawExtension{Raw: []byte(resourceJSON)},
			Targets: appsv1beta1.ResourceDistributionTargets{
				ExcludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{{Name: "ns-3"}},
				},
				IncludedNamespaces: appsv1beta1.ResourceDistributionTargetNamespaces{
					List: []appsv1beta1.ResourceDistributionNamespace{
						{Name: "ns-1"},
						{Name: "ns-2"},
					},
				},
				NamespaceSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"environment": "develop"},
				},
			},
		},
	}
}

func makeEnvironment(addition ...client.Object) {
	env := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-1", Labels: map[string]string{"group": "one", "environment": "develop"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-2", Labels: map[string]string{"group": "one", "environment": "develop"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-3", Labels: map[string]string{"group": "two", "environment": "develop"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-4", Labels: map[string]string{"group": "two", "environment": "test"}}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret-1", Namespace: "ns-1",
			Annotations: map[string]string{SourceResourceDistributionOfResource: "test-resource-distribution"},
		}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test-secret-2", Namespace: "ns-3"}},
	}
	env = append(env, addition...)
	handler.Client = fake.NewClientBuilder().WithScheme(testScheme).WithObjects(env...).Build()
}
