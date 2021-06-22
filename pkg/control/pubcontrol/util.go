/*
Copyright 2021 The Kruise Authors.

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

package pubcontrol

import (
	"context"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetPodUnavailableBudgetForPod(kClient client.Client, finders *controllerfinder.ControllerFinder, pod *corev1.Pod) (*policyv1alpha1.PodUnavailableBudget, error) {
	var err error
	if len(pod.Labels) == 0 {
		return nil, nil
	}

	pubList := &policyv1alpha1.PodUnavailableBudgetList{}
	if err = kClient.List(context.TODO(), pubList, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return nil, err
	}

	var matchedPubs []policyv1alpha1.PodUnavailableBudget
	for _, pub := range pubList.Items {
		// if targetReference isn't nil, priority to take effect
		if pub.Spec.TargetReference != nil {
			targetRef := pub.Spec.TargetReference
			// check whether APIVersion, Kind, Name is equal
			ref := metav1.GetControllerOf(pod)
			if ref == nil {
				continue
			}
			// recursive fetch pod reference, e.g. ref.Kind=Replicas, return podRef.Kind=Deployment
			podRef, err := finders.GetScaleAndSelectorForRef(ref.APIVersion, ref.Kind, pod.Namespace, ref.Name, ref.UID)
			if err != nil {
				return nil, err
			}
			pubRef, err := finders.GetScaleAndSelectorForRef(targetRef.APIVersion, targetRef.Kind, pub.Namespace, targetRef.Name, "")
			if err != nil {
				return nil, err
			}

			// belongs the same workload
			if isReferenceEqual(podRef, pubRef) {
				matchedPubs = append(matchedPubs, pub)
			}
		} else {
			// This error is irreversible, so continue
			labelSelector, err := util.GetFastLabelSelector(pub.Spec.Selector)
			if err != nil {
				continue
			}
			// If a PUB with a nil or empty selector creeps in, it should match nothing, not everything.
			if labelSelector.Empty() || !labelSelector.Matches(labels.Set(pod.Labels)) {
				continue
			}
			matchedPubs = append(matchedPubs, pub)
		}
	}

	if len(matchedPubs) == 0 {
		klog.V(6).Infof("could not find PodUnavailableBudget for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
		return nil, nil
	}
	if len(matchedPubs) > 1 {
		klog.Warningf("Pod %q/%q matches multiple PodUnavailableBudgets. Choose %q arbitrarily.", pod.Namespace, pod.Name, matchedPubs[0].Name)
	}

	return &matchedPubs[0], nil
}

// check APIVersion, Kind, Name
func isReferenceEqual(ref1, ref2 *controllerfinder.ScaleAndSelector) bool {
	gv1, err := schema.ParseGroupVersion(ref1.APIVersion)
	if err != nil {
		return false
	}
	gv2, err := schema.ParseGroupVersion(ref2.APIVersion)
	if err != nil {
		return false
	}
	return gv1.Group == gv2.Group && ref1.Kind == ref2.Kind &&
		ref1.Name == ref2.Name && ref1.UID == ref2.UID
}
