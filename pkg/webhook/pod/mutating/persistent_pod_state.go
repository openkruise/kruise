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

package mutating

import (
	"context"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/configuration"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	InjectedPersistentPodStateKey = "kruise.io/injected-persistent-pod-state"
)

// mutate pod based on static ip
func (h *PodCreateHandler) persistentPodStateMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) (skip bool, err error) {
	// only handler Create Pod Object Request
	if len(req.AdmissionRequest.SubResource) > 0 || req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}

	whiteList, err := configuration.GetPPSWatchCustomWorkloadWhiteList(h.Client)
	if err != nil {
		return false, err
	}
	ref := metav1.GetControllerOf(pod)
	if ref == nil || !whiteList.ValidateAPIVersionAndKind(ref.APIVersion, ref.Kind) {
		return true, nil
	}
	// selector persistentPodState
	persistentPodState := SelectorPersistentPodState(h.Client, appsv1alpha1.TargetReference{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Name:       ref.Name,
	}, pod.Namespace)
	if persistentPodState == nil || len(persistentPodState.Status.PodStates) == 0 {
		return true, nil
	}

	// when data is NotFound, indicates that the pod is created for the first time and the scenario does not require persistent pod state
	podState, ok := persistentPodState.Status.PodStates[pod.Name]
	if !ok || len(podState.NodeTopologyLabels) == 0 {
		return true, nil
	}

	// inject PersistentPodState node affinity in pod
	nodeSelector, preference := createNodeAffinity(persistentPodState.Spec, podState)
	if len(nodeSelector) == 0 && len(preference) == 0 {
		return true, nil
	}

	klog.V(3).Infof("inject node affinity(required: %s, preferred: %s) in pod(%s/%s) for PersistentPodState",
		util.DumpJSON(nodeSelector), util.DumpJSON(preference), pod.Namespace, pod.Name)

	// inject persistentPodState annotation in pod
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[InjectedPersistentPodStateKey] = persistentPodState.Name

	// nodeSelector
	if len(nodeSelector) != 0 {
		if pod.Spec.NodeSelector == nil {
			pod.Spec.NodeSelector = nodeSelector
		} else {
			for k, v := range nodeSelector {
				pod.Spec.NodeSelector[k] = v
			}
		}
	}

	// preferences
	if len(preference) > 0 {
		if pod.Spec.Affinity == nil {
			pod.Spec.Affinity = &corev1.Affinity{}
		}
		if pod.Spec.Affinity.NodeAffinity == nil {
			pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
		}
		pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution, preference...)
	}

	return false, nil
}

// return two parameters:
// 1. required nodeSelector
// 2. preferred []PreferredSchedulingTerm
func createNodeAffinity(spec appsv1alpha1.PersistentPodStateSpec, podState appsv1alpha1.PodState) (map[string]string, []corev1.PreferredSchedulingTerm) {
	// required
	var nodeSelector map[string]string
	if spec.RequiredPersistentTopology != nil {
		nodeSelector = map[string]string{}
		for _, key := range spec.RequiredPersistentTopology.NodeTopologyKeys {
			if value, ok := podState.NodeTopologyLabels[key]; ok {
				nodeSelector[key] = value
			}
		}
	}

	// preferred
	var preferences []corev1.PreferredSchedulingTerm
	for _, item := range spec.PreferredPersistentTopology {
		preference := corev1.PreferredSchedulingTerm{
			Weight: item.Weight,
		}
		for _, key := range item.Preference.NodeTopologyKeys {
			if value, ok := podState.NodeTopologyLabels[key]; ok {
				requirement := corev1.NodeSelectorRequirement{
					Key:      key,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{value},
				}
				preference.Preference.MatchExpressions = append(preference.Preference.MatchExpressions, requirement)
			}
		}
		preferences = append(preferences, preference)
	}

	return nodeSelector, preferences
}

func SelectorPersistentPodState(reader client.Reader, ref appsv1alpha1.TargetReference, ns string) *appsv1alpha1.PersistentPodState {
	ppsList := &appsv1alpha1.PersistentPodStateList{}
	if err := reader.List(context.TODO(), ppsList, &client.ListOptions{Namespace: ns}, utilclient.DisableDeepCopy); err != nil {
		klog.Errorf("List PersistentPodStateList failed: %s", err.Error())
		return nil
	}
	for i := range ppsList.Items {
		pps := &ppsList.Items[i]
		if !pps.DeletionTimestamp.IsZero() {
			continue
		}
		// belongs the same workload
		if util.IsReferenceEqual(ref, pps.Spec.TargetReference) {
			return pps
		}
	}
	return nil
}
