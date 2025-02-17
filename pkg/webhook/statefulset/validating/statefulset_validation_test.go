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

package validating

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	utilpointer "k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestValidateStatefulSet(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	validPodTemplate := v1.PodTemplate{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
				DNSPolicy:     v1.DNSClusterFirst,
				Containers:    []v1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}

	invalidLabels := map[string]string{"NoUppercaseOrSpecialCharsLike=Equals": "b"}
	invalidPodTemplate := v1.PodTemplate{
		Template: v1.PodTemplateSpec{
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
				DNSPolicy:     v1.DNSClusterFirst,
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: invalidLabels,
			},
		},
	}

	invalidTime := int64(60)
	invalidPodTemplate2 := v1.PodTemplate{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"foo": "bar"},
			},
			Spec: v1.PodSpec{
				RestartPolicy:         v1.RestartPolicyOnFailure,
				DNSPolicy:             v1.DNSClusterFirst,
				ActiveDeadlineSeconds: &invalidTime,
			},
		},
	}

	var val1 int32 = 1
	var val2 int32 = 2
	var val3 int32 = 3
	var minus1 int32 = -1
	maxUnavailable1 := intstr.FromInt32(1)
	maxUnavailable120Percent := intstr.FromString("120%")
	successCases := []appsv1beta1.StatefulSet{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.ParallelPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.OnDeleteStatefulSetStrategyType},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
					Type: apps.RollingUpdateStatefulSetStrategyType,
					RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
						return &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition:       &val2,
							PodUpdatePolicy: appsv1beta1.RecreatePodUpdateStrategyType,
							MaxUnavailable:  &maxUnavailable1,
							MinReadySeconds: ptr.To[int32](10),
						}
					}()},
			},
		},
	}

	for i, successCase := range successCases {
		t.Run("success case "+strconv.Itoa(i), func(t *testing.T) {
			setTestDefault(&successCase)
			if errs := validateStatefulSet(&successCase); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	//  TODO: break the failed cases out with expected error instead of just pass with any error
	errorCases := map[string]appsv1beta1.StatefulSet{
		"zero-length ID": {
			ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"missing-namespace": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123"},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"empty selector": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"selector_doesnt_match": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"invalid manifest": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"negative_replicas": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Replicas:            &minus1,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"invalid_label": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Labels: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"invalid_label 2": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Labels: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Template:            invalidPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"invalid_annotation": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Annotations: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"invalid restart policy 1": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						RestartPolicy: v1.RestartPolicyOnFailure,
						DNSPolicy:     v1.DNSClusterFirst,
						Containers:    []v1.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent"}},
					},
					ObjectMeta: metav1.ObjectMeta{
						Labels: validLabels,
					},
				},
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"invalid restart policy 2": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						RestartPolicy: v1.RestartPolicyNever,
						DNSPolicy:     v1.DNSClusterFirst,
						Containers:    []v1.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent"}},
					},
					ObjectMeta: metav1.ObjectMeta{
						Labels: validLabels,
					},
				},
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"invalid update strategy": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: "foo"},
			},
		},
		"empty update strategy": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: ""},
			},
		},
		"invalid rolling update": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.OnDeleteStatefulSetStrategyType,
					RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
						return &appsv1beta1.RollingUpdateStatefulSetStrategy{Partition: &val1}
					}()},
			},
		},
		"invalid rolling update 1": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.OnDeleteStatefulSetStrategyType,
					RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
						return &appsv1beta1.RollingUpdateStatefulSetStrategy{MaxUnavailable: &maxUnavailable120Percent}
					}()},
			},
		},
		"invalid rolling update 2": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.OnDeleteStatefulSetStrategyType,
					RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
						return &appsv1beta1.RollingUpdateStatefulSetStrategy{PodUpdatePolicy: "Unknown"}
					}()},
			},
		},
		"invalid rolling update 3": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.OnDeleteStatefulSetStrategyType,
					RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
						return &appsv1beta1.RollingUpdateStatefulSetStrategy{PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType}
					}()},
			},
		},
		"negative partition": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType,
					RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
						return &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition:       &minus1,
							PodUpdatePolicy: appsv1beta1.RecreatePodUpdateStrategyType,
							MaxUnavailable:  &maxUnavailable1,
							MinReadySeconds: ptr.To[int32](1),
						}
					}()},
			},
		},
		"negative minReadySeconds": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType,
					RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
						Partition:       &minus1,
						PodUpdatePolicy: appsv1beta1.RecreatePodUpdateStrategyType,
						MaxUnavailable:  &maxUnavailable1,
						MinReadySeconds: ptr.To[int32](-1),
					},
				},
			},
		},
		"too large minReadySeconds": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType,
					RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
						Partition:       &minus1,
						PodUpdatePolicy: appsv1beta1.RecreatePodUpdateStrategyType,
						MaxUnavailable:  &maxUnavailable1,
						MinReadySeconds: ptr.To[int32](appsv1beta1.MaxMinReadySeconds + 1),
					},
				},
			},
		},
		"empty pod management policy": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: "",
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"invalid pod management policy": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: "foo",
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
				Replicas:            &val3,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
		"set active deadline seconds": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: appsv1beta1.StatefulSetSpec{
				PodManagementPolicy: "foo",
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            invalidPodTemplate2.Template,
				Replicas:            &val3,
				UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
			},
		},
	}

	for k, v := range errorCases {
		t.Run(k, func(t *testing.T) {
			setTestDefault(&v)
			errs := validateStatefulSet(&v)
			if len(errs) == 0 {
				t.Errorf("expected failure for %s", k)
			}

			for i := range errs {
				f := errs[i].Field
				if !strings.HasPrefix(f, "spec.template.") &&
					f != "metadata.name" &&
					f != "metadata.namespace" &&
					f != "spec.selector" &&
					f != "spec.template" &&
					f != "GCEPersistentDisk.ReadOnly" &&
					f != "spec.replicas" &&
					f != "spec.template.labels" &&
					f != "metadata.annotations" &&
					f != "metadata.labels" &&
					f != "status.replicas" &&
					f != "spec.updateStrategy" &&
					f != "spec.updateStrategy.rollingUpdate" &&
					f != "spec.updateStrategy.rollingUpdate.partition" &&
					f != "spec.updateStrategy.rollingUpdate.maxUnavailable" &&
					f != "spec.updateStrategy.rollingUpdate.minReadySeconds" &&
					f != "spec.updateStrategy.rollingUpdate.podUpdatePolicy" &&
					f != "spec.template.spec.readinessGates" &&
					f != "spec.podManagementPolicy" &&
					f != "spec.template.spec.activeDeadlineSeconds" {
					t.Errorf("%s: missing prefix for: %v", k, errs[i])
				}
			}
		})
	}
}

