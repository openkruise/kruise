package validating

import (
	"context"
	"fmt"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/configuration"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/utils/sidecarcontrol"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"
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

	sidecarSetDemo = &appsv1alpha1.SidecarSet{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 123,
			Name:       "test-sidecarset",
			Labels: map[string]string{
				"app": "sidecar",
			},
		},
		Spec: appsv1alpha1.SidecarSetSpec{
			Containers: []appsv1alpha1.SidecarContainer{
				{
					Container: corev1.Container{
						Name:  "cold-sidecar",
						Image: "cold-image:v1",
					},
					UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
						UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade,
					},
				},
				{
					Container: corev1.Container{
						Name:  "hot-sidecar",
						Image: "hot-image:v1",
					},
					UpgradeStrategy: appsv1alpha1.SidecarContainerUpgradeStrategy{
						UpgradeType:          appsv1alpha1.SidecarContainerHotUpgrade,
						HotUpgradeEmptyImage: "hotupgrade:empty",
					},
				},
			},
		},
	}
)

var (
	testScheme *runtime.Scheme
	handler    = &SidecarSetCreateUpdateHandler{}
)

func init() {
	testScheme = runtime.NewScheme()
	apps.AddToScheme(testScheme)
	corev1.AddToScheme(testScheme)
}

func TestValidateSidecarSet(t *testing.T) {
	errorCases := map[string]appsv1alpha1.SidecarSet{
		"missing-selector": {
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
		"wrong-updateStrategy": {
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
		"wrong-selector": {
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
		"wrong-initContainer": {
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
		"missing-container": {
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
		"wrong-containers": {
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
		"wrong-volumes": {
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
				},
			},
		},
		"wrong-metadata": {
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
		"wrong-name-injectionStrategy": {
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
		"not-existing-injectionStrategy": {
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
	for name, sidecarSet := range errorCases {
		fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(SidecarSetRevisions...).Build()
		handler.Client = fakeClient
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
		handler.historyControl = sidecarcontrol.NewHistoryControl(nil, indexer, webhookutil.GetNamespace())
		allErrs := handler.validateSidecarSetSpec(&sidecarSet, field.NewPath("spec"))
		if len(allErrs) != 1 {
			t.Errorf("%v: expect errors len 1, but got: len %d %v", name, len(allErrs), util.DumpJSON(allErrs))
		} else {
			fmt.Printf("%v: %v\n", name, allErrs)
		}
	}
	_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.SidecarSetPatchPodMetadataDefaultsAllowed))
}

func TestSidecarSetNameConflict(t *testing.T) {
	listDemo := &appsv1alpha1.SidecarSetList{
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
	sidecarDemo := &appsv1alpha1.SidecarSet{
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
	allErrs := validateSidecarConflict(listDemo, sidecarDemo, field.NewPath("spec.containers"))
	if len(allErrs) != 2 {
		t.Errorf("expect errors len 2, but got: %v", len(allErrs))
	} else {
		fmt.Println(allErrs)
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
			errs := validateSidecarConflict(list, sidecar, field.NewPath("spec"))
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
			errs := validateSidecarConflict(list, sidecar, field.NewPath("spec"))
			if len(errs) != cs.expectErrLen {
				t.Fatalf("except ErrLen(%d), but get errs(%d)", cs.expectErrLen, len(errs))
			}
		})
	}
}

func TestValidateSidecarSetPatchMetadataWhitelist(t *testing.T) {
	cases := []struct {
		name          string
		getSidecarSet func() *appsv1alpha1.SidecarSet
		getKruiseCM   func() *corev1.ConfigMap
		expectErr     bool
	}{
		{
			name: "validate sidecarSet no patch Metadata",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				return nil
			},
			expectErr: false,
		},
		{
			name: "validate sidecarSet whitelist failed-1",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				return nil
			},
			expectErr: true,
		},
		{
			name: "validate sidecarSet whitelist success-1",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				demo := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"allowedAnnotationKeyExprs":["key.*"]}]}`,
					},
				}
				return demo
			},
			expectErr: false,
		},
		{
			name: "validate sidecarSet whitelist failed-2",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				demo := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"allowedAnnotationKeyExprs":["key2"]}]}`,
					},
				}
				return demo
			},
			expectErr: true,
		},
		{
			name: "validate sidecarSet whitelist failed-3",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				demo := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"allowedAnnotationKeyExprs":["key.*"],"selector":{"matchLabels":{"app":"other"}}}]}`,
					},
				}
				return demo
			},
			expectErr: true,
		},
		{
			name: "validate sidecarSet whitelist success-2",
			getSidecarSet: func() *appsv1alpha1.SidecarSet {
				demo := sidecarSetDemo.DeepCopy()
				demo.Spec.PatchPodMetadata = []appsv1alpha1.SidecarSetPatchPodMetadata{
					{
						Annotations: map[string]string{
							"key1": "value1",
						},
					},
				}
				return demo
			},
			getKruiseCM: func() *corev1.ConfigMap {
				demo := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configuration.KruiseConfigurationName,
						Namespace: util.GetKruiseNamespace(),
					},
					Data: map[string]string{
						configuration.SidecarSetPatchPodMetadataWhiteListKey: `{"rules":[{"allowedAnnotationKeyExprs":["key.*"],"selector":{"matchLabels":{"app":"sidecar"}}}]}`,
					},
				}
				return demo
			},
			expectErr: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
			if cs.getKruiseCM() != nil {
				err := fakeClient.Create(context.TODO(), cs.getKruiseCM())
				if err != nil {
					fmt.Println(err.Error())
				}
			}
			handler.Client = fakeClient
			err := handler.validateSidecarSetPatchMetadataWhitelist(cs.getSidecarSet())
			if cs.expectErr && err == nil {
				t.Fatalf("ValidateSidecarSetPatchMetadataWhitelist failed")
			} else if !cs.expectErr && err != nil {
				t.Fatalf("ValidateSidecarSetPatchMetadataWhitelist failed: %s", err.Error())
			}
		})
	}
}
