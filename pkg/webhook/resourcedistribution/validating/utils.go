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

package validating

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	DefaultNamespace                     = "default"
	ResourceHashCodeAnnotation           = "kruise.io/resourcedistribution.resource.hashcode"
	ResourceDistributedTimestamp         = "kruise.io/resourcedistribution.resource.distributed.timestamp"
	SourceResourceDistributionOfResource = "kruise.io/resourcedistribution.resource.from"
)

var (
	// supportedGKList is a list that contains all supported resource group, and kind
	// Support CustomResourceDefinition
	/* ADD NEW RESOURCE TYPE HERE*/
	supportedGKList = []schema.GroupKind{
		{Group: "", Kind: "Secret"},
		{Group: "", Kind: "ConfigMap"},
	}

	// ForbiddenNamespaces is a list that contains all forbidden namespaces
	// Resources will never be distributed to these namespaces
	// reused by controller
	/* ADD NEW FORBIDDEN NAMESPACE HERE*/
	ForbiddenNamespaces = []string{
		"kube-system",
		"kube-public",
	}
)

// isSupportedGVK check whether object is supported by ResourceDistribution
func isSupportedGK(object runtime.Object) bool {
	if object == nil {
		return false
	}
	objGVK := object.GetObjectKind().GroupVersionKind().GroupKind()
	for _, gvk := range supportedGKList {
		if reflect.DeepEqual(gvk, objGVK) {
			return true
		}
	}
	return false
}

// isForbiddenNamespace check whether the namespace is forbidden
func isForbiddenNamespace(namespace string) bool {
	for _, forbiddenNamespace := range ForbiddenNamespaces {
		if namespace == forbiddenNamespace {
			return true
		}
	}
	return false
}

// haveSameGKAndName return true if two resources have the same group, kind and name
func haveSameGKAndName(resource, otherResource runtime.Object) bool {
	Name, anotherName := ConvertToUnstructured(resource).GetName(), ConvertToUnstructured(otherResource).GetName()
	GK, anotherGK := resource.GetObjectKind().GroupVersionKind().GroupKind(), otherResource.GetObjectKind().GroupVersionKind().GroupKind()
	return Name == anotherName && reflect.DeepEqual(GK, anotherGK)
}

// ConvertToUnstructured receive runtime.Object, return *unstructured.Unstructured
// reused by controller
func ConvertToUnstructured(resourceObject runtime.Object) (resource *unstructured.Unstructured) {
	switch unstructuredResource := resourceObject.(type) {
	case *unstructured.Unstructured:
		return unstructuredResource
	default:
		return nil
	}
}

// DeserializeResource receive yaml of resource, return runtime.Object
// reused by controller
func DeserializeResource(resourceRawExtension *runtime.RawExtension, fldPath *field.Path) (resource runtime.Object, allErrs field.ErrorList) {
	// 1. check whether resource yaml is empty
	if len(resourceRawExtension.Raw) == 0 {
		return nil, append(allErrs, field.Invalid(fldPath, resource, "empty resource is not allowed"))
	}

	// 2. deserialize resource
	resource, _, err := unstructured.UnstructuredJSONScheme.Decode(resourceRawExtension.Raw, nil, nil)
	if err != nil {
		allErrs = append(allErrs, field.InternalError(fldPath, fmt.Errorf("failed to deserialize resource, please check your spec.resource, err %v", err)))
	}
	return
}