func TestValidateStatefulSetUpdate(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	validPodTemplate1 := v1.PodTemplate{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
				DNSPolicy:     v1.DNSClusterFirst,
				Containers:    []v1.Container{{Name: "abc", Image: "image:v1", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}
	validPodTemplate2 := v1.PodTemplate{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
				DNSPolicy:     v1.DNSClusterFirst,
				Containers:    []v1.Container{{Name: "abc", Image: "image:v2", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}

	validVolumeClaimTemplate := func(size string) v1.PersistentVolumeClaim {
		return v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
			Spec: v1.PersistentVolumeClaimSpec{
				StorageClassName: utilpointer.String("foo/bar"),
				AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				Resources: v1.VolumeResourceRequirements{Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceStorage: resource.MustParse(size),
				}},
			},
		}
	}

	successCases := []struct {
		old *appsv1beta1.StatefulSet
		new *appsv1beta1.StatefulSet
	}{
		{
			old: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					ResourceVersion: "1",
				},
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:             ptr.To[int32](5),
					RevisionHistoryLimit: ptr.To[int32](5),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(1),
					},
					Lifecycle:            &appspub.Lifecycle{PreDelete: &appspub.LifecycleHook{FinalizersHandler: []string{"foo/bar"}}},
					Template:             validPodTemplate1.Template,
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{validVolumeClaimTemplate("30Gi")},
					ScaleStrategy:        &appsv1beta1.StatefulSetScaleStrategy{MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1}},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type:          apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{Partition: ptr.To[int32](5)},
					},
					PersistentVolumeClaimRetentionPolicy: &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
						WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
						WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
					},
				},
			},
			new: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					ResourceVersion: "1",
				},
				Spec: appsv1beta1.StatefulSetSpec{
					Replicas:             ptr.To[int32](10),
					RevisionHistoryLimit: ptr.To[int32](10),
					ReserveOrdinals: []intstr.IntOrString{
						intstr.FromInt32(2),
					},
					Lifecycle:            &appspub.Lifecycle{PreDelete: &appspub.LifecycleHook{FinalizersHandler: []string{"foo/hello"}}},
					Template:             validPodTemplate2.Template,
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{validVolumeClaimTemplate("60Gi")},
					ScaleStrategy:        &appsv1beta1.StatefulSetScaleStrategy{MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2}},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type:          apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{Partition: ptr.To[int32](10)},
					},
					PersistentVolumeClaimRetentionPolicy: &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
						WhenScaled:  appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
						WhenDeleted: appsv1beta1.RetainPersistentVolumeClaimRetentionPolicyType,
					},
				},
			},
		},
	}

	for i, successCase := range successCases {
		t.Run("success case "+strconv.Itoa(i), func(t *testing.T) {
			if errs := ValidateStatefulSetUpdate(successCase.new, successCase.old); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	errorCases := map[string]struct {
		old *appsv1beta1.StatefulSet
		new *appsv1beta1.StatefulSet
	}{
		"selector changed": {
			old: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					ResourceVersion: "1",
				},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: "",
					Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Template:            validPodTemplate1.Template,
					Replicas:            ptr.To[int32](1),
					UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
				},
			},
			new: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					ResourceVersion: "1",
				},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: "",
					Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"app": "bar"}},
					Template:            validPodTemplate1.Template,
					Replicas:            ptr.To[int32](1),
					UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
				},
			},
		},
		"serviceName changed": {
			old: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					ResourceVersion: "1",
				},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: "",
					ServiceName:         "foo",
					Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Template:            validPodTemplate1.Template,
					Replicas:            ptr.To[int32](1),
					UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
				},
			},
			new: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					ResourceVersion: "1",
				},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: "",
					ServiceName:         "bar",
					Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Template:            validPodTemplate1.Template,
					Replicas:            ptr.To[int32](1),
					UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
				},
			},
		},
		"podManagementPolicy changed": {
			old: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					ResourceVersion: "1",
				},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.OrderedReadyPodManagement,
					ServiceName:         "bar",
					Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Template:            validPodTemplate1.Template,
					Replicas:            ptr.To[int32](1),
					UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
				},
			},
			new: &appsv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "foo",
					Namespace:       "bar",
					ResourceVersion: "1",
				},
				Spec: appsv1beta1.StatefulSetSpec{
					PodManagementPolicy: apps.ParallelPodManagement,
					ServiceName:         "bar",
					Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
					Template:            validPodTemplate1.Template,
					Replicas:            ptr.To[int32](1),
					UpdateStrategy:      appsv1beta1.StatefulSetUpdateStrategy{Type: apps.RollingUpdateStatefulSetStrategyType},
				},
			},
		},
	}

	for k, v := range errorCases {
		t.Run(k, func(t *testing.T) {
			setTestDefault(v.old)
			setTestDefault(v.new)
			errs := ValidateStatefulSetUpdate(v.new, v.old)
			if len(errs) == 0 {
				t.Errorf("expected failure for %s", k)
			}

			for i := range errs {
				f := errs[i].Field
				if f != "spec" {
					t.Errorf("%s: missing prefix for: %v", k, errs[i])
				}
			}
		})
	}
}

