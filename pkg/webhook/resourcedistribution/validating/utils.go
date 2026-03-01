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
	ResourceHashCodeAnnotation           = "kruise.io/resourcedistribution.resource.hashcode"
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
)

// isSupportedGVK check whether object is supported by ResourceDistribution
func isSupportedGK(object runtime.Object) bool {
	if object == nil {
		return false
	}
	objGK := object.GetObjectKind().GroupVersionKind().GroupKind()
	for _, gk := range supportedGKList {
		if reflect.DeepEqual(gk, objGK) {
			return true
		}
	}
	return false
}

// haveSameGVKAndName return true if two resources have the same group, version, kind and name
func haveSameGVKAndName(resource, otherResource runtime.Object) bool {
	Name, anotherName := ConvertToUnstructured(resource).GetName(), ConvertToUnstructured(otherResource).GetName()
	GVK, anotherGVK := resource.GetObjectKind().GroupVersionKind(), otherResource.GetObjectKind().GroupVersionKind()
	return Name == anotherName && reflect.DeepEqual(GVK, anotherGVK)
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
