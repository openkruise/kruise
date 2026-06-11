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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
)

func TestPodUnavailableBudgetConvertToV1beta1PromotesDeprecatedAnnotations(t *testing.T) {
	maxUnavailable := intstr.FromString("50%")
	src := &PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupVersion.String(),
			Kind:       "PodUnavailableBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pub-demo",
			Namespace: "default",
			Annotations: map[string]string{
				PubProtectOperationAnnotation:     "DELETE,EVICT",
				PubProtectTotalReplicasAnnotation: "15",
			},
		},
		Spec: PodUnavailableBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "demo"},
			},
			MaxUnavailable: &maxUnavailable,
		},
	}

	dst := &policyv1beta1.PodUnavailableBudget{}
	require.NoError(t, src.ConvertTo(dst))

	assert.Equal(t, policyv1beta1.GroupVersion.String(), dst.APIVersion)
	assert.Equal(t, "PodUnavailableBudget", dst.Kind)
	assert.Equal(t, []policyv1beta1.PubOperation{
		policyv1beta1.PubDeleteOperation,
		policyv1beta1.PubEvictOperation,
	}, dst.Spec.ProtectOperations)
	require.NotNil(t, dst.Spec.ProtectTotalReplicas)
	assert.Equal(t, int32(15), *dst.Spec.ProtectTotalReplicas)
}

func TestPodUnavailableBudgetConvertFromV1beta1BackfillsDeprecatedAnnotations(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	totalReplicas := int32(9)
	src := &policyv1beta1.PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyv1beta1.GroupVersion.String(),
			Kind:       "PodUnavailableBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pub-demo",
			Namespace: "default",
			Annotations: map[string]string{
				"keep": "me",
			},
		},
		Spec: policyv1beta1.PodUnavailableBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "demo"},
			},
			MaxUnavailable:      &maxUnavailable,
			ProtectOperations:   []policyv1beta1.PubOperation{policyv1beta1.PubDeleteOperation, policyv1beta1.PubResizeOperation},
			ProtectTotalReplicas: &totalReplicas,
			IgnoredPodSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"kruise.io/force-deletable": "true"},
			},
		},
	}

	dst := &PodUnavailableBudget{}
	require.NoError(t, dst.ConvertFrom(src))

	assert.Equal(t, GroupVersion.String(), dst.APIVersion)
	assert.Equal(t, "PodUnavailableBudget", dst.Kind)
	assert.Equal(t, "DELETE,RESIZE", dst.Annotations[PubProtectOperationAnnotation])
	assert.Equal(t, "9", dst.Annotations[PubProtectTotalReplicasAnnotation])
	assert.Equal(t, "me", dst.Annotations["keep"])
	assert.Nil(t, dst.Spec.TargetReference)
	assert.Nil(t, dst.Spec.MinAvailable)
}

func TestPodUnavailableBudgetConvertFromV1beta1DropsDefaultProtectOperationsAnnotation(t *testing.T) {
	src := &policyv1beta1.PodUnavailableBudget{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				PubProtectOperationAnnotation: "DELETE",
			},
		},
	}

	dst := &PodUnavailableBudget{}
	require.NoError(t, dst.ConvertFrom(src))
	_, exists := dst.Annotations[PubProtectOperationAnnotation]
	assert.False(t, exists)
}
