package core

import (
	"reflect"
	"testing"

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

func TestGeneratePodName(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		id        string
		shortName string
		longName  string
	}{
		{
			name:      "short prefix case",
			prefix:    "short-prefix",
			id:        "abcdefg",
			shortName: "short-prefix-abcdefg",
			longName:  "short-prefix-abcdefg",
		},
		{
			name:      "long prefix case",
			prefix:    "looooooooooooooooooooooooooooooooooooooooooooooooooooooooong-prefix",
			id:        "abcdefg",
			shortName: "loooooooooooooooooooooooooooooooooooooooooooooooooooooooabcdefg",
			longName:  "looooooooooooooooooooooooooooooooooooooooooooooooooooooooong-prefix-abcdefg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.CloneSetShortPodName, true)()
			generatedName := generatePodName(test.prefix, test.id)
			if generatedName != test.shortName || len(generatedName) > 63 {
				t.Fatalf("expect %s, but got %s", test.shortName, generatedName)
			}
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.CloneSetShortPodName, false)()
			generatedName := generatePodName(test.prefix, test.id)
			if generatedName != test.longName {
				t.Fatalf("expect %s, but got %s", test.longName, generatedName)
			}
		})
	}
}
