package workloadspread

import (
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func TestNestedField(t *testing.T) {
	type args struct {
		obj   map[string]any
		paths []string
	}
	type testCase[T any] struct {
		name    string
		args    args
		want    T
		exists  bool
		wantErr bool
	}
	ud := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ud",
			Namespace: "ns",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: ptr.To(int32(5)),
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset1",
						Replicas: &intstrutil.IntOrString{
							Type:   intstrutil.Int,
							IntVal: 5,
						},
					},
				},
			},
		},
	}
	un, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ud)
	if err != nil {
		t.Fatal(err)
	}
	tests := []testCase[int64]{
		{
			name: "exists",
			args: args{
				obj: map[string]any{
					"m": []any{int64(1)},
				},
				paths: []string{"m", "0"},
			},
			want:   1,
			exists: true,
		},
		{
			name: "no key",
			args: args{
				obj: map[string]any{
					"m": []any{1},
				},
				paths: []string{"n", "0"},
			},
			want:    0,
			exists:  false,
			wantErr: true,
		},
		{
			name: "bad type",
			args: args{
				obj: map[string]any{
					"m": []any{"1"},
				},
				paths: []string{"m", "0"},
			},
			want:    0,
			exists:  false,
			wantErr: true,
		},
		{
			name: "not deep enough",
			args: args{
				obj: map[string]any{
					"m": []any{1},
				},
				paths: []string{"m", "0", "n"},
			},
			want:    0,
			exists:  false,
			wantErr: true,
		},
		{
			name: "bad slice index",
			args: args{
				obj: map[string]any{
					"m": []any{1},
				},
				paths: []string{"m", "a"},
			},
			want:    0,
			exists:  false,
			wantErr: true,
		},
		{
			name: "slice out of range",
			args: args{
				obj: map[string]any{
					"m": []any{1},
				},
				paths: []string{"m", "10"},
			},
			want:    0,
			exists:  false,
			wantErr: true,
		},
		{
			name: "real",
			args: args{
				obj:   un,
				paths: []string{"spec", "topology", "subsets", "0", "replicas"},
			},
			want:    5,
			exists:  true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, exists, err := NestedField[int64](tt.args.obj, tt.args.paths...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NestedField() error = %s, wantErr %v", err.Error(), tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NestedField() got = %v, want %v", got, tt.want)
			}
			if exists != tt.exists {
				t.Errorf("NestedField() exists = %v, want %v", exists, tt.exists)
			}
		})
	}
}

func TestIsPodSelected(t *testing.T) {
	commonFilter := &appsv1alpha1.TargetFilter{
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "selected",
			},
		},
	}
	cases := []struct {
		name     string
		filter   *appsv1alpha1.TargetFilter
		labels   map[string]string
		selected bool
		wantErr  bool
	}{
		{
			name:   "selected",
			filter: commonFilter,
			labels: map[string]string{
				"app": "selected",
			},
			selected: true,
		},
		{
			name:   "not selected",
			filter: commonFilter,
			labels: map[string]string{
				"app": "not-selected",
			},
			selected: false,
		},
		{
			name:   "selector is nil",
			filter: nil,
			labels: map[string]string{
				"app": "selected",
			},
			selected: true,
		},
		{
			name: "selector is invalid",
			filter: &appsv1alpha1.TargetFilter{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "selected",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "Invalid",
							Values:   []string{"selected"},
						},
					},
				},
			},
			selected: false,
			wantErr:  true,
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			selected, err := IsPodSelected(cs.filter, cs.labels)
			if selected != cs.selected || (err != nil) != cs.wantErr {
				t.Fatalf("got unexpected result, actual: [selected=%v,err=%v] expected: [selected=%v,wantErr=%v]",
					selected, err, cs.selected, cs.wantErr)
			}
		})
	}
}
