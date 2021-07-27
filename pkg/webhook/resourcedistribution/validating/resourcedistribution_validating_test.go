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

	v1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var (
	scheme  *runtime.Scheme
	handler = &ResourceDistributionCreateUpdateHandler{}
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(v1beta1.AddToScheme(scheme))
}

func TestResourceDistributionCreateValidation(t *testing.T) {
	// build rd objects
	rdSecret := buildResourceDistributionWithSecret()
	rdConfigMap := buildResourceDistributionWithConfigMap()

	makeEnvironment()

	// test successful cases of secret and configmap distributor
	errs := handler.validateResourceDistribution(rdSecret, nil, nil)
	errs = append(errs, handler.validateResourceDistribution(rdConfigMap, nil, nil)...)
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
	errs := handler.validateResourceDistribution(rdSecret, nil, nil)
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
	errs := handler.validateResourceDistribution(rdSecret, nil, nil)
	if len(errs) != 2 {
		t.Fatalf("failed to validate the wrong resource case, err: %v", errs)
	}
}

func TestResourceDistributionUpdateValidation(t *testing.T) {
	// build rd objects
	oldRD := buildResourceDistributionWithSecret()
	newRD := oldRD.DeepCopy()
	newRD.Spec.WritePolicy = appsv1alpha1.RESOURCEDISTRIBUTION_OVERWRITE_WRITEPOLICY

	makeEnvironment()

	// test successful updating case
	errs := handler.validateResourceDistribution(newRD, oldRD, nil)
	if len(errs) != 0 {
		t.Fatalf("failed to validate the updating case, err: %v", errs)
	}
}

func TestResourceDistributionUpdateConflict(t *testing.T) {
	// resource details
	const changedResourceYaml = `apiVersion: v1
kind: Secret
metadata:
  name: test-secret-modified-name
type: kubernetes.io/basic-auth
stringData:
  username: admin
  password: t0p-Secret`
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
	newRD.Spec.Resource.Raw = []byte(changedResourceYaml)

	makeEnvironment(oldRD, conflictingResource)

	// test failed case under the conflict between new and old resources
	errs := handler.validateResourceDistribution(newRD, oldRD, nil)
	if len(errs) != 1 {
		t.Fatalf("failed to validate the conflict of updating case, err: %v", errs)
	}
}

func TestResourceDistributionWritePolicy(t *testing.T) {
	// build rd object
	rd := buildResourceDistributionWithSecret()

	makeEnvironment()

	// test right policy
	rd.Spec.WritePolicy = appsv1alpha1.RESOURCEDISTRIBUTION_STRICT_WRITEPOLICY
	errs := handler.validateResourceDistribution(rd, nil, nil)
	rd.Spec.WritePolicy = appsv1alpha1.RESOURCEDISTRIBUTION_IGNORE_WRITEPOLICY
	errs = append(errs, handler.validateResourceDistribution(rd, nil, nil)...)
	rd.Spec.WritePolicy = appsv1alpha1.RESOURCEDISTRIBUTION_OVERWRITE_WRITEPOLICY
	errs = append(errs, handler.validateResourceDistribution(rd, nil, nil)...)
	if len(errs) != 0 {
		t.Fatalf("failed to validate right writePolicies, err: %v", errs)
	}

	// test wrong policy
	rd.Spec.WritePolicy = appsv1alpha1.ResourceDistributionWritePolicyType("some-thing-else")
	errs = handler.validateResourceDistribution(rd, nil, nil)
	if len(errs) != 1 {
		t.Fatalf("failed to validate wrong writePolicies, err: %v", errs)
	}
}

