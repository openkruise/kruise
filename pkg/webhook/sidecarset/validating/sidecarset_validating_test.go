package validating

import (
	"context"
	"fmt"
	"strings"
	"testing"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

var (
	sidecarsetList = &appsv1beta1.SidecarSetList{
		Items: []appsv1beta1.SidecarSet{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
				},
			},
		},
	}
	sidecarset = &appsv1beta1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
		Spec: appsv1beta1.SidecarSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"a": "b"},
			},
		},
	}
)

var (
	testScheme *runtime.Scheme
	handler    = &SidecarSetCreateUpdateHandler{}
	always     = corev1.ContainerRestartPolicyAlways
)

func init() {
	testScheme = runtime.NewScheme()
	utilruntime.Must(apps.AddToScheme(testScheme))
	utilruntime.Must(corev1.AddToScheme(testScheme))
}

func TestValidateSidecarSet(t *testing.T) {
	testErrorCases := []struct {
		caseName   string
		sidecarSet appsv1beta1.SidecarSet
		expectErrs int
	}{
		{
			caseName: "missing-selector",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "wrong-updateStrategy",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.RollingUpdateSidecarSetStrategyType,
						ScatterStrategy: appsv1beta1.UpdateScatterStrategy{
							{
								Key:   "key-1",
								Value: "value-1",
							},
							{
								Key:   "key-1",
								Value: "value-1",
							},
						},
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "wrong-selector",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "app-name",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{"app-group", "app-risk-"},
							},
						},
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyEnabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 2,
		},
		{
			caseName: "wrong-namespaceSelector",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "app-name",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{"app-group", "app-risk"},
							},
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "app-name",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{"app-group", "app-risk-"},
							},
						},
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyEnabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 2,
		},
		{
			caseName: "wrong-namespace-selector",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "app-name",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{"app-group", "app-risk"},
							},
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},

					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyEnabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 0, // No error expected now as we only have NamespaceSelector
		},
		{
			caseName: "wrong-initContainer",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					InitContainers: []appsv1beta1.SidecarContainer{
						{
							Container: corev1.Container{
								Name:                     "test-sidecar",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "missing-container",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "wrong-containers",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,

							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "wrong-volumes",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "test-volume",
						},
						{
							Name: "istio-token",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									DefaultMode: pointer.Int32Ptr(420),
									Sources: []corev1.VolumeProjection{
										{
											ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
												Audience:          "istio-ca",
												ExpirationSeconds: ptr.To(int64(43200)),
												Path:              "istio-token",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "wrong-volumeDevice-volumes",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								VolumeDevices: []corev1.VolumeDevice{
									{
										Name:       "disk0",
										DevicePath: "/dev/nvme1",
									},
								},
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "right-volumeDevice-volumes",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								VolumeDevices: []corev1.VolumeDevice{
									{
										Name:       "disk0",
										DevicePath: "/dev/nvme1",
									},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "disk0",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-0",
								},
							},
						},
					},
				},
			},
			expectErrs: 0,
		},
		{
			caseName: "volumeDevice-volumeMount-path-conflict",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								VolumeDevices: []corev1.VolumeDevice{
									{
										Name:       "disk0",
										DevicePath: "/home/admin/log",
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "log",
										MountPath: "/home/admin/log",
									},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "disk0",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-0",
								},
							},
						},
						{
							Name: "log",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
			expectErrs: 2,
		},
		{
			caseName: "wrong-metadata",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
					PatchPodMetadata: []appsv1beta1.SidecarSetPatchPodMetadata{
						{
							Annotations: map[string]string{
								"key1": "value1",
							},
							PatchPolicy: appsv1beta1.SidecarSetMergePatchJsonPatchPolicy,
						},
						{
							Annotations: map[string]string{
								"key1": "value1",
							},
							PatchPolicy: appsv1beta1.SidecarSetOverwritePatchPolicy,
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "wrong-name-injectionStrategy",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					InjectionStrategy: appsv1beta1.SidecarSetInjectionStrategy{
						Revision: &appsv1beta1.SidecarSetInjectRevision{
							CustomVersion: pointer.String("normal-sidecarset-01234"),
						},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "not-existing-injectionStrategy",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					InjectionStrategy: appsv1beta1.SidecarSetInjectionStrategy{
						Revision: &appsv1beta1.SidecarSetInjectRevision{
							RevisionName: pointer.String("test-sidecarset-678235"),
							Policy:       appsv1beta1.AlwaysSidecarSetInjectRevisionPolicy,
						},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "The initContainer in-place upgrade is not currently supported.",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.RollingUpdateSidecarSetStrategyType,
					},
					InitContainers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								RestartPolicy:            &always,
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		// ResourcesPolicy validation test cases
		{
			caseName: "resources-policy-and-resources-conflict",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("100Mi"),
									},
								},
							},
							ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
								TargetContainersMode: appsv1beta1.TargetContainersModeSum,
								ResourcesExpr: appsv1beta1.ResourcesExpr{
									Limits: &appsv1beta1.ResourceExprLimits{
										CPU: "cpu*50%",
									},
								},
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "invalid-target-container-mode",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Containers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
							ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
								TargetContainersMode: "invalid",
								ResourcesExpr: appsv1beta1.ResourcesExpr{
									Limits: &appsv1beta1.ResourceExprLimits{
										CPU: "cpu*50%",
									},
								},
							},
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "initContainer-with-resources-policy-forbidden",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					InitContainers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-init",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								// Plain init-container without RestartPolicy: Always
							},
							ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
								TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
								TargetContainersNameRegex: ".*",
								ResourcesExpr: appsv1beta1.ResourcesExpr{
									Limits: &appsv1beta1.ResourceExprLimits{
										CPU: "cpu*50%",
									},
								},
							},
						},
					},
				},
			},
			expectErrs: 1, // Should fail because ResourcesPolicy is only allowed for native sidecars (RestartPolicy: Always)
		},
		{
			caseName: "native-sidecar-initContainer-with-resources-policy-allowed",
			sidecarSet: appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1beta1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
					InitContainers: []appsv1beta1.SidecarContainer{
						{
							PodInjectPolicy: appsv1beta1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1beta1.ShareVolumePolicy{
								Type: appsv1beta1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1beta1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1beta1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-init",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								RestartPolicy:            &always, // Native sidecar container
							},
							ResourcesPolicy: &appsv1beta1.ResourcesPolicy{
								TargetContainersMode:      appsv1beta1.TargetContainersModeSum,
								TargetContainersNameRegex: ".*",
								ResourcesExpr: appsv1beta1.ResourcesExpr{
									Limits: &appsv1beta1.ResourceExprLimits{
										CPU: "cpu*50%",
									},
								},
							},
						},
					},
				},
			},
			expectErrs: 0, // Should pass because native sidecar (RestartPolicy: Always) can have ResourcesPolicy
		},
	}

	SidecarSetRevisions := []client.Object{
		&apps.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-sidecarset-01234",
			},
		},
		&apps.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-sidecarset-56789",
			},
		},
	}
	_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.SidecarSetPatchPodMetadataDefaultsAllowed))
	for _, tc := range testErrorCases {
		t.Run(tc.caseName, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(SidecarSetRevisions...).Build()
			handler.Client = fakeClient
			allErrs := handler.validateSidecarSetSpec(&tc.sidecarSet, field.NewPath("spec"))
			if len(allErrs) != tc.expectErrs {
				t.Errorf("%v: expect errors len %v, but got: len %d %v", tc.caseName, tc.expectErrs, len(allErrs), util.DumpJSON(allErrs))
			} else {
				fmt.Printf("%v: %v\n", tc.caseName, allErrs)
			}
		})
	}
}

