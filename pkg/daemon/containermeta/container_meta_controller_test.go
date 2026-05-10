package containermeta

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	criapi "k8s.io/cri-api/pkg/apis"
	"github.com/openkruise/kruise/pkg/daemon/criruntime/imageruntime"
)

type mockRuntimeService struct {
	criapi.RuntimeService
}

type mockRuntimeFactory struct{}

func (m *mockRuntimeFactory) GetRuntimeServiceByName(name string) criapi.RuntimeService {
	if name == "docker" || name == "containerd" {
		return &mockRuntimeService{}
	}
	return nil
}

func (m *mockRuntimeFactory) GetImageService() imageruntime.ImageService {
	return nil
}

func (m *mockRuntimeFactory) GetRuntimeService() criapi.RuntimeService {
	return nil
}

func TestGetRuntimeForPod(t *testing.T) {
	c := &Controller{
		runtimeFactory: &mockRuntimeFactory{},
	}

	testCases := []struct {
		name            string
		pod             *v1.Pod
		expectedRuntime bool
		expectedErr     string
	}{
		{
			name:            "empty container statuses",
			pod:             &v1.Pod{},
			expectedRuntime: false,
			expectedErr:     "empty containerStatuses in pod status",
		},
		{
			name: "no existing ID",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{ContainerID: ""},
					},
				},
			},
			expectedRuntime: false,
			expectedErr:     "",
		},
		{
			name: "valid docker runtime",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{ContainerID: ""},
						{ContainerID: "docker://12345678"},
					},
				},
			},
			expectedRuntime: true,
			expectedErr:     "",
		},
		{
			name: "invalid container ID format",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{ContainerID: "invalid-format"},
					},
				},
			},
			expectedRuntime: false,
			expectedErr:     "failed to parse containerID",
		},
		{
			name: "unknown runtime",
			pod: &v1.Pod{
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{ContainerID: "unknown://123"},
					},
				},
			},
			expectedRuntime: false,
			expectedErr:     "not found runtime service for unknown in daemon",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, runtime, err := c.getRuntimeForPod(tc.pod)
			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
			} else {
				assert.NoError(t, err)
				if tc.expectedRuntime {
					assert.NotNil(t, runtime)
				} else {
					assert.Nil(t, runtime)
				}
			}
		})
	}
}
