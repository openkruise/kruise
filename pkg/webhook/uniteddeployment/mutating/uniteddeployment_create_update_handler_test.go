package mutating

import (
	"context"
	"encoding/json"
	"testing"

	evanjsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestUnitedDeploymentCreateUpdateHandlerHandleV1beta1CreateDefaultsAndClearsStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))

	handler := &UnitedDeploymentCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	max := intstr.FromInt(5)
	obj := newBetaDeploymentUnitedDeployment()
	obj.Spec.Replicas = nil
	obj.Spec.RevisionHistoryLimit = nil
	obj.Spec.UpdateStrategy = appsv1beta1.UnitedDeploymentUpdateStrategy{}
	obj.Spec.Topology.Subsets[0].MinReplicas = nil
	obj.Spec.Topology.Subsets[0].MaxReplicas = &max
	obj.Status = appsv1beta1.UnitedDeploymentStatus{Replicas: 99}

	raw := mustMarshalUnitedDeployment(t, obj)
	resp := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "uniteddeployments",
			},
			Object: runtime.RawExtension{Raw: raw},
		},
	})

	require.True(t, resp.Allowed, "unexpected denial: %#v", resp.Result)
	require.NotEmpty(t, resp.Patches)

	mutated := &appsv1beta1.UnitedDeployment{}
	require.NoError(t, json.Unmarshal(applyAdmissionPatches(t, raw, resp.Patches), mutated))

	require.NotNil(t, mutated.Spec.Replicas)
	assert.Equal(t, int32(1), *mutated.Spec.Replicas)
	require.NotNil(t, mutated.Spec.RevisionHistoryLimit)
	assert.Equal(t, int32(10), *mutated.Spec.RevisionHistoryLimit)
	assert.Equal(t, appsv1beta1.ManualUpdateStrategyType, mutated.Spec.UpdateStrategy.Type)
	require.NotNil(t, mutated.Spec.UpdateStrategy.ManualUpdate)
	assert.Empty(t, mutated.Spec.UpdateStrategy.ManualUpdate.Partitions)
	require.NotNil(t, mutated.Spec.Topology.Subsets[0].MinReplicas)
	assert.Equal(t, int32(0), mutated.Spec.Topology.Subsets[0].MinReplicas.IntVal)
	assert.Equal(t, appsv1beta1.UnitedDeploymentStatus{}, mutated.Status)
}

func TestUnitedDeploymentCreateUpdateHandlerHandleV1beta1UpdatePreservesExplicitSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))

	handler := &UnitedDeploymentCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	oldObj := newBetaDeploymentUnitedDeployment()
	oldObj.Spec.UpdateStrategy = appsv1beta1.UnitedDeploymentUpdateStrategy{
		Type: appsv1beta1.ManualUpdateStrategyType,
		ManualUpdate: &appsv1beta1.ManualUpdate{
			Partitions: map[string]int32{"subset-a": 1},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Status = appsv1beta1.UnitedDeploymentStatus{Replicas: 100}

	oldRaw := mustMarshalUnitedDeployment(t, oldObj)
	newRaw := mustMarshalUnitedDeployment(t, newObj)
	resp := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1beta1.GroupVersion.Group,
				Version:  appsv1beta1.GroupVersion.Version,
				Resource: "uniteddeployments",
			},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	})

	require.True(t, resp.Allowed, "unexpected denial: %#v", resp.Result)
	require.NotEmpty(t, resp.Patches)

	mutated := &appsv1beta1.UnitedDeployment{}
	require.NoError(t, json.Unmarshal(applyAdmissionPatches(t, newRaw, resp.Patches), mutated))

	assert.Equal(t, oldObj.Spec, mutated.Spec)
	assert.Equal(t, appsv1beta1.UnitedDeploymentStatus{}, mutated.Status)
}

func newBetaDeploymentUnitedDeployment() *appsv1beta1.UnitedDeployment {
	replicas := int32(5)
	revisionHistoryLimit := int32(12)
	min := intstr.FromInt(1)

	return &appsv1beta1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ud",
			Namespace: "default",
		},
		Spec: appsv1beta1.UnitedDeploymentSpec{
			Replicas:             &replicas,
			RevisionHistoryLimit: &revisionHistoryLimit,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "demo"},
			},
			Template: appsv1beta1.SubsetTemplate{
				DeploymentTemplate: &appsv1beta1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "demo"},
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "demo"},
							},
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyAlways,
								DNSPolicy:     corev1.DNSClusterFirst,
								Containers: []corev1.Container{{
									Name:                     "main",
									Image:                    "nginx:1.25",
									ImagePullPolicy:          corev1.PullIfNotPresent,
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								}},
							},
						},
					},
				},
			},
			Topology: appsv1beta1.Topology{
				Subsets: []appsv1beta1.Subset{{
					Name:        "subset-a",
					MinReplicas: &min,
				}},
			},
		},
	}
}

func mustMarshalUnitedDeployment(t *testing.T, obj *appsv1beta1.UnitedDeployment) []byte {
	t.Helper()
	raw, err := json.Marshal(obj)
	require.NoError(t, err)
	return raw
}

func applyAdmissionPatches(t *testing.T, raw []byte, patches interface{}) []byte {
	t.Helper()
	patchBytes, err := json.Marshal(patches)
	require.NoError(t, err)

	patch, err := evanjsonpatch.DecodePatch(patchBytes)
	require.NoError(t, err)

	mutated, err := patch.Apply(raw)
	require.NoError(t, err)
	return mutated
}