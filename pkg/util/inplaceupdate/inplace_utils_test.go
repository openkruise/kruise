package inplaceupdate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util/podadapter"
)

func TestRefreshRestartCountBaseToPod(t *testing.T) {
	tests := []struct {
		name           string
		pod            *v1.Pod
		expectedBase   map[string]int32
		expectAnnoKey  bool
		expectPatchErr bool
	}{
		{
			name: "No container statuses",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod1",
				},
				Status: v1.PodStatus{}, // 空的ContainerStatuses
			},
			expectedBase:   nil,
			expectAnnoKey:  false,
			expectPatchErr: true,
		},
		{
			name: "All containers with zero restarts",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod2",
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{Name: "app", RestartCount: 0},
						{Name: "sidecar", RestartCount: 0},
					},
				},
			},
			expectedBase: map[string]int32{
				"app":     0,
				"sidecar": 0,
			},
			expectAnnoKey: true,
		},
		{
			name: "Mixed restart counts",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod3",
				},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{Name: "app", RestartCount: 2},
						{Name: "sidecar", RestartCount: 0},
						{Name: "init", RestartCount: 1},
					},
				},
			},
			expectedBase: map[string]int32{
				"app":     2,
				"sidecar": 0,
				"init":    1,
			},
			expectAnnoKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithObjects(tt.pod).Build()

			ctrl := &realControl{
				podAdapter: &podadapter.AdapterRuntimeClient{Client: client},
			}

			gotPod, err := ctrl.RefreshRestartCountBaseToPod(tt.pod.DeepCopy())

			if tt.expectPatchErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			anno := gotPod.Annotations
			if !tt.expectAnnoKey {
				assert.Nil(t, anno)
				return
			}
			assert.NotNil(t, anno)

			infoStr := anno[AnnotationRestartCountInfoKey]
			var info RestartCountInfo
			err = json.Unmarshal([]byte(infoStr), &info)
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedBase, info)
		})
	}
}

func TestSetPodRestartCountBase(t *testing.T) {
	tests := []struct {
		name           string
		initialAnno    map[string]string
		inputBase      string
		expectedAnno   map[string]string
		expectModified bool
	}{
		{
			name:           "Nil annotations",
			initialAnno:    nil,
			inputBase:      `{"base":{"app":2}}`,
			expectedAnno:   map[string]string{AnnotationRestartCountInfoKey: `{"base":{"app":2}}`},
			expectModified: true,
		},
		{
			name:           "Empty annotations",
			initialAnno:    make(map[string]string),
			inputBase:      `{"base":{"app":2}}`,
			expectedAnno:   map[string]string{AnnotationRestartCountInfoKey: `{"base":{"app":2}}`},
			expectModified: true,
		},
		{
			name:           "Existing annotation should be overwritten",
			initialAnno:    map[string]string{AnnotationRestartCountInfoKey: `{"base":{"old":999}}`},
			inputBase:      `{"base":{"app":2}}`,
			expectedAnno:   map[string]string{AnnotationRestartCountInfoKey: `{"base":{"app":2}}`},
			expectModified: true,
		},
		{
			name:           "Nil with empty input",
			initialAnno:    nil,
			inputBase:      "",
			expectedAnno:   map[string]string{AnnotationRestartCountInfoKey: ""},
			expectModified: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pod",
					Annotations: tt.initialAnno,
				},
			}

			setPodRestartCountBase(pod, tt.inputBase)

			if len(tt.expectedAnno) == 0 {
				assert.Nil(t, pod.Annotations)
				return
			}

			assert.Equal(t, tt.expectedAnno, pod.Annotations)
		})
	}
}

func createPodWithInPlaceState(state *appspub.InPlaceUpdateState) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Namespace:   "default",
			Annotations: make(map[string]string),
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{Name: "app", Image: "nginx:1.0"},
			},
		},
	}

	if state != nil {
		stateBytes, _ := json.Marshal(state)
		pod.Annotations[appspub.InPlaceUpdateStateKey] = string(stateBytes)
	}

	return pod
}

func TestCalcInplaceUpdateDuration(t *testing.T) {
	var TestUpdateType = "inplace_1"
	baseTime := time.Now().Add(-5 * time.Minute)
	tests := []struct {
		name           string
		pod            *v1.Pod
		expectedCalled bool
		seconds        int
	}{
		{
			name: "Valid in-place update state with timestamp",
			pod: func() *v1.Pod {
				state := &appspub.InPlaceUpdateState{
					UpdateTimestamp: metav1.Time{Time: baseTime},
					UpdateImages:    true,
				}
				return createPodWithInPlaceState(state)
			}(),
			expectedCalled: true,
			seconds:        300,
		},
		{
			name:           "Nil pod",
			pod:            nil,
			expectedCalled: false,
		},
		{
			name: "Pod without annotations",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-annotations",
				},
			},
			expectedCalled: false,
		},
		{
			name: "Pod with invalid state annotation",
			pod: func() *v1.Pod {
				pod := createPodWithInPlaceState(nil)
				pod.Annotations[appspub.InPlaceUpdateStateKey] = "invalid-json"
				return pod
			}(),
			expectedCalled: false,
		},
		{
			name: "Pod with future timestamp",
			pod: func() *v1.Pod {
				state := &appspub.InPlaceUpdateState{
					UpdateTimestamp: metav1.Time{Time: time.Now().Add(10*time.Minute + 2*time.Second)}, // 将来时间
					UpdateImages:    true,
				}
				return createPodWithInPlaceState(state)
			}(),
			expectedCalled: true,
			seconds:        -600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock metrics.RecordUpdateDuration
			called := false
			fn := func(updateType string, duration int) {
				called = true
				assert.Equal(t, TestUpdateType, updateType)
				if called && tt.expectedCalled {
					assert.Equal(t, tt.seconds, duration/10*10)
				}
			}
			calcInplaceUpdateDuration(tt.pod, fn)
			assert.Equal(t, tt.expectedCalled, called)
		})
	}
}