func TestResourceDistributionResourceConflict(t *testing.T) {
	// build rd object
	rd := buildResourceDistributionWithConfigMap()

	makeEnvironment()

	// test failed cases of the lack of workload kind or apiVersion in workloadLabelSelector
	kind := rd.Spec.Targets.WorkloadLabelSelector.Kind
	rd.Spec.Targets.WorkloadLabelSelector.Kind = ""
	errsKind := handler.validateResourceDistribution(rd, nil, nil)
	if len(errsKind) != 2 {
		t.Fatalf("failed to validate the kind of workload in WorkloadLabelSelector, err %v", errsKind)
	}
	rd.Spec.Targets.WorkloadLabelSelector.Kind = kind
	rd.Spec.Targets.WorkloadLabelSelector.APIVersion = ""
	errsAPIVersion := handler.validateResourceDistribution(rd, nil, nil)
	if len(errsAPIVersion) != 2 {
		t.Fatalf("failed to validate the apiVersion of workload in WorkloadLabelSelector, err %v", errsAPIVersion)
	}
}

func TestAllWorkloadTypesInWorkloadLabelSelector(t *testing.T) {
	var workloadTypes = map[string][]string{
		"v1": {"Pod"},
		"apps/v1":                 {"Deployment", "ReplicaSet", "StatefulSet", "DaemonSet"},
		"apps.kruise.io/v1alpha1": {"StatefulSet", "DaemonSet", "AdvancedCronJob", "CloneSet", "UnitedDeployment", "BroadcastJob", "NodeImage"},
		"batch/v1beta1":           {"CornJob"},
		"batch/v1":                {"Job"},
	}

	// build rd object
	rd := buildResourceDistributionWithSecret()

	// construct env with all workload instance
	makeEnvironment(addAllWorkloadEnvironment()...)

	for apiVersion, kinds := range workloadTypes {
		for _, kind := range kinds {
			// successful case
			rd.Spec.Targets.WorkloadLabelSelector.APIVersion = apiVersion
			rd.Spec.Targets.WorkloadLabelSelector.Kind = kind
			if errs := handler.validateResourceDistribution(rd, nil, nil); len(errs) != 0 {
				t.Fatalf("failed to validate the apiVersion and kind of all workloads in WorkloadLabelSelector, in successful case, apiVersion %s, kind %s, err %v", apiVersion, kind, errs)
			}

			// failed case
			rd.Spec.Targets.WorkloadLabelSelector.APIVersion = apiVersion + "addition"
			rd.Spec.Targets.WorkloadLabelSelector.Kind = kind
			if errs := handler.validateResourceDistribution(rd, nil, nil); len(errs) != 1 {
				t.Fatalf("failed to validate the apiVersion and kind of all workloads in WorkloadLabelSelector, in failed case, apiVersion %s, kind %s, err %v", apiVersion, kind, errs)
			}
		}
	}
}

func addAllWorkloadEnvironment() (env []runtime.Object) {
	//build-in workload types
	env = append(env, &corev1.Pod{}) //pod
	env = append(env, &batchv1.Job{})
	env = append(env, &v1.DaemonSet{})
	env = append(env, &v1.Deployment{})
	env = append(env, &v1.ReplicaSet{})
	env = append(env, &v1.StatefulSet{})
	env = append(env, &v1beta1.CronJob{})

	//kruise workload types
	env = append(env, &appsv1alpha1.CloneSet{})
	env = append(env, &appsv1alpha1.DaemonSet{})
	env = append(env, &appsv1alpha1.StatefulSet{})
	env = append(env, &appsv1alpha1.ImagePullJob{})
	env = append(env, &appsv1alpha1.BroadcastJob{})
	env = append(env, &appsv1alpha1.AdvancedCronJob{})
	env = append(env, &appsv1alpha1.UnitedDeployment{})

	return
}

