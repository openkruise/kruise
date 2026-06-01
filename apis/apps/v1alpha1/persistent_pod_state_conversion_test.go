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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestPersistentPodState_ConvertTo(t *testing.T) {
	cases := []struct {
		name string
		src  *PersistentPodState
		want *v1beta1.PersistentPodState
	}{
		{
			name: "all fields with nodeTopologyKeys",
			src: &PersistentPodState{
				ObjectMeta: metav1.ObjectMeta{Name: "pps", Namespace: "default"},
				Spec: PersistentPodStateSpec{
					TargetReference: TargetReference{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "web",
					},
					PersistentPodAnnotations: []PersistentPodAnnotation{{Key: "foo"}},
					RequiredPersistentTopology: &NodeTopologyTerm{
						NodeTopologyKeys: []string{"kubernetes.io/hostname"},
					},
					PreferredPersistentTopology: []PreferredTopologyTerm{
						{
							Weight: 100,
							Preference: NodeTopologyTerm{
								NodeTopologyKeys: []string{"topology.kubernetes.io/zone"},
							},
						},
					},
					PersistentPodStateRetentionPolicy: PersistentPodStateRetentionPolicyWhenDeleted,
				},
				Status: PersistentPodStateStatus{
					ObservedGeneration: 2,
					PodStates: map[string]PodState{
						"web-0": {
							NodeName:           "node-1",
							NodeTopologyLabels: map[string]string{"kubernetes.io/hostname": "node-1"},
							Annotations:        map[string]string{"foo": "bar"},
						},
					},
				},
			},
			want: &v1beta1.PersistentPodState{
				Spec: v1beta1.PersistentPodStateSpec{
					TargetReference: v1beta1.TargetReference{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       "web",
					},
					PersistentPodAnnotations: []v1beta1.PersistentPodAnnotation{{Key: "foo"}},
					RequiredPersistentTopology: &v1beta1.NodeTopologyTerm{
						Keys: []string{"kubernetes.io/hostname"},
					},
					PreferredPersistentTopology: []v1beta1.PreferredTopologyTerm{
						{
							Weight: 100,
							Preference: v1beta1.NodeTopologyTerm{
								Keys: []string{"topology.kubernetes.io/zone"},
							},
						},
					},
					PersistentPodStateRetentionPolicy: v1beta1.PersistentPodStateRetentionPolicyWhenDeleted,
				},
				Status: v1beta1.PersistentPodStateStatus{
					ObservedGeneration: 2,
					PodStates: map[string]v1beta1.PodState{
						"web-0": {
							NodeName:           "node-1",
							NodeTopologyLabels: map[string]string{"kubernetes.io/hostname": "node-1"},
							Annotations:        map[string]string{"foo": "bar"},
						},
					},
				},
			},
		},
		{
			name: "empty topology",
			src: &PersistentPodState{
				Spec: PersistentPodStateSpec{
					TargetReference: TargetReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "x"},
					PreferredPersistentTopology: []PreferredTopologyTerm{
						{Weight: 50, Preference: NodeTopologyTerm{}},
					},
				},
			},
			want: &v1beta1.PersistentPodState{
				Spec: v1beta1.PersistentPodStateSpec{
					TargetReference: v1beta1.TargetReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "x"},
					PreferredPersistentTopology: []v1beta1.PreferredTopologyTerm{
						{Weight: 50, Preference: v1beta1.NodeTopologyTerm{}},
					},
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dst := &v1beta1.PersistentPodState{}
			require.NoError(t, tt.src.ConvertTo(dst))
			assert.Equal(t, tt.want.Spec, dst.Spec)
			assert.Equal(t, tt.want.Status, dst.Status)
		})
	}
}

