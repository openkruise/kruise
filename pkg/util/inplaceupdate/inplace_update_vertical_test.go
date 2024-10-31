package inplaceupdate

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/appscode/jsonpatch"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateSpec := &UpdateSpec{
				ContainerResources: make(map[string]v1.ResourceRequirements),
			}
			vu := &VerticalUpdate{}
			err := vu.ValidateResourcePatch(tt.op, oldTemp, updateSpec)
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

func TestSyncContainerResource(t *testing.T) {
	vu := VerticalUpdate{}

	type testcase struct {
		name            string
		containerStatus *v1.ContainerStatus
		state           *appspub.InPlaceUpdateState
		expectedState   *appspub.InPlaceUpdateState
		keyNotExist     bool
	}

	container := &v1.ContainerStatus{
		Name: "test-container",
		Resources: &v1.ResourceRequirements{
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse("1"),
				v1.ResourceMemory: resource.MustParse("2Gi"),
			},
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse("100m"),
				v1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	}
	testcases := []testcase{
		{
			name:            "normal case",
			containerStatus: container,
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
					"test-container": {
						Resources: v1.ResourceRequirements{
							Limits: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("2"),
								v1.ResourceMemory: resource.MustParse("7Gi"),
							},
							Requests: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("1"),
								v1.ResourceMemory: resource.MustParse("3Gi"),
							},
						},
					},
				},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
					"test-container": {
						Resources: v1.ResourceRequirements{
							Limits: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("1"),
								v1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Requests: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
		},
		{
			name:            "empty LastContainerStatuses in state",
			containerStatus: container,
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses: nil,
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{
					"test-container": {
						Resources: v1.ResourceRequirements{
							Limits: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("1"),
								v1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Requests: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
		},
		{
			name:            "nil containerStatus",
			containerStatus: nil,
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{},
			},
			keyNotExist: true,
		},
		{
			name: "nil containerStatus resources",
			containerStatus: &v1.ContainerStatus{
				Name:      "test-container",
				Resources: nil,
			},
			state: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{},
			},
			expectedState: &appspub.InPlaceUpdateState{
				LastContainerStatuses: map[string]appspub.InPlaceUpdateContainerStatus{"test-container": {
					Resources: v1.ResourceRequirements{
						Requests: map[v1.ResourceName]resource.Quantity{},
						Limits:   map[v1.ResourceName]resource.Quantity{},
					},
				}},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			vu.SyncContainerResource(tc.containerStatus, tc.state)
			actualContainerStatus, ok := tc.state.LastContainerStatuses[container.Name]
			assert.Equal(t, !tc.keyNotExist, ok, "Container status should be present in the state: %v", tc.keyNotExist)
			b, _ := json.Marshal(actualContainerStatus)
			t.Logf("container status: %v", string(b))
			assert.True(t, reflect.DeepEqual(tc.expectedState.LastContainerStatuses[container.Name], actualContainerStatus), "Container status should match expected state")
		})
	}

}

func TestIsContainerUpdateCompleted(t *testing.T) {
	v := VerticalUpdate{}

	tests := []struct {
		name                string
		container           v1.Container
		containerStatus     v1.ContainerStatus
		lastContainerStatus appspub.InPlaceUpdateContainerStatus
		expectedResult      bool
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},
			expectedResult: true,
		},
		{
			name: "Test status not ok",
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},
			expectedResult: false,
		},
		{
			name: "Test status not ok",
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{v1.ResourceMemory: resource.MustParse("200Mi")},
					Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("50Mi")},
				},
			},
			expectedResult: false,
		},
		{
			name: "Test status not ok",
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("200Mi")},
				},
			},
			expectedResult: false,
		},
		{
			name: "Test status not ok",
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{
				Resources: v1.ResourceRequirements{
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{},
			expectedResult:      true,
		},
		{
			name: "empty container and nil containerStatus",
			container: v1.Container{
				Resources: v1.ResourceRequirements{},
			},
			containerStatus: v1.ContainerStatus{
				Resources: nil,
			},
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{},
			expectedResult:      true,
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{
				Resources: v1.ResourceRequirements{
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
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
			lastContainerStatus: appspub.InPlaceUpdateContainerStatus{
				Resources: v1.ResourceRequirements{
					Limits:   v1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: v1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &v1.Pod{}
			result := v.IsContainerUpdateCompleted(pod, &tt.container, &tt.containerStatus, tt.lastContainerStatus)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
