/*
Copyright 2025 The Kruise Authors.

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

package podprobemarker

import (
	"context"
	"strings"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetPodProbeMarkerForPod(reader client.Reader, pod *corev1.Pod) ([]*appsalphav1.PodProbeMarker, error) {
	var ppms []*appsalphav1.PodProbeMarker
	// new pod have annotation kruise.io/podprobemarker-list
	if str, ok := pod.Annotations[appsalphav1.PodProbeMarkerListAnnotationKey]; ok && str != "" {
		names := strings.Split(str, ",")
		for _, name := range names {
			ppm := &appsalphav1.PodProbeMarker{}
			if err := reader.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: name}, ppm); err != nil {
				klog.ErrorS(err, "Failed to get PodProbeMarker", "name", name)
				continue
			}
			ppms = append(ppms, ppm)
		}
		return ppms, nil
	}

	ppmList := &appsalphav1.PodProbeMarkerList{}
	if err := reader.List(context.TODO(), ppmList, &client.ListOptions{Namespace: pod.Namespace}, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	for i := range ppmList.Items {
		ppm := &ppmList.Items[i]
		// This error is irreversible, so continue
		labelSelector, err := util.ValidatedLabelSelectorAsSelector(ppm.Spec.Selector)
		if err != nil {
			continue
		}
		// If a probemarker with a nil or empty selector creeps in, it should match nothing, not everything.
		if labelSelector.Empty() || !labelSelector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		ppms = append(ppms, ppm)
	}
	return ppms, nil
}
