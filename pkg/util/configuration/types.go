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

package configuration

import (
	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	SidecarSetPatchPodMetadataWhiteListKey = "SidecarSet_PatchPodMetadata_WhiteList"
	PPSWatchCustomWorkloadWhiteList        = "PPS_Watch_Custom_Workload_WhiteList"
	WSWatchCustomWorkloadWhiteList         = "WorkloadSpread_Watch_Custom_Workload_WhiteList"
)

type SidecarSetPatchMetadataWhiteList struct {
	Rules []SidecarSetPatchMetadataWhiteRule `json:"rules"`
}

type SidecarSetPatchMetadataWhiteRule struct {
	// selector sidecarSet against labels
	// If selector is nil, assume that the rules should apply for every sidecarSets
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	// Support for regular expressions
	AllowedAnnotationKeyExprs []string `json:"allowedAnnotationKeyExprs"`
}

type CustomWorkloadWhiteList struct {
	Workloads []schema.GroupVersionKind `json:"workloads,omitempty"`
}

func (p *CustomWorkloadWhiteList) IsValid(gk metav1.GroupKind) bool {
	for _, workload := range p.Workloads {
		if workload.Group == gk.Group && workload.Kind == gk.Kind {
			return true
		}
	}
	return false
}

func (p *CustomWorkloadWhiteList) ValidateAPIVersionAndKind(apiVersion, kind string) bool {
	if p.IsDefaultSupport(apiVersion, kind) {
		return true
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return false
	}
	gk := metav1.GroupKind{Group: gv.Group, Kind: kind}
	return p.IsValid(gk)
}

func (p *CustomWorkloadWhiteList) IsDefaultSupport(apiVersion, kind string) bool {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return false
	}
	if (gv.Group == v1alpha1.GroupVersion.Group || gv.Group == appsv1.GroupName) && kind == "StatefulSet" {
		return true
	}
	return false
}

type WSCustomWorkloadWhiteList struct {
	Workloads []CustomWorkload `json:"workloads,omitempty"`
}

type CustomWorkload struct {
	schema.GroupVersionKind `json:",inline"`
	SubResources            []schema.GroupVersionKind `json:"subResources,omitempty"`
	// ReplicasPath is the replicas field path of this type of workload, such as "spec.replicas"
	ReplicasPath string `json:"replicasPath,omitempty"`
}
