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

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func newDaemonset(name string) *appsv1alpha1.DaemonSet {
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
				ds := newDaemonset("ds1")
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
				ds := newDaemonset("ds1")
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

func TestValidateDaemonSetUpdateStrategy(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	maxSurge := intstr.FromInt(1)

	tests := []struct {
		name      string
		strategy  *appsv1alpha1.DaemonSetUpdateStrategy
		expectErr bool
	}{
		{
			name: "OnDelete strategy",
			strategy: &appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.OnDeleteDaemonSetStrategyType,
			},
			expectErr: false,
		},
		{
			name: "RollingUpdate with maxUnavailable",
			strategy: &appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
			expectErr: false,
		},
		{
			name: "RollingUpdate without RollingUpdate field",
			strategy: &appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
			},
			expectErr: true,
		},
		{
			name: "RollingUpdate with maxSurge",
			strategy: &appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxSurge: &maxSurge,
				},
			},
			expectErr: false,
		},
		{
			name: "Invalid strategy type",
			strategy: &appsv1alpha1.DaemonSetUpdateStrategy{
				Type: "InvalidType",
			},
			expectErr: true,
		},
		{
			name: "Invalid rollingUpdate selector",
			strategy: &appsv1alpha1.DaemonSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
					MaxSurge: &maxSurge,
					Selector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"value1"},
							},
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateDaemonSetUpdateStrategy(tt.strategy, nil)
			if tt.expectErr && len(errs) == 0 {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && len(errs) != 0 {
				t.Errorf("expected no error but got: %v", errs)
			}
		})
	}
}

func TestValidateRollingUpdateDaemonSet(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	maxSurge := intstr.FromInt(1)
	zeroUnavailable := intstr.FromInt(0)
	zeroSurge := intstr.FromInt(0)
	percentValue := intstr.FromString("50%")
	invalidPercent := intstr.FromString("150%")
	partition := intstr.FromInt(5)

	tests := []struct {
		name          string
		rollingUpdate *appsv1alpha1.RollingUpdateDaemonSet
		expectErr     bool
	}{
		{
			name: "Valid maxUnavailable",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				MaxUnavailable: &maxUnavailable,
			},
			expectErr: false,
		},
		{
			name: "Valid maxSurge",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				MaxSurge: &maxSurge,
			},
			expectErr: false,
		},
		{
			name: "Both maxUnavailable and maxSurge",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				MaxUnavailable: &maxUnavailable,
				MaxSurge:       &maxSurge,
			},
			expectErr: true,
		},
		{
			name: "Both zero",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				MaxUnavailable: &zeroUnavailable,
				MaxSurge:       &zeroSurge,
			},
			expectErr: true,
		},
		{
			name: "Standard rolling update type",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:           appsv1alpha1.StandardRollingUpdateType,
				MaxUnavailable: &maxUnavailable,
			},
			expectErr: false,
		},
		{
			name: "InplaceRollingUpdate with maxSurge",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:     appsv1alpha1.InplaceRollingUpdateType,
				MaxSurge: &maxSurge,
			},
			expectErr: true,
		},
		{
			name: "InplaceRollingUpdate with maxUnavailable",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:           appsv1alpha1.InplaceRollingUpdateType,
				MaxUnavailable: &maxUnavailable,
			},
			expectErr: false,
		},
		{
			name: "DeprecatedSurgingRollingUpdate with maxUnavailable",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:           appsv1alpha1.DeprecatedSurgingRollingUpdateType,
				MaxUnavailable: &maxUnavailable,
			},
			expectErr: true,
		},
		{
			name: "DeprecatedSurgingRollingUpdate with maxSurge",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:     appsv1alpha1.DeprecatedSurgingRollingUpdateType,
				MaxSurge: &maxSurge,
			},
			expectErr: false,
		},
		{
			name: "Percent value",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				MaxUnavailable: &percentValue,
			},
			expectErr: false,
		},
		{
			name: "Invalid percent value",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				MaxUnavailable: &invalidPercent,
			},
			expectErr: true,
		},
		{
			name: "With partition",
			rollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
				MaxUnavailable: &maxUnavailable,
				Partition:      &partition,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateRollingUpdateDaemonSet(tt.rollingUpdate, nil)
			if tt.expectErr && len(errs) == 0 {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && len(errs) != 0 {
				t.Errorf("expected no error but got: %v", errs)
			}
		})
	}
}

