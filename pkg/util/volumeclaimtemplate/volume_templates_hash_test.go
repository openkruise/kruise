package volumeclaimtemplate

import (
	"testing"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_vctHasher_GetExpectHash(t *testing.T) {
	type args struct {
		templates []v1.PersistentVolumeClaim
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{
			name: "zero-pvc",
			args: args{
				[]v1.PersistentVolumeClaim{},
			},
			want: 86995377,
		},
		{
			name: "nil-pvc",
			args: args{
				templates: nil,
			},
			want: 86995377,
		},
		{
			name: "single-pvc",
			args: args{
				[]v1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pvc-1",
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
							Resources: v1.VolumeResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("1Gi"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
			want: 1129974074,
		},
		{
			name: "multi-pvcs",
			args: args{
				[]v1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pvc-1",
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
							Resources: v1.VolumeResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("1Gi"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pvc-2",
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
							Resources: v1.VolumeResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("2Gi"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("4Gi"),
								},
							},
						},
					},
				},
			},
			want: 510584408,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &volumeClaimTemplatesHasher{}
			if got := h.getExpectHash(tt.args.templates); got != tt.want {
				t.Errorf("getExpectHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPatchVCTemplateHash(t *testing.T) {
	type args struct {
		revision    *apps.ControllerRevision
		vcTemplates []v1.PersistentVolumeClaim
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "base-revision-none-pvc",
			args: args{
				&apps.ControllerRevision{},
				[]v1.PersistentVolumeClaim{},
			},
			want: "",
		},
		{
			name: "base-revision-nil-pvc",
			args: args{
				&apps.ControllerRevision{},
				nil,
			},
			want: "",
		},
		{
			name: "base-revision-pvc",
			args: args{
				&apps.ControllerRevision{},
				[]v1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pvc-1",
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
							Resources: v1.VolumeResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("1Gi"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
			want: "1129974074",
		},
		{
			name: "base-revision-pvcs",
			args: args{
				&apps.ControllerRevision{},
				[]v1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pvc-1",
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
							Resources: v1.VolumeResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("1Gi"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pvc-2",
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
							Resources: v1.VolumeResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("2Gi"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceStorage: resource.MustParse("4Gi"),
								},
							},
						},
					},
				},
			},
			want: "510584408",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			PatchVCTemplateHash(tt.args.revision, tt.args.vcTemplates)
			if got, _ := GetVCTemplatesHash(tt.args.revision); got != tt.want {
				t.Errorf("getExpectHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetVCTemplatesHash(t *testing.T) {
	type args struct {
		revision *apps.ControllerRevision
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 bool
	}{
		// TODO: Add test cases.
		{
			name: "none-exist hash",
			args: args{
				&apps.ControllerRevision{},
			},
			want:  "",
			want1: false,
		},
		{
			name: "empty hash",
			args: args{
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "",
						},
					},
				},
			},
			want:  "",
			want1: true,
		},
		{
			name: "any hash",
			args: args{
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "1129974074",
						},
					},
				},
			},
			want:  "1129974074",
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := GetVCTemplatesHash(tt.args.revision)
			if got != tt.want {
				t.Errorf("GetVCTemplatesHash() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("GetVCTemplatesHash() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestCanVCTemplateInplaceUpdate(t *testing.T) {
	type args struct {
		oldRevision *apps.ControllerRevision
		newRevision *apps.ControllerRevision
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "both old and new are nil",
			args: args{
				&apps.ControllerRevision{},
				&apps.ControllerRevision{},
			},
			want: true,
		},
		{
			name: "old is nil 1",
			args: args{
				&apps.ControllerRevision{},
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "1129974074",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "old is nil 2",
			args: args{
				&apps.ControllerRevision{},
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "same empty",
			args: args{
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "",
						},
					},
				},
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "same not empty",
			args: args{
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "11",
						},
					},
				},
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "11",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "new is empty",
			args: args{
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "11",
						},
					},
				},
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "different",
			args: args{
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "11",
						},
					},
				},
				&apps.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							HashAnnotation: "22",
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanVCTemplateInplaceUpdate(tt.args.oldRevision, tt.args.newRevision); got != tt.want {
				t.Errorf("CanVCTemplateInplaceUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
