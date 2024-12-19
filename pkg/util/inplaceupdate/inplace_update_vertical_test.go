package inplaceupdate

import (
	"testing"

	"github.com/appscode/jsonpatch"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestValidateResourcePatch(t *testing.T) {
	oldTemp := &v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "test-container",
					Resources: v1.ResourceRequirements{
						Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("100m")},
						Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("512Mi")},
					},
				},
				{
					Name: "test-container2",
					Resources: v1.ResourceRequirements{
						Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("100m")},
						Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("512Mi")},
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		op          *jsonpatch.Operation
		validateFn  func(updateSpec *UpdateSpec) bool
		expectedErr bool
	}{
		{
			name: "valid patch for cpu limits",
			op: &jsonpatch.Operation{
				Path:  "/spec/containers/0/resources/limits/cpu",
				Value: "200m",
			},
			validateFn: func(updateSpec *UpdateSpec) bool {
				res := updateSpec.ContainerResources["test-container"].Limits
				return res.Cpu().String() == "200m"
			},
			expectedErr: false,
		},
		{
			name: "valid patch for memory requests",
			op: &jsonpatch.Operation{
				Path:  "/spec/containers/0/resources/requests/memory",
				Value: "1Gi",
			},
			validateFn: func(updateSpec *UpdateSpec) bool {
				res := updateSpec.ContainerResources["test-container"].Requests
				return res.Memory().String() == "1Gi"
			},
			expectedErr: false,
		},
		{
			name: "invalid patch for non-existent container",
			op: &jsonpatch.Operation{
				Path:  "/spec/containers/2/resources/limits/cpu",
				Value: "200m",
			},
			expectedErr: true,
		},
		{
			name: "invalid patch for non-standard resource",
			op: &jsonpatch.Operation{
				Path:  "/spec/containers/0/resources/limits/gpu",
				Value: "1",
			},
			expectedErr: true,
		},
		{
			name: "invalid patch for non-quantity value",
			op: &jsonpatch.Operation{
				Path:  "/spec/containers/0/resources/limits/cpu",
				Value: "not-a-quantity",
			},
			expectedErr: true,
		},
		{
			name: "add resource",
			op: &jsonpatch.Operation{
				Operation: "add",
				Path:      "/spec/containers/0/resources/limits/cpu",
				Value:     "10m",
			},
			expectedErr: true,
		},
		{
			name: "remove resource",
			op: &jsonpatch.Operation{
				Operation: "remove",
				Path:      "/spec/containers/0/resources/limits/cpu",
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateSpec := &UpdateSpec{
				ContainerResources: make(map[string]v1.ResourceRequirements),
			}
			vu := &NativeVerticalUpdate{}
			err := vu.UpdateInplaceUpdateMetadata(tt.op, oldTemp, updateSpec)
			if tt.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
			if tt.validateFn != nil {
				ok := tt.validateFn(updateSpec)
				assert.True(t, ok)
			}
		})
	}
}

func TestIsContainerUpdateCompleted(t *testing.T) {
	v := NativeVerticalUpdate{}

	tests := []struct {
		name            string
		container       v1.Container
		containerStatus v1.ContainerStatus
		expectedResult  bool
	}{
		{
			name: "Test status ok",
			container: v1.Container{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("100m"), "memory": resource.MustParse("128Mi")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m"), "memory": resource.MustParse("64Mi")},
				},
			},
			containerStatus: v1.ContainerStatus{
				Resources: &v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("100m"), "memory": resource.MustParse("128Mi")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m"), "memory": resource.MustParse("64Mi")},
				},
			},

			expectedResult: true,
		},
		{
			name: "Test status not ok - cpu limit",
			container: v1.Container{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("100m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},
			containerStatus: v1.ContainerStatus{
				Resources: &v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},

			expectedResult: false,
		},
		{
			name: "Test status not ok - mem limit",
			container: v1.Container{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{v1.ResourceMemory: resource.MustParse("100Mi")},
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("50Mi")},
				},
			},
			containerStatus: v1.ContainerStatus{
				Resources: &v1.ResourceRequirements{
					Limits:   v1.ResourceList{v1.ResourceMemory: resource.MustParse("200Mi")},
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("50Mi")},
				},
			},
			expectedResult: false,
		},
		{
			name: "Test status not ok - only mem limit",
			container: v1.Container{
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("100Mi")},
				},
			},
			containerStatus: v1.ContainerStatus{
				Resources: &v1.ResourceRequirements{
					Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("200Mi")},
				},
			},
			expectedResult: false,
		},
		{
			name: "Test status not ok - only mem request",
			container: v1.Container{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("100Mi")},
				},
			},
			containerStatus: v1.ContainerStatus{
				Resources: &v1.ResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("50Mi")},
				},
			},
			expectedResult: false,
		},
		{
			name: "container and containerStatus are empty",
			container: v1.Container{
				Resources: v1.ResourceRequirements{},
			},
			containerStatus: v1.ContainerStatus{
				Resources: &v1.ResourceRequirements{},
			},
			expectedResult: true,
		},
		{
			name: "container is empty",
			container: v1.Container{
				Resources: v1.ResourceRequirements{},
			},
			containerStatus: v1.ContainerStatus{
				Resources: &v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},
			expectedResult: false,
		},
		{
			name: "containerStatus is nil",
			container: v1.Container{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},
			containerStatus: v1.ContainerStatus{
				Resources: nil,
			},
			expectedResult: false,
		},
		{
			name: "containerStatus is empty",
			container: v1.Container{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},
			containerStatus: v1.ContainerStatus{
				Resources: &v1.ResourceRequirements{},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.isContainerUpdateCompleted(&tt.container, &tt.containerStatus)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
