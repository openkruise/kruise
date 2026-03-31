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

package v1alpha1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestUnitedDeploymentConversionRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		ud   *UnitedDeployment
	}{
		{
			name: "statefulset template",
			ud: unitedDeploymentForTemplate(SubsetTemplate{
				StatefulSetTemplate: &StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
					Spec: appsv1.StatefulSetSpec{
						Template: baseUDPodTemplate(),
					},
				},
			}),
		},
		{
			name: "advanced statefulset template",
			ud: unitedDeploymentForTemplate(SubsetTemplate{
				AdvancedStatefulSetTemplate: &AdvancedStatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
					Spec: v1beta1.StatefulSetSpec{
						Template: baseUDPodTemplate(),
					},
				},
			}),
		},
		{
			name: "cloneset template",
			ud: unitedDeploymentForTemplate(SubsetTemplate{
				CloneSetTemplate: &CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
					Spec: v1beta1.CloneSetSpec{
						Template: baseUDPodTemplate(),
					},
				},
			}),
		},
		{
			name: "deployment template",
			ud: unitedDeploymentForTemplate(SubsetTemplate{
				DeploymentTemplate: &DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
					Spec: appsv1.DeploymentSpec{
						Template: baseUDPodTemplate(),
					},
				},
			}),
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dst := &v1beta1.UnitedDeployment{}
			require.NoError(t, tt.ud.ConvertTo(dst))

			assert.Equal(t, tt.ud.Name, dst.Name)
			assert.Equal(t, v1beta1.GroupVersion.String(), dst.APIVersion)
			assert.Equal(t, "UnitedDeployment", dst.Kind)
			assert.Equal(t, v1beta1.ManualUpdateStrategyType, dst.Spec.UpdateStrategy.Type)
			assert.Equal(t, map[string]int32{"subset-a": 1}, dst.Spec.UpdateStrategy.ManualUpdate.Partitions)
			assert.Equal(t, "rev-new", dst.Status.UpdateStatus.UpdatedRevision)
			assert.Len(t, dst.Status.SubsetStatuses, 2)
			assert.Equal(t, map[string]int32{"subset-a": 3, "subset-b": 2}, subsetReplicasFromV1beta1Status(dst.Status.SubsetStatuses))
			assert.True(t, dst.Spec.Topology.ScheduleStrategy.IsAdaptive())

			switch {
			case tt.ud.Spec.Template.StatefulSetTemplate != nil:
				require.NotNil(t, dst.Spec.Template.StatefulSetTemplate)
			case tt.ud.Spec.Template.AdvancedStatefulSetTemplate != nil:
				require.NotNil(t, dst.Spec.Template.AdvancedStatefulSetTemplate)
			case tt.ud.Spec.Template.CloneSetTemplate != nil:
				require.NotNil(t, dst.Spec.Template.CloneSetTemplate)
			case tt.ud.Spec.Template.DeploymentTemplate != nil:
				require.NotNil(t, dst.Spec.Template.DeploymentTemplate)
			}

			roundTrip := &UnitedDeployment{}
			require.NoError(t, roundTrip.ConvertFrom(dst))

			assert.Equal(t, GroupVersion.String(), roundTrip.APIVersion)
			assert.Equal(t, "UnitedDeployment", roundTrip.Kind)

			expected := tt.ud.DeepCopy()
			normalizeUnitedDeploymentTimes(expected)
			normalizeUnitedDeploymentTimes(roundTrip)
			assert.Equal(t, expected, roundTrip)
		})
	}
}

func TestUnitedDeploymentConvertToV1beta1MergesLegacyStatusIntoSubsetStatuses(t *testing.T) {
	ud := unitedDeploymentForTemplate(SubsetTemplate{
		DeploymentTemplate: &DeploymentTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
			Spec: appsv1.DeploymentSpec{
				Template: baseUDPodTemplate(),
			},
		},
	})
	ud.Status.SubsetReplicas = map[string]int32{
		"subset-a": 5,
		"subset-c": 1,
	}
	ud.Status.UpdateStatus.CurrentPartitions = map[string]int32{
		"subset-a": 2,
		"subset-c": 3,
	}

	dst := &v1beta1.UnitedDeployment{}
	require.NoError(t, ud.ConvertTo(dst))

	subsetA := dst.Status.GetSubsetStatus("subset-a")
	require.NotNil(t, subsetA)
	assert.Equal(t, int32(5), subsetA.Replicas)
	assert.Equal(t, int32(2), subsetA.Partition)
	assert.Equal(t, int32(2), subsetA.ReadyReplicas)
	assert.Len(t, subsetA.Conditions, 1)

	subsetC := dst.Status.GetSubsetStatus("subset-c")
	require.NotNil(t, subsetC)
	assert.Equal(t, int32(1), subsetC.Replicas)
	assert.Equal(t, int32(3), subsetC.Partition)
	assert.Empty(t, subsetC.Conditions)
}

