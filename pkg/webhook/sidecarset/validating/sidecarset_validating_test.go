package validating

import (
	"fmt"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	sidecarsetList = &appsv1alpha1.SidecarSetList{
		Items: []appsv1alpha1.SidecarSet{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
				},
			},
		},
	}
	sidecarset = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
		Spec: appsv1alpha1.SidecarSetSpec{
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
		sidecarSet appsv1alpha1.SidecarSet
		expectErrs int
	}{
		{
			caseName: "missing-selector",
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
						ScatterStrategy: appsv1alpha1.UpdateScatterStrategy{
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
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
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
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyEnabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
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
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyEnabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
			caseName: "wrong-namespace",
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
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
					Namespace: "ns-test",
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},

					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyEnabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
			caseName: "wrong-initContainer",
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
					},
					InitContainers: []appsv1alpha1.SidecarContainer{
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
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "wrong-containers",
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,

							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
												ExpirationSeconds: pointer.Int64Ptr(43200),
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
			caseName: "wrong-metadata",
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
							},
							Container: corev1.Container{
								Name:                     "test-sidecar",
								Image:                    "test-image",
								ImagePullPolicy:          corev1.PullIfNotPresent,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
					},
					PatchPodMetadata: []appsv1alpha1.SidecarSetPatchPodMetadata{
						{
							Annotations: map[string]string{
								"key1": "value1",
							},
							PatchPolicy: appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy,
						},
						{
							Annotations: map[string]string{
								"key1": "value1",
							},
							PatchPolicy: appsv1alpha1.SidecarSetOverwritePatchPolicy,
						},
					},
				},
			},
			expectErrs: 1,
		},
		{
			caseName: "wrong-name-injectionStrategy",
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					InjectionStrategy: appsv1alpha1.SidecarSetInjectionStrategy{
						Revision: &appsv1alpha1.SidecarSetInjectRevision{
							CustomVersion: pointer.String("normal-sidecarset-01234"),
						},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					InjectionStrategy: appsv1alpha1.SidecarSetInjectionStrategy{
						Revision: &appsv1alpha1.SidecarSetInjectRevision{
							RevisionName: pointer.String("test-sidecarset-678235"),
							Policy:       appsv1alpha1.AlwaysSidecarSetInjectRevisionPolicy,
						},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.NotUpdateSidecarSetStrategyType,
					},
					Containers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
			sidecarSet: appsv1alpha1.SidecarSet{
				ObjectMeta: metav1.ObjectMeta{Name: "test-sidecarset"},
				Spec: appsv1alpha1.SidecarSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"a": "b"},
					},
					UpdateStrategy: appsv1alpha1.SidecarSetUpdateStrategy{
						Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
					},
					InitContainers: []appsv1alpha1.SidecarContainer{
						{
							PodInjectPolicy: appsv1alpha1.BeforeAppContainerType,
							ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{
								Type: appsv1alpha1.ShareVolumePolicyDisabled,
							},
							UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
								UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
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
		getSidecarSet  func() *appsv1alpha1.SidecarSet
		getSidecarList func() *appsv1alpha1.SidecarSetList
		expect         int
	}{
		{
			name: "test1",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return &appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"a": "b"},
						},
						Containers: []appsv1alpha1.SidecarContainer{
							{
								Container: corev1.Container{Name: "container-name"},
							},
						},
						InitContainers: []appsv1alpha1.SidecarContainer{
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
			getSidecarList: func() *appsv1alpha1.SidecarSetList {
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
							Spec: appsv1alpha1.SidecarSetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"a": "b"},
								},
								Containers: []appsv1alpha1.SidecarContainer{
									{
										Container: corev1.Container{Name: "container-name"},
									},
								},
								InitContainers: []appsv1alpha1.SidecarContainer{
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
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return &appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"a": "b"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						Containers: []appsv1alpha1.SidecarContainer{
							{
								Container: corev1.Container{Name: "container-name"},
							},
						},
					},
				}
			},
			getSidecarList: func() *appsv1alpha1.SidecarSetList {
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
							Spec: appsv1alpha1.SidecarSetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"a": "b"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": "nginx"},
								},
								Containers: []appsv1alpha1.SidecarContainer{
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
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				return &appsv1alpha1.SidecarSet{
					ObjectMeta: metav1.ObjectMeta{Name: "sidecarset2"},
					Spec: appsv1alpha1.SidecarSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"a": "b"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						Containers: []appsv1alpha1.SidecarContainer{
							{
								Container: corev1.Container{Name: "container-name"},
							},
						},
					},
				}
			},
			getSidecarList: func() *appsv1alpha1.SidecarSetList {
				return &appsv1alpha1.SidecarSetList{
					Items: []appsv1alpha1.SidecarSet{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "sidecarset1"},
							Spec: appsv1alpha1.SidecarSetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"a": "b"},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": "redis"},
								},
								Containers: []appsv1alpha1.SidecarContainer{
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
		getSidecarSet     func() *appsv1alpha1.SidecarSet
		getSidecarSetList func() *appsv1alpha1.SidecarSetList
		expectErrLen      int
	}{
		{
			name: "sidecarset annotation key conflict",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarset.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetOverwritePatchPolicy,
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
				demo := sidecarsetList.DeepCopy()
				demo.Items[0].Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy,
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
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarset.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetRetainPatchPolicy,
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
				demo := sidecarsetList.DeepCopy()
				demo.Items[0].Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetOverwritePatchPolicy,
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
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarset.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy,
						Annotations: map[string]string{
							"oom-score": `{"log-agent": 1}`,
						},
					},
				}
				return demo
			},
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
				demo := sidecarsetList.DeepCopy()
				demo.Items[0].Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						PatchPolicy: appsv1alpha1.SidecarSetMergePatchJsonPatchPolicy,
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
		getSidecarSet     func() *appsv1alpha1.SidecarSet
		getSidecarSetList func() *appsv1alpha1.SidecarSetList
		expectErrLen      int
	}{
		{
			name: "sidecarset volume name different",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
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
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
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
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
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
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
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
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
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
			getSidecarSetList: func() *appsv1alpha1.SidecarSetList {
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