func TestSidecarSetNameConflict(t *testing.T) {
	cases := []struct {
		name           string
		getSidecarSet  func() *appsv1beta1.SidecarSet
		getSidecarList func() *appsv1beta1.SidecarSetList
		expect         int
	}{
		{
			name: "test1",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				return &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
					Spec: appsv1beta1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"a": "b"},
						},
						Containers: []appsv1beta1.SidecarContainer{
							{
								Container: corev1.Container{Name: "container-name"},
							},
						},
						InitContainers: []appsv1beta1.SidecarContainer{
							{
								Container: corev1.Container{Name: "init-name"},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "volume-name",
							},
						},
					},
				}
			},
			getSidecarList: func() *appsv1beta1.SidecarSetList {
				return &appsv1beta1.SidecarSetList{
					Items: []appsv1beta1.SidecarSet{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
							Spec: appsv1beta1.SidecarSetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"a": "b"},
								},
								Containers: []appsv1beta1.SidecarContainer{
									{
										Container: corev1.Container{Name: "container-name"},
									},
								},
								InitContainers: []appsv1beta1.SidecarContainer{
									{
										Container: corev1.Container{Name: "init-name"},
									},
								},
								Volumes: []corev1.Volume{
									{
										Name: "volume-name",
									},
								},
							},
						},
					},
				}
			},
			expect: 2,
		},
		{
			name: "test2",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				return &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
					Spec: appsv1beta1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"a": "b"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						Containers: []appsv1beta1.SidecarContainer{
							{
								Container: corev1.Container{Name: "container-name"},
							},
						},
					},
				}
			},
			getSidecarList: func() *appsv1beta1.SidecarSetList {
				return &appsv1beta1.SidecarSetList{
					Items: []appsv1beta1.SidecarSet{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
							Spec: appsv1beta1.SidecarSetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"a": "b"},
								},

								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": "nginx"},
								},

								Containers: []appsv1beta1.SidecarContainer{
									{
										Container: corev1.Container{Name: "container-name"},
									},
								},
							},
						},
					},
				}
			},
			expect: 1,
		},
		{
			name: "test3",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				return &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
					Spec: appsv1beta1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"a": "b"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						Containers: []appsv1beta1.SidecarContainer{
							{
								Container: corev1.Container{Name: "container-name"},
							},
						},
					},
				}
			},
			getSidecarList: func() *appsv1beta1.SidecarSetList {
				return &appsv1beta1.SidecarSetList{
					Items: []appsv1beta1.SidecarSet{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
							Spec: appsv1beta1.SidecarSetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"a": "b"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": "redis"},
								},
								Containers: []appsv1beta1.SidecarContainer{
									{
										Container: corev1.Container{Name: "container-name"},
									},
								},
							},
						},
					},
				}
			},
			expect: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			allErrs := validateSidecarConflict(nil, cs.getSidecarList(), cs.getSidecarSet(), field.NewPath("spec.containers"))
			if cs.expect != len(allErrs) {
				t.Fatalf("expect(%v), but get(%v)", cs.expect, len(allErrs))
			}
		})
	}
}