func buildResourceDistributionWithSecret() (RD *appsv1alpha1.ResourceDistribution) {
	const resourceYaml = `apiVersion: v1
kind: Secret
metadata:
  name: test-secret-1
type: kubernetes.io/basic-auth
stringData:
  username: admin
  password: t0p-Secret`

	RD = &appsv1alpha1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "test-resource-distribution"},
		Spec: appsv1alpha1.ResourceDistributionSpec{
			WritePolicy: appsv1alpha1.RESOURCEDISTRIBUTION_STRICT_WRITEPOLICY,
			Resource: runtime.RawExtension{
				Raw: []byte(resourceYaml),
			},
			Targets: appsv1alpha1.ResourceDistributionTargets{
				All: []appsv1alpha1.ResourceDistributionTargetException{
					{
						Exception: "ns-3",
					},
				},
				Namespaces: []appsv1alpha1.ResourceDistributionNamespace{
					{
						Name: "ns-1",
					},
					{
						Name: "ns-2",
					},
				},
				NamespaceLabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"environment": "develop",
					},
				},
				WorkloadLabelSelector: appsv1alpha1.ResourceDistributionWorkloadLabelSelector{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "CloneSet",
				},
			},
		},
		Status: appsv1alpha1.ResourceDistributionStatus{
			Description:           "Successful",
			ConflictingNamespaces: []appsv1alpha1.ResourceDistributionNamespace{},
		},
	}
	RD.Spec.Targets.WorkloadLabelSelector.MatchLabels = map[string]string{
		"app": "nginx",
	}

	return
}

func buildResourceDistributionWithConfigMap() (RD *appsv1alpha1.ResourceDistribution) {
	const resourceYaml = `apiVersion: v1
kind: ConfigMap
metadata:
  name: game-demo
data:
  player_initial_lives: "3"
  ui_properties_file_name: "user-interface.properties"
  game.properties: |
    enemy.types=aliens,monsters
    player.maximum-lives=5
  user-interface.properties: |
    color.good=purple
    color.bad=yellow
    allow.textmode=true`

	RD = &appsv1alpha1.ResourceDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "test-resource-distribution"},
		Spec: appsv1alpha1.ResourceDistributionSpec{
			WritePolicy: appsv1alpha1.RESOURCEDISTRIBUTION_STRICT_WRITEPOLICY,
			Resource: runtime.RawExtension{
				Raw: []byte(resourceYaml),
			},
			Targets: appsv1alpha1.ResourceDistributionTargets{
				All: []appsv1alpha1.ResourceDistributionTargetException{
					{
						Exception: "ns-3",
					},
				},
				Namespaces: []appsv1alpha1.ResourceDistributionNamespace{
					{
						Name: "ns-1",
					},
					{
						Name: "ns-2",
					},
				},
				NamespaceLabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"environment": "develop",
					},
				},
				WorkloadLabelSelector: appsv1alpha1.ResourceDistributionWorkloadLabelSelector{
					APIVersion: "apps.kruise.io/v1alpha1",
					Kind:       "CloneSet",
				},
			},
		},
		Status: appsv1alpha1.ResourceDistributionStatus{
			Description:           "Successful",
			ConflictingNamespaces: []appsv1alpha1.ResourceDistributionNamespace{},
		},
	}
	RD.Spec.Targets.WorkloadLabelSelector.MatchLabels = map[string]string{
		"app": "nginx",
	}
	return
}

func makeEnvironment(addition ...runtime.Object) {
	env := []runtime.Object{
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
		&appsv1alpha1.CloneSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-1",
				Namespace: "ns-1",
				Labels: map[string]string{
					"app": "nginx",
				},
			},
		},
		&appsv1alpha1.CloneSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-1",
				Namespace: "ns-2",
				Labels: map[string]string{
					"app": "nginx",
				},
			},
		},
		&appsv1alpha1.CloneSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-1",
				Namespace: "ns-3",
				Labels: map[string]string{
					"app": "nginx",
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret-1",
				Namespace: "ns-1",
				Annotations: map[string]string{
					"kruise.io/from-resource-distribution": "test-resource-distribution",
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
	fakeClient := fake.NewFakeClientWithScheme(scheme, env...)
	handler.Client = fakeClient
}
