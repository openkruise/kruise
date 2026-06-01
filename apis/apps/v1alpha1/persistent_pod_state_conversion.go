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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func (src *PersistentPodState) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*appsv1beta1.PersistentPodState)
	if !ok {
		return fmt.Errorf("unsupported hub type %T", dstRaw)
	}
	return convertPersistentPodStateToV1beta1(src, dst)
}

func (dst *PersistentPodState) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*appsv1beta1.PersistentPodState)
	if !ok {
		return fmt.Errorf("unsupported hub type %T", srcRaw)
	}
	return convertPersistentPodStateFromV1beta1(src, dst)
}

func convertPersistentPodStateToV1beta1(src *PersistentPodState, dst *appsv1beta1.PersistentPodState) error {
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec = appsv1beta1.PersistentPodStateSpec{
		TargetReference: appsv1beta1.TargetReference{
			APIVersion: src.Spec.TargetReference.APIVersion,
			Kind:       src.Spec.TargetReference.Kind,
			Name:       src.Spec.TargetReference.Name,
		},
		PersistentPodStateRetentionPolicy: appsv1beta1.PersistentPodStateRetentionPolicyType(src.Spec.PersistentPodStateRetentionPolicy),
	}
	if len(src.Spec.PersistentPodAnnotations) > 0 {
		dst.Spec.PersistentPodAnnotations = make([]appsv1beta1.PersistentPodAnnotation, len(src.Spec.PersistentPodAnnotations))
		for i, item := range src.Spec.PersistentPodAnnotations {
			dst.Spec.PersistentPodAnnotations[i] = appsv1beta1.PersistentPodAnnotation{Key: item.Key}
		}
	}
	if src.Spec.RequiredPersistentTopology != nil {
		dst.Spec.RequiredPersistentTopology = &appsv1beta1.NodeTopologyTerm{
			Keys: append([]string(nil), src.Spec.RequiredPersistentTopology.NodeTopologyKeys...),
		}
	}
	if len(src.Spec.PreferredPersistentTopology) > 0 {
		dst.Spec.PreferredPersistentTopology = make([]appsv1beta1.PreferredTopologyTerm, len(src.Spec.PreferredPersistentTopology))
		for i, item := range src.Spec.PreferredPersistentTopology {
			dst.Spec.PreferredPersistentTopology[i] = appsv1beta1.PreferredTopologyTerm{
				Weight: item.Weight,
				Preference: appsv1beta1.NodeTopologyTerm{
					Keys: append([]string(nil), item.Preference.NodeTopologyKeys...),
				},
			}
		}
	}
	dst.Status = convertPersistentPodStateStatusToV1beta1(src.Status)
	return nil
}

func convertPersistentPodStateFromV1beta1(src *appsv1beta1.PersistentPodState, dst *PersistentPodState) error {
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec = PersistentPodStateSpec{
		TargetReference: TargetReference{
			APIVersion: src.Spec.TargetReference.APIVersion,
			Kind:       src.Spec.TargetReference.Kind,
			Name:       src.Spec.TargetReference.Name,
		},
		PersistentPodStateRetentionPolicy: PersistentPodStateRetentionPolicyType(src.Spec.PersistentPodStateRetentionPolicy),
	}
	if len(src.Spec.PersistentPodAnnotations) > 0 {
		dst.Spec.PersistentPodAnnotations = make([]PersistentPodAnnotation, len(src.Spec.PersistentPodAnnotations))
		for i, item := range src.Spec.PersistentPodAnnotations {
			dst.Spec.PersistentPodAnnotations[i] = PersistentPodAnnotation{Key: item.Key}
		}
	}
	if src.Spec.RequiredPersistentTopology != nil {
		dst.Spec.RequiredPersistentTopology = &NodeTopologyTerm{
			NodeTopologyKeys: append([]string(nil), src.Spec.RequiredPersistentTopology.Keys...),
		}
	}
	if len(src.Spec.PreferredPersistentTopology) > 0 {
		dst.Spec.PreferredPersistentTopology = make([]PreferredTopologyTerm, len(src.Spec.PreferredPersistentTopology))
		for i, item := range src.Spec.PreferredPersistentTopology {
			dst.Spec.PreferredPersistentTopology[i] = PreferredTopologyTerm{
				Weight: item.Weight,
				Preference: NodeTopologyTerm{
					NodeTopologyKeys: append([]string(nil), item.Preference.Keys...),
				},
			}
		}
	}
	dst.Status = convertPersistentPodStateStatusFromV1beta1(src.Status)
	return nil
}

func convertPersistentPodStateStatusToV1beta1(src PersistentPodStateStatus) appsv1beta1.PersistentPodStateStatus {
	dst := appsv1beta1.PersistentPodStateStatus{
		ObservedGeneration: src.ObservedGeneration,
	}
	if len(src.PodStates) == 0 {
		return dst
	}
	dst.PodStates = make(map[string]appsv1beta1.PodState, len(src.PodStates))
	for name, state := range src.PodStates {
		podState := appsv1beta1.PodState{NodeName: state.NodeName}
		if len(state.NodeTopologyLabels) > 0 {
			podState.NodeTopologyLabels = make(map[string]string, len(state.NodeTopologyLabels))
			for k, v := range state.NodeTopologyLabels {
				podState.NodeTopologyLabels[k] = v
			}
		}
		if len(state.Annotations) > 0 {
			podState.Annotations = make(map[string]string, len(state.Annotations))
			for k, v := range state.Annotations {
				podState.Annotations[k] = v
			}
		}
		dst.PodStates[name] = podState
	}
	return dst
}

func convertPersistentPodStateStatusFromV1beta1(src appsv1beta1.PersistentPodStateStatus) PersistentPodStateStatus {
	dst := PersistentPodStateStatus{
		ObservedGeneration: src.ObservedGeneration,
	}
	if len(src.PodStates) == 0 {
		return dst
	}
	dst.PodStates = make(map[string]PodState, len(src.PodStates))
	for name, state := range src.PodStates {
		podState := PodState{NodeName: state.NodeName}
		if len(state.NodeTopologyLabels) > 0 {
			podState.NodeTopologyLabels = make(map[string]string, len(state.NodeTopologyLabels))
			for k, v := range state.NodeTopologyLabels {
				podState.NodeTopologyLabels[k] = v
			}
		}
		if len(state.Annotations) > 0 {
			podState.Annotations = make(map[string]string, len(state.Annotations))
			for k, v := range state.Annotations {
				podState.Annotations[k] = v
			}
		}
		dst.PodStates[name] = podState
	}
	return dst
}