type TestCase struct {
	Input  [2]metav1.LabelSelector
	Output bool
}

func TestSelectorConflict(t *testing.T) {
	testCases := []TestCase{
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"a": "i"},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchLabels: map[string]string{"b": "i"},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						"a": "h",
						"b": "i",
						"c": "j",
					},
				},
				{
					MatchLabels: map[string]string{
						"a": "h",
						"b": "x",
						"c": "j",
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"h", "i", "j"},
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"i", "j"},
						},
					},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"h", "i"},
						},
					},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: false,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"a": "h"},
				},
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			Output: true,
		},
		{
			Input: [2]metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "a",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				{
					MatchLabels: map[string]string{"a": "h"},
				},
			},
			Output: false,
		},
	}

	for i, testCase := range testCases {
		output := util.IsSelectorOverlapping(&testCase.Input[0], &testCase.Input[1])
		if output != testCase.Output {
			t.Errorf("%v: expect %v but got %v", i, testCase.Output, output)
		}
	}
}

func TestSidecarSetPodMetadataConflict(t *testing.T) {
	cases := []struct {
		name              string
		getSidecarSet     func() *appsv1beta1.SidecarSet
		getSidecarSetList func() *appsv1beta1.SidecarSetList
		expectErrLen      int
	}{
		{
			name: "sidecarset annotation key conflict",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				demo := sidecarset.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1beta1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1beta1.SidecarSetOverwritePatchPolicy,
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getSidecarSetList: func() *appsv1beta1.SidecarSetList {
				demo := sidecarsetList.DeepCopy()
				demo.Items[0].Spec.PatchPodMetadata = []appsv1beta1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1beta1.SidecarSetMergePatchJsonPatchPolicy,
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			expectErrLen: 1,
		},
		{
			name: "sidecarset annotation key different",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				demo := sidecarset.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1beta1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1beta1.SidecarSetRetainPatchPolicy,
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getSidecarSetList: func() *appsv1beta1.SidecarSetList {
				demo := sidecarsetList.DeepCopy()
				demo.Items[0].Spec.PatchPodMetadata = []appsv1beta1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1beta1.SidecarSetOverwritePatchPolicy,
						Annotations: map[string]string{
							"key2": "value2",
						},
					},
				}
				return demo
			},
			expectErrLen: 0,
		},
		{
			name: "sidecarset annotation key same, and json",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				demo := sidecarset.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1beta1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1beta1.SidecarSetMergePatchJsonPatchPolicy,
						Annotations: map[string]string{
							"oom-score": `{"log-agent": 1}`,
						},
					},
				}
				return demo
			},
			getSidecarSetList: func() *appsv1beta1.SidecarSetList {
				demo := sidecarsetList.DeepCopy()
				demo.Items[0].Spec.PatchPodMetadata = []appsv1beta1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1beta1.SidecarSetMergePatchJsonPatchPolicy,
						Annotations: map[string]string{
							"oom-score": `{"envoy": 1}`,
						},
					},
				}
				return demo
			},
			expectErrLen: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			sidecar := cs.getSidecarSet()
			list := cs.getSidecarSetList()
			errs := validateSidecarConflict(nil, list, sidecar, field.NewPath("spec"))
			if len(errs) != cs.expectErrLen {
				t.Fatalf("except ErrLen(%d), but get errs(%d)", cs.expectErrLen, len(errs))
			}
		})
	}
}

