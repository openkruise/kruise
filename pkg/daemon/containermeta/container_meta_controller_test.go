package containermeta

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	criapi "k8s.io/cri-api/pkg/apis"
	critesting "k8s.io/cri-api/pkg/apis/testing"

	runtimeimage "github.com/openkruise/kruise/pkg/daemon/criruntime/imageruntime"
)

type fakeRuntimeFactory struct {
	runtimeName string
	runtime     criapi.RuntimeService
}

func (f *fakeRuntimeFactory) GetImageService() runtimeimage.ImageService {
	return nil
}

func (f *fakeRuntimeFactory) GetRuntimeService() criapi.RuntimeService {
	return f.runtime
}

func (f *fakeRuntimeFactory) GetRuntimeServiceByName(name string) criapi.RuntimeService {
	f.runtimeName = name
	return f.runtime
}

func TestGetRuntimeForPodUsesRuntimeName(t *testing.T) {
	fakeRuntime := critesting.NewFakeRuntimeService()

	factory := &fakeRuntimeFactory{
		runtime: fakeRuntime,
	}

	c := &Controller{
		runtimeFactory: factory,
	}

	pod := &v1.Pod{
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					ContainerID: "containerd://abcdef",
				},
			},
		},
	}

	_, _, err := c.getRuntimeForPod(pod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if factory.runtimeName != "containerd" {
		t.Fatalf("expected runtime name containerd, got %s", factory.runtimeName)
	}
}

func TestGetRuntimeForPodInvalidContainerID(t *testing.T) {
	fakeRuntime := critesting.NewFakeRuntimeService()

	c := &Controller{
		runtimeFactory: &fakeRuntimeFactory{
			runtime: fakeRuntime,
		},
	}

	pod := &v1.Pod{
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					ContainerID: "invalid-container-id",
				},
			},
		},
	}

	_, _, err := c.getRuntimeForPod(pod)
	if err == nil {
		t.Fatal("expected error for invalid container ID")
	}
}

func TestGetRuntimeForPodMissingRuntimeName(t *testing.T) {
	fakeRuntime := critesting.NewFakeRuntimeService()

	c := &Controller{
		runtimeFactory: &fakeRuntimeFactory{
			runtime: fakeRuntime,
		},
	}

	pod := &v1.Pod{
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					ContainerID: "://abcdef",
				},
			},
		},
	}

	_, _, err := c.getRuntimeForPod(pod)
	if err == nil {
		t.Fatal("expected error when runtime name is missing")
	}
}