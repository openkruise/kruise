package core

import (
	// "encoding/json" // Removed if not used by other tests in this file
	// "reflect" // Removed if not used by other tests in this file
	"testing"

	"github.com/stretchr/testify/assert" // Ensure this import is correct
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/util/sets" // Keep if used, e.g., by TestIgnorePodUpdateEvent if it uses sets.NewString

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	// "github.com/openkruise/kruise/pkg/util/inplaceupdate" // Removed if not used by other tests
)

// --- Your existing Test_CommonControl_GetUpdateOptions function ---
// (Make sure its imports like "reflect" and "github.com/openkruise/kruise/pkg/util/inplaceupdate" are still needed by IT)
// For example, if Test_CommonControl_GetUpdateOptions uses reflect.DeepEqual, keep "reflect" imported.
// If it uses types from inplaceupdate, keep that imported.
// Based on your original file, Test_CommonControl_GetUpdateOptions does use reflect and inplaceupdate.
import (
	"github.com/openkruise/kruise/pkg/util/inplaceupdate" // Keep for Test_CommonControl_GetUpdateOptions
	"reflect"                                             // Keep for Test_CommonControl_GetUpdateOptions
)

// (Assuming Test_CommonControl_GetUpdateOptions is still here and unchanged from your original)
func Test_CommonControl_GetUpdateOptions(t *testing.T) {
	type fields struct {
		CloneSet *appsv1alpha1.CloneSet
	}

	defaultOps := &inplaceupdate.UpdateOptions{}
	ignoreVCTHashOps := &inplaceupdate.UpdateOptions{IgnoreVolumeClaimTemplatesHashDiff: true}
	tests := []struct {
		name   string
		fields fields
		want   *inplaceupdate.UpdateOptions
	}{
		{
			name: "inplace only update type",
			fields: fields{
				&appsv1alpha1.CloneSet{
					Spec: appsv1alpha1.CloneSetSpec{
						UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
							Type: appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType,
						},
					},
				},
			},
			want: ignoreVCTHashOps,
		},
		{
			name: "inplace if possible update type",
			fields: fields{
				&appsv1alpha1.CloneSet{
					Spec: appsv1alpha1.CloneSetSpec{
						UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
							Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
						},
					},
				},
			},
			want: defaultOps,
		},
		{
			name: "recreate update type",
			fields: fields{
				&appsv1alpha1.CloneSet{
					Spec: appsv1alpha1.CloneSetSpec{
						UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
							Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType, // This was same as above, intentional?
						},
					},
				},
			},
			want: defaultOps,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &commonControl{
				CloneSet: tt.fields.CloneSet,
			}
			if got := c.GetUpdateOptions(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetUpdateOptions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIgnorePodUpdateEvent(t *testing.T) {
	baseCS := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cs", Namespace: "default"},
	}
	preNormalTestFinalizer := "finalizers.sigs.k8s.io/test"

	tests := []struct {
		name     string
		option   func()
		cs       *appsv1alpha1.CloneSet
		oldPod   *v1.Pod
		curPod   *v1.Pod
		expected bool
	}{
		{
			name: "updating pod without InPlaceWorkloadVerticalScaling",
			option: func() {
				utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, false)
			},
			cs:     baseCS,
			oldPod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"}},
			curPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default",
					Annotations: map[string]string{appspub.InPlaceUpdateStateKey: `{"revision":"rev1"}`},
					Labels:      map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStateUpdating)},
				},
				Spec:   v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
				Status: v1.PodStatus{Conditions: []v1.PodCondition{{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue}}},
			},
			expected: true,
		},
		{
			name: "updating pod-condition false without InPlaceWorkloadVerticalScaling",
			option: func() {
				utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, false)
			},
			cs:     baseCS,
			oldPod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"}},
			curPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default",
					Annotations: map[string]string{appspub.InPlaceUpdateStateKey: `{"revision":"rev1"}`},
					Labels:      map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStateUpdating)},
				},
				Spec:   v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
				Status: v1.PodStatus{Conditions: []v1.PodCondition{{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionFalse}}},
			},
			expected: false,
		},
		{
			name: "updating pod with InPlaceWorkloadVerticalScaling",
			option: func() {
				utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, true)
			},
			cs:     baseCS,
			oldPod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"}},
			curPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default",
					Annotations: map[string]string{appspub.InPlaceUpdateStateKey: `{"revision":"rev1"}`},
					Labels:      map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStateUpdating)},
				},
				Spec:   v1.PodSpec{ReadinessGates: []v1.PodReadinessGate{{ConditionType: appspub.InPlaceUpdateReady}}},
				Status: v1.PodStatus{Conditions: []v1.PodCondition{{Type: appspub.InPlaceUpdateReady, Status: v1.ConditionTrue}}},
			},
			expected: false,
		},
		{
			name:     "pod without PreNormal finalizer (no change)",
			option:   func() {},
			cs:       baseCS,
			oldPod:   &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default", Labels: map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}}},
			curPod:   &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default", Labels: map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}}},
			expected: true,
		},
		{
			name:   "update pod with PreNormal finalizer hooked (finalizer added)",
			option: func() {},
			cs: &appsv1alpha1.CloneSet{ObjectMeta: metav1.ObjectMeta{Name: "test-cs-prenormal", Namespace: "default"},
				Spec: appsv1alpha1.CloneSetSpec{
					Lifecycle: &appspub.Lifecycle{
						PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{preNormalTestFinalizer}},
					},
				}},
			oldPod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default", Labels: map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}}},
			curPod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default",
				Labels:     map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)},
				Finalizers: []string{preNormalTestFinalizer},
			}},
			expected: false,
		},
		{
			name:   "update pod with PreNormal finalizer already hooked (no change to this finalizer)",
			option: func() {},
			cs: &appsv1alpha1.CloneSet{ObjectMeta: metav1.ObjectMeta{Name: "test-cs-prenormal", Namespace: "default"},
				Spec: appsv1alpha1.CloneSetSpec{
					Lifecycle: &appspub.Lifecycle{
						PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{preNormalTestFinalizer}},
					},
				}},
			oldPod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default",
				Labels:     map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)},
				Finalizers: []string{preNormalTestFinalizer},
			}},
			curPod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default",
				Labels:     map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)},
				Finalizers: []string{preNormalTestFinalizer},
			}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.option != nil {
				tt.option()
			}
			control := commonControl{CloneSet: tt.cs}
			if got := control.IgnorePodUpdateEvent(tt.oldPod, tt.curPod); got != tt.expected {
				t.Errorf("IgnorePodUpdateEvent() for %s = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestLifecycleFinalizerChanged(t *testing.T) {
	preNormalFin := "kruise.io/pre-normal-hook"
	preDeleteFin := "kruise.io/pre-delete-hook"         // Now used
	inPlaceUpdateFin := "kruise.io/inplace-update-hook" // Now used
	otherFin := "other-finalizer"

	csBase := &appsv1alpha1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cs", Namespace: "default"},
		Spec:       appsv1alpha1.CloneSetSpec{},
	}

	newPod := func(name string, labels map[string]string, finalizers []string) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Labels:     labels,
				Finalizers: finalizers,
			},
		}
	}

	tests := []struct {
		name          string
		csModifier    func(cs *appsv1alpha1.CloneSet)
		oldPod        *v1.Pod
		curPod        *v1.Pod
		expectChanged bool
	}{
		{
			name: "PreNormal: finalizer added, hook defined",
			csModifier: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Lifecycle = &appspub.Lifecycle{
					PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{preNormalFin}},
				}
			},
			oldPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin}),
			curPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin, preNormalFin}),
			expectChanged: true,
		},
		{
			name: "PreNormal: finalizer removed, hook defined",
			csModifier: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Lifecycle = &appspub.Lifecycle{
					PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{preNormalFin}},
				}
			},
			oldPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin, preNormalFin}),
			curPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin}),
			expectChanged: true,
		},
		{
			name: "PreNormal: no relevant PreNormal finalizer change, other finalizer changes",
			csModifier: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Lifecycle = &appspub.Lifecycle{
					PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{preNormalFin}},
				}
			},
			oldPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin}),
			curPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin, "another-fin"}),
			expectChanged: false,
		},
		{
			name: "PreNormal: hook not defined in spec, but finalizer matching name added",
			csModifier: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Lifecycle = &appspub.Lifecycle{}
			},
			oldPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin}),
			curPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin, preNormalFin}),
			expectChanged: false,
		},
		{
			name: "Lifecycle spec is nil, finalizers change",
			csModifier: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Lifecycle = nil
			},
			oldPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin}),
			curPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{otherFin, preNormalFin}),
			expectChanged: false,
		},
		{
			name: "No finalizer change at all, PreNormal hook defined",
			csModifier: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Lifecycle = &appspub.Lifecycle{
					PreNormal: &appspub.LifecycleHook{FinalizersHandler: []string{preNormalFin}},
				}
			},
			oldPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{preNormalFin}),
			curPod:        newPod("pod1", map[string]string{appspub.LifecycleStateKey: string(appspub.LifecycleStatePreparingNormal)}, []string{preNormalFin}),
			expectChanged: false,
		},
		{ // Added test case for preDeleteFin
			name: "PreDelete: finalizer added",
			csModifier: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Lifecycle = &appspub.Lifecycle{
					PreDelete: &appspub.LifecycleHook{FinalizersHandler: []string{preDeleteFin}},
				}
			},
			oldPod:        newPod("pod1", map[string]string{}, []string{otherFin}),
			curPod:        newPod("pod1", map[string]string{}, []string{otherFin, preDeleteFin}),
			expectChanged: true,
		},
		{ // Added test case for inPlaceUpdateFin
			name: "InPlaceUpdate: finalizer added",
			csModifier: func(cs *appsv1alpha1.CloneSet) {
				cs.Spec.Lifecycle = &appspub.Lifecycle{
					InPlaceUpdate: &appspub.LifecycleHook{FinalizersHandler: []string{inPlaceUpdateFin}},
				}
			},
			oldPod:        newPod("pod1", map[string]string{}, []string{otherFin}),
			curPod:        newPod("pod1", map[string]string{}, []string{otherFin, inPlaceUpdateFin}),
			expectChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := csBase.DeepCopy()
			if tt.csModifier != nil {
				tt.csModifier(cs)
			}
			changed := lifecycleFinalizerChanged(cs, tt.oldPod, tt.curPod)
			assert.Equal(t, tt.expectChanged, changed, "Test case: %s", tt.name)
		})
	}
}