func TestGetIntOrPercentValue(t *testing.T) {
	tests := []struct {
		name     string
		input    intstr.IntOrString
		expected int
	}{
		{
			name:     "int value",
			input:    intstr.FromInt(5),
			expected: 5,
		},
		{
			name:     "percent value",
			input:    intstr.FromString("50%"),
			expected: 50,
		},
		{
			name:     "zero int",
			input:    intstr.FromInt(0),
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIntOrPercentValue(tt.input)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetPercentValue(t *testing.T) {
	tests := []struct {
		name          string
		input         intstr.IntOrString
		expectedValue int
		expectedIsPct bool
	}{
		{
			name:          "valid percent",
			input:         intstr.FromString("50%"),
			expectedValue: 50,
			expectedIsPct: true,
		},
		{
			name:          "int value",
			input:         intstr.FromInt(5),
			expectedValue: 0,
			expectedIsPct: false,
		},
		{
			name:          "invalid percent string",
			input:         intstr.FromString("invalid"),
			expectedValue: 0,
			expectedIsPct: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, isPct := getPercentValue(tt.input)
			if value != tt.expectedValue || isPct != tt.expectedIsPct {
				t.Errorf("expected (%d, %v), got (%d, %v)", tt.expectedValue, tt.expectedIsPct, value, isPct)
			}
		})
	}
}

func TestValidateNonnegativeIntOrPercent(t *testing.T) {
	tests := []struct {
		name      string
		input     intstr.IntOrString
		expectErr bool
	}{
		{
			name:      "positive int",
			input:     intstr.FromInt(5),
			expectErr: false,
		},
		{
			name:      "zero",
			input:     intstr.FromInt(0),
			expectErr: false,
		},
		{
			name:      "negative int",
			input:     intstr.FromInt(-5),
			expectErr: true,
		},
		{
			name:      "valid percent",
			input:     intstr.FromString("50%"),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateNonnegativeIntOrPercent(tt.input, nil)
			if tt.expectErr && len(errs) == 0 {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && len(errs) != 0 {
				t.Errorf("expected no error but got: %v", errs)
			}
		})
	}
}

func TestValidateDaemonSetSpec(t *testing.T) {
	validLabels := map[string]string{"app": "test"}
	maxUnavailable := intstr.FromInt(1)
	activeDeadlineSeconds := int64(600)
	revisionHistoryLimit := int32(10)

	tests := []struct {
		name      string
		spec      *appsv1alpha1.DaemonSetSpec
		expectErr bool
	}{
		{
			name: "empty selector",
			spec: &appsv1alpha1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "invalid restart policy",
			spec: &appsv1alpha1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: validLabels},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyOnFailure,
						Containers:    []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
			},
			expectErr: true,
		},
		{
			name: "with activeDeadlineSeconds",
			spec: &appsv1alpha1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: validLabels},
					Spec: corev1.PodSpec{
						RestartPolicy:         corev1.RestartPolicyAlways,
						ActiveDeadlineSeconds: &activeDeadlineSeconds,
						Containers:            []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
			},
			expectErr: true,
		},
		{
			name: "with revisionHistoryLimit",
			spec: &appsv1alpha1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: validLabels},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyAlways,
						Containers:    []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
				RevisionHistoryLimit: &revisionHistoryLimit,
			},
			expectErr: false,
		},
		{
			name: "with inPlaceUpdate lifecycle",
			spec: &appsv1alpha1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: validLabels},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyAlways,
						Containers:    []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
				UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
					Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
				Lifecycle: &appspub.Lifecycle{
					InPlaceUpdate: &appspub.LifecycleHook{},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateDaemonSetSpec(tt.spec, nil)
			if tt.expectErr && len(errs) == 0 {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && len(errs) != 0 {
				t.Errorf("expected no error but got: %v", errs)
			}
		})
	}
}

