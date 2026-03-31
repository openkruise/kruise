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

package defaults

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestSetDefaultsUnitedDeployment(t *testing.T) {
	max := intstr.FromInt(5)
	ud := &appsv1alpha1.UnitedDeployment{
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{{
					Name:        "subset-a",
					MaxReplicas: &max,
				}},
			},
		},
	}

	SetDefaultsUnitedDeployment(ud, false)

	require.NotNil(t, ud.Spec.Replicas)
	require.Equal(t, int32(1), *ud.Spec.Replicas)
	require.NotNil(t, ud.Spec.RevisionHistoryLimit)
	require.Equal(t, int32(10), *ud.Spec.RevisionHistoryLimit)
	require.Equal(t, appsv1alpha1.ManualUpdateStrategyType, ud.Spec.UpdateStrategy.Type)
	require.NotNil(t, ud.Spec.UpdateStrategy.ManualUpdate)
	require.NotNil(t, ud.Spec.Topology.Subsets[0].MinReplicas)
	require.Equal(t, intstr.Int, ud.Spec.Topology.Subsets[0].MinReplicas.Type)
	require.Equal(t, int32(0), ud.Spec.Topology.Subsets[0].MinReplicas.IntVal)
}

func TestSetDefaultsUnitedDeploymentV1beta1(t *testing.T) {
	max := intstr.FromInt(5)
	ud := &appsv1beta1.UnitedDeployment{
		Spec: appsv1beta1.UnitedDeploymentSpec{
			Topology: appsv1beta1.Topology{
				Subsets: []appsv1beta1.Subset{{
					Name:        "subset-a",
					MaxReplicas: &max,
				}},
			},
		},
	}

	SetDefaultsUnitedDeploymentV1beta1(ud, false)

	require.NotNil(t, ud.Spec.Replicas)
	require.Equal(t, int32(1), *ud.Spec.Replicas)
	require.NotNil(t, ud.Spec.RevisionHistoryLimit)
	require.Equal(t, int32(10), *ud.Spec.RevisionHistoryLimit)
	require.Equal(t, appsv1beta1.ManualUpdateStrategyType, ud.Spec.UpdateStrategy.Type)
	require.NotNil(t, ud.Spec.UpdateStrategy.ManualUpdate)
	require.NotNil(t, ud.Spec.Topology.Subsets[0].MinReplicas)
	require.Equal(t, intstr.Int, ud.Spec.Topology.Subsets[0].MinReplicas.Type)
	require.Equal(t, int32(0), ud.Spec.Topology.Subsets[0].MinReplicas.IntVal)
}

func TestSetDefaultsUnitedDeploymentV1beta1PreservesExplicitValues(t *testing.T) {
	replicas := int32(7)
	revisionHistoryLimit := int32(3)
	min := intstr.FromString("25%")
	max := intstr.FromInt(9)
	ud := &appsv1beta1.UnitedDeployment{
		Spec: appsv1beta1.UnitedDeploymentSpec{
			Replicas:             &replicas,
			RevisionHistoryLimit: &revisionHistoryLimit,
			UpdateStrategy: appsv1beta1.UnitedDeploymentUpdateStrategy{
				Type: appsv1beta1.ManualUpdateStrategyType,
				ManualUpdate: &appsv1beta1.ManualUpdate{
					Partitions: map[string]int32{"subset-a": 2},
				},
			},
			Topology: appsv1beta1.Topology{
				Subsets: []appsv1beta1.Subset{{
					Name:        "subset-a",
					MinReplicas: &min,
					MaxReplicas: &max,
				}},
			},
		},
	}

	SetDefaultsUnitedDeploymentV1beta1(ud, false)

	require.NotNil(t, ud.Spec.Replicas)
	require.Equal(t, int32(7), *ud.Spec.Replicas)
	require.NotNil(t, ud.Spec.RevisionHistoryLimit)
	require.Equal(t, int32(3), *ud.Spec.RevisionHistoryLimit)
	require.Equal(t, appsv1beta1.ManualUpdateStrategyType, ud.Spec.UpdateStrategy.Type)
	require.NotNil(t, ud.Spec.UpdateStrategy.ManualUpdate)
	require.Equal(t, map[string]int32{"subset-a": 2}, ud.Spec.UpdateStrategy.ManualUpdate.Partitions)
	require.NotNil(t, ud.Spec.Topology.Subsets[0].MinReplicas)
	require.Equal(t, min, *ud.Spec.Topology.Subsets[0].MinReplicas)
}
