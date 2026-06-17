package imagepulljob

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme
)

func TestNodeImageEventHandler_handleUpdate(t *testing.T) {
	tests := []struct {
		name         string
		jobs         []client.Object
		nodeImage    *appsv1beta1.NodeImage
		oldNodeImage *appsv1beta1.NodeImage
		expectedAdds []types.NamespacedName
	}{
		{
			name: "image spec changed",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "nginx:1.20",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.19", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.19", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job1"},
			},
		},
		{
			name: "image added to spec",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "redis:6.0",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
						"redis": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "6.0", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job1"},
			},
		},
		{
			name: "image removed from spec",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "redis:6.0",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
						"redis": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "6.0", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job1"},
			},
		},
		{
			name: "image status changed",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "nginx:1.20",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhasePulling},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job1"},
			},
		},
		{
			name: "no changes",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "nginx:1.20",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{},
		},
		{
			name: "invalid image",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
					Spec: appsv1beta1.ImagePullJobSpec{
						Image: "redis:6.0:latest",
					},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
						"redis": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "6.0:latest", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock queue
			q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer q.ShutDown()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.jobs...).
				WithIndex(
					&appsv1beta1.ImagePullJob{}, fieldindex.IndexNameForIsActive, fieldindex.IndexImagePullJob,
				).Build()
			// Create event handler
			handler := &nodeImageEventHandler{fakeClient}

			// Call handleUpdate
			handler.handleUpdate(tt.nodeImage, tt.oldNodeImage, q)

			// Check the queue size
			assert.Equal(t, len(tt.expectedAdds), q.Len())

			// Check the added items if needed
			addedItems := make([]types.NamespacedName, 0, len(tt.expectedAdds))
			for i := 0; i < len(tt.expectedAdds); i++ {
				item, _ := q.Get()
				addedItems = append(addedItems, item.NamespacedName)
				q.Done(item)
			}
			assert.ElementsMatch(t, tt.expectedAdds, addedItems)
		})
	}
}

func TestGetChangedSpecTags(t *testing.T) {
	tagSpec := func(tag string, version int64) appsv1beta1.ImageTagSpec {
		return appsv1beta1.ImageTagSpec{Tag: tag, Version: version}
	}

	tests := []struct {
		name     string
		newTags  []appsv1beta1.ImageTagSpec
		oldTags  []appsv1beta1.ImageTagSpec
		expected []string
	}{
		{
			name:     "both empty",
			newTags:  nil,
			oldTags:  nil,
			expected: []string{},
		},
		{
			name:     "identical tags",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1), tagSpec("1.21", 1)},
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1), tagSpec("1.21", 1)},
			expected: []string{},
		},
		{
			name:     "reordered tags: same content different order returns empty",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.21", 1), tagSpec("1.20", 1)},
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1), tagSpec("1.21", 1)},
			expected: []string{},
		},
		{
			name:     "tag added",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1), tagSpec("1.21", 1)},
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1)},
			expected: []string{"1.21"},
		},
		{
			name:     "tag removed",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1)},
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1), tagSpec("1.21", 1)},
			expected: []string{"1.21"},
		},
		{
			name:     "tag version changed",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 2), tagSpec("1.21", 1)},
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1), tagSpec("1.21", 1)},
			expected: []string{"1.20"},
		},
		{
			name:     "all tags replaced",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.22", 1)},
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1)},
			expected: []string{"1.20", "1.22"},
		},
		{
			name:     "duplicate tags in old: last wins in map, compared against new",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 2)},
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1), tagSpec("1.20", 2)},
			expected: []string{},
		},
		{
			name:     "duplicate tags in new: last wins in iteration, delete removes from old map",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1), tagSpec("1.20", 3)},
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 2)},
			expected: []string{"1.20"},
		},
		{
			name:     "old empty, new has tags",
			newTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1)},
			oldTags:  nil,
			expected: []string{"1.20"},
		},
		{
			name:     "new empty, old has tags",
			newTags:  nil,
			oldTags:  []appsv1beta1.ImageTagSpec{tagSpec("1.20", 1)},
			expected: []string{"1.20"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getChangedSpecTags(tt.newTags, tt.oldTags)
			assert.ElementsMatch(t, tt.expected, result.List())
		})
	}
}

