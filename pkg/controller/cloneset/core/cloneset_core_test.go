package core

import (
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
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