func setTestDefault(obj *appsv1beta1.StatefulSet) {
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = new(int32)
		*obj.Spec.Replicas = 0
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = new(int32)
		*obj.Spec.RevisionHistoryLimit = 0
	}
}

var (
	testScheme *runtime.Scheme
)

func init() {
	testScheme = k8sruntime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(testScheme))
	utilruntime.Must(storagev1.AddToScheme(testScheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(testScheme))
}

func newFakeStorageClass(name string, allowExpansion, isDefault bool) *storagev1.StorageClass {
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		AllowVolumeExpansion: &allowExpansion,
	}
	if isDefault {
		sc.Annotations = map[string]string{
			isDefaultStorageClassAnnotation: "true",
		}
	}
	return sc
}

func TestValidateVolumeClaimTemplateUpdate(t *testing.T) {
	allowExpandSC := newFakeStorageClass("allowExpand", true, false)
	disallowExpandSC := newFakeStorageClass("disallowExpand", false, false)
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(allowExpandSC, disallowExpandSC).Build()

	tests := []struct {
		name           string
		sts            *appsv1beta1.StatefulSet
		oldSts         *appsv1beta1.StatefulSet
		expectedErrors bool
	}{
		{
			name: "no update strategy and change sc",
			sts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimUpdateStrategy: appsv1beta1.VolumeClaimUpdateStrategy{},
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &allowExpandSC.Name},
						},
					},
				},
			},
			oldSts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &disallowExpandSC.Name},
						},
					},
				},
			},
			expectedErrors: false,
		},
		{
			name: "on delete update strategy and change sc",
			sts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimUpdateStrategy: appsv1beta1.VolumeClaimUpdateStrategy{
						Type: appsv1beta1.OnPVCDeleteVolumeClaimUpdateStrategyType,
					},
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &allowExpandSC.Name},
						},
					},
				},
			},
			oldSts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &disallowExpandSC.Name},
						},
					},
				},
			},
			expectedErrors: false,
		},
		{
			name: "on pod rolling update strategy and change sc",
			sts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimUpdateStrategy: appsv1beta1.VolumeClaimUpdateStrategy{
						Type: appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType,
					},
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &allowExpandSC.Name},
						},
					},
				},
			},
			oldSts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &disallowExpandSC.Name},
						},
					},
				},
			},
			expectedErrors: true,
		},
		{
			name: "on pod rolling update strategy and expand size with expansion allowed sc",
			sts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimUpdateStrategy: appsv1beta1.VolumeClaimUpdateStrategy{
						Type: appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType,
					},
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &allowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			},
			oldSts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &allowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
			expectedErrors: false,
		},
		{
			name: "on pod rolling update strategy and expand size with expansion disallowed sc",
			sts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimUpdateStrategy: appsv1beta1.VolumeClaimUpdateStrategy{
						Type: appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType,
					},
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &disallowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			},
			oldSts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &disallowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
			expectedErrors: true,
		},
		{
			name: "onDelete update strategy and expand size",
			sts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimUpdateStrategy: appsv1beta1.VolumeClaimUpdateStrategy{
						Type: appsv1beta1.OnPVCDeleteVolumeClaimUpdateStrategyType,
					},
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &allowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			},
			oldSts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &allowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
			expectedErrors: false,
		},
		{
			name: "on pod rolling update strategy and scale down size with expansion allowed sc",
			sts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimUpdateStrategy: appsv1beta1.VolumeClaimUpdateStrategy{
						Type: appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType,
					},
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &allowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
			oldSts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &allowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			},
			expectedErrors: false,
		},
		{
			name: "on pod rolling update strategy and scale down size with expansion disallowed sc",
			sts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimUpdateStrategy: appsv1beta1.VolumeClaimUpdateStrategy{
						Type: appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType,
					},
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &disallowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
			},
			oldSts: &appsv1beta1.StatefulSet{
				Spec: appsv1beta1.StatefulSetSpec{
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"},
							Spec: v1.PersistentVolumeClaimSpec{
								StorageClassName: &disallowExpandSC.Name,
								Resources: v1.VolumeResourceRequirements{
									Requests: map[v1.ResourceName]resource.Quantity{
										v1.ResourceStorage: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			},
			expectedErrors: true,
		},
		// Add more test cases here
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateVolumeClaimTemplateUpdate(fakeClient, tt.sts, tt.oldSts)
			hasErrors := len(errs) > 0
			if hasErrors {
				t.Log(errs.ToAggregate())
			}
			if tt.expectedErrors != hasErrors {
				t.Errorf("TestValidateVolumeClaimTemplateUpdate(%s) expected errors: %v, got: %v", tt.name, tt.expectedErrors, hasErrors)
			}
		})
	}
}

