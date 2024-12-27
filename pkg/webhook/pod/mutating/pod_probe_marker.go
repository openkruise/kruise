/*
Copyright 2024 The Kruise Authors.

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
	"fmt"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/podprobemarker"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kube-openapi/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// mutating relate-pub annotation in pod
func (h *PodCreateHandler) podProbeMarkerMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) (skip bool, err error) {
	if len(req.AdmissionRequest.SubResource) > 0 || req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}
	ppms, err := podprobemarker.GetPodProbeMarkerForPod(h.Client, pod)
	if err != nil {
		return false, err
	} else if len(ppms) == 0 {
		return true, nil
	}

	containers := sets.NewString()
	for _, c := range pod.Spec.Containers {
		containers.Insert(c.Name)
	}
	for _, c := range pod.Spec.InitContainers {
		if util.IsRestartableInitContainer(&c) {
			containers.Insert(c.Name)
		}
	}

	matchedPodProbeMarkerName := sets.NewString()
	matchedProbeKey := sets.NewString()
	matchedConditions := sets.NewString()
	matchedProbes := make([]appsv1alpha1.PodContainerProbe, 0)
	for i := range ppms {
		obj := ppms[i]
		for i := range obj.Spec.Probes {
			probe := obj.Spec.Probes[i]
			key := fmt.Sprintf("%s/%s", probe.ContainerName, probe.Name)
			if matchedConditions.Has(probe.PodConditionType) || matchedProbeKey.Has(key) || !containers.Has(probe.ContainerName) || probe.PodConditionType == "" {
				continue
			}
			// No need to pass in marker related fields
			probe.MarkerPolicy = nil
			matchedProbes = append(matchedProbes, probe)
			matchedProbeKey.Insert(key)
			matchedConditions.Insert(probe.PodConditionType)
			matchedPodProbeMarkerName.Insert(obj.Name)
		}
	}
	if len(matchedProbes) == 0 {
		return true, nil
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	body := util.DumpJSON(matchedProbes)
	pod.Annotations[appsv1alpha1.PodProbeMarkerAnnotationKey] = body
	pod.Annotations[appsv1alpha1.PodProbeMarkerListAnnotationKey] = strings.Join(matchedPodProbeMarkerName.List(), ",")
	klog.V(3).InfoS("mutating add pod annotation", "namespace", pod.Namespace, "name", pod.Name, "key", appsv1alpha1.PodProbeMarkerAnnotationKey, "value", body)
	return false, nil
}
