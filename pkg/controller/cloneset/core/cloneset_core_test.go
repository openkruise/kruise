package core

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
)

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
			// unexpected case: the method should not be called with recreate update strategy type.
			name: "recreate update type",
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
	c := commonControl{CloneSet: &appsv1alpha1.CloneSet{}}

	tests := []struct {
		name     string
		option   func()
		oldPod   *v1.Pod
		curPod   *v1.Pod
		expected bool
	}{
		{
			name: "updating pod without InPlaceWorkloadVerticalScaling",
			option: func() {
				utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, false)
			},
			oldPod: &v1.Pod{},
			curPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appspub.InPlaceUpdateStateKey: "{}",
					},
					Labels: map[string]string{
						appspub.LifecycleStateKey: string(appspub.LifecycleStateUpdating),
					},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   appspub.InPlaceUpdateReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "updating pod-condition false without InPlaceWorkloadVerticalScaling",
			option: func() {
				utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, false)
			},
			oldPod: &v1.Pod{},
			curPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appspub.InPlaceUpdateStateKey: "{}",
					},
					Labels: map[string]string{
						appspub.LifecycleStateKey: string(appspub.LifecycleStateUpdating),
					},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   appspub.InPlaceUpdateReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "updating pod with InPlaceWorkloadVerticalScaling",
			option: func() {
				utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.InPlaceWorkloadVerticalScaling, true)
			},
			oldPod: &v1.Pod{},
			curPod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						appspub.InPlaceUpdateStateKey: "{}",
					},
					Labels: map[string]string{
						appspub.LifecycleStateKey: string(appspub.LifecycleStateUpdating),
					},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   appspub.InPlaceUpdateReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.option()
			if got := c.IgnorePodUpdateEvent(tt.oldPod, tt.curPod); got != tt.expected {
				t.Errorf("IgnorePodUpdateEvent() = %v, want %v", got, tt.expected)
			}
		})
	}
}
