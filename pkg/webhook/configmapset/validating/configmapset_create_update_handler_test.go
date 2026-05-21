package validating

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/configmapset"
)

func init() {
	_ = appsv1alpha1.AddToScheme(scheme.Scheme)
}

func TestValidatePartition(t *testing.T) {
	cases := []struct {
		name      string
		partition *intstr.IntOrString
		expected  bool
	}{
		{"nil partition", nil, false},
		{"valid int partition", &intstr.IntOrString{Type: intstr.Int, IntVal: 1}, true},
		{"zero int partition", &intstr.IntOrString{Type: intstr.Int, IntVal: 0}, true},
		{"negative int partition", &intstr.IntOrString{Type: intstr.Int, IntVal: -1}, false},
		{"valid string partition", &intstr.IntOrString{Type: intstr.String, StrVal: "10%"}, true},
		{"zero string partition", &intstr.IntOrString{Type: intstr.String, StrVal: "0%"}, true},
		{"100 string partition", &intstr.IntOrString{Type: intstr.String, StrVal: "100%"}, true},
		{"invalid string partition - negative", &intstr.IntOrString{Type: intstr.String, StrVal: "-10%"}, false},
		{"invalid string partition - over 100", &intstr.IntOrString{Type: intstr.String, StrVal: "110%"}, false},
		{"invalid string partition - no percent", &intstr.IntOrString{Type: intstr.String, StrVal: "10"}, false},
		{"invalid string partition - non numeric", &intstr.IntOrString{Type: intstr.String, StrVal: "abc%"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validatePartition(tc.partition)
			if result != tc.expected {
				t.Errorf("expected %v, got %v for partition %v", tc.expected, result, tc.partition)
			}
		})
	}
}

func TestValidateMaxUnavailable(t *testing.T) {
	cases := []struct {
		name           string
		maxUnavailable *intstr.IntOrString
		expected       bool
	}{
		{"nil maxUnavailable", nil, false},
		{"valid int maxUnavailable", &intstr.IntOrString{Type: intstr.Int, IntVal: 1}, true},
		{"zero int maxUnavailable", &intstr.IntOrString{Type: intstr.Int, IntVal: 0}, false}, // maxUnavailable > 0 required
		{"negative int maxUnavailable", &intstr.IntOrString{Type: intstr.Int, IntVal: -1}, false},
		{"valid string percent", &intstr.IntOrString{Type: intstr.String, StrVal: "10%"}, true},
		{"zero string percent", &intstr.IntOrString{Type: intstr.String, StrVal: "0%"}, true},
		{"100 string percent", &intstr.IntOrString{Type: intstr.String, StrVal: "100%"}, true},
		{"invalid string percent - over 100", &intstr.IntOrString{Type: intstr.String, StrVal: "110%"}, false},
		{"valid string int", &intstr.IntOrString{Type: intstr.String, StrVal: "10"}, true},
		{"zero string int", &intstr.IntOrString{Type: intstr.String, StrVal: "0"}, true},
		{"negative string int", &intstr.IntOrString{Type: intstr.String, StrVal: "-1"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validateMaxUnavailable(tc.maxUnavailable)
			if result != tc.expected {
				t.Errorf("expected %v, got %v for maxUnavailable %v", tc.expected, result, tc.maxUnavailable)
			}
		})
	}
}

func TestValidateDeleteConfigMapSet(t *testing.T) {
	podActive := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-active",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
			Annotations: map[string]string{
				configmapset.GetConfigMapSetEnabledKey(): "true",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	podInactive := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-inactive",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
			Annotations: map[string]string{
				configmapset.GetConfigMapSetEnabledKey(): "true",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded, // inactive
		},
	}

	podNoLabel := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-no-label",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
			// No configmapset enabled annotation
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	cases := []struct {
		name        string
		pods        []client.Object
		expectError bool
	}{
		{
			name:        "no pods",
			pods:        []client.Object{},
			expectError: false,
		},
		{
			name:        "active pod using cms",
			pods:        []client.Object{podActive},
			expectError: true,
		},
		{
			name:        "inactive pod using cms",
			pods:        []client.Object{podInactive},
			expectError: false,
		},
		{
			name:        "active pod not using cms",
			pods:        []client.Object{podNoLabel},
			expectError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tc.pods...).Build()
			handler := &ConfigMapSetCreateUpdateHandler{
				Client: fakeClient,
			}
			cms := &appsv1alpha1.ConfigMapSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cms",
					Namespace: "default",
				},
				Spec: appsv1alpha1.ConfigMapSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			}
			err := handler.validateDeleteConfigMapSet(context.TODO(), cms)
			if (err != nil) != tc.expectError {
				t.Errorf("expected error: %v, got: %v", tc.expectError, err)
			}
		})
	}
}

