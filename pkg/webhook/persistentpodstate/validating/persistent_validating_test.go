/*
Copyright 2022 The Kruise Authors.

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
	"context"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme

	podStateZoneTopologyLabel = "topology.kubernetes.io/zone"
	podStateNodeTopologyLabel = "kubernetes.io/hostname"

	ppsDemo = appsv1alpha1.PersistentPodState{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1alpha1.GroupVersion.String(),
			Kind:       "PersistentPodState",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "per-test",
		},
		Spec: appsv1alpha1.PersistentPodStateSpec{
			TargetReference: appsv1alpha1.TargetReference{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Name:       "statefulset-test",
			},
			RequiredPersistentTopology: &appsv1alpha1.NodeTopologyTerm{
				NodeTopologyKeys: []string{podStateZoneTopologyLabel},
			},
			PreferredPersistentTopology: []appsv1alpha1.PreferredTopologyTerm{
				{
					Weight: 100,
					Preference: appsv1alpha1.NodeTopologyTerm{
						NodeTopologyKeys: []string{podStateNodeTopologyLabel},
					},
				},
			},
		},
	}
)

func TestValidatingPer(t *testing.T) {
	cases := []struct {
		name          string
		per           func() *appsv1alpha1.PersistentPodState
		expectErrList int
	}{
		{
			name: "valid per, TargetReference",
			per: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				return pps
			},
			expectErrList: 0,
		},
		{
			name: "invalid per, TargetReference are nil",
			per: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				pps.Spec.TargetReference = appsv1alpha1.TargetReference{}
				return pps
			},
			expectErrList: 2,
		},
		{
			name: "invalid per, targetRef Deployment",
			per: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				pps.Spec.TargetReference = appsv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test",
				}
				return pps
			},
			expectErrList: 1,
		},
		{
			name: "invalid per, TopologyConstraint and TopologyPreference are nil",
			per: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				pps.Spec.RequiredPersistentTopology = nil
				pps.Spec.PreferredPersistentTopology = nil
				return pps
			},
			expectErrList: 1,
		},
	}

	decoder := admission.NewDecoder(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	perHandler := PersistentPodStateCreateUpdateHandler{
		Client:  client,
		Decoder: decoder,
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			errList := perHandler.validatingPersistentPodStateFn(cs.per(), nil)
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %s", cs.expectErrList, len(errList), errList.ToAggregate().Error())
			}
		})
	}
}

func TestPerConflictWithOthers(t *testing.T) {
	cases := []struct {
		name          string
		per           func() *appsv1alpha1.PersistentPodState
		otherPers     func() []*appsv1alpha1.PersistentPodState
		expectErrList int
	}{
		{
			name: "no conflict with other pers, and TargetReference",
			per: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				return pps
			},
			otherPers: func() []*appsv1alpha1.PersistentPodState {
				pps1 := ppsDemo.DeepCopy()
				pps1.Name = "pps1"
				pps1.Spec.TargetReference = appsv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "statefulset-test1",
				}
				pps2 := ppsDemo.DeepCopy()
				pps2.Name = "pps2"
				pps2.Spec.TargetReference = appsv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "statefulset-test2",
				}
				return []*appsv1alpha1.PersistentPodState{pps1, pps2}
			},
			expectErrList: 0,
		},
		{
			name: "invalid conflict with other pers, and TargetReference",
			per: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				return pps
			},
			otherPers: func() []*appsv1alpha1.PersistentPodState {
				pps1 := ppsDemo.DeepCopy()
				pps1.Name = "pps1"
				pps1.Spec.TargetReference = appsv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "statefulset-test",
				}
				pps2 := ppsDemo.DeepCopy()
				pps2.Name = "pps2"
				pps2.Spec.TargetReference = appsv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "statefulset-test2",
				}
				return []*appsv1alpha1.PersistentPodState{pps1, pps2}
			},
			expectErrList: 1,
		},
		{
			name: "no conflict with other pers, and Selector, other namespace",
			per: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				return pps
			},
			otherPers: func() []*appsv1alpha1.PersistentPodState {
				pps1 := ppsDemo.DeepCopy()
				pps1.Name = "pps1"
				pps1.Namespace = "pps1"
				return []*appsv1alpha1.PersistentPodState{pps1}
			},
			expectErrList: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme)
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			for _, pps := range cs.otherPers() {
				client.Create(context.TODO(), pps)
			}
			perHandler := PersistentPodStateCreateUpdateHandler{
				Client:  client,
				Decoder: decoder,
			}
			errList := perHandler.validatingPersistentPodStateFn(cs.per(), nil)
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %s", cs.expectErrList, len(errList), errList.ToAggregate().Error())
			}
		})
	}
}

func TestValidatingUpdatePer(t *testing.T) {
	cases := []struct {
		name          string
		old           func() *appsv1alpha1.PersistentPodState
		obj           func() *appsv1alpha1.PersistentPodState
		expectErrList int
	}{
		{
			name: "valid per, targetRef not changed",
			old: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				return pps
			},
			obj: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				return pps
			},
			expectErrList: 0,
		},
		{
			name: "invalid per, targetRef changed",
			old: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				return pps
			},
			obj: func() *appsv1alpha1.PersistentPodState {
				pps := ppsDemo.DeepCopy()
				pps.Spec.TargetReference = appsv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "statefulset-changed",
				}
				return pps
			},
			expectErrList: 1,
		},
	}

	decoder := admission.NewDecoder(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	perHandler := PersistentPodStateCreateUpdateHandler{
		Client:  client,
		Decoder: decoder,
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			errList := perHandler.validatingPersistentPodStateFn(cs.obj(), cs.old())
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %s", cs.expectErrList, len(errList), errList.ToAggregate().Error())
			}
		})
	}
}