func TestValidateDaemonSetV1beta1(t *testing.T) {
	for _, c := range []struct {
		Title             string
		Ds                *appsv1beta1.DaemonSet
		ExpectAllowResult bool
	}{
		{
			"selector not match",
			func() *appsv1beta1.DaemonSet {
				ds := &appsv1beta1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds1",
						Namespace: metav1.NamespaceDefault,
					},
				}
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
			func() *appsv1beta1.DaemonSet {
				maxUnavailable := intstr.FromInt(1)
				ds := &appsv1beta1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ds1",
						Namespace: metav1.NamespaceDefault,
					},
				}
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
				ds.Spec.UpdateStrategy = appsv1beta1.DaemonSetUpdateStrategy{
					Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
						Type:           appsv1beta1.StandardRollingUpdateType,
						MaxUnavailable: &maxUnavailable,
					},
				}
				return ds
			}(),
			true,
		},
	} {
		result, _, err := validatingDaemonSetFnV1beta1(context.TODO(), c.Ds)
		if !reflect.DeepEqual(c.ExpectAllowResult, result) {
			t.Fatalf("case: %s, expected result: %v, got: %v, error: %v", c.Title, c.ExpectAllowResult, result, err)
		}
	}
}

func TestValidateDaemonSetUpdateV1beta1(t *testing.T) {
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
	intOrStr1 := intstr.FromInt(1)
	intOrStr2 := intstr.FromInt(2)

	uid := uuid.NewUUID()

	// Success case
	obj := &appsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds-1", Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "2"},
		Spec: appsv1beta1.DaemonSetSpec{
			Template:      validPodTemplate.Template,
			Selector:      &metav1.LabelSelector{MatchLabels: validLabels},
			BurstReplicas: &intOrStr1,
			UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &intOrStr1,
				},
			},
		},
	}
	oldObj := &appsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds-1", Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "1"},
		Spec: appsv1beta1.DaemonSetSpec{
			Template:      validPodTemplate.Template,
			Selector:      &metav1.LabelSelector{MatchLabels: validLabels},
			BurstReplicas: &intOrStr2,
			UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &intOrStr1,
				},
			},
		},
	}
	if errs := handler.validateDaemonSetUpdateV1beta1(obj, oldObj); len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	// Error case - changing selector
	validLabels2 := map[string]string{"c": "d"}
	obj2 := &appsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds-2", Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "2"},
		Spec: appsv1beta1.DaemonSetSpec{
			Template:      validPodTemplate.Template,
			Selector:      &metav1.LabelSelector{MatchLabels: validLabels2},
			BurstReplicas: &intOrStr1,
			UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &intOrStr1,
				},
			},
		},
	}
	oldObj2 := &appsv1beta1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds-2", Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "1"},
		Spec: appsv1beta1.DaemonSetSpec{
			Template:      validPodTemplate.Template,
			Selector:      &metav1.LabelSelector{MatchLabels: validLabels},
			BurstReplicas: &intOrStr2,
			UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &intOrStr1,
				},
			},
		},
	}
	if errs := handler.validateDaemonSetUpdateV1beta1(obj2, oldObj2); len(errs) == 0 {
		t.Errorf("expected error for forbidden field update")
	}
}

func TestValidateDaemonSetUpdateStrategyV1beta1(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	maxSurge := intstr.FromInt(1)

	tests := []struct {
		name      string
		strategy  *appsv1beta1.DaemonSetUpdateStrategy
		expectErr bool
	}{
		{
			name: "OnDelete strategy",
			strategy: &appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.OnDeleteDaemonSetStrategyType,
			},
			expectErr: false,
		},
		{
			name: "RollingUpdate with maxUnavailable",
			strategy: &appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
				},
			},
			expectErr: false,
		},
		{
			name: "RollingUpdate without RollingUpdate field",
			strategy: &appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
			},
			expectErr: true,
		},
		{
			name: "RollingUpdate with maxSurge",
			strategy: &appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxSurge: &maxSurge,
				},
			},
			expectErr: false,
		},
		{
			name: "Invalid strategy type",
			strategy: &appsv1beta1.DaemonSetUpdateStrategy{
				Type: "InvalidType",
			},
			expectErr: true,
		},
		{
			name: "Invalid rollingUpdate selector",
			strategy: &appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxSurge: &maxSurge,
					Selector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"value1"},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "Invalid rollingUpdate ExemptNodesFromMaxUnavailable",
			strategy: &appsv1beta1.DaemonSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
					MaxSurge: &maxSurge,
					ExemptNodesFromMaxUnavailable: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"value1"},
							},
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateDaemonSetUpdateStrategyV1beta1(tt.strategy, nil)
			if tt.expectErr && len(errs) == 0 {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && len(errs) != 0 {
				t.Errorf("expected no error but got: %v", errs)
			}
		})
	}
}

