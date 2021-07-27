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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This file holds all operations about varying resources in this package;
// If you want to add new-type resource, you only need to modified this file.

// UnifiedResource abstracts all behaviors of Resource
type UnifiedResource interface {
	GetName() string
	GetObject() *runtime.Object
	GetObjectCopy() runtime.Object
	GetObjectMeta() *metav1.ObjectMeta
	GetGroupVersionKind() *schema.GroupVersionKind
}

// Resource holds specific object
type Resource struct {
	Object           runtime.Object
	GroupVersionKind schema.GroupVersionKind
}

func (r *Resource) GetName() string {
	switch resource := r.Object.(type) {
	case *corev1.Secret:
		return resource.Name
	case *corev1.ConfigMap:
		return resource.Name
	default:
		return ""
	}
}

func (r *Resource) GetObjectMeta() *metav1.ObjectMeta {
	switch resource := r.Object.(type) {
	case *corev1.Secret:
		return &resource.ObjectMeta
	case *corev1.ConfigMap:
		return &resource.ObjectMeta
	default:
		return nil
	}
}

func (r *Resource) GetObject() *runtime.Object {
	return &r.Object
}

func (r *Resource) GetObjectCopy() runtime.Object {
	return r.Object.DeepCopyObject()
}

func (r *Resource) GetGroupVersionKind() *schema.GroupVersionKind {
	return &r.GroupVersionKind
}

// MakeUnifiedResourceFromObject receives runtime.Object, return UnifiedResource
func MakeUnifiedResourceFromObject(resourceObject runtime.Object, fldPath *field.Path) (resource UnifiedResource, allErrs field.ErrorList) {
	switch specificObject := resourceObject.(type) {
	case *corev1.Secret, *corev1.ConfigMap:
		resource = UnifiedResource(&Resource{
			Object:           specificObject,
			GroupVersionKind: specificObject.GetObjectKind().GroupVersionKind(),
		})
	default:
		allErrs = append(allErrs, field.Invalid(fldPath, nil, "unsupported resource kind"))
	}

	return
}

// DecodeResource receives yaml of resource, return UnifiedResource
func DecodeResource(resourceYAML *runtime.RawExtension, fldPath *field.Path) (resource UnifiedResource, allErrs field.ErrorList) {
	//Decode Yaml
	if resourceYAML.Raw == nil {
		allErrs = append(allErrs, field.Invalid(fldPath, nil, "empty resource is not allowed"))
		return
	}
	resourceObject, _, err := legacyscheme.Codecs.UniversalDeserializer().Decode(resourceYAML.Raw, nil, nil)
	if err != nil {
		allErrs = append(allErrs, field.InternalError(fldPath, fmt.Errorf("failed to deserialize resource, err %v", err)))
	}

	// convert to specific resource
	resource, errs := MakeUnifiedResourceFromObject(resourceObject, fldPath)
	allErrs = append(allErrs, errs...)

	return
}

// GetConflictingNamespaces returns all conflicting namespaces that contain resource with the same kind and name as Resource
func GetConflictingNamespaces(handlerClient client.Client, resource UnifiedResource, targetNamespaces []string, rdName string, fldPath *field.Path) (conflictingNamespaces []string, allErrs field.ErrorList) {
	for _, namespace := range targetNamespaces {
		switch instance := resource.GetObjectCopy().(type) {
		case *corev1.Secret, *corev1.ConfigMap:
			// check whether conflict resource exists
			if err := handlerClient.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: resource.GetName()}, instance); err != nil {
				if errors.IsNotFound(err) {
					continue
				} else {
					allErrs = append(allErrs, field.InternalError(field.NewPath(""), fmt.Errorf("get secret failed, err: %v", err)))
				}
			}
			uResource, errs := MakeUnifiedResourceFromObject(instance, fldPath)
			// check whether the existing resource belongs to this ResourceDistribution
			if len(errs) != 0 || uResource.GetObjectMeta().Annotations["kruise.io/from-resource-distribution"] == rdName {
				allErrs = append(allErrs, errs...)
			} else {
				conflictingNamespaces = append(conflictingNamespaces, namespace)
			}
		}
	}

	return
}