func TestValidateConfigMapSetSpec(t *testing.T) {
	validPartition := intstr.FromInt(1)
	validMaxUnavailable := intstr.FromInt(1)
	zeroMaxUnavailable := intstr.FromInt(0)
	invalidStringMaxUnavailable := intstr.FromString("invalid")

	cases := []struct {
		name          string
		spec          *appsv1alpha1.ConfigMapSetSpec
		clientObjects []client.Object
		expectErr     bool
		errField      string
	}{
		{
			name: "valid spec",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: false,
		},
		{
			name: "missing selector",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Data: map[string]string{"config": "value"},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.selector",
		},
		{
			name: "missing data",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.data",
		},
		{
			name: "intersecting matchLabelKeys",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test", "env": "prod"},
				},
				Data: map[string]string{"config": "value"},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
					MatchLabelKeys: []string{"env"},
				},
			},
			expectErr: true,
			errField:  "spec.updateStrategy.matchLabelKeys",
		},
		{
			name: "invalid partition - negative int",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &intstr.IntOrString{Type: intstr.Int, IntVal: -1},
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.updateStrategy",
		},
		{
			name: "invalid maxUnavailable - zero",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &zeroMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.updateStrategy",
		},
		{
			name: "invalid maxUnavailable - string format",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &invalidStringMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.updateStrategy",
		},
		{
			name: "duplicate container names",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				Containers: []appsv1alpha1.ConfigMapSetContainer{
					{Name: "app-container"},
					{Name: "app-container"},
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.containers[1].name",
		},
		{
			name: "invalid effect policy - posthook missing config",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				EffectPolicy: &appsv1alpha1.EffectPolicy{
					Type: appsv1alpha1.EffectPolicyTypePostHook,
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.effectPolicy.postHook",
		},
		{
			name: "invalid effect policy - both http and tcp",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				EffectPolicy: &appsv1alpha1.EffectPolicy{
					Type: appsv1alpha1.EffectPolicyTypePostHook,
					PostHook: &appsv1alpha1.PostHookConfig{
						HTTPGet:   []*corev1.HTTPGetAction{{}},
						TCPSocket: []*corev1.TCPSocketAction{{}},
					},
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.effectPolicy.postHook",
		},
		{
			name: "valid sidecarset ref",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
					Type: appsv1alpha1.ReloadSidecarTypeSidecarSet,
					Config: &appsv1alpha1.ReloadSidecarConfigData{
						SidecarSetRef: &appsv1alpha1.SidecarSetRef{
							Name:          "test-sidecarset",
							ContainerName: "reload-container",
						},
					},
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			clientObjects: []client.Object{
				&appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"}, // matching
						},
						Containers: []appsv1alpha1.SidecarContainer{
							{
								Container: corev1.Container{Name: "reload-container", Image: "image:v1"},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid custom configmap - missing reload-sidecar key",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
					Type: appsv1alpha1.ReloadSidecarTypeCustom,
					Config: &appsv1alpha1.ReloadSidecarConfigData{
						ConfigMapRef: &appsv1alpha1.ConfigMapRef{
							Name:      "test-cm",
							Namespace: "default",
						},
					},
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			clientObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
					Data:       map[string]string{"wrong-key": "{}"},
				},
			},
			expectErr: true,
			errField:  "spec.reloadSidecarConfig.config.configMapRef.name",
		},
		{
			name: "invalid custom configmap - not exist",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
					Type: appsv1alpha1.ReloadSidecarTypeCustom,
					Config: &appsv1alpha1.ReloadSidecarConfigData{
						ConfigMapRef: &appsv1alpha1.ConfigMapRef{
							Name:      "not-exist-cm",
							Namespace: "default",
						},
					},
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.reloadSidecarConfig.config.configMapRef.name",
		},
		{
			name: "invalid sidecarset ref - not exist",
			spec: &appsv1alpha1.ConfigMapSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Data: map[string]string{"config": "value"},
				ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
					Type: appsv1alpha1.ReloadSidecarTypeSidecarSet,
					Config: &appsv1alpha1.ReloadSidecarConfigData{
						SidecarSetRef: &appsv1alpha1.SidecarSetRef{
							Name: "not-exist-sidecarset",
						},
					},
				},
				UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
					Partition:      &validPartition,
					MaxUnavailable: &validMaxUnavailable,
				},
			},
			expectErr: true,
			errField:  "spec.reloadSidecarConfig.config.sidecarSetRef.name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tc.clientObjects...).Build()
			handler := &ConfigMapSetCreateUpdateHandler{
				Client: fakeClient,
			}
			err := handler.validateConfigMapSetSpec(context.TODO(), "test-cms", "default", tc.spec, field.NewPath("spec"))
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error on field %s but got none", tc.errField)
				}
				if err.Field != tc.errField {
					t.Errorf("expected error on field %s, but got error: %v", tc.errField, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, but got: %v", err)
				}
			}
		})
	}
}

func TestRevisionHistoryLimitValidation(t *testing.T) {
	// Construct a scenario where the Limit is reached
	revisions := []configmapset.RevisionEntry{
		{Hash: "hash1", CustomVersion: "v1"},
		{Hash: "hash2", CustomVersion: "v2"},
	}
	revBytes, _ := json.Marshal(revisions)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configmapset.GetConfigMapSetHubName("test-cms"),
			Namespace: "default",
		},
		Data: map[string]string{
			"revisions": string(revBytes),
		},
	}

	// Construct a Pod that is using the oldest revision "hash1"
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-1",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
			Annotations: map[string]string{
				configmapset.GetConfigMapSetCurrentRevisionKey("test-cms"): "hash1",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-2",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
			Annotations: map[string]string{
				configmapset.GetConfigMapSetCurrentRevisionKey("test-cms"): "hash2",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(cm, pod1, pod2).Build()
	handler := &ConfigMapSetCreateUpdateHandler{
		Client: fakeClient,
	}

	// Try to publish a new version hash3
	spec := &appsv1alpha1.ConfigMapSetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "test"},
		},
		Data:                 map[string]string{"new": "data"},
		RevisionHistoryLimit: pointer.Int32(2),
		UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
			Partition:      &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
			MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}

	err := handler.validateConfigMapSetSpec(context.TODO(), "test-cms", "default", spec, field.NewPath("spec"))

	if err == nil {
		t.Fatalf("expected error for exceeding RevisionHistoryLimit but got none")
	}

	if err.Field != "spec.revisionHistoryLimit" {
		t.Errorf("expected error on spec.revisionHistoryLimit, got: %v", err)
	}
}
