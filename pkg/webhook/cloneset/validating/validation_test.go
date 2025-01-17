package validating

import (
	"fmt"
	"strconv"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openkruise/kruise/apis/apps/defaults"
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

type testCase struct {
	spec    *appsv1alpha1.CloneSetSpec
	oldSpec *appsv1alpha1.CloneSetSpec
}

func TestValidate(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	validPodTemplate := v1.PodTemplate{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
				DNSPolicy:     v1.DNSClusterFirst,
				Containers:    []v1.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: v1.TerminationMessageReadFile}},
			},
		},
	}
	validPodTemplate1 := v1.PodTemplate{
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
				DNSPolicy:     v1.DNSClusterFirst,
				Containers:    []v1.Container{{Name: "abc", Image: "image1", ImagePullPolicy: "IfNotPresent", TerminationMessagePolicy: v1.TerminationMessageReadFile}},
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
				Containers:    []v1.Container{{Name: "abc", Image: "image2", ImagePullPolicy: "Always", TerminationMessagePolicy: v1.TerminationMessageReadFile}},
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

	var valTrue = true
	var val1 int32 = 1
	var val2 int32 = 2
	var minus1 int32 = -1
	intOrStr0 := intstr.FromInt(0)
	intOrStr1 := intstr.FromInt(1)
	maxUnavailable120Percent := intstr.FromString("120%")

	uid := uuid.NewUUID()
	p0 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "p0",
			Namespace:       metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{{UID: uid, Controller: &valTrue}},
		},
	}

	successCases := []testCase{
		{
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
				ScaleStrategy: appsv1alpha1.CloneSetScaleStrategy{
					PodsToDelete: []string{"p0"},
				},
			},
		},
		{
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
					MaxSurge:       &intOrStr1,
				},
			},
		},
		{
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr0,
					MaxSurge:       &intOrStr1,
				},
			},
		},
		{
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &maxUnavailable120Percent,
				},
			},
		},
		{
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &maxUnavailable120Percent,
					PriorityStrategy: &appspub.UpdatePriorityStrategy{
						WeightPriority: []appspub.UpdatePriorityWeightTerm{
							{Weight: 20, MatchSelector: metav1.LabelSelector{MatchLabels: map[string]string{"key": "foo"}}},
						},
					},
				},
			},
		},
		{
			// test for all acceptable CloneSetSpec changes
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas:             &val1,
				Selector:             &metav1.LabelSelector{MatchLabels: validLabels},
				Template:             validPodTemplate.Template,
				RevisionHistoryLimit: &val1,
				ScaleStrategy: appsv1alpha1.CloneSetScaleStrategy{
					PodsToDelete: []string{"p0"},
				},
				VolumeClaimTemplates: []v1.PersistentVolumeClaim{validVolumeClaimTemplate("30Gi")},
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
			oldSpec: &appsv1alpha1.CloneSetSpec{
				Replicas:             &val2,
				Selector:             &metav1.LabelSelector{MatchLabels: validLabels},
				Template:             validPodTemplate1.Template,
				RevisionHistoryLimit: &val2,
				ScaleStrategy: appsv1alpha1.CloneSetScaleStrategy{
					PodsToDelete: []string{"p1"},
				},
				VolumeClaimTemplates: []v1.PersistentVolumeClaim{validVolumeClaimTemplate("60Gi")},
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.RecreateCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
		},
		{
			// test oldSpec not-exits spec exits case
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas:             &val1,
				Selector:             &metav1.LabelSelector{MatchLabels: validLabels},
				Template:             validPodTemplate.Template,
				RevisionHistoryLimit: &val1,
				ScaleStrategy: appsv1alpha1.CloneSetScaleStrategy{
					PodsToDelete: []string{"p0"},
				},
				VolumeClaimTemplates: []v1.PersistentVolumeClaim{validVolumeClaimTemplate("30Gi")},
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
			oldSpec: &appsv1alpha1.CloneSetSpec{
				Replicas:             &val2,
				Selector:             &metav1.LabelSelector{MatchLabels: validLabels},
				Template:             validPodTemplate1.Template,
				RevisionHistoryLimit: &val2,
				ScaleStrategy: appsv1alpha1.CloneSetScaleStrategy{
					PodsToDelete: []string{},
				},
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.RecreateCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
		},
	}

	for i, successCase := range successCases {
		t.Run("success case "+strconv.Itoa(i), func(t *testing.T) {
			if i == 5 {
				fmt.Println("------")
			}
			obj := appsv1alpha1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cs-%d", i), Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "2"},
				Spec:       *successCase.spec,
			}
			h := CloneSetCreateUpdateHandler{Client: fake.NewClientBuilder().WithObjects(&p0).Build()}
			if successCase.oldSpec == nil {
				if errs := h.validateCloneSet(&obj, nil); len(errs) != 0 {
					t.Errorf("expected success: %v", errs)
				}
			} else {
				oldObj := appsv1alpha1.CloneSet{
					ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cs-%d", i), Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "1"},
					Spec:       *successCase.oldSpec,
				}
				defaults.SetDefaultPodSpec(&oldObj.Spec.Template.Spec)
				defaults.SetDefaultPodSpec(&obj.Spec.Template.Spec)
				if errs := h.validateCloneSetUpdate(&obj, &oldObj); len(errs) != 0 {
					t.Errorf("expected success: %v", errs)
				}
			}
		})
	}

	errorCases := map[string]testCase{
		"invalid-replicas": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &minus1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
		},
		"invalid-template": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: invalidPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
		},
		"invalid-selector": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"a": "b", "c": "d"}},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
		},
		"invalid-update-type": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           "",
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
		},
		"invalid-partition": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(-1)),
					MaxUnavailable: &intOrStr1,
				},
			},
		},
		"invalid-maxUnavailable": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr0,
				},
			},
		},
		"invalid-maxSurge": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr0,
					MaxSurge:       &intOrStr0,
				},
			},
		},
		"invalid-podsToDelete-1": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
				ScaleStrategy: appsv1alpha1.CloneSetScaleStrategy{
					PodsToDelete: []string{"p0", "p0"},
				},
			},
		},
		"invalid-podsToDelete-2": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
				ScaleStrategy: appsv1alpha1.CloneSetScaleStrategy{
					PodsToDelete: []string{"p0", "p1"},
				},
			},
		},
		// test pod-to-delete not-exits case
		"invalid-podsToDelete-3": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
				ScaleStrategy: appsv1alpha1.CloneSetScaleStrategy{
					PodsToDelete: []string{"p1"},
				},
			},
		},
		"invalid-cloneset-update-1": {
			spec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
			oldSpec: &appsv1alpha1.CloneSetSpec{
				Replicas: &val1,
				Selector: &metav1.LabelSelector{MatchLabels: validLabels},
				Template: validPodTemplate2.Template,
				UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
					Type:           appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType,
					Partition:      util.GetIntOrStrPointer(intstr.FromInt(2)),
					MaxUnavailable: &intOrStr1,
				},
			},
		},
	}

	for k, v := range errorCases {
		t.Run(k, func(t *testing.T) {
			obj := appsv1alpha1.CloneSet{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cs-%v", k), Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "2"},
				Spec:       *v.spec,
			}
			h := CloneSetCreateUpdateHandler{Client: fake.NewClientBuilder().WithObjects(&p0).Build()}
			if v.oldSpec == nil {
				if errs := h.validateCloneSet(&obj, nil); len(errs) == 0 {
					t.Errorf("expected failure for %v", k)
				}
			} else {
				oldObj := appsv1alpha1.CloneSet{
					ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cs-%v", k), Namespace: metav1.NamespaceDefault, UID: uid, ResourceVersion: "1"},
					Spec:       *v.oldSpec,
				}
				if errs := h.validateCloneSetUpdate(&obj, &oldObj); len(errs) == 0 {
					t.Errorf("expected failure for %v", k)
				}
			}
		})
	}
}