func TestSidecarSetVolumeConflict(t *testing.T) {
	cases := []struct {
		name              string
		getSidecarSet     func() *appsv1beta1.SidecarSet
		getSidecarSetList func() *appsv1beta1.SidecarSetList
		expectErrLen      int
	}{
		{
			name: "sidecarset volume name different",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				newSidecar := sidecarset.DeepCopy()
				newSidecar.Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-1",
					},
					{
						Name: "volume-2",
					},
				}
				return newSidecar
			},
			getSidecarSetList: func() *appsv1beta1.SidecarSetList {
				newSidecarList := sidecarsetList.DeepCopy()
				newSidecarList.Items[0].Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-3",
					},
					{
						Name: "volume-4",
					},
				}
				return newSidecarList
			},
			expectErrLen: 0,
		},
		{
			name: "sidecarset volume name same and equal",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				newSidecar := sidecarset.DeepCopy()
				newSidecar.Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-1",
					},
					{
						Name: "volume-2",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/home/work",
							},
						},
					},
				}
				return newSidecar
			},
			getSidecarSetList: func() *appsv1beta1.SidecarSetList {
				newSidecarList := sidecarsetList.DeepCopy()
				newSidecarList.Items[0].Spec.Volumes = []corev1.Volume{
					{
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/home/work",
							},
						},
						Name: "volume-2",
					},
					{
						Name: "volume-3",
					},
				}
				return newSidecarList
			},
			expectErrLen: 0,
		},
		{
			name: "sidecarset volume name same, but not equal",
			getSidecarSet: func() *appsv1beta1.SidecarSet {
				newSidecar := sidecarset.DeepCopy()
				newSidecar.Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-1",
					},
					{
						Name: "volume-2",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/home/work-1",
							},
						},
					},
				}
				return newSidecar
			},
			getSidecarSetList: func() *appsv1beta1.SidecarSetList {
				newSidecarList := sidecarsetList.DeepCopy()
				newSidecarList.Items[0].Spec.Volumes = []corev1.Volume{
					{
						Name: "volume-2",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/home/work-2",
							},
						},
					},
					{
						Name: "volume-3",
					},
				}
				return newSidecarList
			},
			expectErrLen: 1,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			sidecar := cs.getSidecarSet()
			list := cs.getSidecarSetList()
			errs := validateSidecarConflict(nil, list, sidecar, field.NewPath("spec"))
			if len(errs) != cs.expectErrLen {
				t.Fatalf("except ErrLen(%d), but get errs(%d)", cs.expectErrLen, len(errs))
			}
		})
	}
}

