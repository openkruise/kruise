package inplaceupdate

import (
	"testing"

	"github.com/appscode/jsonpatch"
	"github.com/google/go-cmp/cmp"
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

func TestGenerateResourcePatch(t *testing.T) {
	v := NativeVerticalUpdate{}
	pod := &v1.Pod{
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
	testCases := []struct {
		name              string
		pod               *v1.Pod
		expectedResources map[string]*v1.ResourceRequirements
		expectedPatch     []byte
	}{
		{
			name: "both limit and request need update",
			pod:  pod,
			expectedResources: map[string]*v1.ResourceRequirements{
				"test-container": {
					Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("200m")},
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("1Gi")},
				},
				"test-container2": {
					Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("300m")},
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("2Gi")},
				},
			},
			expectedPatch: []byte(`{"spec":{"containers":[{"name":"test-container","resources":{"limits":{"cpu":"200m"},"requests":{"memory":"1Gi"}}},{"name":"test-container2","resources":{"limits":{"cpu":"300m"},"requests":{"memory":"2Gi"}}}]}}`),
		},
		{
			name: "only limit need update",
			pod:  pod,
			expectedResources: map[string]*v1.ResourceRequirements{
				"test-container": {
					Limits: v1.ResourceList{v1.ResourceCPU: resource.MustParse("200m")},
				},
			},
			expectedPatch: []byte(`{"spec":{"containers":[{"name":"test-container","resources":{"limits":{"cpu":"200m"}}}]}}`),
		},
		{
			name: "only request need update",
			pod:  pod,
			expectedResources: map[string]*v1.ResourceRequirements{
				"test-container": {
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("1Gi")},
				},
			},
			expectedPatch: []byte(`{"spec":{"containers":[{"name":"test-container","resources":{"requests":{"memory":"1Gi"}}}]}}`),
		},
		{
			name:              "no containers",
			pod:               pod,
			expectedResources: map[string]*v1.ResourceRequirements{},
			expectedPatch:     nil,
		},
		{
			name: "only test-container need update",
			pod:  pod,
			expectedResources: map[string]*v1.ResourceRequirements{
				"test-container": {
					Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("200m")},
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("1Gi")},
				},
			},
			expectedPatch: []byte(`{"spec":{"containers":[{"name":"test-container","resources":{"limits":{"cpu":"200m"},"requests":{"memory":"1Gi"}}}]}}`),
		},
		{
			name: "only test-container2 need update",
			pod:  pod,
			expectedResources: map[string]*v1.ResourceRequirements{
				"test-container2": {
					Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("300m")},
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("2Gi")},
				},
			},
			expectedPatch: []byte(`{"spec":{"containers":[{"name":"test-container2","resources":{"limits":{"cpu":"300m"},"requests":{"memory":"2Gi"}}}]}}`),
		},
		{
			name: "non-resizable resources",
			pod:  pod,
			expectedResources: map[string]*v1.ResourceRequirements{
				"test-container": {
					Limits: v1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
				},
			},
			expectedPatch: nil,
		},
		{
			name: "same resources",
			pod:  pod,
			expectedResources: map[string]*v1.ResourceRequirements{
				"test-container": {
					Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("100m")},
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("512Mi")},
				},
			},
			expectedPatch: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			patch := v.GenerateResourcePatch(tc.pod, tc.expectedResources)
			if diff := cmp.Diff(string(tc.expectedPatch), string(patch)); diff != "" {
				t.Errorf("patch mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
