/*
Copyright 2020 The Kruise Authors.

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
	"reflect"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var podTemplateWithVolumeMount = corev1.PodTemplateSpec{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			"key1": "value1",
		},
	},
	Spec: corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "a",
			Image: "b",
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "volume",
				MountPath: "/data",
			}},
		}},
	},
}

func newDaemonSet(name string) *appsv1alpha1.DaemonSet {
	ds := &appsv1alpha1.DaemonSet{}
	ds.Name = name
	ds.Namespace = metav1.NamespaceDefault
	return ds
}

func TestValidateDaemonSet(t *testing.T) {

	for _, c := range []struct {
		Title             string
		Ds                *appsv1alpha1.DaemonSet
		ExpectAllowResult bool
	}{
		{
			"selector not match",
			func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("ds1")
				ds.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
					},
				}
				ds.Spec.Template = corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"key1": "value2",
						},
					},
					Spec: corev1.PodSpec{},
				}
				return ds
			}(),
			false,
		},
		{
			"selector match",
			func() *appsv1alpha1.DaemonSet {
				maxUnavailable := intstr.FromInt(1)
				ds := newDaemonSet("ds1")
				ds.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
					},
				}
				ds.Spec.Template = corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"key1": "value1",
						},
					},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "a", Image: "b"}}},
				}
				ds.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyAlways
				ds.Spec.UpdateStrategy = appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						Type:           appsv1alpha1.StandardRollingUpdateType,
						MaxUnavailable: &maxUnavailable,
					},
				}
				return ds
			}(),
			true,
		},
		{
			Title: "pod template volumeMounts with volumeClaimTemplates",
			Ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("ds1")
				ds.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
					},
				}
				ds.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "volume",
						},
					},
				}
				ds.Spec.Template = podTemplateWithVolumeMount
				return ds
			}(),
		},
		{
			Title: "pod template volumeMounts with volumeClaimTemplates no match",
			Ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("ds1")
				ds.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"key1": "value1",
					},
				}
				ds.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "volume1",
						},
					},
				}
				ds.Spec.Template = podTemplateWithVolumeMount
				return ds
			}(),
			ExpectAllowResult: false,
		},
	} {
		result, _, err := validatingDaemonSetFn(context.TODO(), c.Ds)
		if !reflect.DeepEqual(c.ExpectAllowResult, result) {
			t.Fatalf("case: %s, expected result: %v, got: %v, error: %v", c.Title, c.ExpectAllowResult, result, err)
		}
	}
}

type testCase struct {
	spec    *appsv1alpha1.DaemonSetSpec
	oldSpec *appsv1alpha1.DaemonSetSpec
}

func TestValidateDaemonSetUpdate(t *testing.T) {
	handler := DaemonSetCreateUpdateHandler{}
	validLabels := map[string]string{"a": "b"}
	validPodTemplate := corev1.PodTemplate{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
				DNSPolicy:     corev1.DNSClusterFirst,
				Containers:    []corev1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: corev1.TerminationMessageReadFile}},
			},
		},
	}
	podTemplateWithVolumeMounts := podTemplateWithVolumeMount.DeepCopy()
	podTemplateWithVolumeMounts.Spec.Containers[0].VolumeMounts = append(podTemplateWithVolumeMounts.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{
			Name:      "volume1",
			MountPath: "/data1",
		},
	)
	intOrStr1 := intstr.FromInt(1)
	intOrStr2 := intstr.FromInt(2)
	successCases := []testCase{
		{
			spec: &appsv1alpha1.DaemonSetSpec{
				Template:      validPodTemplate.Template,
				Selector:      &metav1.LabelSelector{MatchLabels: validLabels},
				BurstReplicas: &intOrStr1,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
					},
				},
			},
			oldSpec: &appsv1alpha1.DaemonSetSpec{
				Template:      validPodTemplate.Template,
				Selector:      &metav1.LabelSelector{MatchLabels: validLabels},
				BurstReplicas: &intOrStr2,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
					},
				},
			},
		},
		{
			spec: &appsv1alpha1.DaemonSetSpec{
				Template:      validPodTemplate.Template,
				Selector:      &metav1.LabelSelector{MatchLabels: validLabels},
				BurstReplicas: &intOrStr1,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
					},
				},
			},
			oldSpec: &appsv1alpha1.DaemonSetSpec{
				Template:      validPodTemplate.Template,
				Selector:      &metav1.LabelSelector{MatchLabels: validLabels},
				BurstReplicas: &intOrStr2,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
					},
				},
			},
		},
		{
			spec: &appsv1alpha1.DaemonSetSpec{
				Template: *podTemplateWithVolumeMounts,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"key1": "value1",
				}},
				BurstReplicas: &intOrStr1,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "volume",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "volume1",
						},
					},
				},
			},
			oldSpec: &appsv1alpha1.DaemonSetSpec{
				Template: podTemplateWithVolumeMount,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"key1": "value1",
				}},
				BurstReplicas: &intOrStr2,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "volume",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "volume1",
						},
					},
				},
			},
		},
	}
	uid := uuid.NewUUID()

	for i, successCase := range successCases {
		obj := &appsv1alpha1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ds-%d", i), Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "2"},
			Spec:       *successCase.spec,
		}
		oldObj := &appsv1alpha1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ds-%d", i), Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "1"},
			Spec:       *successCase.oldSpec,
		}
		t.Run("success case "+strconv.Itoa(i), func(t *testing.T) {
			if errs := handler.validateDaemonSetUpdate(obj, oldObj); len(errs) != 0 {
				t.Errorf("expected success: %v", errs)
			}
		})
	}

	validLabels2 := map[string]string{"c": "d"}

	errorCases := []testCase{
		{
			spec: &appsv1alpha1.DaemonSetSpec{
				Template:      validPodTemplate.Template,
				Selector:      &metav1.LabelSelector{MatchLabels: validLabels2},
				BurstReplicas: &intOrStr1,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
						MaxSurge:       &intOrStr2,
					},
				},
			},
			oldSpec: &appsv1alpha1.DaemonSetSpec{
				Template:      validPodTemplate.Template,
				Selector:      &metav1.LabelSelector{MatchLabels: validLabels},
				BurstReplicas: &intOrStr2,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
						MaxSurge:       &intOrStr2,
					},
				},
			},
		},
		// missing volume1 volume mount
		{
			spec: &appsv1alpha1.DaemonSetSpec{
				Template: *podTemplateWithVolumeMounts,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"key1": "value1",
				}},
				BurstReplicas: &intOrStr1,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "volume",
						},
					},
				},
			},
			oldSpec: &appsv1alpha1.DaemonSetSpec{
				Template: podTemplateWithVolumeMount,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"key1": "value1",
				}},
				BurstReplicas: &intOrStr2,
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &intOrStr1,
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "volume",
						},
					},
				},
			},
		},
	}
	for i, successCase := range errorCases {
		obj := &appsv1alpha1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ds-%d", i), Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "2"},
			Spec:       *successCase.spec,
		}
		oldObj := &appsv1alpha1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ds-%d", i), Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "1"},
			Spec:       *successCase.oldSpec,
		}
		t.Run("error case "+strconv.Itoa(i), func(t *testing.T) {
			if errs := handler.validateDaemonSetUpdate(obj, oldObj); len(errs) == 0 {
				t.Errorf("expected fail: %v", errs)
			}
		})
	}
}
