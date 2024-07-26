package pvc

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCompareWithCheckFn(t *testing.T) {
	// Define test cases
	tests := []struct {
		name               string
		claim              *v1.PersistentVolumeClaim
		template           *v1.PersistentVolumeClaim
		expectedMatch      bool
		expectedResizeOnly bool
	}{
		{
			name: "Matching claim and template",
			claim: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			expectedMatch:      true,
			expectedResizeOnly: false,
		},
		{
			name: "Different storage class",
			claim: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("gold"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			expectedMatch:      false,
			expectedResizeOnly: false,
		},
		{
			name: "Different access modes",
			claim: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadOnlyMany,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			expectedMatch:      false,
			expectedResizeOnly: false,
		},
		{
			name: "Claim requests less storage than template",
			claim: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("500Mi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			expectedMatch:      false,
			expectedResizeOnly: true,
		},
		{
			name: "Claim requests more storage than template",
			claim: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("5Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("5Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: pointerToString("standard"),
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Limits: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			expectedMatch:      true,
			expectedResizeOnly: false,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, resizeOnly := CompareWithCheckFn(tt.claim, tt.template, IsPVCNeedExpand)
			if matched != tt.expectedMatch {
				t.Errorf("Expected match: %v, got: %v", tt.expectedMatch, matched)
			}
			if resizeOnly != tt.expectedResizeOnly {
				t.Errorf("Expected resize only: %v, got: %v", tt.expectedResizeOnly, resizeOnly)
			}
		})
	}
}

func pointerToString(s string) *string {
	return &s
}

func TestPVCCompatibleAndReady(t *testing.T) {
	tests := []struct {
		name           string
		claim          *v1.PersistentVolumeClaim
		template       *v1.PersistentVolumeClaim
		expectedCompat bool
		expectedReady  bool
	}{
		{
			name: "PVC is compatible and ready",
			claim: &v1.PersistentVolumeClaim{
				Status: v1.PersistentVolumeClaimStatus{
					Capacity: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			expectedCompat: true,
			expectedReady:  true,
		},
		{
			name: "PVC is compatible but not ready",
			claim: &v1.PersistentVolumeClaim{
				Status: v1.PersistentVolumeClaimStatus{
					Capacity: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("512Mi"),
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			expectedCompat: true,
			expectedReady:  false,
		},
		{
			name: "PVC is compatible but not ready2",
			claim: &v1.PersistentVolumeClaim{
				Status: v1.PersistentVolumeClaimStatus{
					Capacity: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("512Mi"),
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			expectedCompat: true,
			expectedReady:  false,
		},
		{
			name: "PVC is compatible but not ready3",
			claim: &v1.PersistentVolumeClaim{
				Status: v1.PersistentVolumeClaimStatus{
					Capacity: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("4Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			},
			expectedCompat: true,
			expectedReady:  false,
		},
		{
			name: "PVC is not compatible",
			claim: &v1.PersistentVolumeClaim{
				Status: v1.PersistentVolumeClaimStatus{
					Capacity: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("512Mi"),
					},
				},
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("2Gi"),
						},
					},
				},
			},
			template: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("3Gi"),
						},
					},
				},
			},
			expectedCompat: false,
			expectedReady:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compatible, ready := IsPVCCompatibleAndReady(tt.claim, tt.template)
			if compatible != tt.expectedCompat {
				t.Errorf("expected compatible to be %v, got %v", tt.expectedCompat, compatible)
			}
			if ready != tt.expectedReady {
				t.Errorf("expected ready to be %v, got %v", tt.expectedReady, ready)
			}
		})
	}
}
