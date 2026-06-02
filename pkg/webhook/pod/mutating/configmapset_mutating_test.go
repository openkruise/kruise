package mutating

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/configmapset"
)

func init() {
	_ = appsv1alpha1.AddToScheme(scheme.Scheme)
}

func TestCheckConfigMapSetConflicts(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app-container"},
			},
		},
	}

	cases := []struct {
		name        string
		cmsList     []*appsv1alpha1.ConfigMapSet
		expectError bool
	}{
		{
			name: "no conflicts",
			cmsList: []*appsv1alpha1.ConfigMapSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cms1"},
					Spec: appsv1alpha1.ConfigMapSetSpec{
						Containers: []appsv1alpha1.ConfigMapSetContainer{
							{Name: "app-container", MountPath: "/data/conf1"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cms2"},
					Spec: appsv1alpha1.ConfigMapSetSpec{
						ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
							Type: appsv1alpha1.ReloadSidecarTypeK8s,
							Config: &appsv1alpha1.ReloadSidecarConfigData{
								Name: "another-sidecar",
							},
						},
						Containers: []appsv1alpha1.ConfigMapSetContainer{
							{Name: "app-container", MountPath: "/data/conf2"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "sidecar name conflict",
			cmsList: []*appsv1alpha1.ConfigMapSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cms1"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cms2"},
					Spec: appsv1alpha1.ConfigMapSetSpec{
						ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
							Type: appsv1alpha1.ReloadSidecarTypeK8s,
							Config: &appsv1alpha1.ReloadSidecarConfigData{
								Name: "cms1-reload-sidecar", // Conflicts with default name of cms1
							},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "mount path conflict",
			cmsList: []*appsv1alpha1.ConfigMapSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cms1"},
					Spec: appsv1alpha1.ConfigMapSetSpec{
						Containers: []appsv1alpha1.ConfigMapSetContainer{
							{Name: "app-container", MountPath: "/data/conf1"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cms2"},
					Spec: appsv1alpha1.ConfigMapSetSpec{
						ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
							Type: appsv1alpha1.ReloadSidecarTypeK8s,
							Config: &appsv1alpha1.ReloadSidecarConfigData{
								Name: "another-sidecar",
							},
						},
						Containers: []appsv1alpha1.ConfigMapSetContainer{
							{Name: "app-container", MountPath: "/data/conf1"}, // Same mount path
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			handler := &PodCreateHandler{Client: fakeClient}
			err := handler.checkConfigMapSetConflicts(context.TODO(), pod, tc.cmsList)
			if (err != nil) != tc.expectError {
				t.Errorf("expected error: %v, got: %v", tc.expectError, err)
			}
		})
	}
}

func TestHandlePodRevisionAnnotations(t *testing.T) {
	cms := &appsv1alpha1.ConfigMapSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cms",
			Namespace: "default",
		},
		Spec: appsv1alpha1.ConfigMapSetSpec{
			Data:          map[string]string{"key": "value"},
			CustomVersion: "v2",
			UpdateStrategy: &appsv1alpha1.ConfigMapSetUpdateStrategy{
				Partition: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
		Status: appsv1alpha1.ConfigMapSetStatus{
			CurrentRevision:      "old-hash",
			CurrentCustomVersion: "v1",
		},
	}

	updateHash, _ := configmapset.CalculateHash(cms.Spec.Data)

	podOld1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-old1",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
			Annotations: map[string]string{
				configmapset.GetConfigMapSetCurrentRevisionKey("test-cms"): "old-hash",
				configmapset.GetConfigMapSetUpdateRevisionKey("test-cms"):  "old-hash",
			},
		},
	}

	// All cases should inject old version (CurrentRevision) if it exists, and new version if it doesn't.
	// Since cms has Status.CurrentRevision = "old-hash", it should always get "old-hash".

	t.Run("should always get stable version", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(podOld1).Build()
		handler := &PodCreateHandler{Client: fakeClient}

		newPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-creating",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		}

		err := handler.handlePodRevisionAnnotations(context.TODO(), newPod, cms)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if newPod.Annotations[configmapset.GetConfigMapSetUpdateRevisionKey("test-cms")] != "old-hash" {
			t.Errorf("expected old version %s, got %s", "old-hash", newPod.Annotations[configmapset.GetConfigMapSetUpdateRevisionKey("test-cms")])
		}
	})

	t.Run("should get new version if no current revision", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(podOld1).Build()
		handler := &PodCreateHandler{Client: fakeClient}

		cmsFirstRollout := cms.DeepCopy()
		cmsFirstRollout.Status.CurrentRevision = ""

		newPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-creating",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
		}

		err := handler.handlePodRevisionAnnotations(context.TODO(), newPod, cmsFirstRollout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if newPod.Annotations[configmapset.GetConfigMapSetUpdateRevisionKey("test-cms")] != updateHash {
			t.Errorf("expected new version %s, got %s", updateHash, newPod.Annotations[configmapset.GetConfigMapSetUpdateRevisionKey("test-cms")])
		}
	})
}

func TestGetReloadSidecarName(t *testing.T) {
	cases := []struct {
		name        string
		cms         *appsv1alpha1.ConfigMapSet
		clientObjs  []client.Object
		expected    string
		expectError bool
	}{
		{
			name: "default",
			cms: &appsv1alpha1.ConfigMapSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cms"},
			},
			expected: "test-cms-reload-sidecar",
		},
		{
			name: "k8s config",
			cms: &appsv1alpha1.ConfigMapSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cms"},
				Spec: appsv1alpha1.ConfigMapSetSpec{
					ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
						Type: appsv1alpha1.ReloadSidecarTypeK8s,
						Config: &appsv1alpha1.ReloadSidecarConfigData{
							Name: "custom-k8s-sidecar",
						},
					},
				},
			},
			expected: "custom-k8s-sidecar",
		},
		{
			name: "sidecarset config",
			cms: &appsv1alpha1.ConfigMapSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cms"},
				Spec: appsv1alpha1.ConfigMapSetSpec{
					ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
						Type: appsv1alpha1.ReloadSidecarTypeSidecarSet,
						Config: &appsv1alpha1.ReloadSidecarConfigData{
							SidecarSetRef: &appsv1alpha1.SidecarSetRef{
								ContainerName: "sidecarset-container",
							},
						},
					},
				},
			},
			expected: "sidecarset-container",
		},
		{
			name: "custom configmap",
			cms: &appsv1alpha1.ConfigMapSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cms", Namespace: "default"},
				Spec: appsv1alpha1.ConfigMapSetSpec{
					ReloadSidecarConfig: &appsv1alpha1.ReloadSidecarConfig{
						Type: appsv1alpha1.ReloadSidecarTypeCustom,
						Config: &appsv1alpha1.ReloadSidecarConfigData{
							ConfigMapRef: &appsv1alpha1.ConfigMapRef{
								Name: "custom-cm",
							},
						},
					},
				},
			},
			clientObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "custom-cm", Namespace: "default"},
					Data: map[string]string{
						"reload-sidecar": `{"name": "cm-sidecar", "image": "img:v1"}`,
					},
				},
			},
			expected: "cm-sidecar",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tc.clientObjs...).Build()
			handler := &PodCreateHandler{Client: fakeClient}
			name, err := handler.getReloadSidecarName(context.TODO(), tc.cms)
			if (err != nil) != tc.expectError {
				t.Fatalf("expected error: %v, got: %v", tc.expectError, err)
			}
			if name != tc.expected {
				t.Errorf("expected name %s, got %s", tc.expected, name)
			}
		})
	}
}