func TestValidateSidecarSetCanaryAnnotations(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	tests := []struct {
		name           string
		obj            *appsv1beta1.SidecarSet
		older          *appsv1beta1.SidecarSet
		setupClient    func()
		expectedErrors field.ErrorList
	}{
		{
			name: "non-canary sidecarset",
			obj: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-sidecarset",
					Annotations: map[string]string{},
				},
			},
			older:          nil,
			expectedErrors: field.ErrorList{},
		},
		{
			name: "canary sidecarset with base set to itself",
			obj: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "test-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			older: nil,
			expectedErrors: field.ErrorList{
				&field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "metadata",
					BadValue: map[string]string{},
					Detail:   "base sidecarSet cannot be itself",
				},
			},
			setupClient: func() {
				_ = fakeClient.Create(context.TODO(), &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sidecarset",
					},
				})
			},
		},
		{
			name: "canary sidecarset with valid base that exists",
			obj: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "base-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			older: nil,
			setupClient: func() {
				_ = fakeClient.Create(context.TODO(), &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "base-sidecarset",
					},
				})
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "canary sidecarset with valid base that exists, but base is canary",
			obj: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "base1-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			older: nil,
			setupClient: func() {
				_ = fakeClient.Create(context.TODO(), &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "base1-sidecarset",
						Annotations: map[string]string{
							appsv1beta1.SidecarSetCanaryAnnotation: "true",
							appsv1beta1.SidecarSetBaseAnnotation:   "other-base-sidecarset",
						},
					},
				})
			},
			expectedErrors: field.ErrorList{
				&field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "metadata",
					BadValue: map[string]string{},
					Detail:   "base sidecarSet cannot be canary",
				},
			},
		},
		{
			name: "canary sidecarset with base that does not exist",
			obj: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "nonexistent-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			older: nil,
			expectedErrors: field.ErrorList{
				&field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "metadata",
					BadValue: map[string]string{},
					Detail:   "fetch base sidecarSet[nonexistent-sidecarset] failed: sidecarsets.apps.kruise.io \"nonexistent-sidecarset\" not found",
				},
			},
		},
		{
			name: "updating canary sidecarset with unchanged base",
			obj: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "base-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			older: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "base-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			setupClient: func() {
				_ = fakeClient.Create(context.TODO(), &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "base-sidecarset",
					},
				})
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "updating canary sidecarset with changed base",
			obj: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "new-base-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			older: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "old-base-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			setupClient: func() {
				_ = fakeClient.Create(context.TODO(), &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "new-base-sidecarset",
					},
				})
				_ = fakeClient.Create(context.TODO(), &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "old-base-sidecarset",
					},
				})
			},
			expectedErrors: field.ErrorList{
				&field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "metadata",
					BadValue: map[string]string{},
					Detail:   "annotations[apps.kruise.io/sidecarset-base] is immutable",
				},
			},
		},
		{
			name: "canary sidecarset with RollingUpdate strategy",
			obj: &appsv1beta1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sidecarset",
					Annotations: map[string]string{
						appsv1beta1.SidecarSetCanaryAnnotation: "true",
						appsv1beta1.SidecarSetBaseAnnotation:   "base-sidecarset",
					},
				},
				Spec: appsv1beta1.SidecarSetSpec{
					UpdateStrategy: appsv1beta1.SidecarSetUpdateStrategy{
						Type: appsv1beta1.RollingUpdateSidecarSetStrategyType,
					},
				},
			},
			older: nil,
			setupClient: func() {
				_ = fakeClient.Create(context.TODO(), &appsv1beta1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "base-sidecarset",
					},
				})
			},
			expectedErrors: field.ErrorList{
				&field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "metadata",
					BadValue: map[string]string{},
					Detail:   "RollingUpdate is not supported for canary sidecarSet",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupClient != nil {
				tt.setupClient()
			}

			result := validateSidecarSetCanaryAnnotations(fakeClient, tt.obj, tt.older)

			if len(result) != len(tt.expectedErrors) {
				t.Errorf("Expected %d errors, got %d", len(tt.expectedErrors), len(result))
				t.Logf("Expected: %v", tt.expectedErrors)
				t.Logf("Got: %v", result)
				return
			}

			for i, expectedErr := range tt.expectedErrors {
				if result[i].Type != expectedErr.Type {
					t.Errorf("Error[%d]: expected type %v, got %v", i, expectedErr.Type, result[i].Type)
				}
				if result[i].Field != expectedErr.Field {
					t.Errorf("Error[%d]: expected field %s, got %s", i, expectedErr.Field, result[i].Field)
				}
				if !strings.Contains(result[i].Detail, expectedErr.Detail) {
					t.Errorf("Error[%d]: expected detail %s, got %s", i, expectedErr.Detail, result[i].Detail)
				}
			}
		})
	}
}
