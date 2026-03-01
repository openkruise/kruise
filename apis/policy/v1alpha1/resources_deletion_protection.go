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

package v1alpha1

const (
	// DeletionProtectionKey is a key in object labels and its value can be Always and Cascading.
	// Currently supports Namespace, CustomResourcesDefinition, Deployment, StatefulSet, ReplicaSet, CloneSet, Advanced StatefulSet, UnitedDeployment.
	DeletionProtectionKey = "policy.kruise.io/delete-protection"

	// DeletionProtectionTypeAlways indicates this object will always be forbidden to be deleted, unless the label is removed.
	DeletionProtectionTypeAlways = "Always"
	// DeletionProtectionTypeCascading indicates this object will be forbidden to be deleted, if it has active resources owned.
	DeletionProtectionTypeCascading = "Cascading"
)
