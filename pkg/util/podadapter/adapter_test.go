package podadapter

import (
	"encoding/json"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newAddAnnotationsPatch(t *testing.T, annotations map[string]string) client.Patch {
	type annotationPatch struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}

	var patch annotationPatch
	patch.Metadata.Annotations = annotations

	raw, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("failed to marshal patch: %v", err)
	}
	return client.RawPatch(types.StrategicMergePatchType, raw)
}

func TestPatchToRawPod(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() (*v1.Pod, *v1.Pod, client.Patch)
		expectError bool
		validate    func(*testing.T, *v1.Pod)
	}{
		{
			name: "Success",
			setup: func() (*v1.Pod, *v1.Pod, client.Patch) {
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  "nginx",
								Image: "nginx:latest",
							},
						},
					},
				}
				patch := newAddAnnotationsPatch(t, map[string]string{"image": "nginx:updated"})
				outPod := &v1.Pod{}
				return pod, outPod, patch
			},
			expectError: false,
			validate: func(t *testing.T, result *v1.Pod) {
				expectedImage := "nginx:updated"
				if result.Annotations["image"] != expectedImage {
					t.Errorf("Expected image %s, but got %s", expectedImage, result.Annotations["image"])
				}
			},
		},
		{
			name: "InvalidPatch",
			setup: func() (*v1.Pod, *v1.Pod, client.Patch) {
				pod := &v1.Pod{}
				patch := client.RawPatch(types.StrategicMergePatchType, []byte{})
				outPod := &v1.Pod{}
				return pod, outPod, patch
			},
			expectError: true,
			validate:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod, outPod, patch := tt.setup()
			result, err := patchToRawPod(pod, outPod, patch)

			if tt.expectError && err == nil {
				t.Fatal("Expected an error, but got none")
			} else if !tt.expectError && err != nil {
				t.Fatalf("Expected no error, but got: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}