func TestGetDefaultStorageClass(t *testing.T) {
	// Create StorageClass objects to use in the test.
	newSCWithCreateTime := func(name string, allowExpansion, isDefault bool, createTime time.Time) *storagev1.StorageClass {
		sc := newFakeStorageClass(name, allowExpansion, isDefault)
		sc.CreationTimestamp = metav1.NewTime(createTime)
		return sc
	}
	tests := []struct {
		name      string
		scs       []*storagev1.StorageClass
		expected  *storagev1.StorageClass
		expectNil bool
	}{
		{
			name:      "no storage class",
			scs:       []*storagev1.StorageClass{},
			expected:  nil,
			expectNil: true,
		},
		{
			name: "no default storage class",
			scs: []*storagev1.StorageClass{
				newFakeStorageClass("sc1", true, false),
				newFakeStorageClass("sc2", true, false),
			},
			expected:  nil,
			expectNil: true,
		},
		{
			name: "only one default storage class",
			scs: []*storagev1.StorageClass{
				newFakeStorageClass("sc1", true, true),
				newFakeStorageClass("sc2", true, false),
			},
			expected:  newFakeStorageClass("sc1", true, true),
			expectNil: false,
		},
		{
			name: "multi default storage classes",
			scs: []*storagev1.StorageClass{
				newSCWithCreateTime("sc1", true, true, time.Now().Add(time.Hour)),
				newSCWithCreateTime("sc2", true, true, time.Now()),
				newSCWithCreateTime("sc3", true, true, time.Now().Add(-time.Hour)),
			},
			expected:  newFakeStorageClass("sc1", true, true),
			expectNil: false,
		},
	}

	for _, tt := range tests {
		// Create a fake client to mock API calls.
		builder := fake.NewClientBuilder().WithScheme(testScheme)
		for _, sc := range tt.scs {
			builder.WithObjects(sc)
		}
		client := builder.Build()

		// Test the GetDefaultStorageClass function.
		defaultSC, err := GetDefaultStorageClass(client)
		assert.NoError(t, err)
		if tt.expectNil {
			assert.Nil(t, defaultSC)
		} else {
			assert.NotNil(t, defaultSC)
			assert.Equal(t, tt.expected.Name, defaultSC.Name)
		}

	}

}

