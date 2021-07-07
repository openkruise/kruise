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

package workloadspread

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

const (
	// MatchedWorkloadSpreadSubsetAnnotations matched pod workloadSpread
	MatchedWorkloadSpreadSubsetAnnotations = "apps.kruise.io/matched-workloadspread"

	PodDeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"

	PodDeletionCostDefault  = "0"
	PodDeletionCostPositive = "100"
	PodDeletionCostNegative = "-100"
)

type InjectWorkloadSpread struct {
	// matched WorkloadSpread.Name
	Name string `json:"name"`
	// Subset.Name
	Subset string `json:"subset"`
	// generate id if the Pod' name is nil.
	UID string `json:"uid,omitempty"`
}

func VerifyGroupKind(ref *appsv1alpha1.TargetReference, expectedKind string, expectedGroups []string) (bool, error) {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		klog.Errorf("failed to parse GroupVersion for apiVersion (%s): %s", ref.APIVersion, err.Error())
		return false, err
	}

	if ref.Kind != expectedKind {
		return false, nil
	}

	for _, group := range expectedGroups {
		if group == gv.Group {
			return true, nil
		}
	}

	return false, nil
}