func TestUnitedDeploymentConvertFromV1beta1NormalizesLegacySubsetReplicas(t *testing.T) {
	src := &v1beta1.UnitedDeployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1beta1.GroupVersion.String(),
			Kind:       "UnitedDeployment",
		},
		Status: v1beta1.UnitedDeploymentStatus{
			SubsetStatuses: []v1beta1.UnitedDeploymentSubsetStatus{
				{Name: "", Replicas: 9},
				{Name: "subset-a", Replicas: 3},
			},
		},
	}

	dst := &UnitedDeployment{}
	require.NoError(t, dst.ConvertFrom(src))
	assert.Equal(t, map[string]int32{"subset-a": 3}, dst.Status.SubsetReplicas)
}

func TestUnitedDeploymentConvertFromV1beta1LeavesLegacySubsetReplicasNilWhenNoNamedStatuses(t *testing.T) {
	src := &v1beta1.UnitedDeployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1beta1.GroupVersion.String(),
			Kind:       "UnitedDeployment",
		},
		Status: v1beta1.UnitedDeploymentStatus{
			SubsetStatuses: []v1beta1.UnitedDeploymentSubsetStatus{
				{Name: "", Replicas: 9},
			},
		},
	}

	dst := &UnitedDeployment{}
	require.NoError(t, dst.ConvertFrom(src))
	assert.Nil(t, dst.Status.SubsetReplicas)
}

func unitedDeploymentForTemplate(template SubsetTemplate) *UnitedDeployment {
	replicas := int32(5)
	min := intstr.FromInt(1)
	max := intstr.FromString("80%")
	transitionTime := metav1.NewTime(time.Unix(1710000000, 0).UTC())

	return &UnitedDeployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupVersion.String(),
			Kind:       "UnitedDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
			Labels:    map[string]string{"app": "demo"},
		},
		Spec: UnitedDeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "demo"},
			},
			Template: template,
			Topology: Topology{
				Subsets: []Subset{
					{
						Name:        "subset-a",
						MinReplicas: &min,
						MaxReplicas: &max,
						Patch:       runtime.RawExtension{Raw: []byte(`{"metadata":{"labels":{"subset":"a"}}}`)},
					},
					{
						Name: "subset-b",
					},
				},
				ScheduleStrategy: UnitedDeploymentScheduleStrategy{
					Type: AdaptiveUnitedDeploymentScheduleStrategyType,
					Adaptive: &AdaptiveUnitedDeploymentStrategy{
						RescheduleCriticalSeconds: int32Ptr(45),
						UnschedulableDuration:     int32Ptr(600),
						ReserveUnschedulablePods:  true,
					},
				},
			},
			UpdateStrategy: UnitedDeploymentUpdateStrategy{
				Type: ManualUpdateStrategyType,
				ManualUpdate: &ManualUpdate{
					Partitions: map[string]int32{"subset-a": 1},
				},
			},
			RevisionHistoryLimit: int32Ptr(12),
		},
		Status: UnitedDeploymentStatus{
			ObservedGeneration:   7,
			ReadyReplicas:        4,
			Replicas:             5,
			UpdatedReplicas:      3,
			ReservedPods:         1,
			UpdatedReadyReplicas: 2,
			CollisionCount:       int32Ptr(1),
			CurrentRevision:      "rev-old",
			SubsetReplicas:       map[string]int32{"subset-a": 3, "subset-b": 2},
			SubsetStatuses: []UnitedDeploymentSubsetStatus{
				{
					Name:          "subset-a",
					Replicas:      3,
					ReadyReplicas: 2,
					Partition:     1,
					ReservedPods:  1,
					Conditions: []UnitedDeploymentSubsetCondition{
						{
							Type:               UnitedDeploymentSubsetSchedulable,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: transitionTime,
							Reason:             "reschedule",
							Message:            "reserved pods found",
						},
					},
				},
				{
					Name:          "subset-b",
					Replicas:      2,
					ReadyReplicas: 2,
				},
			},
			Conditions: []UnitedDeploymentCondition{
				{
					Type:               UnitedDeploymentUpdated,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: transitionTime,
					Reason:             "UnitedDeploymentRevisionUpdating",
					Message:            "updating to revision rev-new",
				},
			},
			UpdateStatus: &UpdateStatus{
				UpdatedRevision:   "rev-new",
				CurrentPartitions: map[string]int32{"subset-a": 1, "subset-b": 0},
			},
			LabelSelector: "app=demo",
		},
	}
}

func baseUDPodTemplate() corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "main",
				Image: "nginx:1.25",
			}},
		},
	}
}

func normalizeUnitedDeploymentTimes(ud *UnitedDeployment) {
	for i := range ud.Status.SubsetStatuses {
		for j := range ud.Status.SubsetStatuses[i].Conditions {
			ud.Status.SubsetStatuses[i].Conditions[j].LastTransitionTime = metav1.NewTime(ud.Status.SubsetStatuses[i].Conditions[j].LastTransitionTime.UTC())
		}
	}
	for i := range ud.Status.Conditions {
		ud.Status.Conditions[i].LastTransitionTime = metav1.NewTime(ud.Status.Conditions[i].LastTransitionTime.UTC())
	}
}

func subsetReplicasFromV1beta1Status(statuses []v1beta1.UnitedDeploymentSubsetStatus) map[string]int32 {
	replicas := make(map[string]int32, len(statuses))
	for _, status := range statuses {
		if status.Name == "" {
			continue
		}
		replicas[status.Name] = status.Replicas
	}
	return replicas
}
