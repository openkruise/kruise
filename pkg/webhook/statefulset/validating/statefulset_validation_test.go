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

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilpointer "k8s.io/utils/pointer"
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
	maxUnavailable1 := intstr.FromInt(1)
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
							MinReadySeconds: utilpointer.Int32Ptr(10),
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
							MinReadySeconds: utilpointer.Int32Ptr(1),
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
						MinReadySeconds: utilpointer.Int32Ptr(-1),
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
						MinReadySeconds: utilpointer.Int32Ptr(appsv1beta1.MaxMinReadySeconds + 1),
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
				field := errs[i].Field
				if !strings.HasPrefix(field, "spec.template.") &&
					field != "metadata.name" &&
					field != "metadata.namespace" &&
					field != "spec.selector" &&
					field != "spec.template" &&
					field != "GCEPersistentDisk.ReadOnly" &&
					field != "spec.replicas" &&
					field != "spec.template.labels" &&
					field != "metadata.annotations" &&
					field != "metadata.labels" &&
					field != "status.replicas" &&
					field != "spec.updateStrategy" &&
					field != "spec.updateStrategy.rollingUpdate" &&
					field != "spec.updateStrategy.rollingUpdate.partition" &&
					field != "spec.updateStrategy.rollingUpdate.maxUnavailable" &&
					field != "spec.updateStrategy.rollingUpdate.minReadySeconds" &&
					field != "spec.updateStrategy.rollingUpdate.podUpdatePolicy" &&
					field != "spec.template.spec.readinessGates" &&
					field != "spec.podManagementPolicy" &&
					field != "spec.template.spec.activeDeadlineSeconds" {
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
				Resources: v1.ResourceRequirements{Requests: map[v1.ResourceName]resource.Quantity{
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
					Replicas:             utilpointer.Int32Ptr(5),
					RevisionHistoryLimit: utilpointer.Int32Ptr(5),
					ReserveOrdinals:      []int{1},
					Lifecycle:            &appspub.Lifecycle{PreDelete: &appspub.LifecycleHook{FinalizersHandler: []string{"foo/bar"}}},
					Template:             validPodTemplate1.Template,
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{validVolumeClaimTemplate("30Gi")},
					ScaleStrategy:        &appsv1beta1.StatefulSetScaleStrategy{MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1}},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type:          apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{Partition: utilpointer.Int32Ptr(5)},
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
					Replicas:             utilpointer.Int32Ptr(10),
					RevisionHistoryLimit: utilpointer.Int32Ptr(10),
					ReserveOrdinals:      []int{2},
					Lifecycle:            &appspub.Lifecycle{PreDelete: &appspub.LifecycleHook{FinalizersHandler: []string{"foo/hello"}}},
					Template:             validPodTemplate2.Template,
					VolumeClaimTemplates: []v1.PersistentVolumeClaim{validVolumeClaimTemplate("60Gi")},
					ScaleStrategy:        &appsv1beta1.StatefulSetScaleStrategy{MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2}},
					UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
						Type:          apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{Partition: utilpointer.Int32Ptr(10)},
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
					Replicas:            utilpointer.Int32Ptr(1),
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
					Replicas:            utilpointer.Int32Ptr(1),
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
					Replicas:            utilpointer.Int32Ptr(1),
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
					Replicas:            utilpointer.Int32Ptr(1),
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
					Replicas:            utilpointer.Int32Ptr(1),
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
					Replicas:            utilpointer.Int32Ptr(1),
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
				field := errs[i].Field
				if field != "spec" {
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