func TestPersistentPodState_ConvertFrom(t *testing.T) {
	src := &v1beta1.PersistentPodState{
		ObjectMeta: metav1.ObjectMeta{Name: "pps", Namespace: "ns"},
		Spec: v1beta1.PersistentPodStateSpec{
			TargetReference:          v1beta1.TargetReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "web"},
			PersistentPodAnnotations: []v1beta1.PersistentPodAnnotation{{Key: "foo"}},
			RequiredPersistentTopology: &v1beta1.NodeTopologyTerm{
				Keys: []string{"kubernetes.io/hostname", "topology.kubernetes.io/zone"},
			},
			PreferredPersistentTopology: []v1beta1.PreferredTopologyTerm{
				{Weight: 100, Preference: v1beta1.NodeTopologyTerm{Keys: []string{"topology.kubernetes.io/zone"}}},
			},
			PersistentPodStateRetentionPolicy: v1beta1.PersistentPodStateRetentionPolicyWhenDeleted,
		},
		Status: v1beta1.PersistentPodStateStatus{
			ObservedGeneration: 5,
			PodStates: map[string]v1beta1.PodState{
				"web-0": {
					NodeName:           "node-1",
					NodeTopologyLabels: map[string]string{"kubernetes.io/hostname": "node-1"},
					Annotations:        map[string]string{"foo": "bar"},
				},
			},
		},
	}
	dst := &PersistentPodState{}
	require.NoError(t, dst.ConvertFrom(src))

	// All spec fields map faithfully, with keys renamed back to nodeTopologyKeys.
	assert.Equal(t, src.Spec.TargetReference.Name, dst.Spec.TargetReference.Name)
	assert.Equal(t, []PersistentPodAnnotation{{Key: "foo"}}, dst.Spec.PersistentPodAnnotations)
	assert.Equal(t, []string{"kubernetes.io/hostname", "topology.kubernetes.io/zone"}, dst.Spec.RequiredPersistentTopology.NodeTopologyKeys)
	assert.Equal(t, []string{"topology.kubernetes.io/zone"}, dst.Spec.PreferredPersistentTopology[0].Preference.NodeTopologyKeys)
	assert.Equal(t, int32(100), dst.Spec.PreferredPersistentTopology[0].Weight)
	assert.Equal(t, PersistentPodStateRetentionPolicyType(PersistentPodStateRetentionPolicyWhenDeleted), dst.Spec.PersistentPodStateRetentionPolicy)

	// Status fields map faithfully.
	assert.Equal(t, int64(5), dst.Status.ObservedGeneration)
	assert.Equal(t, "node-1", dst.Status.PodStates["web-0"].NodeName)
	assert.Equal(t, map[string]string{"kubernetes.io/hostname": "node-1"}, dst.Status.PodStates["web-0"].NodeTopologyLabels)
	assert.Equal(t, map[string]string{"foo": "bar"}, dst.Status.PodStates["web-0"].Annotations)
}

func TestPersistentPodState_RoundTrip(t *testing.T) {
	original := &PersistentPodState{
		ObjectMeta: metav1.ObjectMeta{Name: "pps", Namespace: "default", ResourceVersion: "1"},
		Spec: PersistentPodStateSpec{
			TargetReference: TargetReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "web"},
			RequiredPersistentTopology: &NodeTopologyTerm{
				NodeTopologyKeys: []string{"kubernetes.io/hostname"},
			},
			PreferredPersistentTopology: []PreferredTopologyTerm{
				{Weight: 100, Preference: NodeTopologyTerm{NodeTopologyKeys: []string{"topology.kubernetes.io/zone"}}},
			},
		},
		Status: PersistentPodStateStatus{
			ObservedGeneration: 3,
			PodStates: map[string]PodState{
				"web-0": {NodeName: "node-a", NodeTopologyLabels: map[string]string{"kubernetes.io/hostname": "node-a"}},
			},
		},
	}

	hub := &v1beta1.PersistentPodState{}
	require.NoError(t, original.ConvertTo(hub))
	assert.Equal(t, []string{"kubernetes.io/hostname"}, hub.Spec.RequiredPersistentTopology.Keys)
	assert.Equal(t, []string{"topology.kubernetes.io/zone"}, hub.Spec.PreferredPersistentTopology[0].Preference.Keys)

	roundTrip := &PersistentPodState{}
	require.NoError(t, roundTrip.ConvertFrom(hub))
	assert.Equal(t, original.Spec, roundTrip.Spec)
	assert.Equal(t, original.Status, roundTrip.Status)
}

func FuzzPersistentPodStateConversion(f *testing.F) {
	f.Add("hostname", "zone", int32(100))
	f.Fuzz(func(t *testing.T, requiredKey, preferredKey string, weight int32) {
		if weight < 0 {
			weight = -weight
		}
		src := &PersistentPodState{
			Spec: PersistentPodStateSpec{
				TargetReference: TargetReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "fuzz"},
				RequiredPersistentTopology: &NodeTopologyTerm{
					NodeTopologyKeys: []string{requiredKey},
				},
				PreferredPersistentTopology: []PreferredTopologyTerm{
					{Weight: weight, Preference: NodeTopologyTerm{NodeTopologyKeys: []string{preferredKey}}},
				},
			},
		}
		hub := &v1beta1.PersistentPodState{}
		require.NoError(t, src.ConvertTo(hub))
		dst := &PersistentPodState{}
		require.NoError(t, dst.ConvertFrom(hub))
		assert.Equal(t, src.Spec.TargetReference, dst.Spec.TargetReference)
		assert.Equal(t, src.Spec.RequiredPersistentTopology.NodeTopologyKeys, dst.Spec.RequiredPersistentTopology.NodeTopologyKeys)
		assert.Equal(t, src.Spec.PreferredPersistentTopology[0].Preference.NodeTopologyKeys, dst.Spec.PreferredPersistentTopology[0].Preference.NodeTopologyKeys)
	})
}
