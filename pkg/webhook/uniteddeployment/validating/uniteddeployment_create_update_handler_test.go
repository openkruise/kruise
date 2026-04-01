package validating

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestUnitedDeploymentCreateUpdateHandlerHandleV1beta1CreateRejectsReservedStatefulWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))

	handler := &UnitedDeploymentCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	obj := newBetaStatefulUnitedDeployment()
	obj.Spec.Topology.ScheduleStrategy = appsv1beta1.UnitedDeploymentScheduleStrategy{
		Type: appsv1beta1.AdaptiveUnitedDeploymentScheduleStrategyType,
		Adaptive: &appsv1beta1.AdaptiveUnitedDeploymentStrategy{
			ReserveUnschedulablePods: true,
		},
	}

	raw, err := json.Marshal(obj)
	require.NoError(t, err)

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

	require.False(t, resp.Allowed)
	require.NotNil(t, resp.Result)
	require.Equal(t, int32(http.StatusUnprocessableEntity), resp.Result.Code)
}

func TestUnitedDeploymentCreateUpdateHandlerHandleV1beta1CreateAllowsValidDeploymentWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))

	handler := &UnitedDeploymentCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	obj := newBetaDeploymentUnitedDeploymentForValidation()
	raw, err := json.Marshal(obj)
	require.NoError(t, err)

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

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Result)
	require.Equal(t, int32(http.StatusOK), resp.Result.Code)
}

func TestUnitedDeploymentCreateUpdateHandlerHandleV1alpha1CreateAllowsValidDeploymentWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))

	handler := &UnitedDeploymentCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	obj := newAlphaDeploymentUnitedDeploymentForValidation(t)
	raw, err := json.Marshal(obj)
	require.NoError(t, err)

	resp := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "uniteddeployments",
			},
			Object: runtime.RawExtension{Raw: raw},
		},
	})

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Result)
	require.Equal(t, int32(http.StatusOK), resp.Result.Code)
}

func TestUnitedDeploymentCreateUpdateHandlerHandleV1beta1UpdateRejectsNodeSelectorMutation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))

	handler := &UnitedDeploymentCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	oldObj := newBetaDeploymentUnitedDeploymentForValidation()
	newObj := oldObj.DeepCopy()
	newObj.Spec.Topology.Subsets[0].NodeSelectorTerm.MatchExpressions[0].Values = []string{"zone-b"}

	oldRaw, err := json.Marshal(oldObj)
	require.NoError(t, err)
	newRaw, err := json.Marshal(newObj)
	require.NoError(t, err)

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

	require.False(t, resp.Allowed)
	require.NotNil(t, resp.Result)
	require.Equal(t, int32(http.StatusUnprocessableEntity), resp.Result.Code)
}

func TestUnitedDeploymentCreateUpdateHandlerHandleV1beta1UpdateAllowsReplicaChange(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))

	handler := &UnitedDeploymentCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	oldObj := newBetaDeploymentUnitedDeploymentForValidation()
	newObj := oldObj.DeepCopy()
	replicas := int32(5)
	newObj.Spec.Replicas = &replicas

	oldRaw, err := json.Marshal(oldObj)
	require.NoError(t, err)
	newRaw, err := json.Marshal(newObj)
	require.NoError(t, err)

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

	require.True(t, resp.Allowed)
	require.NotNil(t, resp.Result)
	require.Equal(t, int32(http.StatusOK), resp.Result.Code)
}

func TestUnitedDeploymentCreateUpdateHandlerHandleV1alpha1UpdateRejectsNodeSelectorMutation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apis.AddToScheme(scheme))

	handler := &UnitedDeploymentCreateUpdateHandler{
		Decoder: admission.NewDecoder(scheme),
	}

	oldObj := newAlphaDeploymentUnitedDeploymentForValidation(t)
	newObj := oldObj.DeepCopy()
	newObj.Spec.Topology.Subsets[0].NodeSelectorTerm.MatchExpressions[0].Values = []string{"zone-b"}

	oldRaw, err := json.Marshal(oldObj)
	require.NoError(t, err)
	newRaw, err := json.Marshal(newObj)
	require.NoError(t, err)

	resp := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Resource: metav1.GroupVersionResource{
				Group:    appsv1alpha1.GroupVersion.Group,
				Version:  appsv1alpha1.GroupVersion.Version,
				Resource: "uniteddeployments",
			},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	})

	require.False(t, resp.Allowed)
	require.NotNil(t, resp.Result)
	require.Equal(t, int32(http.StatusUnprocessableEntity), resp.Result.Code)
}

func newBetaDeploymentUnitedDeploymentForValidation() *appsv1beta1.UnitedDeployment {
	replicas := int32(3)

	return &appsv1beta1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ud",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Spec: appsv1beta1.UnitedDeploymentSpec{
			Replicas: &replicas,
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
					Name: "subset-a",
					NodeSelectorTerm: corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "topology.kubernetes.io/zone",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-a"},
						}},
					},
				}},
			},
		},
	}
}

func newBetaStatefulUnitedDeployment() *appsv1beta1.UnitedDeployment {
	replicas := int32(3)

	return &appsv1beta1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ud-sts",
			Namespace: "default",
		},
		Spec: appsv1beta1.UnitedDeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "demo"},
			},
			Template: appsv1beta1.SubsetTemplate{
				StatefulSetTemplate: &appsv1beta1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "demo"},
					},
					Spec: appsv1.StatefulSetSpec{
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
					Name: "subset-a",
				}},
			},
		},
	}
}

func newAlphaDeploymentUnitedDeploymentForValidation(t *testing.T) *appsv1alpha1.UnitedDeployment {
	t.Helper()

	alphaObj := &appsv1alpha1.UnitedDeployment{}
	require.NoError(t, alphaObj.ConvertFrom(newBetaDeploymentUnitedDeploymentForValidation()))
	return alphaObj
}