func TestValidateRollingUpdateDaemonSetV1beta1(t *testing.T) {
	maxUnavailable := intstr.FromInt(1)
	maxSurge := intstr.FromInt(1)
	zeroUnavailable := intstr.FromInt(0)
	zeroSurge := intstr.FromInt(0)
	percentValue := intstr.FromString("50%")
	invalidPercent := intstr.FromString("150%")
	partition := intstr.FromInt(5)

	tests := []struct {
		name          string
		rollingUpdate *appsv1beta1.RollingUpdateDaemonSet
		expectErr     bool
	}{
		{
			name: "Valid maxUnavailable",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				MaxUnavailable: &maxUnavailable,
			},
			expectErr: false,
		},
		{
			name: "Valid maxSurge",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				MaxSurge: &maxSurge,
			},
			expectErr: false,
		},
		{
			name: "Both maxUnavailable and maxSurge",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				MaxUnavailable: &maxUnavailable,
				MaxSurge:       &maxSurge,
			},
			expectErr: true,
		},
		{
			name: "Both zero",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				MaxUnavailable: &zeroUnavailable,
				MaxSurge:       &zeroSurge,
			},
			expectErr: true,
		},
		{
			name: "Standard rolling update type",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				Type:           appsv1beta1.StandardRollingUpdateType,
				MaxUnavailable: &maxUnavailable,
			},
			expectErr: false,
		},
		{
			name: "InplaceRollingUpdate with maxSurge",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				Type:     appsv1beta1.InplaceRollingUpdateType,
				MaxSurge: &maxSurge,
			},
			expectErr: true,
		},
		{
			name: "InplaceRollingUpdate with maxUnavailable",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				Type:           appsv1beta1.InplaceRollingUpdateType,
				MaxUnavailable: &maxUnavailable,
			},
			expectErr: false,
		},
		{
			name: "Percent value",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				MaxUnavailable: &percentValue,
			},
			expectErr: false,
		},
		{
			name: "Invalid percent value",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				MaxUnavailable: &invalidPercent,
			},
			expectErr: true,
		},
		{
			name: "With partition",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				MaxUnavailable: &maxUnavailable,
				Partition:      &partition,
			},
			expectErr: false,
		},
		{
			name: "Invalid type",
			rollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
				Type:           "InvalidType",
				MaxUnavailable: &maxUnavailable,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateRollingUpdateDaemonSetV1beta1(tt.rollingUpdate, nil)
			if tt.expectErr && len(errs) == 0 {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && len(errs) != 0 {
				t.Errorf("expected no error but got: %v", errs)
			}
		})
	}
}

func TestValidateDaemonSetSpecV1beta1(t *testing.T) {
	validLabels := map[string]string{"app": "test"}
	maxUnavailable := intstr.FromInt(1)
	activeDeadlineSeconds := int64(600)
	revisionHistoryLimit := int32(10)

	tests := []struct {
		name      string
		spec      *appsv1beta1.DaemonSetSpec
		expectErr bool
	}{
		{
			name: "empty selector",
			spec: &appsv1beta1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "invalid restart policy",
			spec: &appsv1beta1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: validLabels},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyOnFailure,
						Containers:    []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
				UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
					Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
			},
			expectErr: true,
		},
		{
			name: "with activeDeadlineSeconds",
			spec: &appsv1beta1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: validLabels},
					Spec: corev1.PodSpec{
						RestartPolicy:         corev1.RestartPolicyAlways,
						ActiveDeadlineSeconds: &activeDeadlineSeconds,
						Containers:            []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
				UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
					Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
			},
			expectErr: true,
		},
		{
			name: "with revisionHistoryLimit",
			spec: &appsv1beta1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: validLabels},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyAlways,
						Containers:    []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
				UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
					Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
				RevisionHistoryLimit: &revisionHistoryLimit,
			},
			expectErr: false,
		},
		{
			name: "with inPlaceUpdate lifecycle",
			spec: &appsv1beta1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: validLabels},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyAlways,
						Containers:    []corev1.Container{{Name: "test", Image: "test:v1"}},
					},
				},
				UpdateStrategy: appsv1beta1.DaemonSetUpdateStrategy{
					Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
				Lifecycle: &appspub.Lifecycle{
					InPlaceUpdate: &appspub.LifecycleHook{},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateDaemonSetSpecV1beta1(tt.spec, nil)
			if tt.expectErr && len(errs) == 0 {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && len(errs) != 0 {
				t.Errorf("expected no error but got: %v", errs)
			}
		})
	}
}
