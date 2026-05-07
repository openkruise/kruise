/*
Copyright 2026 The Kruise Authors.

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

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestResourceDistributionConvertToV1beta1(t *testing.T) {
	src := &ResourceDistribution{
		TypeMeta: metav1.TypeMeta{APIVersion: GroupVersion.String(), Kind: "ResourceDistribution"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "demo",
			Labels: map[string]string{"app": "demo"},
		},
		Spec: ResourceDistributionSpec{
			Resource: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"demo"},"data":{"k":"v"}}`),
			},
			Targets: ResourceDistributionTargets{
				AllNamespaces: true,
				IncludedNamespaces: ResourceDistributionTargetNamespaces{
					List: []ResourceDistributionNamespace{{Name: "ns-1"}},
				},
				ExcludedNamespaces: ResourceDistributionTargetNamespaces{
					List: []ResourceDistributionNamespace{{Name: "ns-2"}},
				},
			},
		},
		Status: ResourceDistributionStatus{
			Desired:            3,
			Succeeded:          2,
			Failed:             1,
			ObservedGeneration: 7,
			Conditions: []ResourceDistributionCondition{{
				Type:             ResourceDistributionCreateResourceFailed,
				Status:           ResourceDistributionConditionTrue,
				Reason:           "create failed",
				FailedNamespaces: []string{"ns-2"},
			}},
		},
	}

	dst := &appsv1beta1.ResourceDistribution{}
	err := src.ConvertTo(dst)
	assert.NoError(t, err)
	assert.Equal(t, appsv1beta1.GroupVersion.String(), dst.APIVersion)
	assert.Equal(t, "ResourceDistribution", dst.Kind)
	assert.Equal(t, []string{"ns-2"}, dst.Status.Conditions[0].FailedNamespaces)
	assert.Equal(t, "failedNamespaces,omitempty", jsonTagOfFailedNamespacesV1beta1())
}

func TestResourceDistributionConvertFromV1beta1(t *testing.T) {
	src := &appsv1beta1.ResourceDistribution{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1beta1.GroupVersion.String(), Kind: "ResourceDistribution"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "demo",
			Annotations: map[string]string{"keep": "me"},
		},
		Spec: appsv1beta1.ResourceDistributionSpec{
			Resource: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"demo"},"type":"Opaque"}`),
			},
			Targets: appsv1beta1.ResourceDistributionTargets{
				NamespaceLabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"team": "platform"},
				},
			},
		},
		Status: appsv1beta1.ResourceDistributionStatus{
			Conditions: []appsv1beta1.ResourceDistributionCondition{{
				Type:             appsv1beta1.ResourceDistributionNamespaceNotExists,
				Status:           appsv1beta1.ResourceDistributionConditionTrue,
				FailedNamespaces: []string{"ns-a", "ns-b"},
			}},
		},
	}

	dst := &ResourceDistribution{}
	err := dst.ConvertFrom(src)
	assert.NoError(t, err)
	assert.Equal(t, GroupVersion.String(), dst.APIVersion)
	assert.Equal(t, "ResourceDistribution", dst.Kind)
	assert.Equal(t, []string{"ns-a", "ns-b"}, dst.Status.Conditions[0].FailedNamespaces)
	assert.Equal(t, "failedNamespace,omitempty", jsonTagOfFailedNamespacesV1alpha1())
	assert.Equal(t, "me", dst.Annotations["keep"])
}

func jsonTagOfFailedNamespacesV1alpha1() string {
	field, _ := reflectTypeOfResourceDistributionConditionV1alpha1().FieldByName("FailedNamespaces")
	return field.Tag.Get("json")
}

func jsonTagOfFailedNamespacesV1beta1() string {
	field, _ := reflectTypeOfResourceDistributionConditionV1beta1().FieldByName("FailedNamespaces")
	return field.Tag.Get("json")
}

func reflectTypeOfResourceDistributionConditionV1alpha1() reflect.Type {
	return reflect.TypeOf(ResourceDistributionCondition{})
}

func reflectTypeOfResourceDistributionConditionV1beta1() reflect.Type {
	return reflect.TypeOf(appsv1beta1.ResourceDistributionCondition{})
}
