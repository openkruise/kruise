/*
Copyright 2019 The Kruise Authors.

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

package adapter

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

type Adapter interface {
	// NewResourceObject creates an empty subset object.
	NewResourceObject() client.Object
	// NewResourceListObject creates an empty subset list object.
	NewResourceListObject() client.ObjectList
	// GetStatusObservedGeneration returns the observed generation of the subset.
	GetStatusObservedGeneration(subset metav1.Object) int64
	// GetSubsetPods returns all pods of the subset workload.
	GetSubsetPods(obj metav1.Object) ([]*corev1.Pod, error)
	// GetSpecReplicas returns the replicas information of the subset workload.
	GetSpecReplicas(obj metav1.Object) *int32
	// GetSpecPartition returns the partition information of the subset workload if possible.
	GetSpecPartition(obj metav1.Object, pods []*corev1.Pod) *int32
	// GetStatusReplicas returns the replicas from the subset workload status.
	GetStatusReplicas(obj metav1.Object) int32
	// GetStatusReadyReplicas returns the ready replicas information from the subset workload status.
	GetStatusReadyReplicas(obj metav1.Object) int32
	// GetSubsetFailure returns failure information of the subset.
	GetSubsetFailure() *string
	// ApplySubsetTemplate updates the subset to the latest revision.
	ApplySubsetTemplate(ud *alpha1.UnitedDeployment, subsetName, revision string, replicas, partition int32, subset runtime.Object) error
	// PostUpdate does some works after subset updated
	PostUpdate(ud *alpha1.UnitedDeployment, subset runtime.Object, revision string, partition int32) error
}
