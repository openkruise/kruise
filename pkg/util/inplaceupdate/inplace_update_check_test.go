package inplaceupdate

import (
	"testing"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeletcontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

func TestCheckAllContainersHashConsistent_SameDigest(t *testing.T) {
	// Represents the container run with image v1
	containerV1 := v1.Container{
		Name:  "nginx",
		Image: "nginx:v1",
	}
	// Represents the container run with image v2
	containerV2 := v1.Container{
		Name:  "nginx",
		Image: "nginx:v2",
	}

	// Calculate what the hash WOULD be for v1 (which is what the daemon reports is running)
	hashV1 := kubeletcontainer.HashContainer(&containerV1)

	cases := []struct {
		name                 string
		pod                  *v1.Pod
		runtimeContainerMeta *appspub.RuntimeContainerMeta
		expectedResult       bool
	}{
		{
			name: "normal case: hashes match",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-pod"},
				Spec: v1.PodSpec{
					Containers: []v1.Container{containerV1},
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{Name: "nginx", ContainerID: "docker://123", Image: "nginx:v1"},
					},
				},
			},
			runtimeContainerMeta: &appspub.RuntimeContainerMeta{
				Name:        "nginx",
				ContainerID: "docker://123",
				Hashes:      appspub.RuntimeContainerHashes{PlainHash: hashV1},
			},
			expectedResult: true,
		},
		{
			name: "issue case: spec has v2, runtime has v1 hash, but content is same (implied by ID match and Status Image match)",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-pod"},
				Spec: v1.PodSpec{
					Containers: []v1.Container{containerV2}, // Spec wants v2
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						// Kubelet accepted v2, but kept the same ContainerID
						{Name: "nginx", ContainerID: "docker://123", Image: "nginx:v2", ImageID: "docker-pullable://nginx@sha256:same-digest"},
					},
				},
			},
			runtimeContainerMeta: &appspub.RuntimeContainerMeta{
				Name:        "nginx",
				ContainerID: "docker://123",         // Matches running container
				Hashes:      appspub.RuntimeContainerHashes{PlainHash: hashV1}, // Has hash of v1
			},
			expectedResult: true, 
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metaSet := &appspub.RuntimeContainerMetaSet{
				Containers: []appspub.RuntimeContainerMeta{*tc.runtimeContainerMeta},
			}
			got := checkAllContainersHashConsistent(tc.pod, metaSet, plainHash)
			if got != tc.expectedResult {
				t.Errorf("checkAllContainersHashConsistent() = %v, want %v", got, tc.expectedResult)
			}
		})
	}
}