func TestValidateReserveOrdinals(t *testing.T) {
	tests := []struct {
		name            string
		reserveOrdinals []intstr.IntOrString
		expectedErrors  bool
	}{
		{
			name:            "EmptyReserveOrdinals",
			reserveOrdinals: []intstr.IntOrString{},
			expectedErrors:  false,
		},
		{
			name: "ValidStringRange",
			reserveOrdinals: []intstr.IntOrString{
				{Type: intstr.String, StrVal: "1-2"},
			},
			expectedErrors: false,
		},
		{
			name: "InvalidStringRange",
			reserveOrdinals: []intstr.IntOrString{
				{Type: intstr.String, StrVal: "12"},
			},
			expectedErrors: true,
		},
		{
			name: "ValidIntRange",
			reserveOrdinals: []intstr.IntOrString{
				{Type: intstr.Int, IntVal: 1},
			},
			expectedErrors: false,
		},
		{
			name: "NegativeIntRange",
			reserveOrdinals: []intstr.IntOrString{
				{Type: intstr.Int, IntVal: -1},
			},
			expectedErrors: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := &appsv1beta1.StatefulSetSpec{
				ReserveOrdinals: test.reserveOrdinals,
			}
			fldPath := field.NewPath("spec")
			errs := validateReserveOrdinals(spec, fldPath)
			if len(errs) > 0 != test.expectedErrors {
				t.Errorf("validateReserveOrdinals(%v) = %v, want %v", test.reserveOrdinals, errs, test.expectedErrors)
			}
		})
	}
}