func TestGetChangedStatusTags(t *testing.T) {
	tagStatus := func(tag string, version int64, phase appsv1beta1.ImagePullPhase) appsv1beta1.ImageTagStatus {
		return appsv1beta1.ImageTagStatus{Tag: tag, Version: version, Phase: phase}
	}

	tests := []struct {
		name     string
		newTags  []appsv1beta1.ImageTagStatus
		oldTags  []appsv1beta1.ImageTagStatus
		expected []string
	}{
		{
			name:     "both empty",
			newTags:  nil,
			oldTags:  nil,
			expected: []string{},
		},
		{
			name:     "identical status",
			newTags:  []appsv1beta1.ImageTagStatus{tagStatus("1.20", 1, appsv1beta1.ImagePhasePulling)},
			oldTags:  []appsv1beta1.ImageTagStatus{tagStatus("1.20", 1, appsv1beta1.ImagePhasePulling)},
			expected: []string{},
		},
		{
			name: "reordered status: same content different order returns empty",
			newTags: []appsv1beta1.ImageTagStatus{
				tagStatus("1.21", 1, appsv1beta1.ImagePhasePulling),
				tagStatus("1.20", 1, appsv1beta1.ImagePhaseSucceeded),
			},
			oldTags: []appsv1beta1.ImageTagStatus{
				tagStatus("1.20", 1, appsv1beta1.ImagePhaseSucceeded),
				tagStatus("1.21", 1, appsv1beta1.ImagePhasePulling),
			},
			expected: []string{},
		},
		{
			name:     "phase changed",
			newTags:  []appsv1beta1.ImageTagStatus{tagStatus("1.20", 1, appsv1beta1.ImagePhaseSucceeded)},
			oldTags:  []appsv1beta1.ImageTagStatus{tagStatus("1.20", 1, appsv1beta1.ImagePhasePulling)},
			expected: []string{"1.20"},
		},
		{
			name: "status tag added",
			newTags: []appsv1beta1.ImageTagStatus{
				tagStatus("1.20", 1, appsv1beta1.ImagePhasePulling),
				tagStatus("1.21", 1, appsv1beta1.ImagePhasePulling),
			},
			oldTags:  []appsv1beta1.ImageTagStatus{tagStatus("1.20", 1, appsv1beta1.ImagePhasePulling)},
			expected: []string{"1.21"},
		},
		{
			name:     "status tag removed",
			newTags:  nil,
			oldTags:  []appsv1beta1.ImageTagStatus{tagStatus("1.20", 1, appsv1beta1.ImagePhaseSucceeded)},
			expected: []string{"1.20"},
		},
		{
			name:    "duplicate status tags in old: last wins",
			newTags: []appsv1beta1.ImageTagStatus{tagStatus("1.20", 1, appsv1beta1.ImagePhaseSucceeded)},
			oldTags: []appsv1beta1.ImageTagStatus{
				tagStatus("1.20", 1, appsv1beta1.ImagePhasePulling),
				tagStatus("1.20", 1, appsv1beta1.ImagePhaseSucceeded),
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getChangedStatusTags(tt.newTags, tt.oldTags)
			assert.ElementsMatch(t, tt.expected, result.List())
		})
	}
}

func TestNodeImageEventHandler_handleUpdate_TagLevel(t *testing.T) {
	tests := []struct {
		name         string
		jobs         []client.Object
		nodeImage    *appsv1beta1.NodeImage
		oldNodeImage *appsv1beta1.NodeImage
		expectedAdds []types.NamespacedName
	}{
		{
			name: "only enqueue job owning changed tag, not sibling jobs",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag1", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.20"},
				},
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag2", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.21"},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 2},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job-tag1"},
			},
		},
		{
			name: "pullSecrets change enqueues all jobs for that image",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag1", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.20"},
				},
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag2", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.21"},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							PullSecrets: []appsv1beta1.ReferenceObject{{Namespace: "ns", Name: "secret1"}},
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job-tag1"},
				{Namespace: "default", Name: "job-tag2"},
			},
		},
		{
			name: "tag deleted only enqueues owning job",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag1", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.20"},
				},
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag2", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.21"},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job-tag1"},
			},
		},
		{
			name: "status change only enqueues job owning that tag",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag1", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.20"},
				},
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag2", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.21"},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
								{Tag: "1.21", Version: 1, Phase: appsv1beta1.ImagePhasePulling},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhasePulling},
								{Tag: "1.21", Version: 1, Phase: appsv1beta1.ImagePhasePulling},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job-tag1"},
			},
		},
		{
			name: "image deleted from spec enqueues all jobs for that image",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag1", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.20"},
				},
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag2", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.21"},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec:       appsv1beta1.NodeImageSpec{Images: map[string]appsv1beta1.ImageSpec{}},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job-tag1"},
				{Namespace: "default", Name: "job-tag2"},
			},
		},
		{
			name: "spec unchanged, only status single tag change enqueues owning job",
			jobs: []client.Object{
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag1", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.20"},
				},
				&appsv1beta1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{Name: "job-tag2", Namespace: "default"},
					Spec:       appsv1beta1.ImagePullJobSpec{Image: "nginx:1.21"},
				},
			},
			nodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhasePulling},
								{Tag: "1.21", Version: 1, Phase: appsv1beta1.ImagePhaseSucceeded},
							},
						},
					},
				},
			},
			oldNodeImage: &appsv1beta1.NodeImage{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec: appsv1beta1.NodeImageSpec{
					Images: map[string]appsv1beta1.ImageSpec{
						"nginx": {
							Tags: []appsv1beta1.ImageTagSpec{
								{Tag: "1.20", Version: 1},
								{Tag: "1.21", Version: 1},
							},
						},
					},
				},
				Status: appsv1beta1.NodeImageStatus{
					ImageStatuses: map[string]appsv1beta1.ImageStatus{
						"nginx": {
							Tags: []appsv1beta1.ImageTagStatus{
								{Tag: "1.20", Version: 1, Phase: appsv1beta1.ImagePhasePulling},
								{Tag: "1.21", Version: 1, Phase: appsv1beta1.ImagePhasePulling},
							},
						},
					},
				},
			},
			expectedAdds: []types.NamespacedName{
				{Namespace: "default", Name: "job-tag2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer q.ShutDown()

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.jobs...).
				WithIndex(
					&appsv1beta1.ImagePullJob{}, fieldindex.IndexNameForIsActive, fieldindex.IndexImagePullJob,
				).Build()
			handler := &nodeImageEventHandler{fakeClient}

			handler.handleUpdate(tt.nodeImage, tt.oldNodeImage, q)

			assert.Equal(t, len(tt.expectedAdds), q.Len())

			addedItems := make([]types.NamespacedName, 0, len(tt.expectedAdds))
			for i := 0; i < len(tt.expectedAdds); i++ {
				item, _ := q.Get()
				addedItems = append(addedItems, item.NamespacedName)
				q.Done(item)
			}
			assert.ElementsMatch(t, tt.expectedAdds, addedItems)
		})
	}
}
